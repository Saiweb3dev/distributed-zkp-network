package middleware

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

const (
	// MaxRequestBodySize is the maximum allowed request body size (5MB)
	MaxRequestBodySize = 5 << 20 // 5 MB
)

// BodySizeLimit middleware limits the maximum request body size
func BodySizeLimit(maxBytes int64, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Limit request body size
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			// Catch the error from MaxBytesReader
			err := r.ParseForm()
			if err != nil {
				if err.Error() == "http: request body too large" {
					logger.Warn("Request body too large",
						zap.String("path", r.URL.Path),
						zap.String("method", r.Method),
						zap.String("remote_addr", r.RemoteAddr),
						zap.Int64("max_bytes", maxBytes),
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusRequestEntityTooLarge)
					fmt.Fprintf(w, `{"success":false,"error":"Request body too large. Maximum size is %d bytes."}`, maxBytes)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
