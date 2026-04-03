package httpapi

import (
	"net/http"

	"github.com/dlc-01/replicast/internal/auth"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/feed"
	"github.com/dlc-01/replicast/internal/follows"
	"github.com/dlc-01/replicast/internal/posts"
	"github.com/dlc-01/replicast/internal/respond"
	"github.com/dlc-01/replicast/internal/users"
)

type Deps struct {
	AuthSvc   *auth.Service
	UserSvc   *users.Service
	PostSvc   *posts.Service
	FollowSvc *follows.Service
	FeedSvc   *feed.Service
	FedSvc    *federation.Service
	Cfg       *config.Config
}

// NewRouter собирает все маршруты.
// Глобальные middleware: Recovery → RequestID → Logging.
// Go 1.25 ServeMux: "METHOD /path/{param}" без сторонних библиотек.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	authH := auth.NewHandler(d.AuthSvc)
	userH := users.NewHandler(d.UserSvc)
	postH := posts.NewHandler(d.PostSvc)
	followH := follows.NewHandler(d.FollowSvc)
	feedH := feed.NewHandler(d.FeedSvc)
	fedH := federation.NewHandler(d.FedSvc, d.Cfg)

	jwtAuth := RequireAuth(d.Cfg)
	fedAuth := RequireFedAuth(d.Cfg)

	// — Auth (публичные)
	mux.HandleFunc("POST /api/v1/auth/register", authH.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)

	// — Users
	mux.HandleFunc("GET /api/v1/users/{username}", userH.GetProfile)
	mux.HandleFunc("PUT /api/v1/users/me", jwtAuth(userH.UpdateProfile))

	// — Posts
	mux.HandleFunc("POST /api/v1/posts", jwtAuth(postH.Create))
	mux.HandleFunc("GET /api/v1/posts/{id}", postH.Get)
	mux.HandleFunc("PUT /api/v1/posts/{id}", jwtAuth(postH.Update))
	mux.HandleFunc("DELETE /api/v1/posts/{id}", jwtAuth(postH.Delete))

	// — Follows
	mux.HandleFunc("POST /api/v1/follows", jwtAuth(followH.Follow))
	mux.HandleFunc("DELETE /api/v1/follows/{target}", jwtAuth(followH.Unfollow))

	// — Feed
	mux.HandleFunc("GET /api/v1/feed", jwtAuth(feedH.GetFeed))

	// — Federation (межузловые, аутентификация через X-Replicast-Secret)
	mux.HandleFunc("POST /api/v1/federation/events", fedAuth(fedH.ReceiveEvent))
	mux.HandleFunc("POST /api/v1/federation/handshake", fedH.Handshake)
	mux.HandleFunc("POST /api/v1/federation/follows", fedAuth(fedH.ReceiveFollow))
	mux.HandleFunc("GET /api/v1/federation/users/{global_id}", fedH.GetRemoteUser)

	// — Node discovery
	mux.HandleFunc("GET /.well-known/replicast", fedH.WellKnown)

	// — Health
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"node":   d.Cfg.NodeName,
		})
	})

	// Глобальные middleware применяются ко всем маршрутам
	return Chain(mux,
		Recovery,
		RequestID,
		Logging,
	)
}
