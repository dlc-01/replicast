package auth

import (
	"encoding/json"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Username == "" || req.Password == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "username and password required"))
		return
	}

	result, err := h.svc.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		respond.Error(w, r, err)
		return
	}

	respond.JSON(w, http.StatusCreated, map[string]any{
		"token":     result.Token,
		"global_id": result.User.GlobalID,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Username == "" || req.Password == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "username and password required"))
		return
	}

	token, err := h.svc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		respond.Error(w, r, err)
		return
	}

	respond.JSON(w, http.StatusOK, map[string]string{"token": token})
}
