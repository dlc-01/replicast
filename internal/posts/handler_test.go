package posts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/posts"
)

func withPostIdentity(r *http.Request, globalID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxkey.UserGlobalID, globalID))
}

func newHandlerSvc() *posts.Service {
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	return newSvc(newMockPostRepo(), userRepo)
}

// — Create ─────────────────────────────────────────────────────────────

func TestPostHandler_Create(t *testing.T) {
	tests := []struct {
		name       string
		identity   string
		body       any
		wantStatus int
	}{
		{
			name:       "success",
			identity:   "alice@node-a",
			body:       map[string]string{"content": "hello world"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing content",
			identity:   "alice@node-a",
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			identity:   "alice@node-a",
			body:       "not-json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "no identity",
			identity:   "",
			body:       map[string]string{"content": "hello"},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := posts.NewHandler(newHandlerSvc())

			var bodyBytes []byte
			if s, ok := tt.body.(string); ok {
				bodyBytes = []byte(s)
			} else {
				bodyBytes, _ = json.Marshal(tt.body)
			}

			r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(bodyBytes))
			r.Header.Set("Content-Type", "application/json")
			if tt.identity != "" {
				r = withPostIdentity(r, tt.identity)
			}
			w := httptest.NewRecorder()
			h.Create(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// — Get ────────────────────────────────────────────────────────────────

func TestPostHandler_Get_Found(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		Content:  "hello",
	}
	h := posts.NewHandler(newSvc(postRepo, &mockUserResolver{}))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/posts/post:alice@node-a:001", nil)
	r.SetPathValue("id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.Get(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
}

func TestPostHandler_Get_NotFound(t *testing.T) {
	h := posts.NewHandler(newHandlerSvc())

	r := httptest.NewRequest(http.MethodGet, "/api/v1/posts/post:nobody:999", nil)
	r.SetPathValue("id", "post:nobody:999")
	w := httptest.NewRecorder()
	h.Get(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// — Update ─────────────────────────────────────────────────────────────

func TestPostHandler_Update_Success(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
		Content:  "original",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	h := posts.NewHandler(newSvc(postRepo, userRepo))

	body, _ := json.Marshal(map[string]string{"content": "updated"})
	r := httptest.NewRequest(http.MethodPut, "/api/v1/posts/post:alice@node-a:001", bytes.NewReader(body))
	r.SetPathValue("id", "post:alice@node-a:001")
	r = withPostIdentity(r, "alice@node-a")
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
}

func TestPostHandler_Update_Forbidden(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{
		"alice@node-a": "uuid-alice",
		"bob@node-a":   "uuid-bob",
	}}
	h := posts.NewHandler(newSvc(postRepo, userRepo))

	body, _ := json.Marshal(map[string]string{"content": "hacked"})
	r := httptest.NewRequest(http.MethodPut, "/api/v1/posts/post:alice@node-a:001", bytes.NewReader(body))
	r.SetPathValue("id", "post:alice@node-a:001")
	r = withPostIdentity(r, "bob@node-a")
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// — Delete ─────────────────────────────────────────────────────────────

func TestPostHandler_Delete_Success(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	h := posts.NewHandler(newSvc(postRepo, userRepo))

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/post:alice@node-a:001", nil)
	r.SetPathValue("id", "post:alice@node-a:001")
	r = withPostIdentity(r, "alice@node-a")
	w := httptest.NewRecorder()
	h.Delete(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204\nbody: %s", w.Code, w.Body.String())
	}
}

func TestPostHandler_Delete_Forbidden(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{
		"alice@node-a": "uuid-alice",
		"bob@node-a":   "uuid-bob",
	}}
	h := posts.NewHandler(newSvc(postRepo, userRepo))

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/post:alice@node-a:001", nil)
	r.SetPathValue("id", "post:alice@node-a:001")
	r = withPostIdentity(r, "bob@node-a")
	w := httptest.NewRecorder()
	h.Delete(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}
