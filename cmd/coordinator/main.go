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

	"github.com/hashicorp/raft"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/config"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
	coordinatorRaft "github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/raft"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/registry"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/scheduler"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/service"
	pb "github.com/saiweb3dev/distributed-zkp-network/internal/proto/coordinator"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// Version information - populated via ldflags during build
// Example: go build -ldflags "-X main.version=1.0.0"
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// ========================================================================
	// STEP 1: Parse Command-Line Flags
	// ========================================================================
	configPath := flag.String("config", "configs/coordinator.yaml", "Path to configuration file")
	flag.Parse()

	// ========================================================================
	// STEP 2: Initialize Structured Logger
	// ========================================================================
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() // Flush any buffered log entries

	logger.Info("Starting ZKP Coordinator",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("git_commit", gitCommit),
	)

	// ========================================================================
	// STEP 3: Load Configuration from YAML + Environment Variables
	// ========================================================================
	cfg, err := config.LoadCoordinatorConfig(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded successfully",
		zap.String("coordinator_id", cfg.Coordinator.ID),
		zap.Int("grpc_port", cfg.Coordinator.GRPCPort),
		zap.Int("http_port", cfg.Coordinator.HTTPPort),
		zap.String("database_host", cfg.Database.Host),
		zap.Duration("poll_interval", cfg.Coordinator.PollInterval),
	)

	// ========================================================================
	// STEP 4: Establish Database Connection with Connection Pooling
	// ========================================================================
	logger.Info("Connecting to PostgreSQL",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("database", cfg.Database.Database),
	)

	// Configure connection pool parameters
	dbConfig := &postgres.DatabaseConfig{
		MaxOpenConns:    cfg.Database.MaxOpenConns,    // Maximum number of open connections
		MaxIdleConns:    cfg.Database.MaxIdleConns,    // Maximum number of idle connections
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime, // Maximum lifetime of a connection
	}

	db, err := postgres.ConnectPostgreSQL(cfg.GetDatabaseConnectionString(), dbConfig)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Verify database connectivity with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(ctx); err != nil {
		cancel()
		logger.Fatal("Database ping failed", zap.Error(err))
	}
	cancel()

	logger.Info("Database connection established",
		zap.Int("max_open_conns", cfg.Database.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.Database.MaxIdleConns),
	)

	// ========================================================================
	// STEP 5: Initialize Redis Event Bus
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
			logger.Warn("Failed to initialize event bus, continuing with polling only",
				zap.Error(err),
			)
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
	// STEP 6: Initialize Data Access Layer (Repositories)
	// ========================================================================
	taskRepo := postgres.NewTaskRepository(db)
	workerRepo := postgres.NewWorkerRepository(db)

	// ========================================================================
	// STEP 6: Initialize Raft Consensus (High Availability & Leader Election)
	// ========================================================================
	logger.Info("Initializing Raft consensus",
		zap.String("node_id", cfg.Coordinator.Raft.NodeID),
		zap.Int("raft_port", cfg.Coordinator.Raft.RaftPort),
		zap.String("raft_dir", cfg.Coordinator.Raft.RaftDir),
	)

	// Create worker registry first (needed by FSM)
	workerRegistry := registry.NewWorkerRegistry(
		workerRepo,
		cfg.Coordinator.HeartbeatTimeout,
		logger,
		nil, // gRPC service will be set later
	)

	// Create FSM (Finite State Machine) for Raft
	fsm := coordinatorRaft.NewCoordinatorFSM(workerRegistry, taskRepo, logger)

	// Create Raft node
	// Use hostname for advertisable address (required for cluster communication)
	hostname, err := os.Hostname()
	if err != nil {
		logger.Fatal("Failed to get hostname", zap.Error(err))
	}
	raftAddr := fmt.Sprintf("%s:%d", hostname, cfg.Coordinator.Raft.RaftPort)
	raftNode, err := coordinatorRaft.NewRaftNode(
		cfg.Coordinator.Raft.NodeID,
		raftAddr,
		cfg.Coordinator.Raft.RaftDir,
		fsm,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to create Raft node", zap.Error(err))
	}

	// Bootstrap cluster if this is the first node
	if cfg.Coordinator.Raft.Bootstrap && len(cfg.Coordinator.Raft.Peers) > 0 {
		logger.Info("Bootstrapping Raft cluster",
			zap.Int("peer_count", len(cfg.Coordinator.Raft.Peers)),
		)

		var servers []raft.Server
		for _, peer := range cfg.Coordinator.Raft.Peers {
			servers = append(servers, raft.Server{
				ID:      raft.ServerID(peer.ID),
				Address: raft.ServerAddress(peer.Address),
			})
		}

		if err := raftNode.Bootstrap(servers); err != nil {
			logger.Warn("Failed to bootstrap cluster (may already be bootstrapped)",
				zap.Error(err),
			)
		} else {
			logger.Info("Raft cluster bootstrapped successfully")
		}
	}

	// Wait for leadership election
	logger.Info("Waiting for leadership election...")
	time.Sleep(3 * time.Second) // Give Raft time to elect a leader

	raftState := raftNode.GetState()
	logger.Info("Raft node state",
		zap.String("state", raftState.String()),
		zap.Bool("is_leader", raftNode.IsLeader()),
		zap.String("leader", raftNode.GetLeaderAddress()),
	)

	// ========================================================================
	// STEP 7: Initialize Worker Registry (Worker State Management)
	// ========================================================================
	// The registry tracks all registered workers, their health status,
	// and available capacity for task assignment
	logger.Info("Starting worker registry",
		zap.Duration("heartbeat_timeout", cfg.Coordinator.HeartbeatTimeout),
	)

	if err := workerRegistry.Start(); err != nil {
		logger.Fatal("Failed to start worker registry", zap.Error(err))
	}
	defer workerRegistry.Stop()

	// ========================================================================
	// STEP 8: Initialize Task Scheduler (Task Distribution Engine)
	// ========================================================================
	// The scheduler continuously polls for pending tasks and assigns them
	// to available workers using a round-robin strategy
	// ONLY THE RAFT LEADER SCHEDULES TASKS to avoid duplicate assignments
	logger.Info("Initializing task scheduler",
		zap.Duration("poll_interval", cfg.Coordinator.PollInterval),
		zap.Duration("stale_task_timeout", cfg.Coordinator.StaleTaskTimeout),
		zap.Bool("event_driven_enabled", eventBus.IsEnabled()),
	)

	taskScheduler := scheduler.NewTaskScheduler(
		taskRepo,
		workerRegistry,
		raftNode,
		eventBus,
		cfg.Coordinator.PollInterval,
		logger,
	)

	taskScheduler.Start()
	defer taskScheduler.Stop()

	// ========================================================================
	// STEP 9: Configure and Start gRPC Server (Worker Communication)
	// ========================================================================
	logger.Info("Configuring gRPC server",
		zap.String("address", cfg.GetGRPCAddress()),
	)

	// gRPC server options for production reliability
	grpcOpts := []grpc.ServerOption{
		// Keepalive settings to detect dead connections
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute, // Close idle connections after 15min
			MaxConnectionAge:      30 * time.Minute, // Force reconnect after 30min
			MaxConnectionAgeGrace: 5 * time.Minute,  // Allow 5min for graceful shutdown
			Time:                  5 * time.Minute,  // Send keepalive ping every 5min
			Timeout:               20 * time.Second, // Wait 20s for keepalive ack
		}),
		// Keepalive enforcement to prevent resource exhaustion
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Minute, // Minimum time between pings from client
			PermitWithoutStream: true,            // Allow pings even when no streams active
		}),
		// Message size limits (10MB for proof data)
		grpc.MaxRecvMsgSize(10 * 1024 * 1024),
		grpc.MaxSendMsgSize(10 * 1024 * 1024),
	}

	grpcServer := grpc.NewServer(grpcOpts...)

	// Create gRPC service implementation with Raft support
	grpcService := service.NewCoordinatorGRPCService(
		workerRegistry,
		taskRepo,
		raftNode,
		logger,
	)

	// IMPORTANT: Set gRPC service in worker registry to enable task pushing
	// This resolves the circular dependency: registry needs service to push tasks,
	// but service needs registry for worker management
	workerRegistry.SetGRPCService(grpcService)

	// Register the service with the gRPC server
	pb.RegisterCoordinatorServiceServer(grpcServer, grpcService)

	logger.Info("gRPC service registered successfully")

	// Start listening on the configured gRPC port
	lis, err := net.Listen("tcp", cfg.GetGRPCAddress())
	if err != nil {
		logger.Fatal("Failed to listen on gRPC port", zap.Error(err))
	}

	// Start gRPC server in background goroutine
	go func() {
		logger.Info("gRPC server started", zap.String("address", cfg.GetGRPCAddress()))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server terminated with error", zap.Error(err))
		}
	}()

	// ========================================================================
	// STEP 9: Configure and Start HTTP Server (Health Checks & Metrics)
	// ========================================================================
	logger.Info("Configuring HTTP server for observability",
		zap.String("address", cfg.GetHTTPAddress()),
	)

	httpMux := http.NewServeMux()

	// Health check endpoint - returns detailed system status including Raft state
	httpMux.HandleFunc("/health", createHealthHandler(cfg, workerRegistry, taskScheduler, raftNode))

	// Metrics endpoint - Prometheus-compatible metrics
	httpMux.HandleFunc("/metrics", createMetricsHandler(workerRegistry))

	// Root endpoint - service information
	httpMux.HandleFunc("/", createRootHandler(cfg, version))

	httpServer := &http.Server{
		Addr:         cfg.GetHTTPAddress(),
		Handler:      httpMux,
		ReadTimeout:  5 * time.Second,  // Maximum duration for reading request
		WriteTimeout: 10 * time.Second, // Maximum duration for writing response
		IdleTimeout:  60 * time.Second, // Maximum idle time between requests
	}

	// Start HTTP server in background goroutine
	go func() {
		logger.Info("HTTP server started", zap.String("address", cfg.GetHTTPAddress()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server terminated with error", zap.Error(err))
		}
	}()

	// ========================================================================
	// STEP 10: Start Background Task Cleanup Routine
	// ========================================================================
	// Periodically identifies and reassigns stale tasks (tasks that have been
	// in-progress too long, indicating worker failure)
	go startStaleTaskCleanup(cfg, taskScheduler, logger)

	// ========================================================================
	// STEP 11: Signal Coordinator Ready
	// ========================================================================
	logger.Info("Coordinator initialization complete",
		zap.String("coordinator_id", cfg.Coordinator.ID),
		zap.String("grpc_address", cfg.GetGRPCAddress()),
		zap.String("http_address", cfg.GetHTTPAddress()),
		zap.String("status", "READY"),
	)

	// ========================================================================
	// STEP 12: Wait for Shutdown Signal (Ctrl+C, SIGTERM)
	// ========================================================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutdown signal received, initiating graceful shutdown...")

	// ========================================================================
	// STEP 13: Graceful Shutdown (Clean Resource Cleanup)
	// ========================================================================
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop accepting new tasks
	logger.Info("Stopping task scheduler...")
	taskScheduler.Stop()

	// Stop Raft node (step down from leadership if leader)
	logger.Info("Shutting down Raft node...")
	if err := raftNode.Shutdown(); err != nil {
		logger.Error("Raft shutdown failed", zap.Error(err))
	}

	// Stop accepting new gRPC connections and drain existing ones
	logger.Info("Stopping gRPC server...")
	grpcServer.GracefulStop()

	// Stop HTTP server
	logger.Info("Stopping HTTP server...")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown failed", zap.Error(err))
	}

	// Stop worker registry (closes worker connections)
	logger.Info("Stopping worker registry...")
	workerRegistry.Stop()

	// Close database connections
	logger.Info("Closing database connections...")
	if err := db.Close(); err != nil {
		logger.Error("Database close failed", zap.Error(err))
	}

	logger.Info("Coordinator shutdown complete")
}

