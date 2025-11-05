// Implements bounded concurrency pattern for proof generation
// Each worker node runs one pool with multiple goroutines

package executor

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/frontend"
	"github.com/saiweb3dev/distributed-zkp-network/internal/zkp"
	"github.com/saiweb3dev/distributed-zkp-network/internal/zkp/circuits"
	"go.uber.org/zap"
)

// Task represents a proof generation request with all necessary inputs
// This structure is created from the database task r
type Task struct {
	ID          string
	CircuitType string
	InputData   map[string]interface{}
	CreatedAt   time.Time
}

// TaskResult contains the outcome of proof generation
// Success indicates whether the proof was generated without errors
// The proof data and metadata are populated only on success
type TaskResult struct {
	TaskID        string
	Success       bool
	ProofData     []byte
	ProofMetadata map[string]interface{}
	MerkleRoot    string
	Error         error
	Duration      time.Duration
}

// WorkerPool manages a fixed number of goroutines that process tasks concurrently
// The pool prevents resource exhaustion by limiting concurrency to available CPU cores
type WorkerPool struct {
	prover         zkp.Prover
	circuitFactory *circuits.CircuitFactory
	concurrency    int
	logger         *zap.Logger

	tasks   chan Task
	results chan TaskResult

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	activeWorkers int
	mu            sync.Mutex
}

// NewWorkerPool creates a pool with the specified concurrency level
// The concurrency parameter determines how many proofs can generate simultaneously
// A good default is the number of CPU cores minus one for other system operations
func NewWorkerPool(prover zkp.Prover, concurrency int, logger *zap.Logger) *WorkerPool {
	if concurrency <= 0 {
		concurrency = 1
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		prover:         prover,
		circuitFactory: circuits.NewCircuitFactory(),
		concurrency:    concurrency,
		logger:         logger,

		tasks:   make(chan Task, concurrency*2), // Buffer prevents blocking
		results: make(chan TaskResult, concurrency*2),

		ctx:    ctx,
		cancel: cancel,
	}
}

// Start launches the worker goroutines and begins processing tasks
// This method returns immediately after starting all workers
// Call Stop to gracefully shutdown the pool and wait for completion
func (wp *WorkerPool) Start() {
	wp.logger.Info("Starting worker pool",
		zap.Int("concurrency", wp.concurrency),
	)

	for i := 0; i < wp.concurrency; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// Stop initiates graceful shutdown of the worker pool
// It closes the task channel to prevent new submissions, waits for all active
// tasks to complete, and then closes the results channel to signal completion
func (wp *WorkerPool) Stop() {
	wp.logger.Info("Stopping worker pool, waiting for tasks to complete")

	close(wp.tasks)   // Signal no more tasks coming
	wp.wg.Wait()      // Wait for all workers to finish
	close(wp.results) // Signal all results sent
	wp.cancel()       // Cancel context for any blocking operations

	wp.logger.Info("Worker pool stopped")
}

// Submit adds a task to the processing queue
// This method blocks if the task channel buffer is full
// Use context with timeout to prevent indefinite blocking
func (wp *WorkerPool) Submit(ctx context.Context, task Task) error {
	select {
	case wp.tasks <- task:
		wp.logger.Debug("Task submitted to pool",
			zap.String("task_id", task.ID),
			zap.String("circuit_type", task.CircuitType),
		)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timeout submitting task: %w", ctx.Err())
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is shutting down")
	}
}

// Results returns the channel on which completed task results are sent
// Consumers should drain this channel continuously to prevent blocking workers
// The channel is closed when all workers have finished after Stop is called
func (wp *WorkerPool) Results() <-chan TaskResult {
	return wp.results
}

// worker is the main processing loop for each goroutine in the pool
// It continuously receives tasks from the channel, generates proofs, and sends results
// The loop terminates when the tasks channel is closed and drained
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	wp.logger.Info("Worker started",
		zap.Int("worker_id", id),
	)

	for task := range wp.tasks {
		wp.mu.Lock()
		wp.activeWorkers++
		wp.mu.Unlock()

		wp.logger.Info("Worker processing task",
			zap.Int("worker_id", id),
			zap.String("task_id", task.ID),
			zap.String("circuit_type", task.CircuitType),
		)

		result := wp.processTask(task)

		wp.mu.Lock()
		wp.activeWorkers--
		wp.mu.Unlock()

		// Send result with timeout to prevent blocking
		ctx, cancel := context.WithTimeout(wp.ctx, 5*time.Second)
		select {
		case wp.results <- result:
			wp.logger.Info("Task completed",
				zap.Int("worker_id", id),
				zap.String("task_id", result.TaskID),
				zap.Bool("success", result.Success),
				zap.Duration("duration", result.Duration),
			)
		case <-ctx.Done():
			wp.logger.Error("Failed to send result, channel blocked",
				zap.String("task_id", result.TaskID),
			)
		}
		cancel()
	}

	wp.logger.Info("Worker stopped",
		zap.Int("worker_id", id),
	)
}

