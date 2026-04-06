package httpapi

import (
	"net/http"
	"time"

	"github.com/dlc-01/replicast/internal/auth"
	"github.com/dlc-01/replicast/internal/comments"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/dms"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/feed"
	"github.com/dlc-01/replicast/internal/follows"
	"github.com/dlc-01/replicast/internal/likes"
	"github.com/dlc-01/replicast/internal/posts"
	"github.com/dlc-01/replicast/internal/respond"
	"github.com/dlc-01/replicast/internal/users"
)

type Deps struct {
	AuthSvc    *auth.Service
	UserSvc    *users.Service
	PostSvc    *posts.Service
	FollowSvc  *follows.Service
	FeedSvc    *feed.Service
	FedSvc     *federation.Service
	LikeSvc    *likes.Service
	CommentSvc *comments.Service
	DMSvc      *dms.Service
	Cfg        *config.Config
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	authH := auth.NewHandler(d.AuthSvc)
	userH := users.NewHandler(d.UserSvc)
	postH := posts.NewHandler(d.PostSvc)
	followH := follows.NewHandler(d.FollowSvc)
	feedH := feed.NewHandler(d.FeedSvc)
	fedH := federation.NewHandler(d.FedSvc, d.Cfg)
	likeH := likes.NewHandler(d.LikeSvc)
	commentH := comments.NewHandler(d.CommentSvc)
	dmH := dms.NewHandler(d.DMSvc)

	jwtAuth := RequireAuth(d.Cfg)
	fedAuth := RequireFedAuth(d.Cfg)
	optionalAuth := OptionalAuth(d.Cfg)

	// — Auth
	mux.HandleFunc("POST /api/v1/auth/register", authH.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)

	// — Users
	mux.HandleFunc("GET /api/v1/users/{username}", userH.GetProfile)
	mux.HandleFunc("PUT /api/v1/users/me", jwtAuth(userH.UpdateProfile))
	mux.HandleFunc("GET /api/v1/users/{username}/key", userH.GetPublicKey)

	// — Posts
	mux.HandleFunc("POST /api/v1/posts", jwtAuth(postH.Create))
	mux.HandleFunc("GET /api/v1/posts/{global_id}", postH.Get)
	mux.HandleFunc("PUT /api/v1/posts/{global_id}", jwtAuth(postH.Update))
	mux.HandleFunc("DELETE /api/v1/posts/{global_id}", jwtAuth(postH.Delete))

	// — Likes
	mux.HandleFunc("POST /api/v1/posts/{global_id}/like", jwtAuth(likeH.Like))
	mux.HandleFunc("DELETE /api/v1/posts/{global_id}/like", jwtAuth(likeH.Unlike))
	mux.HandleFunc("GET /api/v1/posts/{global_id}/likes", optionalAuth(likeH.GetLikes))

	// — Comments
	mux.HandleFunc("POST /api/v1/posts/{global_id}/comments", jwtAuth(commentH.Create))
	mux.HandleFunc("GET /api/v1/posts/{global_id}/comments", optionalAuth(commentH.List))
	mux.HandleFunc("DELETE /api/v1/comments/{global_id}", jwtAuth(commentH.Delete))

	// — DMs
	mux.HandleFunc("POST /api/v1/conversations", jwtAuth(dmH.StartConversation))
	mux.HandleFunc("GET /api/v1/conversations", jwtAuth(dmH.ListConversations))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", jwtAuth(dmH.SendMessage))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", jwtAuth(dmH.GetMessages))

	// — Follows
	mux.HandleFunc("POST /api/v1/follows", jwtAuth(followH.Follow))
	mux.HandleFunc("DELETE /api/v1/follows/{target}", jwtAuth(followH.Unfollow))

	// — Feed
	mux.HandleFunc("GET /api/v1/feed", jwtAuth(feedH.GetFeed))

	verifyHMAC := VerifyHMAC(d.Cfg)

	// — Federation (межузловые)
	mux.HandleFunc("GET /.well-known/replicast", fedH.WellKnown)
	mux.HandleFunc("POST /api/v1/federation/handshake", fedAuth(verifyHMAC(fedH.Handshake)))
	mux.HandleFunc("POST /api/v1/federation/events", fedAuth(verifyHMAC(fedH.ReceiveEvent)))
	mux.HandleFunc("POST /api/v1/federation/follows", fedAuth(verifyHMAC(fedH.ReceiveFollow)))
	mux.HandleFunc("GET /api/v1/federation/users/{global_id}", fedH.GetRemoteUser)

	// — Health
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"node":   d.Cfg.NodeName,
		})
	})

	rl := NewRateLimiter(100, time.Minute)
	return Chain(mux,
		Recovery,
		RequestID,
		Logging,
		RateLimit(rl),
	)
}
