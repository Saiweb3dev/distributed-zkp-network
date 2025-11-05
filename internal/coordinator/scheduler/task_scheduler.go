// Implements task distribution logic for the coordinator leader
// Only the Raft leader runs the scheduler to avoid duplicate assignments
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/events"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/raft"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/registry"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
)

// TaskScheduler continuously polls for pending tasks and assigns them to available workers
// The scheduler runs only on the Raft leader to ensure single-writer consistency
type TaskScheduler struct {
	taskRepo       postgres.TaskRepository
	workerRegistry *registry.WorkerRegistry
	raftNode       *raft.RaftNode
	eventBus       *events.EventBus
	logger         *zap.Logger

	pollInterval  time.Duration
	assignTimeout time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	metrics SchedulerMetrics
}

// SchedulerMetrics tracks scheduler performance for observability
type SchedulerMetrics struct {
	TasksAssigned     int64
	AssignmentsFailed int64
	PollCycles        int64
	EventTriggered    int64
	LastAssignmentAt  time.Time
}

// NewTaskScheduler creates a scheduler instance
// The poll interval determines how frequently the scheduler checks for new tasks
// A shorter interval provides lower latency but increases database load
func NewTaskScheduler(
	taskRepo postgres.TaskRepository,
	workerRegistry *registry.WorkerRegistry,
	raftNode *raft.RaftNode,
	eventBus *events.EventBus,
	pollInterval time.Duration,
	logger *zap.Logger,
) *TaskScheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &TaskScheduler{
		taskRepo:       taskRepo,
		workerRegistry: workerRegistry,
		raftNode:       raftNode,
		eventBus:       eventBus,
		logger:         logger,
		pollInterval:   pollInterval,
		assignTimeout:  time.Second * 5,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the scheduling loop in a background goroutine
// This method returns immediately after starting the loop
// The scheduler continues until Stop is called or the context is cancelled
func (ts *TaskScheduler) Start() {
	ts.logger.Info("Starting task scheduler",
		zap.Duration("poll_interval", ts.pollInterval),
		zap.Bool("event_driven", ts.eventBus.IsEnabled()),
	)

	// Start event-driven listener if Redis is available
	if ts.eventBus.IsEnabled() {
		go ts.eventDrivenLoop()
	}

	// Always keep polling as fallback (but less frequent)
	go ts.schedulingLoop()
}

// Stop gracefully terminates the scheduling loop
// This method blocks until the current scheduling cycle completes
func (ts *TaskScheduler) Stop() {
	ts.logger.Info("Stopping task scheduler")
	ts.cancel()
}

// eventDrivenLoop listens for task creation events from Redis
// This provides near-instant task scheduling instead of waiting for polling
func (ts *TaskScheduler) eventDrivenLoop() {
	ts.logger.Info("Starting event-driven scheduling loop")

	// Subscribe to task creation events
	eventChan, err := ts.eventBus.Subscribe(ts.ctx, events.EventTaskCreated)
	if err != nil {
		ts.logger.Error("Failed to subscribe to events", zap.Error(err))
		return
	}

	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				ts.logger.Info("Event channel closed")
				return
			}

			// Only process if this node is the leader
			if !ts.raftNode.IsLeader() {
				ts.logger.Debug("Skipping event (not leader)")
				continue
			}

			ts.metrics.EventTriggered++

			ts.logger.Debug("Event received: new task created",
				zap.String("type", string(event.Type)),
				zap.Any("data", event.Data),
			)

			// Process immediately!
			if err := ts.scheduleOneCycle(); err != nil {
				ts.logger.Error("Event-driven schedule failed", zap.Error(err))
			}

		case <-ts.ctx.Done():
			ts.logger.Info("Event-driven loop terminated")
			return
		}
	}
}

