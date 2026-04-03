package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/ctxkey"
	"github.com/dlc-01/replicast/internal/httpapi"
)

func testConfig() *config.Config {
	return &config.Config{
		JWTSecret:    "test-secret-key-that-is-long-enough-32",
		SharedSecret: "node-secret",
	}
}

// makeToken создаёт JWT с правильными полями:
// sub = UUID, global_id = "alice@node-a" — именно так делает auth.Service.
func makeToken(secret, globalID string, exp time.Time) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":       "some-uuid-of-user",
		"global_id": globalID,
		"exp":       exp.Unix(),
	})
	signed, _ := token.SignedString([]byte(secret))
	return signed
}

// makeTokenNoGlobalID — токен без global_id claim (для теста невалидного токена).
func makeTokenNoGlobalID(secret string, exp time.Time) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "some-uuid",
		"exp": exp.Unix(),
	})
	signed, _ := token.SignedString([]byte(secret))
	return signed
}

func TestRequireAuth(t *testing.T) {
	cfg := testConfig()

	// Хендлер читает global_id из контекста и пишет его в тело ответа
	handler := httpapi.RequireAuth(cfg)(func(w http.ResponseWriter, r *http.Request) {
		globalID, _ := r.Context().Value(ctxkey.UserGlobalID).(string)
		w.Write([]byte(globalID))
	})

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid token — global_id в контексте",
			authHeader: "Bearer " + makeToken(cfg.JWTSecret, "alice@node-a", time.Now().Add(time.Hour)),
			wantStatus: http.StatusOK,
			wantBody:   "alice@node-a",
		},
		{
			name:       "missing header",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "expired token",
			authHeader: "Bearer " + makeToken(cfg.JWTSecret, "alice@node-a", time.Now().Add(-time.Hour)),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong secret",
			authHeader: "Bearer " + makeToken("wrong-secret-key-that-is-long-enough-32", "alice@node-a", time.Now().Add(time.Hour)),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no global_id claim",
			authHeader: "Bearer " + makeTokenNoGlobalID(cfg.JWTSecret, time.Now().Add(time.Hour)),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed token",
			authHeader: "Bearer not.a.jwt",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer prefix missing",
			authHeader: makeToken(cfg.JWTSecret, "alice@node-a", time.Now().Add(time.Hour)),
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				r.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			handler(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRequireFedAuth(t *testing.T) {
	cfg := testConfig()
	handler := httpapi.RequireFedAuth(cfg)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		secret     string
		wantStatus int
	}{
		{"valid secret", "node-secret", http.StatusOK},
		{"wrong secret", "wrong", http.StatusForbidden},
		{"empty secret", "", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.Header.Set("X-Replicast-Secret", tt.secret)
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestRecovery_CatchesPanic(t *testing.T) {
	handler := httpapi.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Не должен паниковать
	if !t.Run("no panic", func(t *testing.T) {
		handler.ServeHTTP(w, r)
	}) {
		t.Error("recovery middleware panicked")
	}

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestRequestID_SetsHeader(t *testing.T) {
	handler := httpapi.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := r.Context().Value(ctxkey.RequestID).(string)
		w.Write([]byte(id))
	}))

	t.Run("uses provided X-Request-ID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Request-ID", "my-request-id")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Header().Get("X-Request-ID") != "my-request-id" {
			t.Errorf("X-Request-ID header = %q, want my-request-id", w.Header().Get("X-Request-ID"))
		}
		if w.Body.String() != "my-request-id" {
			t.Errorf("context request_id = %q, want my-request-id", w.Body.String())
		}
	})

	t.Run("generates ID when missing", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Header().Get("X-Request-ID") == "" {
			t.Error("expected X-Request-ID to be set")
		}
	})
}

func TestLogging_PassesThrough(t *testing.T) {
	called := false
	handler := httpapi.Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))

	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Error("handler was not called")
	}
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
}
