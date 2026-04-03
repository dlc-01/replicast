package feed

import (
	"net/http"
	"strconv"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	ownerGlobalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if ownerGlobalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity in context"))
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	items, err := h.svc.GetFeed(r.Context(), ownerGlobalID, limit)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}
