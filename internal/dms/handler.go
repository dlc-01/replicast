package dms

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// StartConversation — POST /api/v1/conversations
func (h *Handler) StartConversation(w http.ResponseWriter, r *http.Request) {
	senderGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if senderGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity"))
		return
	}

	var req struct {
		RecipientGlobalID string `json:"recipient_global_id"`
		SessionKeyForMe   string `json:"session_key_for_me"`   // AES key зашифрованный моим RSA ключом
		SessionKeyForThem string `json:"session_key_for_them"` // AES key зашифрованный их RSA ключом
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RecipientGlobalID == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "recipient_global_id required"))
		return
	}

	conv, err := h.svc.StartConversation(r.Context(), senderGlobalID, req.RecipientGlobalID, req.SessionKeyForMe, req.SessionKeyForThem)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, conv)
}

// ListConversations — GET /api/v1/conversations
func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if userGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity"))
		return
	}

	convs, err := h.svc.ListConversations(r.Context(), userGlobalID)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"items": convs, "count": len(convs)})
}

// SendMessage — POST /api/v1/conversations/{id}/messages
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	senderGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if senderGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity"))
		return
	}
	convID := r.PathValue("id")

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "content required"))
		return
	}

	msg, err := h.svc.SendMessage(r.Context(), senderGlobalID, convID, req.Content)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusCreated, msg)
}

// GetMessages — GET /api/v1/conversations/{id}/messages
func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if userGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity"))
		return
	}
	convID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	msgs, err := h.svc.GetMessages(r.Context(), userGlobalID, convID, limit)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"items": msgs, "count": len(msgs)})
}
