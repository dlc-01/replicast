package users_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/users"
)

// — GetProfile ────────────────────────────────────────────────────────

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
			h := users.NewHandler(users.NewService(repo, testCfg()))

			r := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+tt.username, nil)
			r.SetPathValue("username", tt.username)
			w := httptest.NewRecorder()
			h.GetProfile(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s", w.Code, tt.wantStatus, w.Body.String())
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
	h := users.NewHandler(users.NewService(repo, testCfg()))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/alice", nil)
	r.SetPathValue("username", "alice")
	w := httptest.NewRecorder()
	h.GetProfile(w, r)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["global_id"] != "alice@node-a" {
		t.Errorf("global_id = %v, want alice@node-a", resp["global_id"])
	}
	if _, ok := resp["password_hash"]; ok {
		t.Error("password_hash should NOT be in response")
	}
}

// — UpdateProfile ─────────────────────────────────────────────────────

func TestHandler_UpdateProfile(t *testing.T) {
	repo := newMockRepo()
	repo.data["alice@node-a"] = &port.User{
		ID: "uuid-1", GlobalID: "alice@node-a",
		LocalUsername: "alice", HomeNode: "node-a",
	}
	h := users.NewHandler(users.NewService(repo, testCfg()))

	body, _ := json.Marshal(map[string]string{
		"display_name": "Alice Wonderland",
		"bio":          "curiouser and curiouser",
	})
	r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), ctxkey.UserGlobalID, "alice@node-a"))
	w := httptest.NewRecorder()
	h.UpdateProfile(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204\nbody: %s", w.Code, w.Body.String())
	}
}

func TestHandler_UpdateProfile_NoIdentity(t *testing.T) {
	h := users.NewHandler(users.NewService(newMockRepo(), testCfg()))
	body, _ := json.Marshal(map[string]string{"display_name": "Alice"})
	r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.UpdateProfile(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandler_UpdateProfile_InvalidJSON(t *testing.T) {
	h := users.NewHandler(users.NewService(newMockRepo(), testCfg()))
	r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader([]byte("not-json")))
	r = r.WithContext(context.WithValue(r.Context(), ctxkey.UserGlobalID, "alice@node-a"))
	w := httptest.NewRecorder()
	h.UpdateProfile(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// — GetPublicKey ──────────────────────────────────────────────────────

func TestHandler_GetPublicKey_Success(t *testing.T) {
	repo := newMockRepo()
	repo.byName["alice"] = &port.User{
		GlobalID:      "alice@node-a",
		LocalUsername: "alice",
		HomeNode:      "node-a",
		PublicKey:     "-----BEGIN PUBLIC KEY-----\nMIIBIjANBg==\n-----END PUBLIC KEY-----",
	}
	h := users.NewHandler(users.NewService(repo, testCfg()))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/alice/key", nil)
	r.SetPathValue("username", "alice")
	w := httptest.NewRecorder()
	h.GetPublicKey(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["global_id"] != "alice@node-a" {
		t.Errorf("global_id = %q, want alice@node-a", resp["global_id"])
	}
	if !strings.HasPrefix(resp["public_key"], "-----BEGIN") {
		t.Errorf("public_key = %q, should be PEM", resp["public_key"])
	}
}

func TestHandler_GetPublicKey_UserNotFound(t *testing.T) {
	h := users.NewHandler(users.NewService(newMockRepo(), testCfg()))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/nobody/key", nil)
	r.SetPathValue("username", "nobody")
	w := httptest.NewRecorder()
	h.GetPublicKey(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandler_GetPublicKey_NoKey(t *testing.T) {
	repo := newMockRepo()
	repo.byName["alice"] = &port.User{
		GlobalID: "alice@node-a", LocalUsername: "alice", HomeNode: "node-a",
		PublicKey: "", // ключа нет
	}
	h := users.NewHandler(users.NewService(repo, testCfg()))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/alice/key", nil)
	r.SetPathValue("username", "alice")
	w := httptest.NewRecorder()
	h.GetPublicKey(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no public key", w.Code)
	}
}

func TestHandler_GetPublicKey_NoPrivateKey(t *testing.T) {
	repo := newMockRepo()
	repo.byName["alice"] = &port.User{
		GlobalID: "alice@node-a", LocalUsername: "alice", HomeNode: "node-a",
		PublicKey: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
	}
	h := users.NewHandler(users.NewService(repo, testCfg()))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/alice/key", nil)
	r.SetPathValue("username", "alice")
	w := httptest.NewRecorder()
	h.GetPublicKey(w, r)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	// Ответ содержит только public_key — private_key никогда не возвращается
	if _, ok := resp["private_key"]; ok {
		t.Error("private_key should NEVER be in GetPublicKey response")
	}
}
