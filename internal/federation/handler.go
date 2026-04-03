package federation

import (
	"encoding/json"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
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

// WellKnown — node discovery endpoint.
// GET /.well-known/replicast
func (h *Handler) WellKnown(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, h.svc.NodeInfo())
}

// Handshake — регистрация входящего узла.
// POST /api/v1/federation/handshake
func (h *Handler) Handshake(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeName string `json:"node_name"`
		BaseURL  string `json:"base_url"`
		Secret   string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.NodeName == "" || req.BaseURL == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "node_name and base_url required"))
		return
	}

	if err := h.svc.Handshake(r.Context(), req.NodeName, req.BaseURL, req.Secret); err != nil {
		respond.Error(w, r, err)
		return
	}

	// Возвращаем свои метаданные — двусторонний обмен
	respond.JSON(w, http.StatusOK, h.svc.NodeInfo())
}

// ReceiveEvent — входящее событие от другого узла.
// POST /api/v1/federation/events
func (h *Handler) ReceiveEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID    string          `json:"event_id"`
		EventType  string          `json:"event_type"`
		SourceNode string          `json:"source_node"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "invalid event payload"))
		return
	}
	if req.EventID == "" || req.EventType == "" {
		respond.Error(w, r, apperr.BadRequest("invalid_body", "event_id and event_type required"))
		return
	}

	if err := h.svc.ReceiveEvent(r.Context(), req.EventID, req.SourceNode, req.EventType, req.Payload); err != nil {
		respond.Error(w, r, err)
		return
	}

	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ReceiveFollow — входящая подписка (альтернативный endpoint для совместимости).
// POST /api/v1/federation/follows
func (h *Handler) ReceiveFollow(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, map[string]string{
		"message": "use /federation/events with event_type=user.followed",
	})
}

// GetRemoteUser — профиль пользователя для federation запросов.
// GET /api/v1/federation/users/{global_id}
func (h *Handler) GetRemoteUser(w http.ResponseWriter, r *http.Request) {
	globalID := r.PathValue("global_id")
	if globalID == "" {
		respond.Error(w, r, apperr.BadRequest("missing_param", "global_id required"))
		return
	}

	u, err := h.svc.GetRemoteUser(r.Context(), globalID)
	if err != nil {
		respond.Error(w, r, err)
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"global_id":    u.GlobalID,
		"home_node":    u.HomeNode,
		"display_name": u.DisplayName,
		"bio":          u.Bio,
	})
}
