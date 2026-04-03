package federation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/logger"
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

type mockFollowWriterH struct{}

func (m *mockFollowWriterH) Create(_ context.Context, _ port.Follow) error { return nil }

func newTestHandler(nodeName, baseURL string) *federation.Handler {
	cfg := &config.Config{
		NodeName:        nodeName,
		BaseURL:         baseURL,
		InternalBaseURL: baseURL,
	}
	svc := federation.NewService(
		&mockFedRepo{}, &mockPostRepo{}, &mockUserRepo{}, &mockFeedRepo{}, &mockFollowWriterH{}, logger.Nop(), cfg,
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

	body, _ := json.Marshal(map[string]string{
		"node_name": "node-a",
		"base_url":  "http://node-a:8080",
		"secret":    "shared-secret",
	})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/handshake", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Handshake(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["node"] != "node-b" {
		t.Errorf("node = %q, want node-b", resp["node"])
	}
}

func TestFederationHandler_Handshake_MissingBody(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/handshake", nil)
	w := httptest.NewRecorder()
	h.Handshake(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
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

func TestFederationHandler_ReceiveEvent_ValidBody(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	// user.followed не требует резолва авторов — используем его для handler теста
	payload, _ := json.Marshal(map[string]string{
		"follower_global_id": "bob@node-b",
		"target_global_id":   "alice@node-a",
		"follower_node":      "node-b",
		"follower_base_url":  "http://node-b:8080",
	})
	body, _ := json.Marshal(map[string]any{
		"event_id":    "evt-001",
		"event_type":  "user.followed",
		"source_node": "node-b",
		"payload":     json.RawMessage(payload),
	})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/events", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ReceiveEvent(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestFederationHandler_ReceiveEvent_EmptyBody(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/events", nil)
	w := httptest.NewRecorder()
	h.ReceiveEvent(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestFederationHandler_ReceiveFollow_Returns200(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/federation/follows", nil)
	w := httptest.NewRecorder()
	h.ReceiveFollow(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestFederationHandler_GetRemoteUser_MissingPathValue(t *testing.T) {
	h := newTestHandler("node-a", "http://node-a:8080")

	r := httptest.NewRequest(http.MethodGet, "/api/v1/federation/users/", nil)
	w := httptest.NewRecorder()
	h.GetRemoteUser(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
