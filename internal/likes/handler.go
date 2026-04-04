package likes

import (
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Like — POST /api/v1/posts/{global_id}/like
func (h *Handler) Like(w http.ResponseWriter, r *http.Request) {
	userGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if userGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity in context"))
		return
	}
	postGlobalID := r.PathValue("global_id")
	if postGlobalID == "" {
		respond.Error(w, r, apperr.BadRequest("missing_param", "global_id required"))
		return
	}
	if err := h.svc.Like(r.Context(), userGlobalID, postGlobalID); err != nil {
		respond.Error(w, r, err)
		return
	}
	count, _ := h.svc.GetCount(r.Context(), postGlobalID)
	respond.JSON(w, http.StatusOK, map[string]any{"likes": count})
}

// Unlike — DELETE /api/v1/posts/{global_id}/like
func (h *Handler) Unlike(w http.ResponseWriter, r *http.Request) {
	userGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if userGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity in context"))
		return
	}
	postGlobalID := r.PathValue("global_id")
	if postGlobalID == "" {
		respond.Error(w, r, apperr.BadRequest("missing_param", "global_id required"))
		return
	}
	if err := h.svc.Unlike(r.Context(), userGlobalID, postGlobalID); err != nil {
		respond.Error(w, r, err)
		return
	}
	count, _ := h.svc.GetCount(r.Context(), postGlobalID)
	respond.JSON(w, http.StatusOK, map[string]any{"likes": count})
}

// GetLikes — GET /api/v1/posts/{global_id}/likes
func (h *Handler) GetLikes(w http.ResponseWriter, r *http.Request) {
	postGlobalID := r.PathValue("global_id")
	if postGlobalID == "" {
		respond.Error(w, r, apperr.BadRequest("missing_param", "global_id required"))
		return
	}

	count, err := h.svc.GetCount(r.Context(), postGlobalID)
	if err != nil {
		respond.Error(w, r, err)
		return
	}

	userGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	liked, _ := h.svc.IsLiked(r.Context(), userGlobalID, postGlobalID)

	respond.JSON(w, http.StatusOK, map[string]any{
		"post_global_id": postGlobalID,
		"likes":          count,
		"liked_by_me":    liked,
	})
}
