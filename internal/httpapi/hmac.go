package httpapi

import (
	"bytes"
	"io"
	"net/http"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/respond"
	"github.com/dlc-01/replicast/internal/signing"
)

// VerifyHMAC middleware проверяет HMAC подпись входящего федерационного запроса.
// Если заголовки подписи отсутствуют — пропускает (обратная совместимость).
// Если заголовки есть — проверяет подпись и timestamp (защита от replay атак).
func VerifyHMAC(cfg *config.Config) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			body, err := signing.Verify(r, cfg.SharedSecret)
			if err != nil {
				respond.Error(w, r, apperr.Forbidden("invalid_signature", err.Error()))
				return
			}
			// Восстанавливаем тело если оно было прочитано
			if body != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			next(w, r)
		}
	}
}
