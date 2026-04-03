package users

import (
	"encoding/json"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		respond.Error(w, r, apperr.BadRequest("missing_param", "username required"))
		return
	}

	u, err := h.svc.GetProfile(r.Context(), username)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(u))
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	globalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
	if globalID == "" {
		respond.Error(w, r, apperr.Unauthorized("missing_identity", "no identity in context"))
		return
	}

	var req struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "invalid json"))
		return
	}

	u, err := h.svc.UpdateProfile(r.Context(), globalID, req.DisplayName, req.Bio)
	if err != nil {
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(u))
}

// userResponse — публичное представление пользователя (без hash).
type userResponse struct {
	GlobalID    string `json:"global_id"`
	Username    string `json:"username"`
	HomeNode    string `json:"home_node"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
}

func toResponse(u *port.User) userResponse {
	return userResponse{
		GlobalID:    u.GlobalID,
		Username:    u.LocalUsername,
		HomeNode:    u.HomeNode,
		DisplayName: u.DisplayName,
		Bio:         u.Bio,
	}
}
