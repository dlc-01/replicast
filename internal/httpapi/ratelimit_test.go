package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/httpapi"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := httpapi.NewRateLimiter(3, time.Minute)

	// Первые 3 — разрешены
	for i := 0; i < 3; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	// Четвёртый — заблокирован
	if rl.Allow("192.168.1.1") {
		t.Error("4th request should be blocked")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := httpapi.NewRateLimiter(1, time.Minute)

	if !rl.Allow("1.1.1.1") {
		t.Error("first IP should be allowed")
	}
	if !rl.Allow("2.2.2.2") {
		t.Error("second IP should be allowed (different IP)")
	}
	if rl.Allow("1.1.1.1") {
		t.Error("first IP should be blocked on second request")
	}
}

func TestRateLimit_Middleware_Blocks(t *testing.T) {
	rl := httpapi.NewRateLimiter(2, time.Minute)
	handler := httpapi.RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i+1, w.Code)
		}
	}

	// Третий запрос — заблокирован
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("3rd request: status = %d, want 400 (rate limited)", w.Code)
	}
}

func TestRateLimit_Middleware_ResetsAfterWindow(t *testing.T) {
	rl := httpapi.NewRateLimiter(1, 50*time.Millisecond) // короткое окно для теста
	handler := httpapi.RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	make1Request := func() int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.2:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	// Первый — ок
	if make1Request() != http.StatusOK {
		t.Error("first request should be ok")
	}
	// Второй сразу — заблокирован
	if make1Request() == http.StatusOK {
		t.Error("second request should be blocked")
	}

	// Ждём сброса окна
	time.Sleep(60 * time.Millisecond)

	// После сброса — снова ок
	if make1Request() != http.StatusOK {
		t.Error("request after window reset should be ok")
	}
}
