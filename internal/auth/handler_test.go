package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dlc-01/replicast/internal/auth"
	"github.com/dlc-01/replicast/internal/logger"
)

// makeBody — хелпер: строки оставляет как есть, остальное маршалит в JSON.
func makeBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	if s, ok := v.(string); ok {
		return bytes.NewReader([]byte(s))
	}
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// — Тесты Register ────────────────────────────────────────────────────

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
			repo := newMockAuthRepo()
			svc := auth.NewService(repo, logger.Nop(), authTestCfg())
			h := auth.NewHandler(svc)

			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeBody(t, tt.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.Register(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestAuthHandler_Register_Duplicate(t *testing.T) {
	repo := newMockAuthRepo()
	svc := auth.NewService(repo, logger.Nop(), authTestCfg())
	h := auth.NewHandler(svc)

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})

	r1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.Register(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first register: got %d, want %d\nbody: %s", w1.Code, http.StatusCreated, w1.Body.String())
	}

	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.Register(w2, r2)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate register: got %d, want %d", w2.Code, http.StatusConflict)
	}
}

func TestAuthHandler_Register_ReturnsTokenAndGlobalID(t *testing.T) {
	repo := newMockAuthRepo()
	h := auth.NewHandler(auth.NewService(repo, logger.Nop(), authTestCfg()))

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["token"] == "" {
		t.Error("token should not be empty")
	}
	if resp["global_id"] != "alice@node-a" {
		t.Errorf("global_id = %q, want alice@node-a", resp["global_id"])
	}
}

func TestAuthHandler_Register_ReturnsE2EKeys(t *testing.T) {
	repo := newMockAuthRepo()
	h := auth.NewHandler(auth.NewService(repo, logger.Nop(), authTestCfg()))

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201\nbody: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["public_key"] == "" {
		t.Error("public_key should be returned on registration")
	}
	if !strings.HasPrefix(resp["public_key"], "-----BEGIN") {
		t.Error("public_key should be PEM format")
	}
	if resp["private_key"] == "" {
		t.Error("private_key should be returned ONCE on registration")
	}
	if !strings.HasPrefix(resp["private_key"], "-----BEGIN") {
		t.Error("private_key should be PEM format")
	}
}

func TestAuthHandler_Register_TwoUsers_DifferentKeys(t *testing.T) {
	repo := newMockAuthRepo()
	h := auth.NewHandler(auth.NewService(repo, logger.Nop(), authTestCfg()))

	regAndGetKey := func(username string) string {
		body, _ := json.Marshal(map[string]string{"username": username, "password": "password123"})
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Register(w, r)
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		return resp["public_key"]
	}

	if regAndGetKey("alice") == regAndGetKey("bob") {
		t.Error("different users should have different public keys")
	}
}

// — Тесты Login ───────────────────────────────────────────────────────

func TestAuthHandler_Login(t *testing.T) {
	repo := newMockAuthRepo()
	svc := auth.NewService(repo, logger.Nop(), authTestCfg())
	h := auth.NewHandler(svc)

	regBody, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})
	rReg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	rReg.Header.Set("Content-Type", "application/json")
	h.Register(httptest.NewRecorder(), rReg)

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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", makeBody(t, tt.body))
			req.Header.Set("Content-Type", "application/json")
			rw := httptest.NewRecorder()
			h.Login(rw, req)

			if rw.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\nbody: %s", rw.Code, tt.wantStatus, rw.Body.String())
			}
		})
	}
}

func TestAuthHandler_Login_NoPrivateKey(t *testing.T) {
	repo := newMockAuthRepo()
	h := auth.NewHandler(auth.NewService(repo, logger.Nop(), authTestCfg()))

	// Регистрируемся
	regBody, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})
	r1 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(regBody))
	r1.Header.Set("Content-Type", "application/json")
	h.Register(httptest.NewRecorder(), r1)

	// Логинимся
	loginBody, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})
	r2 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(loginBody))
	r2.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r2)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	// private_key НЕ возвращается при логине — только при регистрации
	if resp["private_key"] != "" {
		t.Error("private_key should NOT be returned on login — only on registration")
	}
}
