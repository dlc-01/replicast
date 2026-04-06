package users

import (
	"encoding/json"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// GetProfile — GET /api/v1/users/{username}
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
	respond.JSON(w, http.StatusOK, u)
}

// UpdateProfile — PUT /api/v1/users/me
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
		respond.Error(w, r, apperr.BadRequest("invalid_body", "invalid JSON"))
		return
	}

	if _, err := h.svc.UpdateProfile(r.Context(), globalID, req.DisplayName, req.Bio); err != nil {
		respond.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPublicKey — GET /api/v1/users/{username}/key
// Возвращает публичный RSA ключ пользователя для E2E шифрования DM.
func (h *Handler) GetPublicKey(w http.ResponseWriter, r *http.Request) {
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

	if u.PublicKey == "" {
		respond.Error(w, r, apperr.NotFound("no_key", "user has no public key"))
		return
	}

	respond.JSON(w, http.StatusOK, map[string]string{
		"global_id":  u.GlobalID,
		"public_key": u.PublicKey,
	})
}
