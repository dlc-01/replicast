package federation

import (
	"net/http"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct {
	svc *Service
	cfg *config.Config
}

func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

// ReceiveEvent — TODO Фаза 2.
func (h *Handler) ReceiveEvent(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusNotImplemented, map[string]string{
		"error": "federation not implemented in phase 1",
	})
}

// Handshake возвращает метаданные узла — работает уже в Фазе 1.
func (h *Handler) Handshake(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, h.svc.NodeInfo())
}

// ReceiveFollow — TODO Фаза 2.
func (h *Handler) ReceiveFollow(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusNotImplemented, map[string]string{
		"error": "federation not implemented in phase 1",
	})
}

// GetRemoteUser — TODO Фаза 2.
func (h *Handler) GetRemoteUser(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusNotImplemented, map[string]string{
		"error": "federation not implemented in phase 1",
	})
}

// WellKnown возвращает метаданные узла для node discovery — работает в Фазе 1.
func (h *Handler) WellKnown(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, h.svc.NodeInfo())
}
