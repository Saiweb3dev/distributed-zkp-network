// cmd/worker/main.go
// Entry point for the ZKP worker node
// Workers are stateless compute nodes that connect to coordinators and generate zero-knowledge proofs
// Each worker runs multiple goroutines for concurrent proof generation

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/common/config"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
	"github.com/saiweb3dev/distributed-zkp-network/internal/worker"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// Version information for tracking worker deployments
	version = "1.0.0"

	// Default timeouts for graceful shutdown
	shutdownTimeout = 30 * time.Second
)

func main() {
	// ========================================================================
	// STEP 1: Parse Command-Line Flags
	// ========================================================================
	// Allows runtime configuration file specification for different environments
	// Example: go run cmd/worker/main.go -config configs/dev/worker-local.yaml
	configPath := flag.String("config", "configs/worker.yaml", "Path to worker configuration file")
	showVersion := flag.Bool("version", false, "Display version information and exit")
	flag.Parse()

	// Display version and exit if requested
	if *showVersion {
		fmt.Printf("ZKP Worker Node v%s\n", version)
		fmt.Printf("Go Version: %s\n", runtime.Version())
		fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("CPUs: %d\n", runtime.NumCPU())
		os.Exit(0)
	}

	// ========================================================================
	// STEP 2: Initialize Structured Logger
	// ========================================================================
	// Configure production-ready logger with JSON output and ISO8601 timestamps
	// Structured logging enables better parsing by log aggregators (ELK, Splunk, Datadog)
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() // Flush buffered logs on exit

	logger.Info("Starting ZKP Worker Node",
		zap.String("version", version),
		zap.String("config_path", *configPath),
		zap.Int("available_cpus", runtime.NumCPU()),
	)

	// ========================================================================
	// STEP 3: Load Worker Configuration
	// ========================================================================
	// Configuration includes:
	// - Worker ID (unique identifier across cluster)
	// - Coordinator address (gRPC endpoint)
	// - Concurrency level (number of parallel proof generations)
	// - Heartbeat interval (health check frequency)
	// - ZKP curve parameters (BN254, BLS12-381, etc.)
	cfg, err := config.LoadWorkerConfig(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration",
			zap.Error(err),
			zap.String("config_path", *configPath),
		)
	}

	// Validate critical configuration parameters
	if err := validateConfig(cfg); err != nil {
		logger.Fatal("Invalid configuration",
			zap.Error(err),
		)
	}

	logger.Info("Worker configuration loaded successfully",
		zap.String("worker_id", cfg.Worker.ID),
		zap.String("coordinator_address", cfg.Worker.CoordinatorAddress),
		zap.Int("concurrency", cfg.Worker.Concurrency),
		zap.Duration("heartbeat_interval", cfg.Worker.HeartbeatInterval),
		zap.String("zkp_curve", cfg.ZKP.Curve),
	)

	var eventBus *events.EventBus

	if cfg.Redis.Enabled {
		logger.Info("Initializing Redis Event Bus",
			zap.String("host", cfg.Redis.Host),
			zap.Int("port", cfg.Redis.Port),
			zap.Bool("caching_enabled", cfg.Redis.CachingEnabled),
		)

		eventBusConfig := events.EventBusConfig{
			Host:         cfg.Redis.Host,
			Port:         cfg.Redis.Port,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			MaxRetries:   cfg.Redis.MaxRetries,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
			DialTimeout:  cfg.Redis.DialTimeout,
			ReadTimeout:  cfg.Redis.ReadTimeout,
			WriteTimeout: cfg.Redis.WriteTimeout,
			Enabled:      cfg.Redis.Enabled,
		}
		var err error
		eventBus, err = events.NewEventBus(eventBusConfig, logger)
		if err != nil {
			logger.Warn("Failed to initialize event bus, continuing without events",
				zap.Error(err),
			)
			// Create disabled event bus for graceful degradation
			eventBus = events.NewDisabledEventBus(logger)
		} else {
			logger.Info("Event bus initialized successfully")
			defer eventBus.Close()
		}
	} else {
		logger.Info("Redis event bus disabled in configuration")
		eventBus = events.NewDisabledEventBus(logger)
	}

	// ========================================================================
	// STEP 4: Initialize Worker Instance
	// ========================================================================
	// The worker encapsulates:
	// - gRPC client connection to coordinator
	// - Worker pool for concurrent proof generation
	// - Heartbeat sender for health monitoring
	// - Task receiver for incoming assignments
	// - Result reporter for completion notifications
	workerCfg := worker.Config{
		WorkerID:           cfg.Worker.ID,
		CoordinatorAddress: cfg.Worker.CoordinatorAddress,
		Concurrency:        cfg.Worker.Concurrency,
		HeartbeatInterval:  cfg.Worker.HeartbeatInterval,
		ZKPCurve:           cfg.ZKP.Curve,
		EventBus:           eventBus,
	}

	w, err := worker.NewWorker(workerCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create worker instance",
			zap.Error(err),
		)
	}

	// ========================================================================
	// STEP 5: Connect to Coordinator and Start Worker
	// ========================================================================
	// Worker startup sequence:
	// 1. Establish gRPC connection to coordinator
	// 2. Send registration request with capabilities
	// 3. Open persistent streaming RPC for task delivery
	// 4. Start worker pool goroutines for proof generation
	// 5. Begin heartbeat loop (every 5 seconds)
	// 6. Start result reporter loop
	logger.Info("Connecting to coordinator and starting worker services")

	if err := w.Start(); err != nil {
		logger.Fatal("Failed to start worker",
			zap.Error(err),
		)
	}

	logger.Info("Worker started successfully",
		zap.String("status", "READY"),
		zap.String("worker_id", cfg.Worker.ID),
	)

	// ========================================================================
	// STEP 6: Display Worker Status
	// ========================================================================
	// Show initial statistics for operator visibility
	stats := w.GetStats()
	logger.Info("Worker statistics",
		zap.String("worker_id", stats.WorkerID),
		zap.Int("pool_concurrency", stats.PoolConcurrency),
		zap.Int("active_tasks", stats.ActiveTasks),
		zap.Int("queued_tasks", stats.QueuedTasks),
		zap.Bool("connected", stats.Connected),
	)

	// ========================================================================
	// STEP 7: Wait for Shutdown Signal
	// ========================================================================
	// Block main goroutine until interrupt signal received
	// Handles both Ctrl+C (SIGINT) and Kubernetes termination (SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	logger.Info("Shutdown signal received",
		zap.String("signal", sig.String()),
	)

	// ========================================================================
	// STEP 8: Graceful Shutdown
	// ========================================================================
	// Shutdown sequence with timeout to prevent hanging:
	// 1. Stop accepting new tasks from coordinator
	// 2. Wait for in-progress proof generations to complete (max 30s)
	// 3. Report any pending results to coordinator
	// 4. Close gRPC connection to coordinator
	// 5. Release all resources (memory, goroutines)
	logger.Info("Initiating graceful shutdown",
		zap.Duration("timeout", shutdownTimeout),
	)

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// Stop worker gracefully
	shutdownComplete := make(chan struct{})
	go func() {
		w.Stop()
		close(shutdownComplete)
	}()

	// Wait for shutdown to complete or timeout
	select {
	case <-shutdownComplete:
		logger.Info("Worker shutdown completed successfully",
			zap.String("status", "STOPPED"),
		)
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded, forcing termination",
			zap.Duration("timeout", shutdownTimeout),
		)
	}

	logger.Info("Worker node terminated")
}

