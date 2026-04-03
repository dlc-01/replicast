package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/auth"
	"github.com/dlc-01/replicast/internal/logger"
)

func TestAuthHandler_Register(t *testing.T) {
	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "success",
			body:       map[string]string{"username": "bob", "password": "password123"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing password",
			body:       map[string]string{"username": "bob"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing username",
			body:       map[string]string{"password": "password123"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			body:       "not-json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Каждый тест получает свой репозиторий — нет общего состояния
			repo := newMockAuthRepo()
			svc := auth.NewService(repo, logger.Nop(), authTestCfg())
			h := auth.NewHandler(svc)

			var bodyBytes []byte
			if s, ok := tt.body.(string); ok {
				bodyBytes = []byte(s)
			} else {
				bodyBytes, _ = json.Marshal(tt.body)
			}

			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(bodyBytes))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.Register(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestAuthHandler_Register_Duplicate(t *testing.T) {
	repo := newMockAuthRepo()
	svc := auth.NewService(repo, logger.Nop(), authTestCfg())
	h := auth.NewHandler(svc)

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})

	// Первый запрос — успех
	r1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.Register(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first register: got %d, want %d", w1.Code, http.StatusCreated)
	}

	// Второй запрос — конфликт
	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.Register(w2, r2)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate register: got %d, want %d", w2.Code, http.StatusConflict)
	}
}

func TestAuthHandler_Login(t *testing.T) {
	repo := newMockAuthRepo()
	svc := auth.NewService(repo, logger.Nop(), authTestCfg())
	h := auth.NewHandler(svc)

	// Регистрируем пользователя
	regBody, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "success",
			body:       map[string]string{"username": "alice", "password": "password123"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong password",
			body:       map[string]string{"username": "alice", "password": "wrong"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "unknown user",
			body:       map[string]string{"username": "nobody", "password": "password123"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid json",
			body:       "not-json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			if s, ok := tt.body.(string); ok {
				bodyBytes = []byte(s)
			} else {
				bodyBytes, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rw := httptest.NewRecorder()

			h.Login(rw, req)

			if rw.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s",
					rw.Code, tt.wantStatus, rw.Body.String())
			}
		})
	}
}
