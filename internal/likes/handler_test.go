package likes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/httpapi"
	"github.com/dlc-01/replicast/internal/likes"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

func withLikeIdentity(r *http.Request, globalID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxkey.UserGlobalID, globalID))
}

func newTestLikeHandler() *likes.Handler {
	repo := newMockLikeRepo()
	fed := &mockLikeFedEnqueuer{}
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := likes.NewService(repo, fed, posts, logger.Nop(), &config.Config{NodeName: "node-a"})
	return likes.NewHandler(svc)
}

func TestLikeHandler_Like_Success(t *testing.T) {
	h := newTestLikeHandler()
	r := withLikeIdentity(httptest.NewRequest(http.MethodPost, "/", nil), "alice@node-a")
	r.SetPathValue("global_id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.Like(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["likes"].(float64) != 1 {
		t.Errorf("likes = %v, want 1", resp["likes"])
	}
}

func TestLikeHandler_Like_NoIdentity(t *testing.T) {
	h := newTestLikeHandler()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.SetPathValue("global_id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.Like(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestLikeHandler_Unlike_Success(t *testing.T) {
	h := newTestLikeHandler()

	// Лайкаем
	r1 := withLikeIdentity(httptest.NewRequest(http.MethodPost, "/", nil), "alice@node-a")
	r1.SetPathValue("global_id", "post:alice@node-a:001")
	h.Like(httptest.NewRecorder(), r1)

	// Анлайкаем
	r2 := withLikeIdentity(httptest.NewRequest(http.MethodDelete, "/", nil), "alice@node-a")
	r2.SetPathValue("global_id", "post:alice@node-a:001")
	w2 := httptest.NewRecorder()
	h.Unlike(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w2.Code, http.StatusOK)
	}
	var resp map[string]any
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp["likes"].(float64) != 0 {
		t.Errorf("likes = %v, want 0 after unlike", resp["likes"])
	}
}

func TestLikeHandler_GetLikes_Success(t *testing.T) {
	h := newTestLikeHandler()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("global_id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.GetLikes(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["likes"]; !ok {
		t.Error("response should contain likes field")
	}
}

func TestLikeHandler_GetLikes_ShowsLikedByMe(t *testing.T) {
	h := newTestLikeHandler()

	// Alice лайкает
	r1 := withLikeIdentity(httptest.NewRequest(http.MethodPost, "/", nil), "alice@node-a")
	r1.SetPathValue("global_id", "post:alice@node-a:001")
	h.Like(httptest.NewRecorder(), r1)

	// Проверяем liked_by_me для alice
	r2 := withLikeIdentity(httptest.NewRequest(http.MethodGet, "/", nil), "alice@node-a")
	r2.SetPathValue("global_id", "post:alice@node-a:001")
	w2 := httptest.NewRecorder()
	h.GetLikes(w2, r2)

	var resp map[string]any
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp["liked_by_me"] != true {
		t.Errorf("liked_by_me = %v, want true", resp["liked_by_me"])
	}
}

func TestLikeHandler_GetLikes_WithUsers(t *testing.T) {
	repo := newMockLikeRepo()
	repo.hideLikes = false
	fed := &mockLikeFedEnqueuer{}
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := likes.NewService(repo, fed, posts, logger.Nop(), &config.Config{NodeName: "node-a"})
	h := likes.NewHandler(svc)

	// Alice лайкает
	r1 := withLikeIdentity(httptest.NewRequest(http.MethodPost, "/", nil), "alice@node-a")
	r1.SetPathValue("global_id", "post:alice@node-a:001")
	h.Like(httptest.NewRecorder(), r1)

	// Запрашиваем лайки
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.SetPathValue("global_id", "post:alice@node-a:001")
	w2 := httptest.NewRecorder()
	h.GetLikes(w2, r2)

	var resp map[string]any
	json.NewDecoder(w2.Body).Decode(&resp)

	if resp["hidden"] != false {
		t.Errorf("hidden = %v, want false", resp["hidden"])
	}
	if _, ok := resp["users"]; !ok {
		t.Error("users field should be present when not hidden")
	}
}

