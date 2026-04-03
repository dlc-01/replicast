package follows_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/follows"
	"github.com/dlc-01/replicast/internal/port"
)

// newHandlerSvc — сервис с alice и известным node-b для handler тестов.
func newHandlerSvc() *follows.Service {
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	nodeRepo := newMockNodeRegistry()
	nodeRepo.nodes["node-b"] = &port.Node{Name: "node-b", BaseURL: "http://node-b:8080"}
	disc := &mockDiscoverer{results: map[string][2]string{
		"node-b": {"node-b", "http://node-b:8080"},
	}}
	return newSvc(newMockFollowRepo(), userRepo, &mockFedEnqueuer{}, nodeRepo, disc)
}

func withIdentity(r *http.Request, globalID string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxkey.UserGlobalID, globalID)
	return r.WithContext(ctx)
}

// — Follow handler ────────────────────────────────────────────────────

func TestFollowHandler_Follow(t *testing.T) {
	tests := []struct {
		name       string
		identity   string
		body       any
		wantStatus int
	}{
		{
			name:       "success",
			identity:   "alice@node-a",
			body:       map[string]string{"target_global_id": "bob@node-a"},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "missing target_global_id",
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
			name:       "self follow",
			identity:   "alice@node-a",
			body:       map[string]string{"target_global_id": "alice@node-a"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := follows.NewHandler(newHandlerSvc())

			var bodyBytes []byte
			if s, ok := tt.body.(string); ok {
				bodyBytes = []byte(s)
			} else {
				bodyBytes, _ = json.Marshal(tt.body)
			}

			r := httptest.NewRequest(http.MethodPost, "/api/v1/follows", bytes.NewReader(bodyBytes))
			r.Header.Set("Content-Type", "application/json")
			r = withIdentity(r, tt.identity)
			w := httptest.NewRecorder()

			h.Follow(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestFollowHandler_Follow_Duplicate(t *testing.T) {
	h := follows.NewHandler(newHandlerSvc())
	body, _ := json.Marshal(map[string]string{"target_global_id": "bob@node-a"})

	r1 := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/follows", bytes.NewReader(body)), "alice@node-a")
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.Follow(w1, r1)
	if w1.Code != http.StatusNoContent {
		t.Fatalf("first follow: got %d, want %d", w1.Code, http.StatusNoContent)
	}

	r2 := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/follows", bytes.NewReader(body)), "alice@node-a")
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.Follow(w2, r2)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate follow: got %d, want %d", w2.Code, http.StatusConflict)
	}
}

// — Unfollow handler ──────────────────────────────────────────────────

func TestFollowHandler_Unfollow(t *testing.T) {
	svc := newHandlerSvc()
	h := follows.NewHandler(svc)

	_ = svc.Follow(context.Background(), "alice@node-a", "bob@node-a")

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/follows/bob@node-a", nil)
	r.SetPathValue("target", "bob@node-a")
	r = withIdentity(r, "alice@node-a")
	w := httptest.NewRecorder()

	h.Unfollow(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestFollowHandler_Unfollow_NotFollowing(t *testing.T) {
	h := follows.NewHandler(newHandlerSvc())

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/follows/bob@node-a", nil)
	r.SetPathValue("target", "bob@node-a")
	r = withIdentity(r, "alice@node-a")
	w := httptest.NewRecorder()

	h.Unfollow(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestFollowHandler_NoIdentity(t *testing.T) {
	h := follows.NewHandler(newHandlerSvc())

	body, _ := json.Marshal(map[string]string{"target_global_id": "bob@node-a"})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/follows", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Follow(w, r)

	if w.Code == http.StatusNoContent {
		t.Error("should not succeed with empty identity")
	}
}
