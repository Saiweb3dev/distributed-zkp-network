// Implements the CoordinatorService gRPC interface.
// This is the bridge between network calls and the coordinator's business logic.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/constants"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/raft"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/registry"
	pb "github.com/saiweb3dev/distributed-zkp-network/internal/proto/coordinator"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// taskStream holds stream channel and metadata for a worker connection.
type taskStream struct {
	channel      chan *pb.TaskAssignment
	lastActivity time.Time
	mu           sync.Mutex
}

// CoordinatorGRPCService implements the protobuf-defined CoordinatorService.
// It delegates to the WorkerRegistry and TaskRepository for actual work.
type CoordinatorGRPCService struct {
	pb.UnimplementedCoordinatorServiceServer

	workerRegistry *registry.WorkerRegistry
	taskRepo       postgres.TaskRepository
	raftNode       *raft.RaftNode
	logger         *zap.Logger

	// Task streaming: map of worker_id -> stream info for pushing tasks
	taskStreams map[string]*taskStream
	streamsMu   sync.RWMutex

	// Graceful shutdown support
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	shutdownWg     sync.WaitGroup

	// Completed task tracking for idempotency
	completedTasks map[string]bool
	completedMu    sync.RWMutex
}

// NewCoordinatorGRPCService creates a new gRPC service instance.
func NewCoordinatorGRPCService(
	workerRegistry *registry.WorkerRegistry,
	taskRepo postgres.TaskRepository,
	raftNode *raft.RaftNode,
	logger *zap.Logger,
) *CoordinatorGRPCService {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	service := &CoordinatorGRPCService{
		workerRegistry: workerRegistry,
		taskRepo:       taskRepo,
		raftNode:       raftNode,
		logger:         logger,
		taskStreams:    make(map[string]*taskStream),
		completedTasks: make(map[string]bool),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}

	// Start keepalive goroutine for stream health
	service.shutdownWg.Add(1)
	go service.streamKeepaliveLoop()

	return service
}

// Shutdown gracefully stops the gRPC service.
func (s *CoordinatorGRPCService) Shutdown() error {
	s.logger.Info("Initiating gRPC service shutdown")

	// Signal shutdown
	s.shutdownCancel()

	// Close all active streams
	s.closeAllStreams()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		s.shutdownWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("gRPC service shutdown complete")
		return nil
	case <-time.After(constants.ComponentShutdownTimeout):
		s.logger.Warn("gRPC service shutdown timed out, forcing close")
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

// closeAllStreams closes all active worker streams.
func (s *CoordinatorGRPCService) closeAllStreams() {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()

	s.logger.Info("Closing all worker streams", zap.Int("count", len(s.taskStreams)))

	for workerID, stream := range s.taskStreams {
		close(stream.channel)
		s.logger.Debug("Closed stream", zap.String("worker_id", workerID))
	}

	s.taskStreams = make(map[string]*taskStream)
}

// streamKeepaliveLoop periodically sends keepalive messages on idle streams.
func (s *CoordinatorGRPCService) streamKeepaliveLoop() {
	defer s.shutdownWg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Stream keepalive loop panic",
				zap.Any("panic", r),
				zap.Stack("stack"),
			)
		}
	}()

	ticker := time.NewTicker(constants.StreamKeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sendKeepaliveToIdleStreams()
		case <-s.shutdownCtx.Done():
			s.logger.Info("Stream keepalive loop stopped")
			return
		}
	}
}

// sendKeepaliveToIdleStreams sends keepalive messages to idle worker streams.
func (s *CoordinatorGRPCService) sendKeepaliveToIdleStreams() {
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()

	now := time.Now()
	idleThreshold := constants.StreamKeepaliveInterval

	for workerID, stream := range s.taskStreams {
		stream.mu.Lock()
		timeSinceActivity := now.Sub(stream.lastActivity)
		stream.mu.Unlock()

		if timeSinceActivity > idleThreshold {
			// Send keepalive (empty task with special flag)
			keepalive := &pb.TaskAssignment{
				TaskId:      "", // Empty indicates keepalive
				CircuitType: "",
			}

			select {
			case stream.channel <- keepalive:
				stream.mu.Lock()
				stream.lastActivity = now
				stream.mu.Unlock()
			default:
				// Channel full, worker is slow
				s.logger.Warn("Worker stream buffer full, skipping keepalive",
					zap.String("worker_id", workerID),
				)
			}
		}
	}
}

