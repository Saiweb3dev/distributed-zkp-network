// Implements the CoordinatorService gRPC interface
// This is the bridge between network calls and the coordinator's business logic

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/raft"
	"github.com/saiweb3dev/distributed-zkp-network/internal/coordinator/registry"
	pb "github.com/saiweb3dev/distributed-zkp-network/internal/proto/coordinator"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// CoordinatorGRPCService implements the protobuf-defined CoordinatorService
// It delegates to the WorkerRegistry and TaskRepository for actual work
type CoordinatorGRPCService struct {
	pb.UnimplementedCoordinatorServiceServer

	workerRegistry *registry.WorkerRegistry
	taskRepo       postgres.TaskRepository
	raftNode       *raft.RaftNode
	logger         *zap.Logger

	// Task streaming: map of worker_id -> channel for pushing tasks
	taskStreams map[string]chan *pb.TaskAssignment
	streamsMu   sync.RWMutex
}

// NewCoordinatorGRPCService creates a new gRPC service instance
func NewCoordinatorGRPCService(
	workerRegistry *registry.WorkerRegistry,
	taskRepo postgres.TaskRepository,
	raftNode *raft.RaftNode,
	logger *zap.Logger,
) *CoordinatorGRPCService {
	return &CoordinatorGRPCService{
		workerRegistry: workerRegistry,
		taskRepo:       taskRepo,
		raftNode:       raftNode,
		logger:         logger,
		taskStreams:    make(map[string]chan *pb.TaskAssignment),
	}
}

// RegisterWorker handles worker registration requests
// This is called once when a worker starts up
func (s *CoordinatorGRPCService) RegisterWorker(
	ctx context.Context,
	req *pb.RegisterWorkerRequest,
) (*pb.RegisterWorkerResponse, error) {
	// If we're not the leader, redirect to leader
	if !s.raftNode.IsLeader() {
		leaderAddr := s.raftNode.GetLeaderAddress()
		if leaderAddr == "" {
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

	// Extract worker's IP address from gRPC context
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Internal, "failed to get peer info")
	}

	// Use peer address as worker address
	// In production, worker should provide its own address
	workerAddress := peerInfo.Addr.String()

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

// ReportCompletion handles successful task completion notifications
// Worker sends this when it finishes generating a proof
func (s *CoordinatorGRPCService) ReportCompletion(
	ctx context.Context,
	req *pb.TaskCompletionRequest,
) (*pb.TaskCompletionResponse, error) {
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

	// Create a channel for this worker's task stream
	taskChan := make(chan *pb.TaskAssignment, 10)

	// Register the stream
	s.streamsMu.Lock()
	s.taskStreams[workerID] = taskChan
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

// PushTaskToWorker sends a task to a specific worker via its stream
// Called by the task scheduler when assigning work
func (s *CoordinatorGRPCService) PushTaskToWorker(
	workerID string,
	task *postgres.Task,
) error {
	s.streamsMu.RLock()
	taskChan, exists := s.taskStreams[workerID]
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
	case taskChan <- assignment:
		return nil
	case <-time.After(time.Second * 5):
		return fmt.Errorf("timeout sending task to worker")
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
