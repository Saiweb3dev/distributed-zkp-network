// Implements database operations for task management
// This is the single source of truth for task state

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	// "github.com/lib/pq"
)

// Task represents a proof generation request in the database
type Task struct {
	ID            string
	CircuitType   string
	InputData     map[string]interface{}
	Status        string
	ErrorMessage  string
	WorkerID      string
	AssignedAt    time.Time
	CreatedAt     time.Time
	StartedAt     time.Time
	CompletedAt   time.Time
	ProofData     []byte
	ProofMetadata map[string]interface{}
	MerkleRoot    string
	RetryCount    int
	MaxRetries    int
}

// TaskRepository defines operations for task persistence
type TaskRepository interface {
	CreateTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, taskID string) (*Task, error)
	GetPendingTasks(ctx context.Context, limit int) ([]*Task, error)
	GetStaleTasks(ctx context.Context, staleThreshold time.Duration) ([]*Task, error)
	AssignTask(ctx context.Context, taskID, workerID string) error
	StartTask(ctx context.Context, taskID, workerID string) error
	UnassignTask(ctx context.Context, taskID string) error
	UpdateTaskStatus(ctx context.Context, taskID, status string) error
	CompleteTask(ctx context.Context, taskID string, proofData []byte, metadata map[string]interface{}, merkleRoot string) error
	FailTask(ctx context.Context, taskID, errorMessage string) error
	ResetTaskToPending(ctx context.Context, taskID string) error
}

// taskRepository is the PostgreSQL implementation
type taskRepository struct {
	db *sql.DB
}

// DatabaseConfig holds connection pool configuration
type DatabaseConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// NewTaskRepository creates a repository instance
func NewTaskRepository(db *sql.DB) TaskRepository {
	return &taskRepository{db: db}
}

// CreateTask inserts a new task into the database
func (r *taskRepository) CreateTask(ctx context.Context, task *Task) error {
	query := `
		INSERT INTO tasks (
			id, circuit_type, input_data, status, retry_count, max_retries, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	log.Printf("Task before marshal Body: %+v", task)
	// Marshal input_data to JSON
	inputJSON, err := json.Marshal(task.InputData)
	if err != nil {
		return fmt.Errorf("failed to marshal input_data: %w", err)
	}

	log.Printf("Task after marshal Body: %+v", string(inputJSON))

	// Generate UUID if not provided
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	task.CreatedAt = time.Now()
	task.Status = "pending"

	_, err = r.db.ExecContext(
		ctx,
		query,
		task.ID,
		task.CircuitType,
		inputJSON,
		task.Status,
		task.RetryCount,
		task.MaxRetries,
		task.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert task: %w", err)
	}

	return nil
}

// GetTask retrieves a specific task by ID
func (r *taskRepository) GetTask(ctx context.Context, taskID string) (*Task, error) {
	query := `
		SELECT 
			id, circuit_type, input_data, status, error_message,
			worker_id, assigned_at, created_at, started_at, completed_at,
			proof_data, proof_metadata, merkle_root, retry_count, max_retries
		FROM tasks
		WHERE id = $1
	`

	task := &Task{}
	var inputJSON, metadataJSON []byte
	var workerID, errorMessage, merkleRoot sql.NullString
	var assignedAt, startedAt, completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, taskID).Scan(
		&task.ID,
		&task.CircuitType,
		&inputJSON,
		&task.Status,
		&errorMessage,
		&workerID,
		&assignedAt,
		&task.CreatedAt,
		&startedAt,
		&completedAt,
		&task.ProofData,
		&metadataJSON,
		&merkleRoot,
		&task.RetryCount,
		&task.MaxRetries,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query task: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(inputJSON, &task.InputData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input_data: %w", err)
	}

	if metadataJSON != nil {
		if err := json.Unmarshal(metadataJSON, &task.ProofMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proof_metadata: %w", err)
		}
	}

	// Handle nullable fields
	if workerID.Valid {
		task.WorkerID = workerID.String
	}
	if errorMessage.Valid {
		task.ErrorMessage = errorMessage.String
	}
	if merkleRoot.Valid {
		task.MerkleRoot = merkleRoot.String
	}
	if assignedAt.Valid {
		task.AssignedAt = assignedAt.Time
	}
	if startedAt.Valid {
		task.StartedAt = startedAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = completedAt.Time
	}

	return task, nil
}

// GetPendingTasks retrieves tasks waiting for assignment
func (r *taskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*Task, error) {
	query := `
		SELECT id, circuit_type, input_data, created_at
		FROM tasks
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]*Task, 0)

	for rows.Next() {
		task := &Task{}
		var inputJSON []byte

		if err := rows.Scan(&task.ID, &task.CircuitType, &inputJSON, &task.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan task row: %w", err)
		}

		if err := json.Unmarshal(inputJSON, &task.InputData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input_data: %w", err)
		}

		task.Status = "pending"
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetStaleTasks retrieves tasks that have been in-progress too long
func (r *taskRepository) GetStaleTasks(ctx context.Context, staleThreshold time.Duration) ([]*Task, error) {
	query := `
		SELECT id, worker_id, started_at
		FROM tasks
		WHERE status = 'in_progress'
		AND started_at < $1
		ORDER BY started_at ASC
	`

	cutoff := time.Now().Add(-staleThreshold)

	rows, err := r.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]*Task, 0)

	for rows.Next() {
		task := &Task{}
		if err := rows.Scan(&task.ID, &task.WorkerID, &task.StartedAt); err != nil {
			return nil, fmt.Errorf("failed to scan stale task: %w", err)
		}

		task.Status = "in_progress"
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// AssignTask atomically assigns a task to a worker using the database function
func (r *taskRepository) AssignTask(ctx context.Context, taskID, workerID string) error {
	query := `SELECT assign_task_to_worker($1, $2)`

	var success bool
	err := r.db.QueryRowContext(ctx, query, taskID, workerID).Scan(&success)
	if err != nil {
		return fmt.Errorf("failed to assign task: %w", err)
	}

	if !success {
		return fmt.Errorf("task assignment failed (task may not be pending)")
	}

	return nil
}

// StartTask transitions a task from assigned to in_progress
func (r *taskRepository) StartTask(ctx context.Context, taskID, workerID string) error {
	query := `SELECT start_task($1, $2)`

	var success bool
	err := r.db.QueryRowContext(ctx, query, taskID, workerID).Scan(&success)
	if err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	if !success {
		return fmt.Errorf("task start failed (task may not be assigned to this worker)")
	}

	return nil
}

// UnassignTask resets a task to pending status
func (r *taskRepository) UnassignTask(ctx context.Context, taskID string) error {
	query := `
		UPDATE tasks
		SET status = 'pending', worker_id = NULL, assigned_at = NULL
		WHERE id = $1 AND status = 'assigned'
	`

	result, err := r.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("failed to unassign task: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found or not in assigned status")
	}

	return nil
}

