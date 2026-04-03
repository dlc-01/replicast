package users_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/users"
)

func TestHandler_GetProfile(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		setup      func(*mockUserRepo)
		wantStatus int
	}{
		{
			name:     "found",
			username: "alice",
			setup: func(r *mockUserRepo) {
				r.byName["alice"] = &port.User{
					GlobalID:      "alice@node-a",
					LocalUsername: "alice",
					HomeNode:      "node-a",
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			username:   "nobody",
			setup:      func(r *mockUserRepo) {},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			tt.setup(repo)

			svc := users.NewService(repo, testCfg())
			h := users.NewHandler(svc)

			r := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+tt.username, nil)
			r.SetPathValue("username", tt.username)
			w := httptest.NewRecorder()

			h.GetProfile(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandler_GetProfile_ResponseShape(t *testing.T) {
	repo := newMockRepo()
	repo.byName["alice"] = &port.User{
		GlobalID:      "alice@node-a",
		LocalUsername: "alice",
		HomeNode:      "node-a",
		DisplayName:   "Alice",
		Bio:           "hello",
	}

	svc := users.NewService(repo, testCfg())
	h := users.NewHandler(svc)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/alice", nil)
	r.SetPathValue("username", "alice")
	w := httptest.NewRecorder()
	h.GetProfile(w, r)

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["global_id"] != "alice@node-a" {
		t.Errorf("global_id = %v, want alice@node-a", resp["global_id"])
	}
	// password_hash не должен уходить клиенту
	if _, ok := resp["password_hash"]; ok {
		t.Error("password_hash should not be in response")
	}
}

func TestHandler_UpdateProfile(t *testing.T) {
	repo := newMockRepo()
	repo.data["alice@node-a"] = &port.User{
		ID:            "uuid-1",
		GlobalID:      "alice@node-a",
		LocalUsername: "alice",
		HomeNode:      "node-a",
	}

	svc := users.NewService(repo, testCfg())
	h := users.NewHandler(svc)

	body, _ := json.Marshal(map[string]string{
		"display_name": "Alice Wonderland",
		"bio":          "curiouser and curiouser",
	})

	r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
	ctx := context.WithValue(r.Context(), ctxkey.UserGlobalID, "alice@node-a")
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()

	h.UpdateProfile(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
}

func TestHandler_UpdateProfile_NoIdentity(t *testing.T) {
	svc := users.NewService(newMockRepo(), testCfg())
	h := users.NewHandler(svc)

	body, _ := json.Marshal(map[string]string{"display_name": "Alice"})
	r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.UpdateProfile(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandler_UpdateProfile_InvalidJSON(t *testing.T) {
	svc := users.NewService(newMockRepo(), testCfg())
	h := users.NewHandler(svc)

	r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader([]byte("not-json")))
	ctx := context.WithValue(r.Context(), ctxkey.UserGlobalID, "alice@node-a")
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()

	h.UpdateProfile(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
