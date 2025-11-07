// Main worker node logic that ties together the worker pool and coordinator client
// Workers are stateless compute nodes that only care about generating proofs

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
	"github.com/saiweb3dev/distributed-zkp-network/internal/worker/client"
	"github.com/saiweb3dev/distributed-zkp-network/internal/worker/executor"
	"github.com/saiweb3dev/distributed-zkp-network/internal/zkp"
	"go.uber.org/zap"
)

// Worker represents a compute node that generates zero-knowledge proofs
// It maintains a connection to the coordinator cluster and processes assigned tasks
type Worker struct {
	id                string
	coordinatorClient *client.CoordinatorClient
	workerPool        *executor.WorkerPool
	eventBus          *events.EventBus
	logger            *zap.Logger

	heartbeatInterval time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
}

// Config holds worker node configuration
type Config struct {
	WorkerID           string
	CoordinatorAddress string
	Concurrency        int
	HeartbeatInterval  time.Duration
	ZKPCurve           string
	EventBus           *events.EventBus // Optional: for publishing task lifecycle events
}

// NewWorker creates a worker instance with the specified configuration
func NewWorker(cfg Config, logger *zap.Logger) (*Worker, error) {
	// Initialize ZKP prover (reusing Phase 1 code)
	prover, err := zkp.NewGroth16Prover(cfg.ZKPCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize prover: %w", err)
	}

	// Create worker pool for concurrent proof generation
	pool := executor.NewWorkerPool(prover, cfg.Concurrency, logger)

	// Create coordinator client for communication
	coordClient, err := client.NewCoordinatorClient(
		cfg.WorkerID,
		cfg.CoordinatorAddress,
		cfg.Concurrency,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create coordinator client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Use provided event bus or create disabled one
	eventBusToUse := cfg.EventBus
	if eventBusToUse == nil {
		eventBusToUse = events.NewDisabledEventBus(logger)
	}

	return &Worker{
		id:                cfg.WorkerID,
		coordinatorClient: coordClient,
		workerPool:        pool,
		eventBus:          eventBusToUse,
		logger:            logger,
		heartbeatInterval: cfg.HeartbeatInterval,
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

// Start begins the worker's operation
// This connects to the coordinator, registers the worker, and starts processing tasks
func (w *Worker) Start() error {
	w.logger.Info("Starting worker node",
		zap.String("worker_id", w.id),
		zap.Duration("heartbeat_interval", w.heartbeatInterval),
	)

	// Step 1: Connect to coordinator cluster
	if err := w.coordinatorClient.Connect(w.ctx); err != nil {
		return fmt.Errorf("failed to connect to coordinator: %w", err)
	}

	// Step 2: Register with coordinator
	if err := w.coordinatorClient.Register(w.ctx); err != nil {
		return fmt.Errorf("failed to register with coordinator: %w", err)
	}

	// Step 3: Start worker pool
	w.workerPool.Start()

	// Step 4: Start heartbeat sender
	go w.heartbeatLoop()

	// Step 5: Start result reporter
	go w.resultReporterLoop()

	// Step 6: Start task receiver
	go w.taskReceiverLoop()

	w.logger.Info("Worker started successfully",
		zap.String("worker_id", w.id),
	)

	return nil
}

// Stop gracefully shuts down the worker
// This drains the task queue, waits for in-progress proofs to complete,
// and then disconnects from the coordinator
func (w *Worker) Stop() {
	w.logger.Info("Stopping worker node",
		zap.String("worker_id", w.id),
	)

	// Signal shutdown
	w.cancel()

	// Stop accepting new tasks
	w.workerPool.Stop()

	// Disconnect from coordinator
	w.coordinatorClient.Disconnect()

	w.logger.Info("Worker stopped",
		zap.String("worker_id", w.id),
	)
}

// heartbeatLoop sends periodic heartbeats to the coordinator
// This signals that the worker is alive and available for work
func (w *Worker) heartbeatLoop() {
	ticker := time.NewTicker(w.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(w.ctx, time.Second*3)

			if err := w.coordinatorClient.SendHeartbeat(ctx); err != nil {
				w.logger.Error("Failed to send heartbeat",
					zap.Error(err),
				)
			}

			cancel()

		case <-w.ctx.Done():
			w.logger.Info("Heartbeat loop terminated")
			return
		}
	}
}

// taskReceiverLoop receives task assignments from the coordinator
// Tasks arrive via gRPC and are submitted to the worker pool
func (w *Worker) taskReceiverLoop() {
	taskChan := w.coordinatorClient.TaskChannel()

	for {
		select {
		case task, ok := <-taskChan:
			if !ok {
				w.logger.Info("Task channel closed")
				return
			}

			w.logger.Info("Received task assignment",
				zap.String("task_id", task.ID),
				zap.String("circuit_type", task.CircuitType),
			)

			// Parse JSON InputData string to map
			var inputData map[string]interface{}
			if err := json.Unmarshal([]byte(task.InputData), &inputData); err != nil {
				w.logger.Error("Failed to parse task input data",
					zap.String("task_id", task.ID),
					zap.Error(err),
				)
				w.reportTaskFailure(task.ID, err)
				continue
			}

			// Convert client.Task to executor.Task
			executorTask := executor.Task{
				ID:          task.ID,
				CircuitType: task.CircuitType,
				InputData:   inputData,
				CreatedAt:   task.CreatedAt,
			}

			// Submit to worker pool
			ctx, cancel := context.WithTimeout(w.ctx, time.Second*5)
			if err := w.workerPool.Submit(ctx, executorTask); err != nil {
				w.logger.Error("Failed to submit task to pool",
					zap.String("task_id", task.ID),
					zap.Error(err),
				)
				w.reportTaskFailure(task.ID, err)
			}
			cancel()

		case <-w.ctx.Done():
			w.logger.Info("Task receiver loop terminated")
			return
		}
	}
}

// resultReporterLoop monitors the worker pool for completed tasks
// Results are sent back to the coordinator via gRPC
func (w *Worker) resultReporterLoop() {
	resultChan := w.workerPool.Results()

	for {
		select {
		case result, ok := <-resultChan:
			if !ok {
				w.logger.Info("Result channel closed")
				return
			}

			if result.Success {
				w.reportTaskSuccess(result)
			} else {
				w.reportTaskFailure(result.TaskID, result.Error)
			}

		case <-w.ctx.Done():
			w.logger.Info("Result reporter loop terminated")
			return
		}
	}
}

// reportTaskSuccess sends a successful result back to the coordinator
func (w *Worker) reportTaskSuccess(result executor.TaskResult) {
	ctx, cancel := context.WithTimeout(w.ctx, time.Second*10)
	defer cancel()

	if err := w.coordinatorClient.ReportCompletion(ctx, result); err != nil {
		w.logger.Error("Failed to report task completion",
			zap.String("task_id", result.TaskID),
			zap.Error(err),
		)
		// TODO: Implement retry logic for result reporting
	} else {
		w.logger.Info("Task completion reported",
			zap.String("task_id", result.TaskID),
			zap.Duration("duration", result.Duration),
		)

		// Publish EventTaskCompleted event
		// This allows real-time monitoring and client notifications
		if w.eventBus.IsEnabled() {
			event := events.Event{
				Type:      events.EventTaskCompleted,
				Timestamp: time.Now().Unix(),
				Data: map[string]interface{}{
					"task_id":     result.TaskID,
					"worker_id":   w.id,
					"duration_ms": result.Duration.Milliseconds(),
				},
			}

			if err := w.eventBus.Publish(ctx, event); err != nil {
				w.logger.Warn("Failed to publish task completion event",
					zap.Error(err),
					zap.String("task_id", result.TaskID),
				)
			} else {
				w.logger.Debug("Task completion event published",
					zap.String("task_id", result.TaskID),
				)
			}
		}
	}
}

// reportTaskFailure sends a failure report to the coordinator
func (w *Worker) reportTaskFailure(taskID string, err error) {
	ctx, cancel := context.WithTimeout(w.ctx, time.Second*10)
	defer cancel()

	if reportErr := w.coordinatorClient.ReportFailure(ctx, taskID, err.Error()); reportErr != nil {
		w.logger.Error("Failed to report task failure",
			zap.String("task_id", taskID),
			zap.Error(reportErr),
		)
	} else {
		w.logger.Info("Task failure reported",
			zap.String("task_id", taskID),
			zap.Error(err),
		)

		// Publish EventTaskFailed event
		// This allows real-time failure monitoring and alerting
		if w.eventBus.IsEnabled() {
			event := events.Event{
				Type:      events.EventTaskFailed,
				Timestamp: time.Now().Unix(),
				Data: map[string]interface{}{
					"task_id":   taskID,
					"worker_id": w.id,
					"error":     err.Error(),
				},
			}

			if pubErr := w.eventBus.Publish(ctx, event); pubErr != nil {
				w.logger.Warn("Failed to publish task failure event",
					zap.Error(pubErr),
					zap.String("task_id", taskID),
				)
			} else {
				w.logger.Debug("Task failure event published",
					zap.String("task_id", taskID),
				)
			}
		}
	}
}

// GetStats returns current worker statistics for monitoring
func (w *Worker) GetStats() WorkerStats {
	poolStats := w.workerPool.GetStats()

	return WorkerStats{
		WorkerID:        w.id,
		PoolConcurrency: poolStats.Concurrency,
		ActiveTasks:     poolStats.ActiveWorkers,
		QueuedTasks:     poolStats.QueuedTasks,
		Connected:       w.coordinatorClient.IsConnected(),
	}
}

// WorkerStats contains worker node metrics
type WorkerStats struct {
	WorkerID        string
	PoolConcurrency int
	ActiveTasks     int
	QueuedTasks     int
	Connected       bool
}
