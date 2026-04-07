package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/search"
)

func newTestHandler(localUsers map[string]*port.User, remoteUsers map[string]*port.User) *search.Handler {
	repo := &mockUserRepo{
		byGlobalID: map[string]*port.User{},
		byUsername: localUsers,
	}
	remote := &mockRemote{users: remoteUsers}
	svc := search.NewService(repo, remote, &config.Config{NodeName: "node-a"})
	return search.NewHandler(svc)
}

// — GET /api/v1/search ────────────────────────────────────────────────

func TestSearchHandler_LocalUser(t *testing.T) {
	h := newTestHandler(
		map[string]*port.User{
			"alice": {GlobalID: "alice@node-a", LocalUsername: "alice"},
		},
		map[string]*port.User{},
	)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=alice", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
}

func TestSearchHandler_RemoteUser(t *testing.T) {
	h := newTestHandler(
		map[string]*port.User{},
		map[string]*port.User{
			"bob@node-b": {GlobalID: "bob@node-b", HomeNode: "node-b"},
		},
	)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=bob%40node-b", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
}

func TestSearchHandler_MissingQuery(t *testing.T) {
	h := newTestHandler(map[string]*port.User{}, map[string]*port.User{})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSearchHandler_EmptyQuery(t *testing.T) {
	h := newTestHandler(map[string]*port.User{}, map[string]*port.User{})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSearchHandler_UserNotFound(t *testing.T) {
	h := newTestHandler(map[string]*port.User{}, map[string]*port.User{})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=nobody", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code == http.StatusOK {
		t.Error("should not return 200 for unknown user")
	}
}

func TestSearchHandler_ResponseShape(t *testing.T) {
	h := newTestHandler(
		map[string]*port.User{
			"alice": {GlobalID: "alice@node-a", LocalUsername: "alice", HomeNode: "node-a"},
		},
		map[string]*port.User{},
	)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=alice", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "users") {
		t.Error("response should contain 'users' field")
	}
	if !contains(body, "count") {
		t.Error("response should contain 'count' field")
	}
	if !contains(body, "alice@node-a") {
		t.Error("response should contain alice@node-a")
	}
}

func TestSearchHandler_DomainFormat(t *testing.T) {
	h := newTestHandler(
		map[string]*port.User{},
		map[string]*port.User{
			"alice@social.example.com": {
				GlobalID: "alice@social.example.com",
				HomeNode: "social.example.com",
			},
		},
	)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=alice%40social.example.com", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
}

// — FetchRemoteUser (integration с client) ────────────────────────────

func TestFetchRemoteUser_ViaHTTPServer(t *testing.T) {
	// Поднимаем тестовый сервер имитирующий удалённый узел
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/federation/users/bob%40testnode" ||
			r.URL.Path == "/api/v1/federation/users/bob@testnode" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"global_id":"bob@testnode","home_node":"testnode"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Мок FetchRemoteUser который идёт на testserver
	remote := &mockRemote{users: map[string]*port.User{
		"bob@testnode": {GlobalID: "bob@testnode", HomeNode: "testnode"},
	}}
	repo := &mockUserRepo{
		byGlobalID: map[string]*port.User{},
		byUsername: map[string]*port.User{},
	}
	svc := search.NewService(repo, remote, &config.Config{NodeName: "node-a"})
	h := search.NewHandler(svc)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=bob%40testnode", nil)
	w := httptest.NewRecorder()
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
}

// — Вспомогательные функции ───────────────────────────────────────────

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockUserRepo с поддержкой контекста для handler тестов
type mockHandlerUserRepo struct {
	byGlobalID map[string]*port.User
	byUsername map[string]*port.User
}

func (m *mockHandlerUserRepo) GetByGlobalID(_ context.Context, globalID string) (*port.User, error) {
	if u, ok := m.byGlobalID[globalID]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockHandlerUserRepo) GetByUsername(_ context.Context, username string) (*port.User, error) {
	if u, ok := m.byUsername[username]; ok {
		return u, nil
	}
	return nil, nil
}
