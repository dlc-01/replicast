package dms_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/dms"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

func withDMIdentity(r *http.Request, globalID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxkey.UserGlobalID, globalID))
}

func newTestDMHandler() *dms.Handler {
	repo := newMockDMRepo()
	fed := &mockDMFed{}
	svc := dms.NewService(repo, fed, logger.Nop(), &config.Config{NodeName: "node-a"})
	return dms.NewHandler(svc)
}

func TestDMHandler_StartConversation_Success(t *testing.T) {
	h := newTestDMHandler()
	body, _ := json.Marshal(map[string]string{"recipient_global_id": "bob@node-b"})
	r := withDMIdentity(
		httptest.NewRequest(http.MethodPost, "/api/v1/conversations", bytes.NewReader(body)), "alice@node-a",
	)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StartConversation(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var conv port.Conversation
	json.NewDecoder(w.Body).Decode(&conv)
	if conv.ID == "" {
		t.Error("conversation ID should not be empty")
	}
}

func TestDMHandler_StartConversation_NoIdentity(t *testing.T) {
	h := newTestDMHandler()
	body, _ := json.Marshal(map[string]string{"recipient_global_id": "bob@node-b"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StartConversation(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDMHandler_StartConversation_MissingRecipient(t *testing.T) {
	h := newTestDMHandler()
	body, _ := json.Marshal(map[string]string{})
	r := withDMIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a",
	)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StartConversation(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDMHandler_SendMessage_Success(t *testing.T) {
	h := newTestDMHandler()

	// Создаём диалог
	body, _ := json.Marshal(map[string]string{"recipient_global_id": "bob@node-b"})
	r1 := withDMIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a",
	)
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.StartConversation(w1, r1)

	var conv port.Conversation
	json.NewDecoder(w1.Body).Decode(&conv)

	// Отправляем сообщение
	msgBody, _ := json.Marshal(map[string]string{"content": "hello bob!"})
	r2 := withDMIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(msgBody)), "alice@node-a",
	)
	r2.Header.Set("Content-Type", "application/json")
	r2.SetPathValue("id", conv.ID)
	w2 := httptest.NewRecorder()
	h.SendMessage(w2, r2)

	if w2.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", w2.Code, http.StatusCreated, w2.Body.String())
	}
}

func TestDMHandler_SendMessage_EmptyContent(t *testing.T) {
	h := newTestDMHandler()

	body, _ := json.Marshal(map[string]string{"recipient_global_id": "bob@node-b"})
	r1 := withDMIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a",
	)
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.StartConversation(w1, r1)
	var conv port.Conversation
	json.NewDecoder(w1.Body).Decode(&conv)

	msgBody, _ := json.Marshal(map[string]string{"content": ""})
	r2 := withDMIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(msgBody)), "alice@node-a",
	)
	r2.Header.Set("Content-Type", "application/json")
	r2.SetPathValue("id", conv.ID)
	w2 := httptest.NewRecorder()
	h.SendMessage(w2, r2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w2.Code, http.StatusBadRequest)
	}
}

func TestDMHandler_GetMessages_Success(t *testing.T) {
	h := newTestDMHandler()

	// Создаём диалог и отправляем сообщение
	convBody, _ := json.Marshal(map[string]string{"recipient_global_id": "bob@node-b"})
	r1 := withDMIdentity(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(convBody)), "alice@node-a")
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.StartConversation(w1, r1)
	var conv port.Conversation
	json.NewDecoder(w1.Body).Decode(&conv)

	msgBody, _ := json.Marshal(map[string]string{"content": "test message"})
	r2 := withDMIdentity(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(msgBody)), "alice@node-a")
	r2.Header.Set("Content-Type", "application/json")
	r2.SetPathValue("id", conv.ID)
	h.SendMessage(httptest.NewRecorder(), r2)

	// Получаем сообщения
	r3 := withDMIdentity(httptest.NewRequest(http.MethodGet, "/", nil), "alice@node-a")
	r3.SetPathValue("id", conv.ID)
	w3 := httptest.NewRecorder()
	h.GetMessages(w3, r3)

	if w3.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w3.Code, http.StatusOK, w3.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w3.Body).Decode(&resp)
	if resp["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1", resp["count"])
	}
}

func TestDMHandler_ListConversations_Success(t *testing.T) {
	h := newTestDMHandler()

	body, _ := json.Marshal(map[string]string{"recipient_global_id": "bob@node-b"})
	r1 := withDMIdentity(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a")
	r1.Header.Set("Content-Type", "application/json")
	h.StartConversation(httptest.NewRecorder(), r1)

	r2 := withDMIdentity(httptest.NewRequest(http.MethodGet, "/", nil), "alice@node-a")
	w2 := httptest.NewRecorder()
	h.ListConversations(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w2.Code, http.StatusOK)
	}
	var resp map[string]any
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1", resp["count"])
	}
}
