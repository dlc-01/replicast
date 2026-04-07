package posts

import (
	"encoding/json"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	globalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if globalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity in context"))
		return
	}

	var req struct {
		Content   string `json:"content"`
		HideLikes bool   `json:"hide_likes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "content required"))
		return
	}

	p, err := h.svc.Create(r.Context(), globalID, req.Content, req.HideLikes)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusCreated, p)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context(), r.PathValue("global_id"))
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, p)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	authorGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "content required"))
		return
	}

	p, err := h.svc.Update(r.Context(), r.PathValue("global_id"), authorGlobalID, req.Content)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, p)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	authorGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)

	if err := h.svc.Delete(r.Context(), r.PathValue("global_id"), authorGlobalID); err != nil {
		respond.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