// schedulingLoop is the main control flow that runs continuously on the leader
// Each iteration performs three operations: fetch pending tasks, select workers,
// and assign tasks while handling any errors that occur
func (ts *TaskScheduler) schedulingLoop() {
	ticker := time.NewTicker(ts.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// CRITICAL: Only leader schedules tasks
			if !ts.raftNode.IsLeader() {
				ts.logger.Debug("Skipping schedule cycle (not leader)")
				continue
			}
			ts.metrics.PollCycles++

			if err := ts.scheduleOneCycle(); err != nil {
				ts.logger.Error("Scheduling cycle failed",
					zap.Error(err),
				)
			}

		case <-ts.ctx.Done():
			ts.logger.Info("Scheduling loop terminated")
			return
		}
	}
}

// scheduleOneCycle performs a single iteration of task assignment
// This method fetches pending tasks from PostgreSQL, identifies available workers,
// and attempts to assign each task to an appropriate worker
func (ts *TaskScheduler) scheduleOneCycle() error {
	ctx, cancel := context.WithTimeout(ts.ctx, ts.assignTimeout)
	defer cancel()

	// Step 1: Fetch pending tasks from database
	tasks, err := ts.taskRepo.GetPendingTasks(ctx, 10) // Batch size
	if err != nil {
		return fmt.Errorf("failed to fetch pending tasks: %w", err)
	}

	if len(tasks) == 0 {
		// No work to do, this is normal
		return nil
	}

	ts.logger.Debug("Found pending tasks",
		zap.Int("count", len(tasks)),
	)

	// Step 2: Get list of available workers
	workers := ts.workerRegistry.GetAvailableWorkers()
	if len(workers) == 0 {
		ts.logger.Warn("No available workers for task assignment")
		return nil
	}

	ts.logger.Debug("Available workers",
		zap.Int("count", len(workers)),
	)

	// Step 3: Assign tasks to workers using round-robin strategy
	workerIndex := 0
	for _, task := range tasks {
		// Select next worker (round-robin)
		worker := workers[workerIndex%len(workers)]
		workerIndex++

		// Attempt assignment
		if err := ts.assignTaskToWorker(ctx, task, worker); err != nil {
			ts.logger.Error("Failed to assign task",
				zap.String("task_id", task.ID),
				zap.String("worker_id", worker.ID),
				zap.Error(err),
			)
			ts.metrics.AssignmentsFailed++
			continue
		}

		ts.metrics.TasksAssigned++
		ts.metrics.LastAssignmentAt = time.Now()

		ts.logger.Info("Task assigned",
			zap.String("task_id", task.ID),
			zap.String("worker_id", worker.ID),
			zap.String("circuit_type", task.CircuitType),
		)
	}

	return nil
}

// assignTaskToWorker performs the atomic operation of assigning a task to a worker
// This method updates the database, notifies the worker registry, and sends the
// work payload to the worker via gRPC
func (ts *TaskScheduler) assignTaskToWorker(
	ctx context.Context,
	task *postgres.Task,
	worker *registry.WorkerInfo,
) error {
	// Step 1: Update database (atomic operation via stored procedure)
	if err := ts.taskRepo.AssignTask(ctx, task.ID, worker.ID); err != nil {
		return fmt.Errorf("database assignment failed: %w", err)
	}

	// Step 2: Notify worker registry to update capacity tracking
	if err := ts.workerRegistry.IncrementTaskCount(worker.ID); err != nil {
		// Log but don't fail - the database is the source of truth
		ts.logger.Warn("Failed to update worker registry",
			zap.String("worker_id", worker.ID),
			zap.Error(err),
		)
	}

	// Step 3: Send task to worker via gRPC
	// The worker client is maintained by the registry
	if err := ts.workerRegistry.SendTask(ctx, worker.ID, task); err != nil {
		// If gRPC send fails, we need to unassign the task
		ts.logger.Error("Failed to send task to worker, unassigning",
			zap.String("task_id", task.ID),
			zap.String("worker_id", worker.ID),
			zap.Error(err),
		)

		// Revert the assignment
		if revertErr := ts.taskRepo.UnassignTask(ctx, task.ID); revertErr != nil {
			ts.logger.Error("Failed to revert task assignment",
				zap.String("task_id", task.ID),
				zap.Error(revertErr),
			)
		}

		ts.workerRegistry.DecrementTaskCount(worker.ID)
		return fmt.Errorf("gRPC send failed: %w", err)
	}

	return nil
}

