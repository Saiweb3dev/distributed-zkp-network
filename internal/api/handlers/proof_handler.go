package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
)

// ============================================================================
// HTTP Request/Response Models (Phase 2: Async Task Flow)
// ============================================================================

// MerkleProofRequest - Client submits this to create a task
type MerkleProofRequest struct {
	Leaves    []string `json:"leaves"`
	LeafIndex int      `json:"leaf_index"`
}

// TaskCreatedResponse - Returned immediately after task creation
type TaskCreatedResponse struct {
	Success bool   `json:"success"`
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// TaskStatusResponse - Returned when client polls for result
type TaskStatusResponse struct {
	Success      bool                   `json:"success"`
	TaskID       string                 `json:"task_id"`
	Status       string                 `json:"status"` // pending, assigned, in_progress, completed, failed
	Proof        string                 `json:"proof,omitempty"`
	MerkleRoot   string                 `json:"merkle_root,omitempty"`
	PublicInputs map[string]interface{} `json:"public_inputs,omitempty"`
	CircuitType  string                 `json:"circuit_type,omitempty"`
	Error        string                 `json:"error,omitempty"`
	CreatedAt    string                 `json:"created_at"`
	CompletedAt  string                 `json:"completed_at,omitempty"`
}

// ErrorResponse - Standard error response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// ============================================================================
// ProofHandler (Phase 2: Task-Based Architecture)
// ============================================================================

type ProofHandler struct {
	taskRepo postgres.TaskRepository
	logger   *zap.Logger
}

// NewProofHandler creates handler for Phase 2 async task processing
func NewProofHandler(taskRepo postgres.TaskRepository, logger *zap.Logger) *ProofHandler {
	return &ProofHandler{
		taskRepo: taskRepo,
		logger:   logger,
	}
}

// ============================================================================
//Client Submits Request → API Gateway Creates Task
// ============================================================================

// SubmitMerkleProofTask creates a new task in the database (async)
func (h *ProofHandler) SubmitMerkleProofTask(w http.ResponseWriter, r *http.Request) {
	var req MerkleProofRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	log.Printf("Request Body: %+v", req)

	// Validate request
	if err := h.validateMerkleRequest(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request", err)
		return
	}

	h.logger.Info("Received Merkle proof task submission",
		zap.Int("num_leaves", len(req.Leaves)),
		zap.Int("leaf_index", req.LeafIndex),
	)

	// Need more data about the task like maybe client id, priority, etc.
	// Step 2: Create task in database (status: pending)
	task := &postgres.Task{
		CircuitType: "merkle_proof",
		InputData: map[string]interface{}{
			"leaves":     req.Leaves,
			"leaf_index": req.LeafIndex,
		},
		Status:     "pending",
		MaxRetries: 3,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.taskRepo.CreateTask(ctx, task); err != nil {
		h.logger.Error("Failed to create task", zap.Error(err))
		h.respondError(w, http.StatusInternalServerError, "Failed to create task", err)
		return
	}

	h.logger.Info("Task created successfully",
		zap.String("task_id", task.ID),
		zap.String("status", task.Status),
	)

	// Step 3: Return task ID immediately (don't wait for proof generation)
	response := TaskCreatedResponse{
		Success: true,
		TaskID:  task.ID,
		Status:  task.Status,
		Message: "Task created successfully. Use GET /api/v1/tasks/{id} to check status",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted for async processing
	json.NewEncoder(w).Encode(response)
}

// ============================================================================
//Client Polls for Result → API Gateway Returns from Database
// ============================================================================

// GetTaskStatus retrieves the current status and result of a task
func (h *ProofHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]

	if taskID == "" {
		h.respondError(w, http.StatusBadRequest, "Task ID is required", nil)
		return
	}

	h.logger.Info("Fetching task status",
		zap.String("task_id", taskID),
	)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	//Retrieve task from database
	task, err := h.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		h.logger.Error("Failed to get task", zap.Error(err), zap.String("task_id", taskID))
		h.respondError(w, http.StatusNotFound, "Task not found", err)
		return
	}

	// Build response based on task status
	response := TaskStatusResponse{
		Success:     true,
		TaskID:      task.ID,
		Status:      task.Status,
		CircuitType: task.CircuitType,
		CreatedAt:   task.CreatedAt.Format(time.RFC3339),
	}

	// If completed, include proof data
	if task.Status == "completed" {
		response.Proof = hex.EncodeToString(task.ProofData)
		response.MerkleRoot = task.MerkleRoot
		response.PublicInputs = task.ProofMetadata
		response.CompletedAt = task.CompletedAt.Format(time.RFC3339)
	}

	// If failed, include error message
	if task.Status == "failed" {
		response.Error = task.ErrorMessage
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *ProofHandler) validateMerkleRequest(req *MerkleProofRequest) error {
	if len(req.Leaves) == 0 {
		return fmt.Errorf("leaves array cannot be empty")
	}

	if len(req.Leaves) > 1024 {
		return fmt.Errorf("too many leaves (max 1024)")
	}

	if req.LeafIndex < 0 || req.LeafIndex >= len(req.Leaves) {
		return fmt.Errorf("leaf_index out of range")
	}

	return nil
}

func (h *ProofHandler) respondError(w http.ResponseWriter, statusCode int, message string, err error) {
	errorMsg := message
	if err != nil {
		errorMsg = fmt.Sprintf("%s: %v", message, err)
		h.logger.Error(message, zap.Error(err))
	}

	response := ErrorResponse{
		Success: false,
		Error:   errorMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func (h *ProofHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
		"mode":   "async-task-processing",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