// RegisterWorker handles worker registration requests
// This is called once when a worker starts up
func (s *CoordinatorGRPCService) RegisterWorker(
	ctx context.Context,
	req *pb.RegisterWorkerRequest,
) (*pb.RegisterWorkerResponse, error) {
	// Only leaders can register workers (ensures consistency via Raft)
	if !s.raftNode.IsLeader() {
		leaderAddr := s.raftNode.GetLeaderAddress()

		if leaderAddr == "" {
			s.logger.Warn("No leader elected for registration",
				zap.String("worker_id", req.WorkerId),
			)
			return &pb.RegisterWorkerResponse{
				Success: false,
				Message: "No leader elected, retry later",
			}, nil
		}

		s.logger.Info("Not leader, redirecting registration",
			zap.String("worker_id", req.WorkerId),
			zap.String("leader", leaderAddr),
		)

		return &pb.RegisterWorkerResponse{
			Success: false,
			Message: fmt.Sprintf("Not leader, connect to: %s", leaderAddr),
		}, nil
	}

	// We're the leader, handle registration
	s.logger.Info("Worker registration request",
		zap.String("worker_id", req.WorkerId),
		zap.Int32("max_concurrent", req.MaxConcurrentTasks),
	)

	// Extract worker's address from gRPC peer context
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		s.logger.Error("Failed to get peer info for worker registration",
			zap.String("worker_id", req.WorkerId),
		)
		return nil, status.Error(codes.Internal, "failed to get peer info")
	}

	// Use peer address as worker address
	// Note: In production with NAT/load balancers, consider adding advertised_address to protobuf
	workerAddress := peerInfo.Addr.String()
	s.logger.Debug("Extracted worker address from peer",
		zap.String("worker_id", req.WorkerId),
		zap.String("address", workerAddress),
	)

	// Convert capabilities map
	capabilities := make(map[string]interface{})
	for k, v := range req.Capabilities {
		capabilities[k] = v
	}

	// Register with the worker registry
	err := s.workerRegistry.RegisterWorker(
		ctx,
		req.WorkerId,
		workerAddress,
		int(req.MaxConcurrentTasks),
		capabilities,
	)

	if err != nil {
		s.logger.Error("Failed to register worker",
			zap.String("worker_id", req.WorkerId),
			zap.Error(err),
		)
		return &pb.RegisterWorkerResponse{
			Success: false,
			Message: fmt.Sprintf("Registration failed: %v", err),
		}, nil
	}

	return &pb.RegisterWorkerResponse{
		Success: true,
		Message: "Worker registered successfully",
	}, nil
}

// SendHeartbeat processes heartbeat messages from workers
// Called every 5 seconds to maintain worker health status
func (s *CoordinatorGRPCService) SendHeartbeat(
	ctx context.Context,
	req *pb.HeartbeatRequest,
) (*pb.HeartbeatResponse, error) {
	s.logger.Debug("Heartbeat received",
		zap.String("worker_id", req.WorkerId),
	)

	// Update heartbeat timestamp in registry
	err := s.workerRegistry.UpdateHeartbeat(req.WorkerId)
	if err != nil {
		s.logger.Warn("Failed to update heartbeat",
			zap.String("worker_id", req.WorkerId),
			zap.Error(err),
		)
		return &pb.HeartbeatResponse{Success: false}, nil
	}

	return &pb.HeartbeatResponse{Success: true}, nil
}

// ReportCompletion handles successful task completion notifications.
// Worker sends this when it finishes generating a proof.
// Implements idempotency to handle duplicate completion reports safely.
func (s *CoordinatorGRPCService) ReportCompletion(
	ctx context.Context,
	req *pb.TaskCompletionRequest,
) (*pb.TaskCompletionResponse, error) {
	// Check if already completed (idempotency)
	s.completedMu.RLock()
	alreadyCompleted := s.completedTasks[req.TaskId]
	s.completedMu.RUnlock()

	if alreadyCompleted {
		s.logger.Warn("Duplicate completion report ignored",
			zap.String("task_id", req.TaskId),
			zap.String("worker_id", req.WorkerId),
		)
		return &pb.TaskCompletionResponse{
			Success: true,
			Message: "Task already completed (duplicate ignored)",
		}, nil
	}

	s.logger.Info("Task completion reported",
		zap.String("task_id", req.TaskId),
		zap.String("worker_id", req.WorkerId),
		zap.Int64("duration_ms", req.DurationMs),
	)

	// Update task status in database
	err := s.taskRepo.CompleteTask(
		ctx,
		req.TaskId,
		req.ProofData,
		map[string]interface{}{
			"metadata": string(req.ProofMetadataJson),
		},
		req.MerkleRoot,
	)

	if err != nil {
		s.logger.Error("Failed to complete task",
			zap.String("task_id", req.TaskId),
			zap.Error(err),
		)
		return &pb.TaskCompletionResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update task: %v", err),
		}, nil
	}

	// Mark as completed for idempotency
	s.completedMu.Lock()
	s.completedTasks[req.TaskId] = true
	s.completedMu.Unlock()

	// Decrement worker's task count
	if err := s.workerRegistry.DecrementTaskCount(req.WorkerId); err != nil {
		s.logger.Warn("Failed to decrement worker task count",
			zap.String("worker_id", req.WorkerId),
			zap.Error(err),
		)
	}

	return &pb.TaskCompletionResponse{
		Success: true,
		Message: "Task completed successfully",
	}, nil
}

