package feed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/feed"
	"github.com/dlc-01/replicast/internal/port"
)

func withFeedIdentity(r *http.Request, globalID string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxkey.UserGlobalID, globalID)
	return r.WithContext(ctx)
}

func TestFeedHandler_GetFeed_Success(t *testing.T) {
	now := time.Now()
	repo := newMockFeedRepo()
	repo.items = []port.FeedPost{
		{PostGlobalID: "post:alice@node-a:001", Content: "hello", AuthorGlobalID: "alice@node-a", CreatedAt: now},
	}
	h := feed.NewHandler(feed.NewService(repo))

	r := withFeedIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil), "bob@node-a")
	w := httptest.NewRecorder()
	h.GetFeed(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1", resp["count"])
	}
}

func TestFeedHandler_GetFeed_Empty(t *testing.T) {
	h := feed.NewHandler(feed.NewService(newMockFeedRepo()))

	r := withFeedIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil), "bob@node-a")
	w := httptest.NewRecorder()
	h.GetFeed(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["count"].(float64) != 0 {
		t.Errorf("count = %v, want 0", resp["count"])
	}
}

func TestFeedHandler_GetFeed_NoIdentity(t *testing.T) {
	h := feed.NewHandler(feed.NewService(newMockFeedRepo()))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
	w := httptest.NewRecorder()
	h.GetFeed(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestFeedHandler_GetFeed_LimitFromQuery(t *testing.T) {
	var capturedLimit int
	repo := &mockFeedRepoCapture{
		onGetFeed: func(_ context.Context, _ string, limit int) ([]port.FeedPost, error) {
			capturedLimit = limit
			return []port.FeedPost{}, nil
		},
	}
	h := feed.NewHandler(feed.NewService(repo))

	r := withFeedIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/feed?limit=25", nil), "bob@node-a")
	w := httptest.NewRecorder()
	h.GetFeed(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedLimit != 25 {
		t.Errorf("limit = %d, want 25", capturedLimit)
	}
}

func TestFeedHandler_GetFeed_InvalidLimit_UsesDefault(t *testing.T) {
	var capturedLimit int
	repo := &mockFeedRepoCapture{
		onGetFeed: func(_ context.Context, _ string, limit int) ([]port.FeedPost, error) {
			capturedLimit = limit
			return []port.FeedPost{}, nil
		},
	}
	h := feed.NewHandler(feed.NewService(repo))

	r := withFeedIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/feed?limit=not-a-number", nil), "bob@node-a")
	h.GetFeed(httptest.NewRecorder(), r)

	if capturedLimit != 50 {
		t.Errorf("limit = %d, want 50 (default)", capturedLimit)
	}
}
