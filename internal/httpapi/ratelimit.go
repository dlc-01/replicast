package httpapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/respond"
)

// bucket — счётчик запросов для одного IP.
type bucket struct {
	count   int
	resetAt time.Time
	mu      sync.Mutex
}

// RateLimiter ограничивает количество запросов с одного IP.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	limit   int
	window  time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		limit:   limit,
		window:  window,
	}
	// Фоновая очистка истёкших бакетов каждые 5 минут
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{}
		rl.buckets[ip] = b
	}
	rl.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if now.After(b.resetAt) {
		b.count = 0
		b.resetAt = now.Add(rl.window)
	}

	b.count++
	return b.count <= rl.limit
}

func (rl *RateLimiter) cleanup() {
	for range time.Tick(5 * time.Minute) {
		rl.mu.Lock()
		now := time.Now()
		for ip, b := range rl.buckets {
			b.mu.Lock()
			if now.After(b.resetAt) {
				delete(rl.buckets, ip)
			}
			b.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

// RateLimit middleware ограничивает запросы по IP.
func RateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			// Берём только IP без порта
			for i := len(ip) - 1; i >= 0; i-- {
				if ip[i] == ':' {
					ip = ip[:i]
					break
				}
			}
			if !rl.Allow(ip) {
				respond.Error(w, r, apperr.BadRequest("rate_limit_exceeded", "too many requests, slow down"))
				w.Header().Set("Retry-After", "60")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
