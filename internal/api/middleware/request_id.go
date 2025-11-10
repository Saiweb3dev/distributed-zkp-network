package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	RequestIDHeader = "X-Request-ID"
	RequestIDKey    = "request_id"
	ClientIPKey     = "client_ip"
	UserAgentKey    = "user_agent"
)

// RequestID middleware adds a unique request ID to each request
// This enables distributed tracing and correlation across services
func RequestID(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if request already has an ID (from client or upstream proxy)
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" {
				// Generate new UUID for this request
				requestID = uuid.New().String()
			}

			// Add request ID to response headers (helps with debugging)
			w.Header().Set(RequestIDHeader, requestID)

			// Extract client IP (handle X-Forwarded-For if behind proxy)
			clientIP := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				clientIP = forwarded
			}
			if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				clientIP = realIP
			}

			// Extract user agent
			userAgent := r.UserAgent()

			// Add to context for downstream handlers
			ctx := r.Context()
			ctx = context.WithValue(ctx, RequestIDKey, requestID)
			ctx = context.WithValue(ctx, ClientIPKey, clientIP)
			ctx = context.WithValue(ctx, UserAgentKey, userAgent)

			// Add to logger for structured logging
			logger := logger.With(
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
			)

			// Store logger in context as well
			ctx = context.WithValue(ctx, "logger", logger)

			// Continue with enriched context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetRequestID retrieves request ID from context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// GetClientIP retrieves client IP from context
func GetClientIP(ctx context.Context) string {
	if clientIP, ok := ctx.Value(ClientIPKey).(string); ok {
		return clientIP
	}
	return "unknown"
}

// GetUserAgent retrieves user agent from context
func GetUserAgent(ctx context.Context) string {
	if userAgent, ok := ctx.Value(UserAgentKey).(string); ok {
		return userAgent
	}
	return "unknown"
}

// GetLogger retrieves logger from context (with request ID already attached)
func GetLogger(ctx context.Context, fallback *zap.Logger) *zap.Logger {
	if logger, ok := ctx.Value("logger").(*zap.Logger); ok {
		return logger
	}
	return fallback
}
