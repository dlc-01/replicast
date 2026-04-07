package search

import (
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/respond"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Search — GET /api/v1/search?q=bob@node-b
// Ищет пользователя по global_id.
// Если пользователь на этом узле — возвращает из локальной БД.
// Если на другом узле — делает запрос на тот узел и кэширует результат.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respond.Error(w, r, apperr.BadRequest("missing_query", "q parameter required"))
		return
	}

	result, err := h.svc.Search(r.Context(), q)
	if err != nil {
		respond.Error(w, r, err)
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"users": result,
		"count": len(result),
	})
}