// ============================================================================
// HTTP Handler Factory Functions
// ============================================================================

// createHealthHandler returns a handler that provides detailed health status
func createHealthHandler(
	cfg *config.CoordinatorConfig,
	workerRegistry *registry.WorkerRegistry,
	taskScheduler *scheduler.TaskScheduler,
	raftNode *coordinatorRaft.RaftNode,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := workerRegistry.GetStats()
		schedulerMetrics := taskScheduler.GetMetrics()
		raftState := raftNode.GetState()
		isLeader := raftNode.IsLeader()
		leaderAddr := raftNode.GetLeaderAddress()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
			"status": "healthy",
			"coordinator_id": "%s",
			"raft": {
				"state": "%s",
				"is_leader": %t,
				"leader_address": "%s",
				"node_id": "%s"
			},
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
			raftState.String(),
			isLeader,
			leaderAddr,
			cfg.Coordinator.Raft.NodeID,
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
	}
}

// createMetricsHandler returns a handler that exposes Prometheus-style metrics
func createMetricsHandler(workerRegistry *registry.WorkerRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := workerRegistry.GetStats()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP coordinator_workers_total Total number of registered workers\n")
		fmt.Fprintf(w, "# TYPE coordinator_workers_total gauge\n")
		fmt.Fprintf(w, "coordinator_workers_total %d\n", stats.TotalWorkers)
		fmt.Fprintf(w, "# HELP coordinator_workers_active Number of active workers\n")
		fmt.Fprintf(w, "# TYPE coordinator_workers_active gauge\n")
		fmt.Fprintf(w, "coordinator_workers_active %d\n", stats.ActiveWorkers)
		fmt.Fprintf(w, "# HELP coordinator_workers_suspect Number of suspect workers\n")
		fmt.Fprintf(w, "# TYPE coordinator_workers_suspect gauge\n")
		fmt.Fprintf(w, "coordinator_workers_suspect %d\n", stats.SuspectWorkers)
		fmt.Fprintf(w, "# HELP coordinator_workers_dead Number of dead workers\n")
		fmt.Fprintf(w, "# TYPE coordinator_workers_dead gauge\n")
		fmt.Fprintf(w, "coordinator_workers_dead %d\n", stats.DeadWorkers)
		fmt.Fprintf(w, "# HELP coordinator_capacity_total Total worker capacity\n")
		fmt.Fprintf(w, "# TYPE coordinator_capacity_total gauge\n")
		fmt.Fprintf(w, "coordinator_capacity_total %d\n", stats.TotalCapacity)
		fmt.Fprintf(w, "# HELP coordinator_capacity_used Used worker capacity\n")
		fmt.Fprintf(w, "# TYPE coordinator_capacity_used gauge\n")
		fmt.Fprintf(w, "coordinator_capacity_used %d\n", stats.UsedCapacity)
	}
}

