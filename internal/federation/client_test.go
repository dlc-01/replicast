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

	_, err := newTestClient().FetchWellKnown(context.Background(), srv.URL[7:])
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestClient_FetchWellKnown_ConnectionRefused(t *testing.T) {
	_, err := newTestClient().FetchWellKnown(context.Background(), "localhost:19999")
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

	node, baseURL, err := newTestClient().FetchWellKnownInfo(context.Background(), srv.URL[7:])
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

	payload, _ := json.Marshal(map[string]string{"key": "value"})
	err := newTestClient().SendEvent(context.Background(), srv.URL, "secret", federation.EventPayload{
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

	err := newTestClient().SendEvent(context.Background(), srv.URL, "wrong-secret", federation.EventPayload{})
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestClient_SendEvent_AddsHMACHeaders(t *testing.T) {
	var gotNode, gotTimestamp, gotSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNode = r.Header.Get("X-Replicast-Node")
		gotTimestamp = r.Header.Get("X-Replicast-Timestamp")
		gotSig = r.Header.Get("X-Replicast-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload, _ := json.Marshal(map[string]string{})
	newTestClient().SendEvent(context.Background(), srv.URL, "secret", federation.EventPayload{
		EventID: "evt-001", EventType: "post.created", Payload: payload,
	})

	if gotNode != "node-a" {
		t.Errorf("X-Replicast-Node = %q, want node-a", gotNode)
	}
	if gotTimestamp == "" {
		t.Error("X-Replicast-Timestamp should be set")
	}
	if gotSig == "" {
		t.Error("X-Replicast-Signature should be set")
	}
}
