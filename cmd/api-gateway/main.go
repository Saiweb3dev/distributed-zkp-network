// Application entry point - where everything comes together
// Follows clean architecture: main is just wiring, logic is in packages


package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/api/handlers"
	"github.com/saiweb3dev/distributed-zkp-network/internal/api/router"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/config"
	zkp "github.com/saiweb3dev/distributed-zkp-network/internal/zkp"
	"go.uber.org/zap"
)

// Version info (set via ldflags during build)
var (
	version	 = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// ========================================================================
	// Step 1: Parse CLI Flags
	// ========================================================================
	
	configPath := flag.String("config", "configs/api-gateway.yaml", "Path to config file")
	flag.Parse()
	
	// ========================================================================
	// Step 2: Load Configuration
	// ========================================================================
	
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	
	// ========================================================================
	// Step 3: Initialize Logger
	// ========================================================================
	
	logger, err := initLogger(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	
	logger.Info("Starting ZKP Network API Gateway",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("git_commit", gitCommit),
		zap.String("config", *configPath),
	)
	
	// ========================================================================
	// Step 4: Initialize ZKP Prover
	// ========================================================================
	
	logger.Info("Initializing ZKP prover", zap.String("curve", cfg.ZKP.Curve))
	
	prover, err := zkp.NewGroth16Prover(cfg.ZKP.Curve)
	if err != nil {
		logger.Fatal("Failed to initialize prover", zap.Error(err))
	}
	
	logger.Info("Prover initialized successfully")
	
	// ========================================================================
	// Step 5: Initialize Handlers
	// ========================================================================
	
	proofHandler := handlers.NewProofHandler(prover, logger)
	
	// ========================================================================
	// Step 6: Setup Router
	// ========================================================================
	
	r := router.SetupRouter(proofHandler, logger)
	
	// ========================================================================
	// Step 7: Create HTTP Server
	// ========================================================================
	
	srv := &http.Server{
		Addr:         cfg.GetServerAddress(),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}
	
	// ========================================================================
	// Step 8: Start Server in Goroutine
	// ========================================================================
	
	go func() {
		logger.Info("Starting HTTP server",
			zap.String("address", cfg.GetServerAddress()),
		)
		
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()
	
	logger.Info("Server started successfully",
		zap.String("address", cfg.GetServerAddress()),
		zap.String("environment", getEnvironment(cfg)),
	)
	
	// ========================================================================
	// Step 9: Graceful Shutdown
	// ========================================================================
	
	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	
	sig := <-quit
	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	
	// Give outstanding requests time to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	logger.Info("Shutting down server gracefully...")
	
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}
	
	logger.Info("Server stopped")
}

// ============================================================================
// Helper Functions
// ============================================================================

// initLogger creates a configured zap logger
func initLogger(cfg config.LoggingConfig) (*zap.Logger, error) {
	var zapConfig zap.Config
	
	// Choose config based on environment
	if cfg.Level == "debug" {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}
	
	// Set log level
	var level zap.AtomicLevel
	switch cfg.Level {
	case "debug":
		level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	
	zapConfig.Level = level
	
	// Set output format
	if cfg.Format == "console" {
		zapConfig.Encoding = "console"
	} else {
		zapConfig.Encoding = "json"
	}
	
	return zapConfig.Build()
}

// getEnvironment returns environment name based on config
func getEnvironment(cfg *config.Config) string {
	if cfg.IsProduction() {
		return "production"
	}
	return "development"
}