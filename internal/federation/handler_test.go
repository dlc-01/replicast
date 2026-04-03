package federation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/port"
)

// — Моки (минимальные заглушки для Фазы 1) ──────────────────────────

type mockFedRepo struct{}

func (m *mockFedRepo) EnqueueEvent(_ context.Context, _ port.OutboxEvent) error { return nil }
func (m *mockFedRepo) GetPendingEvents(_ context.Context, _ int) ([]port.OutboxRow, error) {
	return nil, nil
}
func (m *mockFedRepo) MarkDelivered(_ context.Context, _ string) error               { return nil }
func (m *mockFedRepo) MarkFailed(_ context.Context, _ string, _ int) error           { return nil }
func (m *mockFedRepo) IsProcessed(_ context.Context, _ string) (bool, error)         { return false, nil }
func (m *mockFedRepo) MarkProcessed(_ context.Context, _, _ string) error            { return nil }
func (m *mockFedRepo) GetNodeByName(_ context.Context, _ string) (*port.Node, error) { return nil, nil }
func (m *mockFedRepo) UpsertNode(_ context.Context, _ port.Node) error               { return nil }

type mockPostRepo struct{}

func (m *mockPostRepo) Create(_ context.Context, _ port.Post) error             { return nil }
func (m *mockPostRepo) GetByID(_ context.Context, _ string) (*port.Post, error) { return nil, nil }
func (m *mockPostRepo) GetByGlobalID(_ context.Context, _ string) (*port.Post, error) {
	return nil, nil
}
func (m *mockPostRepo) Update(_ context.Context, _, _ string) (*port.Post, error) { return nil, nil }
func (m *mockPostRepo) Delete(_ context.Context, _ string) (*port.Post, error)    { return nil, nil }
func (m *mockPostRepo) GetFollowerNodes(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

type mockUserRepo struct{}

func (m *mockUserRepo) Create(_ context.Context, _ port.User) error             { return nil }
func (m *mockUserRepo) GetByID(_ context.Context, _ string) (*port.User, error) { return nil, nil }
func (m *mockUserRepo) GetByGlobalID(_ context.Context, _ string) (*port.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetByUsername(_ context.Context, _ string) (*port.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetUUIDByGlobalID(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockUserRepo) UpdateProfile(_ context.Context, _, _, _ string) error         { return nil }
func (m *mockUserRepo) UpsertRemote(_ context.Context, _ port.User) error             { return nil }
func (m *mockUserRepo) UsernameExists(_ context.Context, _ string) (bool, error)      { return false, nil }
func (m *mockUserRepo) GetPasswordHash(_ context.Context, _ string) (string, error)   { return "", nil }

type mockFeedRepo struct{}

func (m *mockFeedRepo) AddItem(_ context.Context, _ port.FeedItem) error { return nil }
func (m *mockFeedRepo) RemoveItem(_ context.Context, _, _ string) error  { return nil }
func (m *mockFeedRepo) GetFeed(_ context.Context, _ string, _ int) ([]port.FeedPost, error) {
	return nil, nil
}
func (m *mockFeedRepo) GetFollowerUserIDs(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

// — Хелпер ───────────────────────────────────────────────────────────

func newTestHandler(nodeName, baseURL string) *federation.Handler {
	cfg := &config.Config{
		NodeName: nodeName,
		BaseURL:  baseURL,
	}
	svc := federation.NewService(
		&mockFedRepo{}, &mockPostRepo{}, &mockUserRepo{}, &mockFeedRepo{}, cfg,
	)
	return federation.NewHandler(svc, cfg)
}

// — Тесты ────────────────────────────────────────────────────────────

func TestFederationHandler_WellKnown(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodGet, "/.well-known/replicast", nil)
	w := httptest.NewRecorder()
	h.WellKnown(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["node"] != "node-a" {
		t.Errorf("node = %q, want node-a", resp["node"])
	}
	if resp["base_url"] != "http://node-a:8080" {
		t.Errorf("base_url = %q, want http://node-a:8080", resp["base_url"])
	}
	if resp["version"] != "1" {
		t.Errorf("version = %q, want 1", resp["version"])
	}
}

func TestFederationHandler_Handshake(t *testing.T) {
	h := newTestHandler("node-b", "http://node-b:8080")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/handshake", nil)
	w := httptest.NewRecorder()
	h.Handshake(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["node"] != "node-b" {
		t.Errorf("node = %q, want node-b", resp["node"])
	}
}

func TestFederationHandler_WellKnown_DifferentNodes(t *testing.T) {
	tests := []struct {
		nodeName string
		baseURL  string
	}{
		{"node-a", "http://node-a:8080"},
		{"node-b", "https://petya.ru"},
		{"node-c", "https://vasya.mesh"},
	}

	for _, tt := range tests {
		t.Run(tt.nodeName, func(t *testing.T) {
			h := newTestHandler(tt.nodeName, tt.baseURL)

			r := httptest.NewRequest(http.MethodGet, "/.well-known/replicast", nil)
			w := httptest.NewRecorder()
			h.WellKnown(w, r)

			var resp map[string]string
			json.NewDecoder(w.Body).Decode(&resp)

			if resp["node"] != tt.nodeName {
				t.Errorf("node = %q, want %q", resp["node"], tt.nodeName)
			}
			if resp["base_url"] != tt.baseURL {
				t.Errorf("base_url = %q, want %q", resp["base_url"], tt.baseURL)
			}
		})
	}
}

func TestFederationHandler_ReceiveEvent_NotImplemented(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/events", nil)
	w := httptest.NewRecorder()
	h.ReceiveEvent(w, r)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestFederationHandler_ReceiveFollow_NotImplemented(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/follows", nil)
	w := httptest.NewRecorder()
	h.ReceiveFollow(w, r)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestFederationHandler_GetRemoteUser_NotImplemented(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodGet, "/api/v1/federation/users/alice@node-b", nil)
	w := httptest.NewRecorder()
	h.GetRemoteUser(w, r)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}