// processTask executes the actual proof generation for a single task
// This method contains the core integration between the distributed system
// and the Phase 1 proof generation logic that remains unchanged
func (wp *WorkerPool) processTask(task Task) TaskResult {
	startTime := time.Now()

	result := TaskResult{
		TaskID: task.ID,
	}

	// Parse circuit type and create appropriate circuit instance
	circuit, witness, err := wp.parseTaskInput(task)
	if err != nil {
		result.Error = fmt.Errorf("invalid task input: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Generate proof using Phase 1 prover (unchanged)
	proof, err := wp.prover.GenerateProof(circuit, witness)
	if err != nil {
		result.Error = fmt.Errorf("proof generation failed: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Populate successful result
	result.Success = true
	result.ProofData = proof.ProofData
	result.ProofMetadata = proof.PublicInputs
	result.Duration = time.Since(startTime)

	// Extract Merkle root if this was a Merkle proof
	if root, ok := task.InputData["merkle_root"].(string); ok {
		result.MerkleRoot = root
	}

	return result
}

// parseTaskInput converts the database task record into circuit and witness instances
// This method bridges between the JSON representation in PostgreSQL and the
// Go structures required by the proof generation system
func (wp *WorkerPool) parseTaskInput(task Task) (circuit frontend.Circuit, witness frontend.Circuit, err error) {
	switch task.CircuitType {
	case "merkle_proof":
		return wp.parseMerkleProofTask(task)
	default:
		return nil, nil, fmt.Errorf("unknown circuit type: %s", task.CircuitType)
	}
}

// parseMerkleProofTask extracts Merkle proof parameters from task input data
// The input data structure must match what the API Gateway stored in PostgreSQL
// This creates both the circuit template for compilation and the witness for proving
func (wp *WorkerPool) parseMerkleProofTask(task Task) (circuit frontend.Circuit, witness frontend.Circuit, err error) {
	// Extract required fields from input_data JSON
	leaves, ok := task.InputData["leaves"].([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("missing or invalid 'leaves' field")
	}

	leafIndex, ok := task.InputData["leaf_index"].(float64) // JSON numbers are float64
	if !ok {
		return nil, nil, fmt.Errorf("missing or invalid 'leaf_index' field")
	}

	// Convert leaf strings to bytes (same as handler)
	leafBytes := make([][]byte, len(leaves))
	for i, leaf := range leaves {
		leafStr, ok := leaf.(string)
		if !ok {
			return nil, nil, fmt.Errorf("invalid leaf at index %d", i)
		}

		// Remove 0x prefix if present
		if len(leafStr) > 2 && leafStr[:2] == "0x" {
			leafStr = leafStr[2:]
		}

		// Decode hex string to bytes
		leafBytes[i], err = hex.DecodeString(leafStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid hex at leaf %d: %w", i, err)
		}
	}

	// Build Merkle tree (using Phase 1 code)
	// Need to pass ecc.ID for proper field element handling
	tree := zkp.NewMerkleTree(leafBytes, ecc.BN254)
	if tree == nil {
		return nil, nil, fmt.Errorf("failed to build Merkle tree")
	}

	// Generate Merkle proof path
	path, indices, err := tree.GenerateProof(int(leafIndex))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate Merkle proof: %w", err)
	}

	// Create circuit template (for compilation) - use factory method
	circuitTemplate := wp.circuitFactory.MerkleProof(tree.Depth)

	// Create witness with actual field element values (same pattern as handler)
	witnessCircuit := wp.createMerkleWitness(
		tree.Depth,
		leafBytes[int(leafIndex)],
		path,
		indices,
		tree.Root,
	)

	return circuitTemplate, witnessCircuit, nil
}

// createMerkleWitness populates a circuit instance with field element values
// This method performs the same field conversions as the Phase 1 handler
func (wp *WorkerPool) createMerkleWitness(
	depth int,
	leaf []byte,
	path [][]byte,
	indices []int,
	root []byte,
) *circuits.MerkleProofCircuitInputs {
	// Convert bytes to field elements using zkp.BytesToFieldElement (same as handler)
	leafField := zkp.BytesToFieldElement(leaf)
	rootField := zkp.BytesToFieldElement(root)

	// Convert path to field elements - MUST be []frontend.Variable, not []interface{}!
	pathFields := make([]frontend.Variable, len(path))
	for i, p := range path {
		pathFields[i] = zkp.BytesToFieldElement(p)
	}

	// Convert indices to field elements
	indicesFields := make([]frontend.Variable, len(indices))
	for i, idx := range indices {
		indicesFields[i] = idx
	}

	return &circuits.MerkleProofCircuitInputs{
		Leaf:        leafField,
		Path:        pathFields,
		PathIndices: indicesFields,
		Root:        rootField,
		Depth:       depth,
	}
}

// GetStats returns current pool statistics for monitoring
// These metrics help track pool utilization and identify bottlenecks
func (wp *WorkerPool) GetStats() PoolStats {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	return PoolStats{
		Concurrency:    wp.concurrency,
		ActiveWorkers:  wp.activeWorkers,
		QueuedTasks:    len(wp.tasks),
		PendingResults: len(wp.results),
	}
}

// PoolStats contains current worker pool metrics
type PoolStats struct {
	Concurrency    int
	ActiveWorkers  int
	QueuedTasks    int
	PendingResults int
}
