package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/respond"
)

// Chain объединяет middleware справа налево: первый в списке — самый внешний.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// Logging логирует каждый запрос: метод, путь, статус, latency.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.InfoContext(r.Context(), "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", r.RemoteAddr,
			"request_id", r.Context().Value(ctxkey.RequestID),
		)
	})
}

// Recovery перехватывает panic и возвращает 500.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
					"method", r.Method,
				)
				respond.Error(w, r, apperr.Internal("panic", fmt.Errorf("%v", rec)))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID добавляет X-Request-ID в контекст и ответ.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxkey.RequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth валидирует JWT и кладёт global_id в контекст.
// JWT содержит два поля: sub (UUID) и global_id (alice@node-a).
// В контекст кладём global_id — именно его ожидают все хендлеры.
func RequireAuth(cfg *config.Config) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				respond.Error(w, r, apperr.Unauthorized("missing_token", "authorization header required"))
				return
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, apperr.Unauthorized("invalid_token", "unexpected signing method")
				}
				return []byte(cfg.JWTSecret), nil
			})
			if err != nil || !token.Valid {
				respond.Error(w, r, apperr.Unauthorized("invalid_token", "invalid or expired token"))
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				respond.Error(w, r, apperr.Unauthorized("invalid_token", "invalid claims"))
				return
			}

			// global_id — это alice@node-a, не UUID
			// sub — это UUID пользователя (для внутренних операций с БД)
			globalID, _ := claims["global_id"].(string)
			if globalID == "" {
				respond.Error(w, r, apperr.Unauthorized("invalid_token", "missing global_id claim"))
				return
			}

			ctx := context.WithValue(r.Context(), ctxkey.UserGlobalID, globalID)
			next(w, r.WithContext(ctx))
		}
	}
}

// RequireFedAuth проверяет X-Replicast-Secret для межузловых запросов.
func RequireFedAuth(cfg *config.Config) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Replicast-Secret") != cfg.SharedSecret {
				respond.Error(w, r, apperr.Forbidden("invalid_secret", "invalid node secret"))
				return
			}
			next(w, r)
		}
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(status int) {
	sw.status = status
	sw.ResponseWriter.WriteHeader(status)
}
