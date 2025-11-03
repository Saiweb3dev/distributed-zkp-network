// Maintains real-time state of all connected workers
// Tracks health, capacity, and provides gRPC clients for communication

package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// WorkerInfo contains the current state of a registered worker
// This structure is maintained in-memory for fast access during scheduling
type WorkerInfo struct {
	ID                 string
	Address            string
	MaxConcurrentTasks int
	CurrentTaskCount   int
	Status             WorkerStatus
	LastHeartbeatAt    time.Time
	RegisteredAt       time.Time
	Capabilities       map[string]interface{}

	// gRPC connection for sending tasks
	conn   *grpc.ClientConn
	client WorkerClient // gRPC client stub
}

// WorkerStatus represents the health state of a worker
type WorkerStatus string

const (
	WorkerStatusActive   WorkerStatus = "active"
	WorkerStatusInactive WorkerStatus = "inactive"
	WorkerStatusSuspect  WorkerStatus = "suspect" // Missed heartbeats
	WorkerStatusDead     WorkerStatus = "dead"
)

// WorkerClient is the gRPC interface for communicating with workers
// This would be generated from protobuf definitions in production
type WorkerClient interface {
	AssignTask(ctx context.Context, task *postgres.Task) error
}

// TaskPusher interface abstracts the gRPC service to avoid import cycles
type TaskPusher interface {
	PushTaskToWorker(workerID string, task *postgres.Task) error
}

// WorkerRegistry manages all registered workers and their state
// The registry maintains both in-memory state for fast lookups and
// synchronizes with PostgreSQL for persistence across coordinator restarts
type WorkerRegistry struct {
	workers map[string]*WorkerInfo
	mu      sync.RWMutex

	workerRepo postgres.WorkerRepository
	logger     *zap.Logger

	heartbeatTimeout time.Duration
	cleanupInterval  time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	grpcService TaskPusher
}

// NewWorkerRegistry creates a registry instance
// The heartbeat timeout determines how long to wait before marking a worker suspect
func NewWorkerRegistry(
	workerRepo postgres.WorkerRepository,
	heartbeatTimeout time.Duration,
	logger *zap.Logger,
	grpcService TaskPusher,
) *WorkerRegistry {
	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerRegistry{
		workers:          make(map[string]*WorkerInfo),
		workerRepo:       workerRepo,
		logger:           logger,
		heartbeatTimeout: heartbeatTimeout,
		cleanupInterval:  time.Minute,
		ctx:              ctx,
		cancel:           cancel,
		grpcService:      grpcService,
	}
}

// Start begins background tasks for worker health monitoring
// This includes periodic cleanup of dead workers and heartbeat validation
func (wr *WorkerRegistry) Start() error {
	wr.logger.Info("Starting worker registry")

	// Load existing workers from database
	if err := wr.loadWorkersFromDB(); err != nil {
		return fmt.Errorf("failed to load workers from database: %w", err)
	}

	// Start cleanup goroutine
	go wr.cleanupLoop()

	return nil
}

// Stop gracefully terminates the registry
// This closes all gRPC connections to workers and stops background tasks
func (wr *WorkerRegistry) Stop() {
	wr.logger.Info("Stopping worker registry")

	wr.cancel()

	// Close all gRPC connections
	wr.mu.Lock()
	defer wr.mu.Unlock()

	for _, worker := range wr.workers {
		if worker.conn != nil {
			worker.conn.Close()
		}
	}
}

