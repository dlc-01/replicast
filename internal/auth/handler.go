package auth

import (
	"encoding/json"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register — POST /api/v1/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "invalid JSON"))
		return
	}
	if req.Username == "" {
		respond.Error(w, r, apperr.BadRequest("missing_username", "username required"))
		return
	}
	if req.Password == "" {
		respond.Error(w, r, apperr.BadRequest("missing_password", "password required"))
		return
	}

	result, err := h.svc.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		respond.Error(w, r, err)
		return
	}

	// private_key возвращается ОДИН РАЗ при регистрации
	// Клиент должен сохранить его — сервер его не хранит
	respond.JSON(w, http.StatusCreated, map[string]any{
		"token":       result.Token,
		"global_id":   result.User.GlobalID,
		"public_key":  result.User.PublicKey,
		"private_key": result.PrivateKey,
	})
}

// Login — POST /api/v1/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "invalid JSON"))
		return
	}
	if req.Username == "" || req.Password == "" {
		respond.Error(w, r, apperr.BadRequest("missing_fields", "username and password required"))
		return
	}

	token, err := h.svc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		respond.Error(w, r, err)
		return
	}

	globalID := req.Username + "@" + h.svc.cfg.NodeName
	respond.JSON(w, http.StatusOK, map[string]string{
		"token":     token,
		"global_id": globalID,
	})
}
