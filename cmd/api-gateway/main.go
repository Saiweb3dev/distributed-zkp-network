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

	_ "github.com/lib/pq"
	"github.com/saiweb3dev/distributed-zkp-network/internal/api/handlers"
	"github.com/saiweb3dev/distributed-zkp-network/internal/api/router"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/config"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
)

// Version info (set via ldflags during build)
var (
	version   = "dev"
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
	logger.Info("Starting ZKP Network API Gateway (Phase 2)",
		zap.String("version", version),
		zap.String("mode", "async-task-creation"),
	)

	// ========================================================================
	// Step 4: Connect to PostgreSQL
	// ========================================================================

	logger.Info("Connecting to PostgreSQL",
		zap.String("host", cfg.Database.Host),
		zap.String("database", cfg.Database.Database),
	)

	// Pass connection pool config to ConnectPostgreSQL
	dbConfig := &postgres.DatabaseConfig{
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	}

	db, err := postgres.ConnectPostgreSQL(cfg.GetDatabaseConnectionString(), dbConfig)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		logger.Fatal("Database ping failed", zap.Error(err))
	}

	// Till Here

	logger.Info("Database connected successfully")

	// ========================================================================
	// Step 5: Initialize Redis Event Bus
	// ========================================================================

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
	// Step 6 : Initialize Repositories
	// ========================================================================

	taskRepo := postgres.NewTaskRepository(db)

	// ========================================================================
	// Step 7 Initialize Handlers
	// ========================================================================

	// Phase 2: Handler uses task repository for async processing
	proofHandler := handlers.NewProofHandler(taskRepo, eventBus, logger)

	// ========================================================================
	// Step 8: Setup Router
	// ========================================================================

	r := router.SetupRouter(proofHandler, logger)

	// ========================================================================
	// Step 9: Create HTTP Server
	// ========================================================================

	srv := &http.Server{
		Addr:         cfg.GetServerAddress(),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// ========================================================================
	// Step 10: Start Server in Goroutine
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
	// Step 11: Graceful Shutdown
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
