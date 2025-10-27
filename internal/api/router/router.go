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
	
	// 2. Logging - log every request
	r.Use(middleware.Logging(logger))
	
	// 3. CORS - allow browser requests
	r.Use(middleware.CORS())
	
	// 4. Timeout - prevent slow requests from hanging forever
	r.Use(middleware.Timeout(30 * time.Second))

	// ========================================================================
	// API Routes
	// ========================================================================
	
	// API v1 subrouter
	api := r.PathPrefix("/api/v1").Subrouter()
	
	// Proof generation endpoints
	proofs := api.PathPrefix("/proofs").Subrouter()
	proofs.HandleFunc("/merkle", proofHandler.GenerateMerkleProof).Methods("POST")
	
	// ========================================================================
	// Health & Status
	// ========================================================================
	
	r.HandleFunc("/health", proofHandler.HealthCheck).Methods("GET")
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service": "zkp-network", "version": "0.1.0"}`))
	}).Methods("GET")
	
	return r
}