func TestLikeHandler_GetLikes_Hidden(t *testing.T) {
	repo := newMockLikeRepo()
	repo.hideLikes = true
	fed := &mockLikeFedEnqueuer{}
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := likes.NewService(repo, fed, posts, logger.Nop(), &config.Config{NodeName: "node-a"})
	h := likes.NewHandler(svc)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("global_id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.GetLikes(w, r)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["hidden"] != true {
		t.Errorf("hidden = %v, want true", resp["hidden"])
	}
	if _, ok := resp["users"]; ok {
		t.Error("users field should NOT be present when hidden")
	}
}

func TestRateLimiter_Allow_UnderLimit(t *testing.T) {
	rl := httpapi.NewRateLimiter(5, time.Minute)
	for i := 0; i < 5; i++ {
		if !rl.Allow("1.1.1.1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_Allow_OverLimit(t *testing.T) {
	rl := httpapi.NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		rl.Allow("1.1.1.1")
	}
	if rl.Allow("1.1.1.1") {
		t.Error("4th request should be blocked")
	}
}

func TestRateLimiter_DifferentIPs_Independent(t *testing.T) {
	rl := httpapi.NewRateLimiter(1, time.Minute)

	if !rl.Allow("1.1.1.1") {
		t.Error("first IP should be allowed")
	}
	if !rl.Allow("2.2.2.2") {
		t.Error("second IP should be allowed independently")
	}
	if rl.Allow("1.1.1.1") {
		t.Error("first IP second request should be blocked")
	}
}

func TestRateLimiter_ResetsAfterWindow(t *testing.T) {
	rl := httpapi.NewRateLimiter(1, 50*time.Millisecond)

	if !rl.Allow("1.1.1.1") {
		t.Error("first request should be allowed")
	}
	if rl.Allow("1.1.1.1") {
		t.Error("second request should be blocked")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("1.1.1.1") {
		t.Error("request after window reset should be allowed")
	}
}

func TestRateLimiter_ZeroLimit_BlocksAll(t *testing.T) {
	rl := httpapi.NewRateLimiter(0, time.Minute)
	if rl.Allow("1.1.1.1") {
		t.Error("zero limit should block all requests")
	}
}

// — RateLimit middleware ───────────────────────────────────────────────

func TestRateLimit_Middleware_AllowsUnderLimit(t *testing.T) {
	rl := httpapi.NewRateLimiter(5, time.Minute)
	handler := httpapi.RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.1:9000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i+1, w.Code)
		}
	}
}

func TestRateLimit_Middleware_BlocksOverLimit(t *testing.T) {
	rl := httpapi.NewRateLimiter(2, time.Minute)
	handler := httpapi.RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.2:9000"
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.2:9000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code == http.StatusOK {
		t.Error("3rd request should be blocked")
	}
}

func TestRateLimit_Middleware_DifferentIPs_Independent(t *testing.T) {
	rl := httpapi.NewRateLimiter(1, time.Minute)
	handler := httpapi.RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP A — исчерпывает лимит
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.0.0.3:9000"
	handler.ServeHTTP(httptest.NewRecorder(), r1)

	// IP B — должен пройти
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.0.0.4:9000"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("different IP should not be affected, status = %d", w2.Code)
	}
}

func TestRateLimit_Middleware_StripPort(t *testing.T) {
	rl := httpapi.NewRateLimiter(1, time.Minute)
	handler := httpapi.RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Одинаковый IP разные порты — должны считаться как один
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.0.0.5:1111"
	handler.ServeHTTP(httptest.NewRecorder(), r1)

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.0.0.5:2222"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	if w2.Code == http.StatusOK {
		t.Error("same IP different port should be rate limited together")
	}
}
