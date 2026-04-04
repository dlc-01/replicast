package comments_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/comments"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

func withCommentIdentity(r *http.Request, globalID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxkey.UserGlobalID, globalID))
}

func newTestCommentHandler() *comments.Handler {
	repo := newMockCommentRepo()
	fed := &mockCommentFed{}
	posts := &mockCommentPostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := comments.NewService(repo, fed, posts, logger.Nop(), &config.Config{NodeName: "node-a"})
	return comments.NewHandler(svc)
}

func TestCommentHandler_Create_Success(t *testing.T) {
	h := newTestCommentHandler()
	body, _ := json.Marshal(map[string]string{"content": "nice post!"})
	r := withCommentIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a",
	)
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("global_id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestCommentHandler_Create_NoIdentity(t *testing.T) {
	h := newTestCommentHandler()
	body, _ := json.Marshal(map[string]string{"content": "test"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("global_id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestCommentHandler_Create_EmptyContent(t *testing.T) {
	h := newTestCommentHandler()
	body, _ := json.Marshal(map[string]string{"content": ""})
	r := withCommentIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a",
	)
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("global_id", "post:alice@node-a:001")
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCommentHandler_List_Success(t *testing.T) {
	h := newTestCommentHandler()

	// Создаём комментарий
	body, _ := json.Marshal(map[string]string{"content": "hello"})
	r1 := withCommentIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a",
	)
	r1.Header.Set("Content-Type", "application/json")
	r1.SetPathValue("global_id", "post:alice@node-a:001")
	h.Create(httptest.NewRecorder(), r1)

	// Получаем список
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.SetPathValue("global_id", "post:alice@node-a:001")
	w2 := httptest.NewRecorder()
	h.List(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w2.Code, http.StatusOK)
	}
	var resp map[string]any
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1", resp["count"])
	}
}

func TestCommentHandler_Delete_Success(t *testing.T) {
	h := newTestCommentHandler()

	// Создаём
	body, _ := json.Marshal(map[string]string{"content": "delete me"})
	r1 := withCommentIdentity(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "alice@node-a",
	)
	r1.Header.Set("Content-Type", "application/json")
	r1.SetPathValue("global_id", "post:alice@node-a:001")
	w1 := httptest.NewRecorder()
	h.Create(w1, r1)

	var created port.Comment
	json.NewDecoder(w1.Body).Decode(&created)

	// Удаляем
	r2 := withCommentIdentity(httptest.NewRequest(http.MethodDelete, "/", nil), "alice@node-a")
	r2.SetPathValue("global_id", created.GlobalID)
	w2 := httptest.NewRecorder()
	h.Delete(w2, r2)

	if w2.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d\nbody: %s", w2.Code, http.StatusNoContent, w2.Body.String())
	}
}

func TestCommentHandler_Delete_NoIdentity(t *testing.T) {
	h := newTestCommentHandler()
	r := httptest.NewRequest(http.MethodDelete, "/", nil)
	r.SetPathValue("global_id", "comment:alice@node-a:001")
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
