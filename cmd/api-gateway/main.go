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
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/cache"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/config"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/health"
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
	// Step 1: Parse CLI Flags & Display Version
	// ========================================================================

	configPath := flag.String("config", "configs/api-gateway.yaml", "Path to config file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("API Gateway version %s\n", version)
		fmt.Printf("Build time: %s\n", buildTime)
		fmt.Printf("Git commit: %s\n", gitCommit)
		os.Exit(0)
	}

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
	defer func() {
		if err := logger.Sync(); err != nil {
			// Ignore errors on stdout/stderr sync (common on Linux)
			if err.Error() != "sync /dev/stdout: inappropriate ioctl for device" &&
				err.Error() != "sync /dev/stderr: inappropriate ioctl for device" {
				fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
			}
		}
	}()

	logger.Info("Starting ZKP Network API Gateway (Phase 3)",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("git_commit", gitCommit),
		zap.String("mode", "event-driven-architecture"),
		zap.String("config_path", *configPath),
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		logger.Fatal("Database ping failed", zap.Error(err))
	}

	logger.Info("Database connected successfully")

	// ========================================================================
	// Step 4.5: Run Database Migrations (if needed)
	// ========================================================================

	// TODO: Implement proper migration system (e.g., golang-migrate)
	// For now, we assume migrations are run manually or via init scripts
	logger.Info("Skipping automatic migrations (run manually)")

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
	// Step 5.5: Initialize Cache Layer
	// ========================================================================

	var cacheLayer *cache.CacheLayer

	if cfg.Redis.CachingEnabled {
		logger.Info("Initializing Cache Layer",
			zap.String("host", cfg.Redis.Host),
			zap.Int("port", cfg.Redis.Port),
			zap.Duration("default_ttl", cfg.Redis.DefaultTTL),
		)

		cacheConfig := cache.CacheConfig{
			Host:         cfg.Redis.Host,
			Port:         cfg.Redis.Port,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			TTL:          cfg.Redis.DefaultTTL,
			MaxRetries:   cfg.Redis.MaxRetries,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
			DialTimeout:  cfg.Redis.DialTimeout,
			ReadTimeout:  cfg.Redis.ReadTimeout,
			WriteTimeout: cfg.Redis.WriteTimeout,
			Enabled:      cfg.Redis.CachingEnabled,
		}

		var err error
		cacheLayer, err = cache.NewCacheLayer(cacheConfig, logger)
		if err != nil {
			logger.Warn("Failed to initialize cache, continuing without caching",
				zap.Error(err),
			)
			cacheLayer = cache.NewDisabledCacheLayer(logger)
		} else {
			logger.Info("Cache layer initialized successfully")
			defer cacheLayer.Close()
		}
	} else {
		logger.Info("Cache layer disabled in configuration")
		cacheLayer = cache.NewDisabledCacheLayer(logger)
	}

	// ========================================================================
	// Step 6 : Initialize Repositories
	// ========================================================================

	taskRepo := postgres.NewTaskRepository(db)

	// ========================================================================
	// Step 7 Initialize Handlers (with Cache Layer)
	// ========================================================================

	// Phase 2: Handler uses task repository for async processing with caching
	proofHandler := handlers.NewProofHandler(taskRepo, eventBus, cacheLayer, logger)

	// ========================================================================
	// Step 7.5: Perform Health Check Before Starting Server
	// ========================================================================

	healthChecker := health.NewChecker(logger)
	logger.Info("Performing startup health checks...")

	healthCtx, healthCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer healthCancel()

	if err := healthChecker.WaitForHealthy(healthCtx, db, eventBus, cacheLayer, 30*time.Second); err != nil {
		logger.Fatal("Startup health check failed", zap.Error(err))
	}

	logger.Info("All systems operational, ready to accept traffic")

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
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	logger.Info("Shutting down server gracefully...")

	if err := srv.Shutdown(shutdownCtx); err != nil {
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