// createRootHandler returns a handler that provides service information
func createRootHandler(cfg *config.CoordinatorConfig, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"service": "zkp-coordinator",
			"version": "%s",
			"coordinator_id": "%s",
			"endpoints": {
				"health": "/health",
				"metrics": "/metrics"
			}
		}`, version, cfg.Coordinator.ID)
	}
}

// ============================================================================
// Background Service Functions
// ============================================================================

// startStaleTaskCleanup runs a background routine to detect and reassign stale tasks
func startStaleTaskCleanup(
	cfg *config.CoordinatorConfig,
	taskScheduler *scheduler.TaskScheduler,
	logger *zap.Logger,
) {
	ticker := time.NewTicker(cfg.Coordinator.CleanupInterval)
	defer ticker.Stop()

	logger.Info("Stale task cleanup routine started",
		zap.Duration("cleanup_interval", cfg.Coordinator.CleanupInterval),
		zap.Duration("stale_threshold", cfg.Coordinator.StaleTaskTimeout),
	)

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := taskScheduler.HandleStaleTasks(ctx, cfg.Coordinator.StaleTaskTimeout); err != nil {
			logger.Error("Failed to handle stale tasks", zap.Error(err))
		}
		cancel()
	}
}

// ============================================================================
// Utility Functions
// ============================================================================

// initLogger creates a production-ready structured logger
func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return config.Build()
}
