package router

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/saiweb3dev/distributed-zkp-network/internal/api/handlers"
	"github.com/saiweb3dev/distributed-zkp-network/internal/api/middleware"
	"go.uber.org/zap"
)

// SetupRouter creates and configures the HTTP router
func SetupRouter(proofHandler *handlers.ProofHandler, logger *zap.Logger) *mux.Router {

	r := mux.NewRouter()

	// ========================================================================
	// Global Middleware (applies to ALL routes)
	// ========================================================================

	// 1. Recovery - catch panics and return 500 instead of crashing
	r.Use(middleware.Recovery(logger))

	// 2. Request ID - add unique ID to each request for tracing
	r.Use(middleware.RequestID(logger))

	// 3. Body size limit (5MB)
	r.Use(middleware.BodySizeLimit(middleware.MaxRequestBodySize, logger))

	// 4. Rate limiting (10 req/sec per IP, burst of 20)
	rateLimiter := middleware.NewRateLimiter(10.0, 20, logger)
	r.Use(rateLimiter.Middleware())

	// 5. Logging - log every request
	r.Use(middleware.Logging(logger))

	// 6. CORS - allow browser requests
	r.Use(middleware.CORS())

	// 7. Timeout - prevent slow requests from hanging forever
	r.Use(middleware.Timeout(30 * time.Second))

	// ========================================================================
	// API Routes (Phase 2: Async Task-Based)
	// ========================================================================

	// API v1 subrouter
	api := r.PathPrefix("/api/v1").Subrouter()

	// Task submission endpoints
	tasks := api.PathPrefix("/tasks").Subrouter()
	tasks.HandleFunc("/merkle", proofHandler.SubmitMerkleProofTask).Methods("POST")
	tasks.HandleFunc("/{id}", proofHandler.GetTaskStatus).Methods("GET")
	// ========================================================================
	// Health & Status
	// ========================================================================

	r.HandleFunc("/health", proofHandler.HealthCheck).Methods("GET")
	r.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ready"}`))
	}).Methods("GET")
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service": "zkp-network", "version": "0.1.0"}`))
	}).Methods("GET")

	return r
}