// HandleStaleTasks identifies and reassigns tasks that have been in-progress too long
// This recovery mechanism handles worker failures where heartbeats stopped but the
// task remains marked as in-progress in the database
func (ts *TaskScheduler) HandleStaleTasks(ctx context.Context, staleThreshold time.Duration) error {
	staleTasks, err := ts.taskRepo.GetStaleTasks(ctx, staleThreshold)
	if err != nil {
		return fmt.Errorf("failed to fetch stale tasks: %w", err)
	}

	if len(staleTasks) == 0 {
		return nil
	}

	ts.logger.Warn("Found stale tasks for reassignment",
		zap.Int("count", len(staleTasks)),
	)

	for _, task := range staleTasks {
		// Mark worker as potentially dead
		ts.workerRegistry.MarkWorkerSuspect(task.WorkerID)

		// Reset task to pending status for reassignment
		if err := ts.taskRepo.ResetTaskToPending(ctx, task.ID); err != nil {
			ts.logger.Error("Failed to reset stale task",
				zap.String("task_id", task.ID),
				zap.Error(err),
			)
			continue
		}

		ts.logger.Info("Stale task reset for reassignment",
			zap.String("task_id", task.ID),
			zap.String("previous_worker", task.WorkerID),
			zap.Duration("stale_duration", time.Since(task.StartedAt)),
		)
	}

	return nil
}

// GetMetrics returns current scheduler statistics for monitoring
func (ts *TaskScheduler) GetMetrics() SchedulerMetrics {
	return ts.metrics
}

// SchedulingPolicy defines the strategy for selecting workers for tasks
// This interface allows swapping scheduling algorithms without changing core logic
type SchedulingPolicy interface {
	SelectWorker(task *postgres.Task, workers []*registry.WorkerInfo) (*registry.WorkerInfo, error)
}

// RoundRobinPolicy implements simple round-robin worker selection
type RoundRobinPolicy struct {
	nextIndex int
}

func (p *RoundRobinPolicy) SelectWorker(
	task *postgres.Task,
	workers []*registry.WorkerInfo,
) (*registry.WorkerInfo, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	worker := workers[p.nextIndex%len(workers)]
	p.nextIndex++

	return worker, nil
}

// LeastLoadedPolicy selects the worker with the most available capacity
type LeastLoadedPolicy struct{}

func (p *LeastLoadedPolicy) SelectWorker(
	task *postgres.Task,
	workers []*registry.WorkerInfo,
) (*registry.WorkerInfo, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	var bestWorker *registry.WorkerInfo
	maxAvailableSlots := -1

	for _, worker := range workers {
		availableSlots := worker.MaxConcurrentTasks - worker.CurrentTaskCount
		if availableSlots > maxAvailableSlots {
			maxAvailableSlots = availableSlots
			bestWorker = worker
		}
	}

	return bestWorker, nil
}

// PriorityPolicy considers task priority when selecting workers
// Higher priority tasks get assigned to workers with more capacity
type PriorityPolicy struct{}

func (p *PriorityPolicy) SelectWorker(
	task *postgres.Task,
	workers []*registry.WorkerInfo,
) (*registry.WorkerInfo, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	// For now, treat all tasks equally
	// In future, could parse priority from task.InputData
	// and prefer faster workers for high-priority tasks

	// Default to least loaded
	leastLoaded := &LeastLoadedPolicy{}
	return leastLoaded.SelectWorker(task, workers)
}

// SchedulerConfig holds tunable parameters for the scheduler
type SchedulerConfig struct {
	PollInterval     time.Duration    // How often to check for new tasks
	BatchSize        int              // Max tasks to assign per cycle
	StaleTaskTimeout time.Duration    // When to consider a task stale
	Policy           SchedulingPolicy // Worker selection strategy
}

// DefaultSchedulerConfig returns reasonable defaults for production use
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		PollInterval:     time.Second * 2,
		BatchSize:        10,
		StaleTaskTimeout: time.Minute * 5,
		Policy:           &RoundRobinPolicy{},
	}
}
