package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/cache"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
	"github.com/saiweb3dev/distributed-zkp-network/internal/storage/postgres"
	"go.uber.org/zap"
)

// ============================================================================
// HTTP Request/Response Models
// ============================================================================

type MerkleProofRequest struct {
	Leaves    []string `json:"leaves"`
	LeafIndex int      `json:"leaf_index"`
}

type TaskCreatedResponse struct {
	Success   bool   `json:"success"`
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type TaskStatusResponse struct {
	Success      bool                   `json:"success"`
	TaskID       string                 `json:"task_id"`
	Status       string                 `json:"status"`
	Proof        string                 `json:"proof,omitempty"`
	MerkleRoot   string                 `json:"merkle_root,omitempty"`
	PublicInputs map[string]interface{} `json:"public_inputs,omitempty"`
	CircuitType  string                 `json:"circuit_type,omitempty"`
	Error        string                 `json:"error,omitempty"`
	CreatedAt    string                 `json:"created_at"`
	CompletedAt  string                 `json:"completed_at,omitempty"`
	RequestID    string                 `json:"request_id,omitempty"`
}

type ErrorResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// ============================================================================
// ProofHandler - REFACTORED
// ============================================================================

type ProofHandler struct {
	taskRepo   postgres.TaskRepository
	eventBus   *events.EventBus
	cacheLayer *cache.CacheLayer
	logger     *zap.Logger
}

func NewProofHandler(
	taskRepo postgres.TaskRepository,
	eventBus *events.EventBus,
	cacheLayer *cache.CacheLayer,
	logger *zap.Logger,
) *ProofHandler {
	return &ProofHandler{
		taskRepo:   taskRepo,
		eventBus:   eventBus,
		cacheLayer: cacheLayer,
		logger:     logger,
	}
}

func (h *ProofHandler) SubmitMerkleProofTask(w http.ResponseWriter, r *http.Request) {
	// Extract or generate request ID for tracing
	requestID := h.getRequestID(r)
	ctx := context.WithValue(r.Context(), "request_id", requestID)

	// Step 1: Parse and validate request
	var req MerkleProofRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body", err, requestID)
		return
	}

	if err := h.validateMerkleRequest(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request", err, requestID)
		return
	}

	h.logger.Info("Received Merkle proof task submission",
		zap.Int("num_leaves", len(req.Leaves)),
		zap.Int("leaf_index", req.LeafIndex),
		zap.String("request_id", requestID),
	)

	// Step 2: Create context with realistic timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second) // FIXED: Increased from 10s
	defer cancel()

	// Step 3: Check for duplicate task using idempotency
	taskID, isDuplicate, err := h.checkDuplicateTask(ctx, req, requestID)
	if err != nil {
		h.respondError(w, http.StatusServiceUnavailable, "Failed to check duplicate", err, requestID)
		return
	}

	if isDuplicate {
		h.logger.Info("Returning existing task (deduplicated)",
			zap.String("task_id", taskID),
			zap.String("request_id", requestID),
		)

		response := TaskCreatedResponse{
			Success:   true,
			TaskID:    taskID,
			Status:    "cached",
			Message:   "Task already exists. Use GET /api/v1/tasks/{id} to check status",
			RequestID: requestID,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("X-Request-ID", requestID)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	// Step 4: Create new task
	task, err := h.createNewTask(ctx, req, requestID)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to create task", err, requestID)
		return
	}

	// Step 5: Publish event (non-blocking, failure is OK)
	go h.publishTaskCreatedEvent(context.Background(), task, requestID)

	// Step 6: Return success response
	response := TaskCreatedResponse{
		Success:   true,
		TaskID:    task.ID,
		Status:    task.Status,
		Message:   "Task created successfully. Use GET /api/v1/tasks/{id} to check status",
		RequestID: requestID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *ProofHandler) checkDuplicateTask(
	ctx context.Context,
	req MerkleProofRequest,
	requestID string,
) (taskID string, isDuplicate bool, err error) {

	// Generate deterministic cache key
	cacheKey, err := h.cacheLayer.GenerateRequestKey("merkle_proof", req)
	if err != nil {
		return "", false, fmt.Errorf("failed to generate cache key: %w", err)
	}

	h.logger.Debug("Checking for duplicate task",
		zap.String("cache_key", cacheKey),
		zap.String("request_id", requestID),
	)

	lockAcquired, err := h.cacheLayer.AcquireLock(ctx, cacheKey, 30*time.Second)
	if err != nil {
		h.logger.Error("Lock acquisition error",
			zap.Error(err),
			zap.String("request_id", requestID),
		)
		return "", false, fmt.Errorf("lock service error: %w", err)
	}

	if lockAcquired {
		defer func() {
			if releaseErr := h.cacheLayer.ReleaseLock(context.Background(), cacheKey); releaseErr != nil {
				h.logger.Error("Failed to release lock",
					zap.Error(releaseErr),
					zap.String("cache_key", cacheKey),
					zap.String("request_id", requestID),
				)
			}
		}()

		cachedTaskID, found, err := h.cacheLayer.GetByRequestHash(ctx, "merkle_proof", req)
		if err != nil {
			h.logger.Warn("Cache read error after lock acquisition",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			// Continue to create task (cache failure shouldn't block)
			return "", false, nil
		}

		if found {
			// Another instance created it before we got the lock
			if id, ok := cachedTaskID.(string); ok {
				if _, parseErr := uuid.Parse(id); parseErr != nil {
					h.logger.Error("Invalid UUID in cache",
						zap.String("id", id),
						zap.String("request_id", requestID),
					)
					// Clear corrupted cache and proceed with creation
					_ = h.cacheLayer.InvalidateTaskResult(ctx, id)
					return "", false, nil
				}
				return id, true, nil
			}
		}

		// No cached result - we should create the task
		return "", false, nil
	}

	// Lock not acquired - another request is creating the task
	// Wait and retry with exponential backoff
	h.logger.Debug("Lock held by another request, entering retry loop",
		zap.String("cache_key", cacheKey),
		zap.String("request_id", requestID),
	)

	for attempt := 0; attempt < MaxLockRetries; attempt++ {
		// Exponential backoff: 50ms, 100ms, 200ms
		retryDelay := InitialRetryDelay << attempt

		select {
		case <-ctx.Done():
			h.logger.Warn("Context cancelled during lock retry",
				zap.String("request_id", requestID),
			)
			return "", false, ctx.Err()

		case <-time.After(retryDelay):
			// Check if task was created by another instance
			cachedTaskID, found, err := h.cacheLayer.GetByRequestHash(ctx, "merkle_proof", req)
			if err != nil {
				h.logger.Warn("Cache error during retry",
					zap.Error(err),
					zap.Int("attempt", attempt+1),
					zap.String("request_id", requestID),
				)
				continue
			}

			if found {
				if id, ok := cachedTaskID.(string); ok && id != "" {
					if _, parseErr := uuid.Parse(id); parseErr != nil {
						h.logger.Error("Invalid UUID in cache during retry",
							zap.String("id", id),
							zap.String("request_id", requestID),
						)
						continue
					}

					h.logger.Info("Found existing task during retry",
						zap.String("task_id", id),
						zap.Int("attempt", attempt+1),
						zap.String("request_id", requestID),
					)
					return id, true, nil
				}
			}
		}
	}

	h.logger.Warn("Cannot acquire lock after retries",
		zap.String("cache_key", cacheKey),
		zap.String("request_id", requestID),
	)

	return "", false, fmt.Errorf("request is being processed by another instance, please retry")
}

func (h *ProofHandler) createNewTask(
	ctx context.Context,
	req MerkleProofRequest,
	requestID string,
) (*postgres.Task, error) {

	clientIP := h.getClientIP(ctx)
	userAgent := h.getUserAgent(ctx)

	task := &postgres.Task{
		CircuitType: "merkle_proof",
		InputData: map[string]interface{}{
			"leaves":     req.Leaves,
			"leaf_index": req.LeafIndex,
			"client_ip":  clientIP,
			"user_agent": userAgent,
			"request_id": requestID,
			"timestamp":  time.Now().Unix(),
		},
		Status:     "pending",
		MaxRetries: DefaultTaskMaxRetries,
	}

	if err := h.taskRepo.CreateTask(ctx, task); err != nil {
		h.logger.Error("Failed to create task in database",
			zap.Error(err),
			zap.String("request_id", requestID),
		)
		return nil, err
	}

	// Cache task ID for deduplication
	if err := h.cacheLayer.SetByRequestHash(ctx, "merkle_proof", req, task.ID, RequestDedupCacheTTL); err != nil {
		h.logger.Warn("Failed to cache task ID",
			zap.Error(err),
			zap.String("task_id", task.ID),
			zap.String("request_id", requestID),
		)
	}

	h.logger.Info("Task created successfully",
		zap.String("task_id", task.ID),
		zap.String("status", task.Status),
		zap.String("request_id", requestID),
	)

	return task, nil
}

func (h *ProofHandler) publishTaskCreatedEvent(
	ctx context.Context,
	task *postgres.Task,
	requestID string,
) {
	event := events.Event{
		Type:      events.EventTaskCreated,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"circuit_type": task.CircuitType,
			"status":       task.Status,
			"request_id":   requestID,
		},
	}

	// Use background context with timeout (don't block on event publishing)
	publishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.eventBus.Publish(publishCtx, event); err != nil {
		h.logger.Warn("Failed to publish task creation event",
			zap.Error(err),
			zap.String("task_id", task.ID),
			zap.String("request_id", requestID),
		)
		// Event publish failure is non-critical - polling fallback exists
	} else {
		h.logger.Debug("Task creation event published",
			zap.String("task_id", task.ID),
			zap.String("event_type", string(events.EventTaskCreated)),
			zap.String("request_id", requestID),
		)
	}
}

func (h *ProofHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	requestID := h.getRequestID(r)
	vars := mux.Vars(r)
	taskID := vars["id"]

	if taskID == "" {
		h.respondError(w, http.StatusBadRequest, "Task ID is required", nil, requestID)
		return
	}

	if _, err := uuid.Parse(taskID); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid task ID format", err, requestID)
		return
	}

	h.logger.Debug("Fetching task status",
		zap.String("task_id", taskID),
		zap.String("request_id", requestID),
	)

	ctx, cancel := context.WithTimeout(r.Context(), TaskQueryTimeout)
	defer cancel()

	// Check cache first (only for completed/failed tasks)
	if cachedResult, found, _ := h.cacheLayer.GetTaskResult(ctx, taskID); found {
		if result, ok := cachedResult.(TaskStatusResponse); ok {
			result.RequestID = requestID
			h.logger.Debug("Returning cached task result",
				zap.String("task_id", taskID),
				zap.String("request_id", requestID),
			)

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Header().Set("X-Request-ID", requestID)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(result)
			return
		}

		// Cache corruption - invalidate and fetch from DB
		h.logger.Warn("Invalid cached result type, fetching from DB",
			zap.String("task_id", taskID),
			zap.String("request_id", requestID),
		)
		_ = h.cacheLayer.InvalidateTaskResult(ctx, taskID)
	}

	// Cache miss or corruption - fetch from database
	task, err := h.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		h.logger.Error("Failed to get task",
			zap.Error(err),
			zap.String("task_id", taskID),
			zap.String("request_id", requestID),
		)
		h.respondError(w, http.StatusNotFound, "Task not found", err, requestID)
		return
	}

	// Build response
	response := TaskStatusResponse{
		Success:     true,
		TaskID:      task.ID,
		Status:      task.Status,
		CircuitType: task.CircuitType,
		CreatedAt:   task.CreatedAt.Format(time.RFC3339),
		RequestID:   requestID,
	}

	// Add proof data if completed
	if task.Status == "completed" {
		response.Proof = hex.EncodeToString(task.ProofData)
		response.MerkleRoot = task.MerkleRoot
		response.PublicInputs = task.ProofMetadata
		response.CompletedAt = task.CompletedAt.Format(time.RFC3339)

		// Cache completed tasks (long TTL)
		_ = h.cacheLayer.SetTaskResult(ctx, taskID, response, CompletedTaskCacheTTL)
	}

	// Add error if failed
	if task.Status == "failed" {
		response.Error = task.ErrorMessage
		// Cache failed tasks (medium TTL)
		_ = h.cacheLayer.SetTaskResult(ctx, taskID, response, 10*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("X-Request-ID", requestID)
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
	if len(req.Leaves) > 1000 {
		return fmt.Errorf("too many leaves (max 1000, got %d)", len(req.Leaves))
	}
	if req.LeafIndex < 0 || req.LeafIndex >= len(req.Leaves) {
		return fmt.Errorf("invalid leaf_index: must be between 0 and %d", len(req.Leaves)-1)
	}
	return nil
}

func (h *ProofHandler) respondError(
	w http.ResponseWriter,
	statusCode int,
	message string,
	err error,
	requestID string,
) {
	h.logger.Error(message,
		zap.Error(err),
		zap.Int("status_code", statusCode),
		zap.String("request_id", requestID),
	)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Success:   false,
		Error:     message,
		RequestID: requestID,
	}

	_ = json.NewEncoder(w).Encode(response)
}

func (h *ProofHandler) getRequestID(r *http.Request) string {
	// Check if request ID already exists (from middleware)
	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		return reqID
	}
	// Generate new request ID
	return uuid.New().String()
}

func (h *ProofHandler) getClientIP(ctx context.Context) string {
	// Extract from context (set by middleware)
	if ip, ok := ctx.Value("client_ip").(string); ok {
		return ip
	}
	return "unknown"
}

func (h *ProofHandler) getUserAgent(ctx context.Context) string {
	// Extract from context (set by middleware)
	if ua, ok := ctx.Value("user_agent").(string); ok {
		return ua
	}
	return "unknown"
}

func (h *ProofHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "healthy", "service": "api-gateway"}`))
}
