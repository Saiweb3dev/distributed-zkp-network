// Worker service constants
// Centralized configuration values for timeouts, limits, and defaults

package constants

import "time"

// =============================================================================
// Worker Pool Configuration
// =============================================================================

// WorkerPoolBufferMultiplier determines task/result channel buffer size
// Buffer = Concurrency * WorkerPoolBufferMultiplier
const WorkerPoolBufferMultiplier = 2

// WorkerPoolShutdownTimeout is max time to wait for worker pool shutdown
const WorkerPoolShutdownTimeout = 30 * time.Second

// =============================================================================
// Coordinator Client Configuration
// =============================================================================

// CoordinatorClientTaskChannelBuffer is the buffer size for incoming task channel
const CoordinatorClientTaskChannelBuffer = 10

// CoordinatorDialTimeout is max time to wait for coordinator connection
const CoordinatorDialTimeout = 10 * time.Second

// CoordinatorConnectionTestTimeout is timeout for connection validation
const CoordinatorConnectionTestTimeout = 5 * time.Second

// =============================================================================
// gRPC Keepalive Configuration
// =============================================================================

// GRPCKeepaliveTime is interval between keepalive pings
const GRPCKeepaliveTime = 10 * time.Second

// GRPCKeepaliveTimeout is time to wait for ping acknowledgement
const GRPCKeepaliveTimeout = 3 * time.Second

// GRPCKeepalivePermitWithoutStream allows pings even without active streams
const GRPCKeepalivePermitWithoutStream = true

// =============================================================================
// Heartbeat Configuration
// =============================================================================

// HeartbeatDefaultInterval is default time between heartbeats
const HeartbeatDefaultInterval = 5 * time.Second

// HeartbeatTimeout is max time for a single heartbeat RPC
const HeartbeatTimeout = 10 * time.Second

// HeartbeatMaxRetries is max retry attempts for heartbeat
const HeartbeatMaxRetries = 3

// HeartbeatBaseBackoff is initial backoff for heartbeat retries
const HeartbeatBaseBackoff = 500 * time.Millisecond

// HeartbeatMaxBackoff is maximum backoff for heartbeat retries
const HeartbeatMaxBackoff = 5 * time.Second

// =============================================================================
// Task Processing Configuration
// =============================================================================

// TaskStartTimeout is max time for notifying coordinator that task is starting
const TaskStartTimeout = 5 * time.Second

// TaskSubmitTimeout is max time to wait when submitting task to worker pool
const TaskSubmitTimeout = 30 * time.Second

// TaskResultTimeout is max time for a single task result report
const TaskResultTimeout = 10 * time.Second

// TaskResultMaxRetries is max retry attempts for result reporting
const TaskResultMaxRetries = 5

// TaskResultBaseBackoff is initial backoff for result retries
const TaskResultBaseBackoff = time.Second

// TaskResultMaxBackoff is maximum backoff for result retries
const TaskResultMaxBackoff = 30 * time.Second

// TaskFailureMaxRetries is max retry attempts for failure reporting
const TaskFailureMaxRetries = 3

// TaskFailureBaseBackoff is initial backoff for failure retries
const TaskFailureBaseBackoff = time.Second

// TaskFailureMaxBackoff is maximum backoff for failure retries
const TaskFailureMaxBackoff = 10 * time.Second

// =============================================================================
// Stream Reconnection Configuration
// =============================================================================

// StreamReconnectInitialBackoff is initial delay after stream failure
const StreamReconnectInitialBackoff = time.Second

// StreamReconnectMaxBackoff is maximum delay between stream reconnection attempts
const StreamReconnectMaxBackoff = time.Minute

// =============================================================================
// Event Bus Configuration
// =============================================================================

// EventPublishTimeout is max time to wait when publishing an event
const EventPublishTimeout = 5 * time.Second

// =============================================================================
// Shutdown Configuration
// =============================================================================

// ShutdownTimeout is max time to wait for graceful shutdown
const ShutdownTimeout = 30 * time.Second

// ShutdownGracePeriod is time to allow in-flight tasks to complete
const ShutdownGracePeriod = 25 * time.Second

// =============================================================================
// Retry Configuration Presets
// =============================================================================

// DefaultRetryMaxAttempts is standard retry count for non-critical operations
const DefaultRetryMaxAttempts = 3

// DefaultRetryBaseBackoff is standard initial backoff
const DefaultRetryBaseBackoff = time.Second

// DefaultRetryMaxBackoff is standard maximum backoff
const DefaultRetryMaxBackoff = time.Minute

// AggressiveRetryMaxAttempts is retry count for critical operations
const AggressiveRetryMaxAttempts = 5

// AggressiveRetryBaseBackoff is initial backoff for critical operations
const AggressiveRetryBaseBackoff = time.Second

// AggressiveRetryMaxBackoff is maximum backoff for critical operations
const AggressiveRetryMaxBackoff = 30 * time.Second

// =============================================================================
// Validation Configuration
// =============================================================================

// MinWorkerConcurrency is minimum allowed worker pool size
const MinWorkerConcurrency = 1

// MaxWorkerConcurrency is maximum recommended worker pool size
const MaxWorkerConcurrency = 64

// MinHeartbeatInterval is minimum allowed heartbeat frequency
const MinHeartbeatInterval = time.Second

// MaxHeartbeatInterval is maximum allowed heartbeat frequency
const MaxHeartbeatInterval = time.Minute

// =============================================================================
// Supported Circuit Types
// =============================================================================

var SupportedCircuitTypes = []string{
	"merkle_proof",
	"range_proof",
	"addition_proof",
}

// =============================================================================
// Supported ZKP Curves
// =============================================================================

var SupportedZKPCurves = []string{
	"BN254",
	"BLS12-381",
	"BLS12-377",
}
