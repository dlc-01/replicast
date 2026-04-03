package respond

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON сериализует v и пишет статус.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("respond: encode failed", "err", err)
	}
}

// Error определяет HTTP статус из apperr.AppError и пишет тело.
// Неизвестные ошибки → 500.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	if appErr, ok := apperr.As(err); ok {
		if appErr.Status >= 500 {
			slog.ErrorContext(r.Context(), "internal error",
				"code", appErr.Code,
				"err", appErr.Err,
				"path", r.URL.Path,
				"method", r.Method,
			)
		}
		JSON(w, appErr.Status, errorResponse{
			Code:    appErr.Code,
			Message: appErr.Message,
		})
		return
	}
	slog.ErrorContext(r.Context(), "unhandled error",
		"err", err,
		"path", r.URL.Path,
		"method", r.Method,
	)
	JSON(w, http.StatusInternalServerError, errorResponse{
		Code:    "internal_error",
		Message: "internal error",
	})
}
