// internal/worker/client/coordinator_client.go
// gRPC client for worker to communicate with coordinator cluster
// Handles connection management, registration, heartbeats, and result reporting

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	pb "github.com/saiweb3dev/distributed-zkp-network/internal/proto/coordinator"
	"github.com/saiweb3dev/distributed-zkp-network/internal/worker/executor"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// CoordinatorClient manages the worker's connection to the coordinator
// It maintains a gRPC connection and provides methods for all coordinator interactions
type CoordinatorClient struct {
	workerID           string
	coordinatorAddr    string
	maxConcurrentTasks int
	logger             *zap.Logger

	// gRPC connection and client
	conn   *grpc.ClientConn
	client pb.CoordinatorServiceClient

	// Task receiving
	taskChan         chan *Task
	taskStreamCancel context.CancelFunc

	// Connection state
	connected bool
	mu        sync.RWMutex
}

// Task represents a work assignment from the coordinator
// This mirrors the database Task structure but is simpler
type Task struct {
	ID          string
	CircuitType string
	InputData   string
	CreatedAt   time.Time
}

// NewCoordinatorClient creates a new coordinator client instance
func NewCoordinatorClient(
	workerID string,
	coordinatorAddr string,
	maxConcurrentTasks int,
	logger *zap.Logger,
) (*CoordinatorClient, error) {
	return &CoordinatorClient{
		workerID:           workerID,
		coordinatorAddr:    coordinatorAddr,
		maxConcurrentTasks: maxConcurrentTasks,
		logger:             logger,
		taskChan:           make(chan *Task, 10),
		connected:          false,
	}, nil
}

// Connect establishes a gRPC connection to the coordinator
func (c *CoordinatorClient) Connect(ctx context.Context) error {
	c.logger.Info("Connecting to coordinator",
		zap.String("address", c.coordinatorAddr),
	)

	// Configure gRPC connection with keepalive
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // Send keepalive ping every 10s
			Timeout:             3 * time.Second,  // Wait 3s for ping ack
			PermitWithoutStream: true,             // Send pings even without active streams
		}),
	}

	// Establish connection with timeout
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, c.coordinatorAddr, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.client = pb.NewCoordinatorServiceClient(conn)
	c.connected = true
	c.mu.Unlock()

	c.logger.Info("Connected to coordinator successfully")

	return nil
}

// Register sends worker registration to the coordinator
func (c *CoordinatorClient) Register(ctx context.Context) error {
	c.logger.Info("Registering worker with coordinator",
		zap.String("worker_id", c.workerID),
		zap.Int("max_concurrent", c.maxConcurrentTasks),
	)

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	// Prepare registration request
	req := &pb.RegisterWorkerRequest{
		WorkerId:           c.workerID,
		MaxConcurrentTasks: int32(c.maxConcurrentTasks),
		Capabilities: map[string]string{
			"circuits": "merkle,range,addition", // Example capabilities
		},
	}

	// Send registration with timeout
	regCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.RegisterWorker(regCtx, req)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("registration rejected: %s", resp.Message)
	}

	c.logger.Info("Worker registered successfully",
		zap.String("message", resp.Message),
	)

	// Start receiving tasks after successful registration
	go c.receiveTasksLoop(ctx)

	return nil
}

// SendHeartbeat sends a heartbeat to the coordinator
// This should be called every 5 seconds to maintain worker health status
func (c *CoordinatorClient) SendHeartbeat(ctx context.Context) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	req := &pb.HeartbeatRequest{
		WorkerId:  c.workerID,
		Timestamp: time.Now().Unix(),
	}

	resp, err := client.SendHeartbeat(ctx, req)
	if err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("heartbeat rejected")
	}

	c.logger.Debug("Heartbeat sent successfully")

	return nil
}