// ============================================================================
// Helper Functions
// ============================================================================

// initLogger creates a production-ready structured logger with custom configuration
// Returns a logger that outputs JSON with ISO8601 timestamps for easy parsing
func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()

	// Configure structured output format
	config.Encoding = "json"
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)

	// ISO8601 timestamps for consistency with coordinator
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	// Add caller information for debugging
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	return config.Build()
}

// validateConfig ensures all critical configuration parameters are valid
// Returns error if any validation fails
func validateConfig(cfg *config.WorkerConfig) error {
	// Worker ID is required for coordinator tracking
	if cfg.Worker.ID == "" {
		return fmt.Errorf("worker.id cannot be empty")
	}

	// Coordinator address is required for gRPC connection
	if cfg.Worker.CoordinatorAddress == "" {
		return fmt.Errorf("worker.coordinator_address cannot be empty")
	}

	// Concurrency must be positive and reasonable
	if cfg.Worker.Concurrency <= 0 {
		return fmt.Errorf("worker.concurrency must be positive, got: %d", cfg.Worker.Concurrency)
	}
	if cfg.Worker.Concurrency > runtime.NumCPU()*2 {
		return fmt.Errorf("worker.concurrency too high (%d), max recommended: %d",
			cfg.Worker.Concurrency, runtime.NumCPU()*2)
	}

	// Heartbeat interval must be reasonable (1-60 seconds)
	if cfg.Worker.HeartbeatInterval < time.Second {
		return fmt.Errorf("worker.heartbeat_interval too low: %v", cfg.Worker.HeartbeatInterval)
	}
	if cfg.Worker.HeartbeatInterval > time.Minute {
		return fmt.Errorf("worker.heartbeat_interval too high: %v", cfg.Worker.HeartbeatInterval)
	}

	// ZKP curve must be specified
	if cfg.ZKP.Curve == "" {
		return fmt.Errorf("zkp.curve cannot be empty")
	}

	return nil
}
