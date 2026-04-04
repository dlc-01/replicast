package comments

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

// Create — POST /api/v1/posts/{global_id}/comments
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if userGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity"))
		return
	}
	postGlobalID := r.PathValue("global_id")

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "content required"))
		return
	}

	c, err := h.svc.Create(r.Context(), userGlobalID, postGlobalID, req.Content)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusCreated, c)
}

// List — GET /api/v1/posts/{global_id}/comments
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	postGlobalID := r.PathValue("global_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	items, err := h.svc.GetByPost(r.Context(), postGlobalID, limit)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

// Delete — DELETE /api/v1/comments/{global_id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if userGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity"))
		return
	}
	globalID := r.PathValue("global_id")

	if err := h.svc.Delete(r.Context(), globalID, userGlobalID); err != nil {
		respond.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