// ReportCompletion sends a successful task result to the coordinator
func (c *CoordinatorClient) ReportCompletion(
	ctx context.Context,
	result executor.TaskResult,
) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	// Marshal ProofMetadata to JSON
	metadataJSON, err := json.Marshal(result.ProofMetadata)
	if err != nil {
		return fmt.Errorf("failed to marshal proof metadata: %w", err)
	}

	req := &pb.TaskCompletionRequest{
		TaskId:            result.TaskID,
		WorkerId:          c.workerID,
		ProofData:         result.ProofData,
		ProofMetadataJson: metadataJSON,
		MerkleRoot:        result.MerkleRoot,
		DurationMs:        result.Duration.Milliseconds(),
	}

	resp, err := client.ReportCompletion(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to report completion: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("completion report rejected: %s", resp.Message)
	}

	c.logger.Debug("Task completion reported",
		zap.String("task_id", result.TaskID),
	)

	return nil
}

// ReportFailure sends a task failure notification to the coordinator
func (c *CoordinatorClient) ReportFailure(
	ctx context.Context,
	taskID string,
	errorMsg string,
) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	req := &pb.TaskFailureRequest{
		TaskId:       taskID,
		WorkerId:     c.workerID,
		ErrorMessage: errorMsg,
	}

	resp, err := client.ReportFailure(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to report failure: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("failure report rejected: %s", resp.Message)
	}

	c.logger.Debug("Task failure reported",
		zap.String("task_id", taskID),
	)

	return nil
}

// receiveTasksLoop maintains a streaming connection to receive task assignments
// This runs in a goroutine and pushes received tasks to the task channel
func (c *CoordinatorClient) receiveTasksLoop(ctx context.Context) {
	c.logger.Info("Starting task receive loop")

	// Create cancellable context for this stream
	streamCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.taskStreamCancel = cancel
	c.mu.Unlock()

	for {
		select {
		case <-streamCtx.Done():
			c.logger.Info("Task receive loop terminated")
			return
		default:
			// Attempt to establish stream
			if err := c.receiveTasksStream(streamCtx); err != nil {
				c.logger.Error("Task stream error, retrying",
					zap.Error(err),
				)

				// Wait before retrying
				select {
				case <-time.After(5 * time.Second):
					continue
				case <-streamCtx.Done():
					return
				}
			}
		}
	}
}

// receiveTasksStream establishes and maintains a single streaming RPC connection
func (c *CoordinatorClient) receiveTasksStream(ctx context.Context) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	// Open streaming RPC
	req := &pb.ReceiveTasksRequest{
		WorkerId: c.workerID,
	}

	stream, err := client.ReceiveTasks(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to open task stream: %w", err)
	}

	c.logger.Info("Task stream established")

	// Receive tasks from stream
	for {
		assignment, err := stream.Recv()
		if err == io.EOF {
			c.logger.Info("Task stream closed by coordinator")
			return nil
		}
		if err != nil {
			return fmt.Errorf("stream receive error: %w", err)
		}

		// Convert protobuf message to Task struct
		task := &Task{
			ID:          assignment.TaskId,
			CircuitType: assignment.CircuitType,
			InputData:   string(assignment.InputDataJson),
			CreatedAt:   time.Unix(assignment.CreatedAt, 0),
		}

		c.logger.Info("Received task assignment",
			zap.String("task_id", task.ID),
			zap.String("circuit_type", task.CircuitType),
		)

		// Send to task channel (non-blocking)
		select {
		case c.taskChan <- task:
			// Task sent successfully
		case <-ctx.Done():
			return ctx.Err()
		default:
			c.logger.Warn("Task channel full, dropping task",
				zap.String("task_id", task.ID),
			)
		}
	}
}

// TaskChannel returns the channel for receiving task assignments
func (c *CoordinatorClient) TaskChannel() <-chan *Task {
	return c.taskChan
}

// IsConnected returns whether the client is connected to coordinator
func (c *CoordinatorClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Disconnect closes the connection to the coordinator
func (c *CoordinatorClient) Disconnect() {
	c.logger.Info("Disconnecting from coordinator")

	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel task stream
	if c.taskStreamCancel != nil {
		c.taskStreamCancel()
	}

	// Close connection
	if c.conn != nil {
		_ = c.conn.Close() // Best effort close
		c.conn = nil
	}

	c.client = nil
	c.connected = false

	// Close task channel
	close(c.taskChan)

	c.logger.Info("Disconnected from coordinator")
}
