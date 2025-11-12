// Package constants defines coordinator-wide constants for timeouts, intervals, and limits.
// Centralizing these values improves maintainability and makes tuning easier.
package constants

import "time"

// ============================================================================
// gRPC Configuration
// ============================================================================

const (
	// GRPCMaxSendMsgSize is the maximum message size for outbound gRPC messages.
	// Set to 10MB to accommodate large proof data.
	GRPCMaxSendMsgSize = 10 * 1024 * 1024 // 10MB

	// GRPCMaxRecvMsgSize is the maximum message size for inbound gRPC messages.
	GRPCMaxRecvMsgSize = 10 * 1024 * 1024 // 10MB

	// GRPCKeepaliveMaxConnectionIdle is the max time a connection can be idle before closing.
	GRPCKeepaliveMaxConnectionIdle = 15 * time.Minute

	// GRPCKeepaliveMaxConnectionAge is the max lifetime of a connection before forcing reconnect.
	GRPCKeepaliveMaxConnectionAge = 30 * time.Minute

	// GRPCKeepaliveMaxConnectionAgeGrace is the grace period for completing RPCs before forced close.
	GRPCKeepaliveMaxConnectionAgeGrace = 5 * time.Minute

	// GRPCKeepaliveTime is the interval for sending keepalive pings.
	GRPCKeepaliveTime = 5 * time.Minute

	// GRPCKeepaliveTimeout is how long to wait for ping acknowledgment.
	GRPCKeepaliveTimeout = 20 * time.Second
)

// ============================================================================
// Task Stream Configuration
// ============================================================================

const (
	// TaskStreamBufferSize is the channel buffer size for task assignment streams.
	// Prevents blocking when pushing tasks to workers.
	TaskStreamBufferSize = 10

	// TaskPushTimeout is the max time to wait when pushing a task to a worker stream.
	// Prevents indefinite blocking if worker is slow to consume.
	TaskPushTimeout = 5 * time.Second

	// StreamKeepaliveInterval is how often to send keepalive messages on idle streams.
	StreamKeepaliveInterval = 30 * time.Second
)

// ============================================================================
// Task Scheduling Configuration
// ============================================================================

const (
	// TaskSchedulerPollInterval is how often the scheduler checks for pending tasks.
	TaskSchedulerPollInterval = 2 * time.Second

	// TaskBatchSize is the maximum number of tasks to fetch in a single poll.
	TaskBatchSize = 10

	// TaskAssignTimeout is the max time to wait when assigning a task to a worker.
	TaskAssignTimeout = 5 * time.Second

	// StaleTaskThreshold is how long a task can be in-progress before considered stale.
	StaleTaskThreshold = 5 * time.Minute

	// StaleTaskCheckInterval is how often to scan for stale tasks.
	StaleTaskCheckInterval = 1 * time.Minute
)

// ============================================================================
// Worker Registry Configuration
// ============================================================================

const (
	// WorkerHeartbeatTimeout is how long without heartbeat before marking worker dead.
	WorkerHeartbeatTimeout = 30 * time.Second

	// WorkerCleanupInterval is how often to check for dead workers.
	WorkerCleanupInterval = 1 * time.Minute

	// WorkerDBSyncInterval is how often to sync worker state to database.
	WorkerDBSyncInterval = 30 * time.Second

	// WorkerRegistrationTimeout is the max time for worker registration operations.
	WorkerRegistrationTimeout = 5 * time.Second

	// WorkerHeartbeatUpdateTimeout is the max time for heartbeat DB updates.
	WorkerHeartbeatUpdateTimeout = 10 * time.Second
)

// ============================================================================
// Raft Configuration
// ============================================================================

const (
	// RaftLeaderElectionTimeout is how long to wait for leader election on startup.
	RaftLeaderElectionTimeout = 15 * time.Second

	// RaftLeaderElectionPollInterval is how often to check for leader during election.
	RaftLeaderElectionPollInterval = 100 * time.Millisecond
)

// ============================================================================
// Shutdown Configuration
// ============================================================================

const (
	// GracefulShutdownTimeout is the total time allowed for graceful shutdown.
	GracefulShutdownTimeout = 30 * time.Second

	// ComponentShutdownTimeout is the time allowed for each component to shutdown.
	ComponentShutdownTimeout = 10 * time.Second

	// StreamDrainTimeout is the time to wait for in-flight stream operations.
	StreamDrainTimeout = 5 * time.Second
)

// ============================================================================
// Database Configuration
// ============================================================================

const (
	// DBOperationTimeout is the default timeout for database operations.
	DBOperationTimeout = 5 * time.Second

	// DBBatchOperationTimeout is the timeout for batch database operations.
	DBBatchOperationTimeout = 15 * time.Second

	// DBMaxRetries is the maximum number of retries for transient DB errors.
	DBMaxRetries = 3

	// DBRetryBackoff is the initial backoff duration between retries.
	DBRetryBackoff = 100 * time.Millisecond
)

// ============================================================================
// Retry Configuration
// ============================================================================

const (
	// DefaultMaxRetries is the default maximum number of retry attempts.
	DefaultMaxRetries = 3

	// RetryBackoffMultiplier is the factor to multiply backoff duration on each retry.
	RetryBackoffMultiplier = 2

	// MaxRetryBackoff is the maximum backoff duration between retries.
	MaxRetryBackoff = 30 * time.Second
)

// ============================================================================
// Task Status Constants
// ============================================================================

const (
	TaskStatusPending    = "pending"
	TaskStatusAssigned   = "assigned"
	TaskStatusInProgress = "in_progress"
	TaskStatusCompleted  = "completed"
	TaskStatusFailed     = "failed"
)

// ============================================================================
// Worker Status Constants
// ============================================================================

const (
	WorkerStatusActive  = "active"
	WorkerStatusSuspect = "suspect"
	WorkerStatusDead    = "dead"
)
