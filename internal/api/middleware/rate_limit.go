package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"go.uber.org/zap"
)

// RateLimiter implements token bucket rate limiting per IP
type RateLimiter struct {
	visitors map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	logger   *zap.Logger
}

// NewRateLimiter creates a new rate limiter
// rate: requests per second
// burst: maximum burst size
func NewRateLimiter(reqPerSecond float64, burst int, logger *zap.Logger) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*rate.Limiter),
		rate:     rate.Limit(reqPerSecond),
		burst:    burst,
		logger:   logger,
	}

	// Cleanup old visitors every minute
	go rl.cleanupVisitors()

	return rl
}

// getVisitor returns the rate limiter for a given IP
func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.visitors[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[ip] = limiter
	}

	return limiter
}

// cleanupVisitors removes stale rate limiters
func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, limiter := range rl.visitors {
			// Remove if no tokens have been used in the last minute
			if limiter.Tokens() == float64(rl.burst) {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns the rate limiting middleware
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client IP (handle X-Forwarded-For if behind proxy)
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = forwarded
			}
			if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				ip = realIP
			}

			limiter := rl.getVisitor(ip)

			if !limiter.Allow() {
				rl.logger.Warn("Rate limit exceeded",
					zap.String("ip", ip),
					zap.String("path", r.URL.Path),
				)

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"success":false,"error":"Rate limit exceeded. Please try again later."}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}