// UpdateTaskStatus changes a task's status
func (r *taskRepository) UpdateTaskStatus(ctx context.Context, taskID, status string) error {
	query := `UPDATE tasks SET status = $1 WHERE id = $2`

	_, err := r.db.ExecContext(ctx, query, status, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// CompleteTask marks a task as completed using the database function
func (r *taskRepository) CompleteTask(
	ctx context.Context,
	taskID string,
	proofData []byte,
	metadata map[string]interface{},
	merkleRoot string,
) error {
	query := `SELECT complete_task($1, $2, $3, $4)`

	// Marshal metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var success bool
	err = r.db.QueryRowContext(
		ctx,
		query,
		taskID,
		proofData,
		metadataJSON,
		sql.NullString{String: merkleRoot, Valid: merkleRoot != ""},
	).Scan(&success)

	if err != nil {
		return fmt.Errorf("failed to complete task: %w", err)
	}

	if !success {
		return fmt.Errorf("task completion failed (task may not be in_progress)")
	}

	return nil
}

// FailTask marks a task as failed using the database function
func (r *taskRepository) FailTask(ctx context.Context, taskID, errorMessage string) error {
	query := `SELECT fail_task($1, $2)`

	var success bool
	err := r.db.QueryRowContext(ctx, query, taskID, errorMessage).Scan(&success)
	if err != nil {
		return fmt.Errorf("failed to mark task as failed: %w", err)
	}

	if !success {
		return fmt.Errorf("task failure marking failed")
	}

	return nil
}

// ResetTaskToPending resets a stale task back to pending
func (r *taskRepository) ResetTaskToPending(ctx context.Context, taskID string) error {
	query := `
		UPDATE tasks
		SET 
			status = 'pending',
			worker_id = NULL,
			assigned_at = NULL,
			started_at = NULL,
			retry_count = retry_count + 1
		WHERE id = $1 AND status = 'in_progress'
	`

	result, err := r.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("failed to reset task: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found or not in_progress")
	}

	return nil
}

// ============================================================================
// Worker Repository
// ============================================================================

// WorkerRepository defines operations for worker persistence
type WorkerRepository interface {
	CreateWorker(ctx context.Context, worker *WorkerInfo) error
	GetWorker(ctx context.Context, workerID string) (*WorkerInfo, error)
	GetActiveWorkers(ctx context.Context) ([]*WorkerInfo, error)
	UpdateHeartbeat(ctx context.Context, workerID string) error
	IncrementTaskCount(ctx context.Context, workerID string) error
	DecrementTaskCount(ctx context.Context, workerID string) error
	MarkWorkerDead(ctx context.Context, workerID string) error
}

// WorkerInfo matches the registry WorkerInfo structure
type WorkerInfo struct {
	ID                 string
	Address            string
	MaxConcurrentTasks int
	CurrentTaskCount   int
	Status             string
	LastHeartbeatAt    time.Time
	RegisteredAt       time.Time
	Capabilities       map[string]interface{}
}

// workerRepository is the PostgreSQL implementation
type workerRepository struct {
	db *sql.DB
}

// NewWorkerRepository creates a repository instance
func NewWorkerRepository(db *sql.DB) WorkerRepository {
	return &workerRepository{db: db}
}

// CreateWorker inserts a new worker registration
func (r *workerRepository) CreateWorker(ctx context.Context, worker *WorkerInfo) error {
	query := `
		INSERT INTO workers (
			id, address, max_concurrent_tasks, status, last_heartbeat_at, registered_at, capabilities
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE
		SET 
			address = EXCLUDED.address,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			status = 'active'
	`

	capJSON, err := json.Marshal(worker.Capabilities)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	_, err = r.db.ExecContext(
		ctx,
		query,
		worker.ID,
		worker.Address,
		worker.MaxConcurrentTasks,
		"active",
		time.Now(),
		time.Now(),
		capJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to insert worker: %w", err)
	}

	return nil
}

// GetWorker retrieves a specific worker by ID
func (r *workerRepository) GetWorker(ctx context.Context, workerID string) (*WorkerInfo, error) {
	query := `
		SELECT 
			id, address, max_concurrent_tasks, current_task_count,
			status, last_heartbeat_at, registered_at, capabilities
		FROM workers
		WHERE id = $1
	`

	worker := &WorkerInfo{}
	var capJSON []byte

	err := r.db.QueryRowContext(ctx, query, workerID).Scan(
		&worker.ID,
		&worker.Address,
		&worker.MaxConcurrentTasks,
		&worker.CurrentTaskCount,
		&worker.Status,
		&worker.LastHeartbeatAt,
		&worker.RegisteredAt,
		&capJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("worker not found: %s", workerID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query worker: %w", err)
	}

	if capJSON != nil {
		if err := json.Unmarshal(capJSON, &worker.Capabilities); err != nil {
			return nil, fmt.Errorf("failed to unmarshal capabilities: %w", err)
		}
	}

	return worker, nil
}

// GetActiveWorkers retrieves all active workers
func (r *workerRepository) GetActiveWorkers(ctx context.Context) ([]*WorkerInfo, error) {
	query := `
		SELECT 
			id, address, max_concurrent_tasks, current_task_count,
			status, last_heartbeat_at, registered_at
		FROM workers
		WHERE status = 'active'
		ORDER BY registered_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active workers: %w", err)
	}
	defer rows.Close()

	workers := make([]*WorkerInfo, 0)

	for rows.Next() {
		worker := &WorkerInfo{}
		if err := rows.Scan(
			&worker.ID,
			&worker.Address,
			&worker.MaxConcurrentTasks,
			&worker.CurrentTaskCount,
			&worker.Status,
			&worker.LastHeartbeatAt,
			&worker.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan worker row: %w", err)
		}

		workers = append(workers, worker)
	}

	return workers, nil
}

// UpdateHeartbeat records a heartbeat from a worker
func (r *workerRepository) UpdateHeartbeat(ctx context.Context, workerID string) error {
	query := `
		UPDATE workers
		SET last_heartbeat_at = $1
		WHERE id = $2
	`

	_, err := r.db.ExecContext(ctx, query, time.Now(), workerID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	return nil
}

// IncrementTaskCount increases a worker's task count
func (r *workerRepository) IncrementTaskCount(ctx context.Context, workerID string) error {
	query := `
		UPDATE workers
		SET current_task_count = current_task_count + 1
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, workerID)
	if err != nil {
		return fmt.Errorf("failed to increment task count: %w", err)
	}

	return nil
}

// DecrementTaskCount decreases a worker's task count
func (r *workerRepository) DecrementTaskCount(ctx context.Context, workerID string) error {
	query := `
		UPDATE workers
		SET current_task_count = GREATEST(current_task_count - 1, 0)
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, workerID)
	if err != nil {
		return fmt.Errorf("failed to decrement task count: %w", err)
	}

	return nil
}

// MarkWorkerDead marks a worker as dead
func (r *workerRepository) MarkWorkerDead(ctx context.Context, workerID string) error {
	query := `
		UPDATE workers
		SET status = 'dead'
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, workerID)
	if err != nil {
		return fmt.Errorf("failed to mark worker dead: %w", err)
	}

	return nil
}

// ============================================================================
// Database Connection Helper
// ============================================================================

// ConnectPostgreSQL establishes a connection to PostgreSQL
func ConnectPostgreSQL(connString string, cfg *DatabaseConfig) (*sql.DB, error) {
	log.Printf("Test Log Connecting to PostgreSQL: %s", connString)
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool (do it once here, not in main)
	if cfg != nil {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
		db.SetMaxIdleConns(cfg.MaxIdleConns)
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		// Sensible defaults if no config provided
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// TaskResult holds the results of a completed task
type TaskResult struct {
	ProofData     []byte
	ProofMetadata string
	MerkleRoot    string
	DurationMs    int64
}

// RunMigrations executes SQL migration files
func RunMigrations(db *sql.DB, migrationsPath string) error {
	// TODO: Implement migration runner
	// In production, use a library like golang-migrate or pressly/goose
	return nil
}
