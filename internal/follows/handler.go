package follows

import (
	"encoding/json"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Follow — POST /api/v1/follows
func (h *Handler) Follow(w http.ResponseWriter, r *http.Request) {
	followerGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if followerGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity in context"))
		return
	}

	var req struct {
		TargetGlobalID string `json:"target_global_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetGlobalID == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "target_global_id required"))
		return
	}

	if err := h.svc.Follow(r.Context(), followerGlobalID, req.TargetGlobalID); err != nil {
		respond.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unfollow — DELETE /api/v1/follows/{target}
func (h *Handler) Unfollow(w http.ResponseWriter, r *http.Request) {
	followerGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if followerGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity in context"))
		return
	}

	target := r.PathValue("target")
	if target == "" {
		respond.Error(w, r, apperr.BadRequest("missing_param", "target required"))
		return
	}

	if err := h.svc.Unfollow(r.Context(), followerGlobalID, target); err != nil {
		respond.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