// RegisterWorker adds a new worker to the registry
// This is called when a worker connects to the coordinator for the first time
func (wr *WorkerRegistry) RegisterWorker(
	ctx context.Context,
	workerID, address string,
	maxConcurrentTasks int,
	capabilities map[string]interface{},
) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	// Check if worker already registered
	if existing, ok := wr.workers[workerID]; ok {
		wr.logger.Info("Worker reconnected",
			zap.String("worker_id", workerID),
			zap.String("address", address),
		)

		// Update heartbeat and reactivate
		existing.LastHeartbeatAt = time.Now()
		existing.Status = WorkerStatusActive
		return nil
	}

	wr.logger.Info("Registering worker",
		zap.String("worker_id", workerID),
		zap.String("address", address),
		zap.Int("max_concurrent", maxConcurrentTasks),
	)

	// NOTE: We DON'T dial back to the worker!
	// Workers are gRPC clients that connect to us via streaming RPC.
	// They maintain persistent connections through ReceiveTasks() stream.

	// Create worker info (no gRPC connection needed)
	worker := &WorkerInfo{
		ID:                 workerID,
		Address:            address,
		MaxConcurrentTasks: maxConcurrentTasks,
		CurrentTaskCount:   0,
		Status:             WorkerStatusActive,
		LastHeartbeatAt:    time.Now(),
		RegisteredAt:       time.Now(),
		Capabilities:       capabilities,
		conn:               nil, // Workers connect to us, not the other way
	}

	wr.workers[workerID] = worker

	// Persist to database
	dbWorker := &postgres.WorkerInfo{
		ID:                 worker.ID,
		Address:            worker.Address,
		MaxConcurrentTasks: worker.MaxConcurrentTasks,
		Status:             string(worker.Status),
		LastHeartbeatAt:    worker.LastHeartbeatAt,
		RegisteredAt:       worker.RegisteredAt,
		Capabilities:       worker.Capabilities,
	}
	if err := wr.workerRepo.CreateWorker(ctx, dbWorker); err != nil {
		wr.logger.Error("Failed to persist worker registration",
			zap.String("worker_id", workerID),
			zap.Error(err),
		)
		// Continue anyway - in-memory state is primary
	}

	wr.logger.Info("Worker registered",
		zap.String("worker_id", workerID),
		zap.String("address", address),
		zap.Int("max_concurrent", maxConcurrentTasks),
	)

	return nil
}

// UpdateHeartbeat records a heartbeat from a worker
// This is called frequently (every 5s) by workers to signal they are alive
func (wr *WorkerRegistry) UpdateHeartbeat(workerID string) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	worker, ok := wr.workers[workerID]
	if !ok {
		return fmt.Errorf("unknown worker: %s", workerID)
	}

	worker.LastHeartbeatAt = time.Now()

	// Reactivate if previously suspect
	if worker.Status == WorkerStatusSuspect {
		worker.Status = WorkerStatusActive
		wr.logger.Info("Worker recovered from suspect state",
			zap.String("worker_id", workerID),
		)
	}

	return nil
}

// GetAvailableWorkers returns workers that can accept new tasks
// A worker is available if it is active, has capacity, and has recent heartbeats
func (wr *WorkerRegistry) GetAvailableWorkers() []*WorkerInfo {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	available := make([]*WorkerInfo, 0)
	now := time.Now()

	for _, worker := range wr.workers {
		// Check if worker is healthy
		if worker.Status != WorkerStatusActive {
			continue
		}

		// Check heartbeat freshness
		if now.Sub(worker.LastHeartbeatAt) > wr.heartbeatTimeout {
			continue
		}

		// Check capacity
		if worker.CurrentTaskCount >= worker.MaxConcurrentTasks {
			continue
		}

		available = append(available, worker)
	}

	return available
}

// GetWorker retrieves a specific worker by ID
func (wr *WorkerRegistry) GetWorker(workerID string) (*WorkerInfo, error) {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	worker, ok := wr.workers[workerID]
	if !ok {
		return nil, fmt.Errorf("worker not found: %s", workerID)
	}

	return worker, nil
}

// IncrementTaskCount increases a worker's active task count
// This is called when the scheduler assigns a new task to the worker
func (wr *WorkerRegistry) IncrementTaskCount(workerID string) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	worker, ok := wr.workers[workerID]
	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	worker.CurrentTaskCount++

	wr.logger.Debug("Worker task count incremented",
		zap.String("worker_id", workerID),
		zap.Int("count", worker.CurrentTaskCount),
	)

	return nil
}

// DecrementTaskCount decreases a worker's active task count
// This is called when a task completes or fails
func (wr *WorkerRegistry) DecrementTaskCount(workerID string) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	worker, ok := wr.workers[workerID]
	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	if worker.CurrentTaskCount > 0 {
		worker.CurrentTaskCount--
	}

	wr.logger.Debug("Worker task count decremented",
		zap.String("worker_id", workerID),
		zap.Int("count", worker.CurrentTaskCount),
	)

	return nil
}

