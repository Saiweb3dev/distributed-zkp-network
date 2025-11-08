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
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/cache"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
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
// ProofHandler (Phase 2: Task-Based Architecture with Caching)
// ============================================================================

type ProofHandler struct {
	taskRepo   postgres.TaskRepository
	eventBus   *events.EventBus
	cacheLayer *cache.CacheLayer
	logger     *zap.Logger
}

// NewProofHandler creates handler for Phase 2 async task processing with caching
func NewProofHandler(taskRepo postgres.TaskRepository, eventBus *events.EventBus, cacheLayer *cache.CacheLayer, logger *zap.Logger) *ProofHandler {
	return &ProofHandler{
		taskRepo:   taskRepo,
		eventBus:   eventBus,
		cacheLayer: cacheLayer,
		logger:     logger,
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Step 1.5: Check cache FIRST for duplicate request (prevents redundant processing)
	// Check if this exact request was already processed
	if cachedTaskID, found, err := h.cacheLayer.GetByRequestHash(ctx, "merkle_proof", req); found && err == nil {
		// Type assert the cached value
		var taskID string
		switch v := cachedTaskID.(type) {
		case string:
			taskID = v
		default:
			h.logger.Warn("Cached task ID has unexpected type",
				zap.String("type", fmt.Sprintf("%T", v)),
				zap.Any("value", v),
			)
			// If type assertion fails, skip cache and create new task
		}

		if taskID != "" {
			h.logger.Info("Duplicate request detected, returning cached task ID",
				zap.String("task_id", taskID),
				zap.Int("num_leaves", len(req.Leaves)),
			)

			response := TaskCreatedResponse{
				Success: true,
				TaskID:  taskID,
				Status:  "cached",
				Message: "Task already exists. Use GET /api/v1/tasks/{id} to check status",
			}

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
			return
		}
	} else if err != nil {
		h.logger.Warn("Cache lookup error, proceeding with task creation", zap.Error(err))
	}

	// Step 1.6: Acquire distributed lock to prevent race conditions
	cacheKey, _ := h.cacheLayer.GenerateRequestKey("merkle_proof", req)
	lockAcquired, _ := h.cacheLayer.AcquireLock(ctx, cacheKey, 30*time.Second)
	if !lockAcquired {
		// Another gateway is currently processing this request
		// Wait a moment and check cache again (it should be there by now)
		time.Sleep(100 * time.Millisecond)

		if cachedTaskID, found, _ := h.cacheLayer.GetByRequestHash(ctx, "merkle_proof", req); found {
			taskID := cachedTaskID.(string)
			h.logger.Info("Lock contention resolved, returning cached task ID",
				zap.String("task_id", taskID),
			)

			response := TaskCreatedResponse{
				Success: true,
				TaskID:  taskID,
				Status:  "cached",
				Message: "Task already exists. Use GET /api/v1/tasks/{id} to check status",
			}

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
			return
		}

		// If still not found, it's a transient lock issue - proceed with creation
		h.logger.Warn("Lock acquisition failed but no cached result found, proceeding with task creation")
	}
	defer h.cacheLayer.ReleaseLock(ctx, cacheKey)

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

	if err := h.taskRepo.CreateTask(ctx, task); err != nil {
		h.logger.Error("Failed to create task", zap.Error(err))
		h.respondError(w, http.StatusInternalServerError, "Failed to create task", err)
		return
	}

	// Cache task ID by request hash (short TTL for deduplication)
	if err := h.cacheLayer.SetByRequestHash(ctx, "merkle_proof", req, task.ID, 5*time.Minute); err != nil {
		h.logger.Warn("Failed to cache task ID by request hash", zap.Error(err))
	} else {
		h.logger.Debug("Task ID cached by request hash",
			zap.String("task_id", task.ID),
		)
	}

	h.logger.Info("Task created successfully",
		zap.String("task_id", task.ID),
		zap.String("status", task.Status),
	)

	// NEW: Publish event for instant notification to coordinators
	// This triggers immediate scheduling instead of waiting for poll interval
	event := events.Event{
		Type:      events.EventTaskCreated,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"circuit_type": task.CircuitType,
			"status":       task.Status,
		},
	}

	if err := h.eventBus.Publish(ctx, event); err != nil {
		h.logger.Warn("Failed to publish task creation event",
			zap.Error(err),
			zap.String("task_id", task.ID),
		)
		// Don't fail the request - polling fallback still works
		// The coordinator will pick up the task via polling if event fails
	} else {
		h.logger.Debug("Task creation event published",
			zap.String("task_id", task.ID),
			zap.String("event_type", string(events.EventTaskCreated)),
		)
	}

	// Step 3: Return task ID immediately (don't wait for proof generation)
	response := TaskCreatedResponse{
		Success: true,
		TaskID:  task.ID,
		Status:  task.Status,
		Message: "Task created successfully. Use GET /api/v1/tasks/{id} to check status",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted for async processing
	_ = json.NewEncoder(w).Encode(response)
}

// ============================================================================
//Client Polls for Result → API Gateway Returns from Database
// ============================================================================

// GetTaskStatus retrieves the current status and result of a task (with caching)
func (h *ProofHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]

	if taskID == "" {
		h.respondError(w, http.StatusBadRequest, "Task ID is required", nil)
		return
	}

	h.logger.Debug("Fetching task status",
		zap.String("task_id", taskID),
	)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Step 1: Check cache first (only for completed/failed tasks with long TTL)
	if cachedResult, found, _ := h.cacheLayer.GetTaskResult(ctx, taskID); found {
		h.logger.Debug("Returning cached task result",
			zap.String("task_id", taskID),
		)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(cachedResult)
		return
	}

	// Step 2: Cache miss - retrieve task from database
	task, err := h.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		h.logger.Error("Failed to get task", zap.Error(err), zap.String("task_id", taskID))
		h.respondError(w, http.StatusNotFound, "Task not found", err)
		return
	}

	// Step 3: Build response based on task status
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

		// Cache completed task results (long TTL - 30 minutes)
		_ = h.cacheLayer.SetTaskResult(ctx, taskID, response, 30*time.Minute)
		h.logger.Debug("Cached completed task result",
			zap.String("task_id", taskID),
		)
	}

	// If failed, include error message and cache it
	if task.Status == "failed" {
		response.Error = task.ErrorMessage

		// Cache failed task results (medium TTL - 10 minutes)
		_ = h.cacheLayer.SetTaskResult(ctx, taskID, response, 10*time.Minute)
		h.logger.Debug("Cached failed task result",
			zap.String("task_id", taskID),
		)
	}

	// Pending/processing tasks are NOT cached (always fresh from DB)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
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
	_ = json.NewEncoder(w).Encode(response)
}

func (h *ProofHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
		"mode":   "async-task-processing",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
