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
	"github.com/saiweb3dev/distributed-zkp-network/internal/worker/constants"
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

	// Step 4: Start heartbeat sender with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("Heartbeat loop panic - worker may become unresponsive",
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()
		w.heartbeatLoop()
	}()

	// Step 5: Start result reporter with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("Result reporter loop panic - results may be lost",
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()
		w.resultReporterLoop()
	}()

	// Step 6: Start task receiver with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("Task receiver loop panic - no new tasks will be received",
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()
		w.taskReceiverLoop()
	}()

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
			w.sendHeartbeatWithRetry()

		case <-w.ctx.Done():
			w.logger.Info("Heartbeat loop terminated")
			return
		}
	}
}

// sendHeartbeatWithRetry sends a heartbeat with retry logic
func (w *Worker) sendHeartbeatWithRetry() {
	// Use shorter retry config for heartbeats (they happen frequently)
	cfg := RetryConfig{
		MaxRetries:  constants.HeartbeatMaxRetries,
		BaseBackoff: constants.HeartbeatBaseBackoff,
		MaxBackoff:  constants.HeartbeatMaxBackoff,
	}

	err := RetryWithBackoff(
		w.ctx,
		cfg,
		w.logger,
		"send_heartbeat",
		func(ctx context.Context) error {
			heartbeatCtx, cancel := context.WithTimeout(ctx, constants.HeartbeatTimeout)
			defer cancel()
			return w.coordinatorClient.SendHeartbeat(heartbeatCtx)
		},
	)

	if err != nil {
		w.logger.Error("Failed to send heartbeat after retries - worker may be marked dead",
			zap.Error(err),
		)
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

			// Notify coordinator that we're starting to process this task
			// This transitions task status from 'assigned' to 'in_progress'
			startCtx, startCancel := context.WithTimeout(w.ctx, constants.TaskStartTimeout)
			if err := w.coordinatorClient.StartTask(startCtx, task.ID); err != nil {
				w.logger.Warn("Failed to notify task start (will still process task)",
					zap.String("task_id", task.ID),
					zap.Error(err),
				)
				// Don't fail the task - continue processing even if notification fails
				// The coordinator will accept completion from 'assigned' status temporarily
			}
			startCancel()

			// Submit to worker pool with longer timeout
			// Allows for momentary queue buildup without false failures
			ctx, cancel := context.WithTimeout(w.ctx, constants.TaskSubmitTimeout)
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
// Handles results concurrently to prevent backpressure on worker pool
func (w *Worker) resultReporterLoop() {
	resultChan := w.workerPool.Results()

	for {
		select {
		case result, ok := <-resultChan:
			if !ok {
				w.logger.Info("Result channel closed")
				return
			}

			// Handle each result in a goroutine to prevent blocking
			// This allows multiple results to be reported concurrently
			go func(r executor.TaskResult) {
				defer func() {
					if rec := recover(); rec != nil {
						w.logger.Error("Panic in result reporter",
							zap.String("task_id", r.TaskID),
							zap.Any("panic", rec),
							zap.Stack("stack"),
						)
					}
				}()

				if r.Success {
					w.reportTaskSuccess(r)
				} else {
					w.reportTaskFailure(r.TaskID, r.Error)
				}
			}(result)

		case <-w.ctx.Done():
			w.logger.Info("Result reporter loop terminated")
			return
		}
	}
}

// reportTaskSuccess sends a successful result back to the coordinator
// Uses aggressive retry since losing successful proofs is unacceptable
func (w *Worker) reportTaskSuccess(result executor.TaskResult) {
	operation := fmt.Sprintf("report_task_completion:%s", result.TaskID)

	err := RetryWithBackoff(
		w.ctx,
		AggressiveRetryConfig(), // 5 retries with exponential backoff
		w.logger.With(zap.String("task_id", result.TaskID)),
		operation,
		func(ctx context.Context) error {
			reportCtx, cancel := context.WithTimeout(ctx, constants.TaskResultTimeout)
			defer cancel()
			return w.coordinatorClient.ReportCompletion(reportCtx, result)
		},
	)

	if err != nil {
		w.logger.Error("Failed to report task completion after all retries - result may be lost",
			zap.String("task_id", result.TaskID),
			zap.Error(err),
		)
		return
	}

	// Success - log and publish event
	w.logger.Info("Task completion reported",
		zap.String("task_id", result.TaskID),
		zap.Duration("duration", result.Duration),
	)

	w.publishTaskEvent(events.EventTaskCompleted, result.TaskID, map[string]interface{}{
		"duration_ms": result.Duration.Milliseconds(),
	})
}

// reportTaskFailure sends a failure report to the coordinator
// Uses retry logic to ensure coordinator knows about failures
func (w *Worker) reportTaskFailure(taskID string, taskErr error) {
	operation := fmt.Sprintf("report_task_failure:%s", taskID)

	reportErr := RetryWithBackoff(
		w.ctx,
		DefaultRetryConfig(), // 3 retries with exponential backoff
		w.logger.With(zap.String("task_id", taskID)),
		operation,
		func(ctx context.Context) error {
			reportCtx, cancel := context.WithTimeout(ctx, constants.TaskResultTimeout)
			defer cancel()
			return w.coordinatorClient.ReportFailure(reportCtx, taskID, taskErr.Error())
		},
	)

	if reportErr != nil {
		w.logger.Error("Failed to report task failure after all retries",
			zap.String("task_id", taskID),
			zap.Error(reportErr),
			zap.NamedError("original_error", taskErr),
		)
		return
	}

	// Success - log and publish event
	w.logger.Info("Task failure reported",
		zap.String("task_id", taskID),
		zap.Error(taskErr),
	)

	w.publishTaskEvent(events.EventTaskFailed, taskID, map[string]interface{}{
		"error": taskErr.Error(),
	})
}

// publishTaskEvent publishes task lifecycle events to the event bus
// Consolidates event publishing logic with consistent error handling
func (w *Worker) publishTaskEvent(eventType events.EventType, taskID string, extraData map[string]interface{}) {
	if !w.eventBus.IsEnabled() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), constants.EventPublishTimeout)
	defer cancel()

	// Base event data
	data := map[string]interface{}{
		"task_id":   taskID,
		"worker_id": w.id,
	}

	// Merge extra data
	for k, v := range extraData {
		data[k] = v
	}

	event := events.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	if err := w.eventBus.Publish(ctx, event); err != nil {
		w.logger.Warn("Failed to publish event",
			zap.String("event_type", string(eventType)),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
	} else {
		w.logger.Debug("Event published",
			zap.String("event_type", string(eventType)),
			zap.String("task_id", taskID),
		)
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
