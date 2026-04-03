package federation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
)

func newTestClient() *federation.Client {
	return federation.NewClient(&config.Config{
		NodeName:     "node-a",
		SharedSecret: "secret",
	})
}

func TestClient_FetchWellKnown_Success(t *testing.T) {
	// Поднимаем тестовый HTTP сервер имитирующий удалённый узел
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/replicast" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"node":     "node-b",
			"base_url": "http://node-b:8080",
			"version":  "1",
		})
	}))
	defer srv.Close()

	client := newTestClient()
	// srv.URL уже содержит http://127.0.0.1:PORT — передаём как nodeName с портом
	nodeName := srv.URL[7:] // убираем "http://"

	wk, err := client.FetchWellKnown(context.Background(), nodeName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wk.Node != "node-b" {
		t.Errorf("node = %q, want node-b", wk.Node)
	}
	if wk.BaseURL != "http://node-b:8080" {
		t.Errorf("base_url = %q, want http://node-b:8080", wk.BaseURL)
	}
}

func TestClient_FetchWellKnown_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient()
	nodeName := srv.URL[7:]

	_, err := client.FetchWellKnown(context.Background(), nodeName)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestClient_FetchWellKnown_ConnectionRefused(t *testing.T) {
	client := newTestClient()

	_, err := client.FetchWellKnown(context.Background(), "localhost:19999")
	if err == nil {
		t.Error("expected error for refused connection")
	}
}

func TestClient_FetchWellKnownInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"node":     "node-b",
			"base_url": "http://node-b:8080",
			"version":  "1",
		})
	}))
	defer srv.Close()

	client := newTestClient()
	nodeName := srv.URL[7:]

	node, baseURL, err := client.FetchWellKnownInfo(context.Background(), nodeName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node != "node-b" {
		t.Errorf("node = %q, want node-b", node)
	}
	if baseURL != "http://node-b:8080" {
		t.Errorf("base_url = %q, want http://node-b:8080", baseURL)
	}
}

func TestClient_SendEvent_Success(t *testing.T) {
	var received federation.EventPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/federation/events" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Replicast-Secret") != "secret" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient()
	payload, _ := json.Marshal(map[string]string{"key": "value"})

	err := client.SendEvent(context.Background(), srv.URL, "secret", federation.EventPayload{
		EventID:    "evt-001",
		EventType:  "post.created",
		SourceNode: "node-a",
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.EventID != "evt-001" {
		t.Errorf("event_id = %q, want evt-001", received.EventID)
	}
}

func TestClient_SendEvent_WrongSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Replicast-Secret") != "correct-secret" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient()
	err := client.SendEvent(context.Background(), srv.URL, "wrong-secret", federation.EventPayload{})
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}