// MarkWorkerSuspect marks a worker as potentially unhealthy
// This is called when a task assigned to the worker becomes stale
func (wr *WorkerRegistry) MarkWorkerSuspect(workerID string) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	worker, ok := wr.workers[workerID]
	if !ok {
		return
	}

	worker.Status = WorkerStatusSuspect

	wr.logger.Warn("Worker marked as suspect",
		zap.String("worker_id", workerID),
	)
}

// SendTask sends a task assignment to a worker via gRPC
// This establishes the actual communication with the worker node
func (wr *WorkerRegistry) SendTask(
	ctx context.Context,
	workerID string,
	task *postgres.Task,
) error {
	wr.mu.RLock()
	worker, ok := wr.workers[workerID]
	wr.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	// Send task via gRPC service's streaming connection
	if wr.grpcService != nil {
		return wr.grpcService.PushTaskToWorker(workerID, task)
	}

	// Fallback to old method (should not happen)
	if worker.client == nil {
		return fmt.Errorf("no gRPC connection available")
	}

	return worker.client.AssignTask(ctx, task)
}

// cleanupLoop periodically checks for dead workers and removes them
// Dead workers are those that haven't sent heartbeats for extended periods
func (wr *WorkerRegistry) cleanupLoop() {
	ticker := time.NewTicker(wr.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			wr.performCleanup()

		case <-wr.ctx.Done():
			return
		}
	}
}

// performCleanup identifies and removes dead workers
func (wr *WorkerRegistry) performCleanup() {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	now := time.Now()
	deadThreshold := wr.heartbeatTimeout * 3 // 3x heartbeat timeout = dead

	deadWorkers := make([]string, 0)

	for id, worker := range wr.workers {
		timeSinceHeartbeat := now.Sub(worker.LastHeartbeatAt)

		if timeSinceHeartbeat > deadThreshold {
			worker.Status = WorkerStatusDead
			deadWorkers = append(deadWorkers, id)

			wr.logger.Warn("Worker marked as dead",
				zap.String("worker_id", id),
				zap.Duration("time_since_heartbeat", timeSinceHeartbeat),
			)

			// Close gRPC connection
			if worker.conn != nil {
				worker.conn.Close()
			}

			// Remove from in-memory map
			delete(wr.workers, id)
		} else if timeSinceHeartbeat > wr.heartbeatTimeout && worker.Status == WorkerStatusActive {
			// Mark as suspect if heartbeat is late
			worker.Status = WorkerStatusSuspect

			wr.logger.Warn("Worker marked as suspect due to late heartbeat",
				zap.String("worker_id", id),
				zap.Duration("time_since_heartbeat", timeSinceHeartbeat),
			)
		}
	}

	// Update database for dead workers
	if len(deadWorkers) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		for _, workerID := range deadWorkers {
			if err := wr.workerRepo.MarkWorkerDead(ctx, workerID); err != nil {
				wr.logger.Error("Failed to mark worker dead in database",
					zap.String("worker_id", workerID),
					zap.Error(err),
				)
			}
		}
	}
}

// loadWorkersFromDB restores worker state from PostgreSQL
// This is called on coordinator startup to recover from restarts
func (wr *WorkerRegistry) loadWorkersFromDB() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	workers, err := wr.workerRepo.GetActiveWorkers(ctx)
	if err != nil {
		return fmt.Errorf("failed to query workers: %w", err)
	}

	wr.logger.Info("Loaded workers from database",
		zap.Int("count", len(workers)),
	)
	
	return nil
}

// GetStats returns current registry statistics
func (wr *WorkerRegistry) GetStats() RegistryStats {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	stats := RegistryStats{
		TotalWorkers: len(wr.workers),
	}

	for _, worker := range wr.workers {
		switch worker.Status {
		case WorkerStatusActive:
			stats.ActiveWorkers++
			stats.TotalCapacity += worker.MaxConcurrentTasks
			stats.UsedCapacity += worker.CurrentTaskCount
		case WorkerStatusSuspect:
			stats.SuspectWorkers++
		case WorkerStatusDead:
			stats.DeadWorkers++
		}
	}

	return stats
}

// RegistryStats contains worker registry metrics
type RegistryStats struct {
	TotalWorkers   int
	ActiveWorkers  int
	SuspectWorkers int
	DeadWorkers    int
	TotalCapacity  int
	UsedCapacity   int
}

func (wr *WorkerRegistry) SetGRPCService(grpcService TaskPusher) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	wr.grpcService = grpcService
}
