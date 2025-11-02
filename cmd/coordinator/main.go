package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/config"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/registry"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/scheduler"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	 pb "github.com/saiweb3dev/distributed-zkp-network/internal/proto/coordinator"
    "github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/service"
)

// Version info
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "configs/coordinator.yaml", "Path to config file")
	flag.Parse()

	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting Coordinator",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("git_commit", gitCommit),
	)

	// Load configuration
	cfg, err := config.LoadCoordinatorConfig(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("coordinator_id", cfg.Coordinator.ID),
		zap.Int("grpc_port", cfg.Coordinator.GRPCPort),
		zap.Int("http_port", cfg.Coordinator.HTTPPort),
		zap.String("database_host", cfg.Database.Host),
	)

	// Connect to PostgreSQL
	logger.Info("Connecting to PostgreSQL",
		zap.String("host", cfg.Database.Host),
		zap.String("database", cfg.Database.Database),
	)

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

	// Configure connection pool
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	// Test database connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(ctx); err != nil {
		cancel()
		logger.Fatal("Database ping failed", zap.Error(err))
	}
	cancel()

	logger.Info("Database connected successfully")

	// Initialize repositories
	taskRepo := postgres.NewTaskRepository(db)
	workerRepo := postgres.NewWorkerRepository(db)

	// Initialize worker registry
	logger.Info("Initializing worker registry",
		zap.Duration("heartbeat_timeout", cfg.Coordinator.HeartbeatTimeout),
	)

	workerRegistry := registry.NewWorkerRegistry(
		workerRepo,
		cfg.Coordinator.HeartbeatTimeout,
		logger,
		nil,
	)

	if err := workerRegistry.Start(); err != nil {
		logger.Fatal("Failed to start worker registry", zap.Error(err))
	}
	defer workerRegistry.Stop()

	// Initialize task scheduler
	logger.Info("Initializing task scheduler",
		zap.Duration("poll_interval", cfg.Coordinator.PollInterval),
	)

	taskScheduler := scheduler.NewTaskScheduler(
		taskRepo,
		workerRegistry,
		cfg.Coordinator.PollInterval,
		logger,
	)

	taskScheduler.Start()
	defer taskScheduler.Stop()

	// Start gRPC server for worker connections
	logger.Info("Starting gRPC server",
		zap.String("address", cfg.GetGRPCAddress()),
	)

	
	grpcServer := grpc.NewServer()

	grpcService := service.NewCoordinatorGRPCService(
		workerRegistry,
		taskRepo,
		logger,
	)

	// 4. Set grpcService in workerRegistry
	workerRegistry.SetGRPCService(grpcService)
	
	// Register gRPC service with server
	pb.RegisterCoordinatorServiceServer(grpcServer, grpcService)
	
	logger.Info("gRPC service registered")

	lis, err := net.Listen("tcp", cfg.GetGRPCAddress())
	if err != nil {
		logger.Fatal("Failed to listen on gRPC port", zap.Error(err))
	}

	go func() {
		logger.Info("gRPC server listening", zap.String("address", cfg.GetGRPCAddress()))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server failed", zap.Error(err))
		}
	}()

	// Start HTTP server for health checks and metrics
	logger.Info("Starting HTTP server",
		zap.String("address", cfg.GetHTTPAddress()),
	)

	httpMux := http.NewServeMux()

	// Health check endpoint
	httpMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		stats := workerRegistry.GetStats()
		schedulerMetrics := taskScheduler.GetMetrics()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
			"status": "healthy",
			"coordinator_id": "%s",
			"workers": {
				"total": %d,
				"active": %d,
				"suspect": %d,
				"dead": %d
			},
			"capacity": {
				"total": %d,
				"used": %d,
				"available": %d
			},
			"scheduler": {
				"tasks_assigned": %d,
				"assignments_failed": %d,
				"poll_cycles": %d
			}
		}`,
			cfg.Coordinator.ID,
			stats.TotalWorkers,
			stats.ActiveWorkers,
			stats.SuspectWorkers,
			stats.DeadWorkers,
			stats.TotalCapacity,
			stats.UsedCapacity,
			stats.TotalCapacity-stats.UsedCapacity,
			schedulerMetrics.TasksAssigned,
			schedulerMetrics.AssignmentsFailed,
			schedulerMetrics.PollCycles,
		)
	})

	// Simple metrics endpoint (basic stats)
	httpMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := workerRegistry.GetStats()

		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "# Coordinator Metrics\n")
		fmt.Fprintf(w, "coordinator_workers_total %d\n", stats.TotalWorkers)
		fmt.Fprintf(w, "coordinator_workers_active %d\n", stats.ActiveWorkers)
		fmt.Fprintf(w, "coordinator_workers_suspect %d\n", stats.SuspectWorkers)
		fmt.Fprintf(w, "coordinator_workers_dead %d\n", stats.DeadWorkers)
		fmt.Fprintf(w, "coordinator_capacity_total %d\n", stats.TotalCapacity)
		fmt.Fprintf(w, "coordinator_capacity_used %d\n", stats.UsedCapacity)
	})

	// Root endpoint
	httpMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"service": "zkp-coordinator",
			"version": "%s",
			"coordinator_id": "%s"
		}`, version, cfg.Coordinator.ID)
	})

	httpServer := &http.Server{
		Addr:    cfg.GetHTTPAddress(),
		Handler: httpMux,
	}

	go func() {
		logger.Info("HTTP server listening", zap.String("address", cfg.GetHTTPAddress()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", zap.Error(err))
		}
	}()

	// Start stale task cleanup routine
	go func() {
		ticker := time.NewTicker(cfg.Coordinator.CleanupInterval)
		defer ticker.Stop()

		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := taskScheduler.HandleStaleTasks(ctx, cfg.Coordinator.StaleTaskTimeout); err != nil {
				logger.Error("Failed to handle stale tasks", zap.Error(err))
			}
			cancel()
		}
	}()

	logger.Info("Coordinator started successfully",
		zap.String("coordinator_id", cfg.Coordinator.ID),
		zap.String("grpc_address", cfg.GetGRPCAddress()),
		zap.String("http_address", cfg.GetHTTPAddress()),
	)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutdown signal received, gracefully stopping...")

	// Graceful shutdown
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop scheduler
	taskScheduler.Stop()

	// Stop gRPC server
	grpcServer.GracefulStop()

	// Stop HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown failed", zap.Error(err))
	}

	// Stop worker registry
	workerRegistry.Stop()

	logger.Info("Coordinator stopped successfully")
}

// initLogger creates a configured zap logger
func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