// ReportFailure handles task failure notifications
// Worker sends this when proof generation fails
func (s *CoordinatorGRPCService) ReportFailure(
	ctx context.Context,
	req *pb.TaskFailureRequest,
) (*pb.TaskFailureResponse, error) {
	s.logger.Warn("Task failure reported",
		zap.String("task_id", req.TaskId),
		zap.String("worker_id", req.WorkerId),
		zap.String("error", req.ErrorMessage),
	)

	// Update task status to failed
	err := s.taskRepo.FailTask(ctx, req.TaskId, req.ErrorMessage)
	if err != nil {
		s.logger.Error("Failed to mark task as failed",
			zap.String("task_id", req.TaskId),
			zap.Error(err),
		)
		return &pb.TaskFailureResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update task: %v", err),
		}, nil
	}

	// Decrement worker's task count
	if err := s.workerRegistry.DecrementTaskCount(req.WorkerId); err != nil {
		s.logger.Warn("Failed to decrement worker task count",
			zap.String("worker_id", req.WorkerId),
			zap.Error(err),
		)
	}

	return &pb.TaskFailureResponse{
		Success: true,
		Message: "Task failure recorded",
	}, nil
}

// ReceiveTasks establishes a long-lived streaming connection for task delivery
// The coordinator can push tasks to the worker through this stream
// This is a server-streaming RPC - worker opens connection and waits for tasks
func (s *CoordinatorGRPCService) ReceiveTasks(
	req *pb.ReceiveTasksRequest,
	stream pb.CoordinatorService_ReceiveTasksServer,
) error {
	workerID := req.WorkerId

	// Only leaders push tasks
	if !s.raftNode.IsLeader() {
		s.logger.Info("Non-leader, not pushing tasks",
			zap.String("worker_id", workerID),
		)

		// Keep stream open but don't push tasks
		// Workers will reconnect on leadership change
		<-stream.Context().Done()
		return stream.Context().Err()
	}

	s.logger.Info("Worker connected for task stream",
		zap.String("worker_id", workerID),
	)

	// Create a channel for this worker's task stream with bounded buffer
	taskChan := make(chan *pb.TaskAssignment, constants.TaskStreamBufferSize)

	// Create stream metadata
	streamInfo := &taskStream{
		channel:      taskChan,
		lastActivity: time.Now(),
	}

	// Register the stream
	s.streamsMu.Lock()
	s.taskStreams[workerID] = streamInfo
	s.streamsMu.Unlock()

	// Clean up when connection closes
	defer func() {
		s.streamsMu.Lock()
		delete(s.taskStreams, workerID)
		close(taskChan)
		s.streamsMu.Unlock()

		s.logger.Info("Worker task stream closed",
			zap.String("worker_id", workerID),
		)
	}()

	// Stream tasks to worker until connection closes
	for {
		select {
		case task, ok := <-taskChan:
			if !ok {
				return nil
			}

			// Send task assignment to worker
			if err := stream.Send(task); err != nil {
				s.logger.Error("Failed to send task to worker",
					zap.String("worker_id", workerID),
					zap.String("task_id", task.TaskId),
					zap.Error(err),
				)
				return err
			}

			s.logger.Info("Task sent to worker",
				zap.String("worker_id", workerID),
				zap.String("task_id", task.TaskId),
			)

		case <-stream.Context().Done():
			// Client disconnected
			return stream.Context().Err()
		}
	}
}

// PushTaskToWorker sends a task to a specific worker via its stream.
// Called by the task scheduler when assigning work.
func (s *CoordinatorGRPCService) PushTaskToWorker(
	workerID string,
	task *postgres.Task,
) error {
	s.streamsMu.RLock()
	streamInfo, exists := s.taskStreams[workerID]
	s.streamsMu.RUnlock()

	if !exists {
		return fmt.Errorf("worker stream not found: %s", workerID)
	}

	// Marshal InputData to JSON
	inputDataJSON, err := json.Marshal(task.InputData)
	if err != nil {
		return fmt.Errorf("failed to marshal input data: %w", err)
	}

	// Convert postgres.Task to protobuf TaskAssignment
	assignment := &pb.TaskAssignment{
		TaskId:        task.ID,
		CircuitType:   task.CircuitType,
		InputDataJson: inputDataJSON,
		CreatedAt:     task.CreatedAt.Unix(),
	}

	// Non-blocking send with timeout
	select {
	case streamInfo.channel <- assignment:
		// Update last activity time
		streamInfo.mu.Lock()
		streamInfo.lastActivity = time.Now()
		streamInfo.mu.Unlock()
		return nil
	case <-time.After(constants.TaskPushTimeout):
		return fmt.Errorf("timeout sending task to worker %s", workerID)
	case <-s.shutdownCtx.Done():
		return fmt.Errorf("service shutting down")
	}
}

// GetConnectedWorkers returns list of workers with active streams
func (s *CoordinatorGRPCService) GetConnectedWorkers() []string {
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()

	workers := make([]string, 0, len(s.taskStreams))
	for workerID := range s.taskStreams {
		workers = append(workers, workerID)
	}

	return workers
}
