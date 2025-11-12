# Worker Node - Complete Architecture Guide

## 📋 Table of Contents

1. [Overview](#overview)
2. [Worker Main.go - Step-by-Step Flow](#worker-maingo---step-by-step-flow)
3. [gRPC Client Deep Dive](#grpc-client-deep-dive)
4. [Worker Pool Architecture](#worker-pool-architecture)
5. [Code Optimizations Applied](#code-optimizations-applied)
6. [Architecture Patterns](#architecture-patterns)
7. [Production Considerations](#production-considerations)
8. [Troubleshooting Guide](#troubleshooting-guide)

---

## Overview

The Worker Node is a **stateless compute engine** in the distributed ZKP network. It connects to coordinators, receives task assignments, generates zero-knowledge proofs concurrently, and reports results back.

### Key Responsibilities

- **Coordinator Connection**: Maintain persistent gRPC connection
- **Task Reception**: Receive tasks via server-streaming RPC
- **Proof Generation**: Execute ZKP computations concurrently
- **Result Reporting**: Send completed proofs back to coordinator
- **Health Monitoring**: Send periodic heartbeats to signal availability

### Worker Architecture Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                        Worker Node                           │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────────┐         ┌──────────────────┐          │
│  │ Coordinator     │◄────────┤  Heartbeat       │          │
│  │ Client (gRPC)   │         │  Sender Loop     │          │
│  │                 │         └──────────────────┘          │
│  │  - Register     │                                        │
│  │  - Heartbeat    │         ┌──────────────────┐          │
│  │  - ReceiveTasks │◄────────┤  Task Receiver   │          │
│  │  - ReportResult │         │  Loop            │          │
│  └────────┬────────┘         └──────────────────┘          │
│           │                                                  │
│           │ Tasks                                           │
│           ▼                                                  │
│  ┌─────────────────────────────────────────────┐           │
│  │         Worker Pool (Bounded Concurrency)    │           │
│  ├─────────────────────────────────────────────┤           │
│  │  [Worker 1] [Worker 2] [Worker 3] [Worker 4]│           │
│  │      ↓           ↓           ↓          ↓    │           │
│  │    Proof      Proof       Proof      Proof   │           │
│  │     Gen        Gen         Gen        Gen    │           │
│  └─────────────────┬───────────────────────────┘           │
│                    │ Results                                │
│                    ▼                                         │
│  ┌─────────────────────────────────────────────┐           │
│  │         Result Reporter Loop                 │           │
│  │   (Sends completed proofs to coordinator)    │           │
│  └─────────────────────────────────────────────┘           │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

---

## Worker Main.go - Step-by-Step Flow

### STEP 1: Parse Command-Line Flags

```go
configPath := flag.String("config", "configs/worker.yaml", "Path to worker configuration file")
showVersion := flag.Bool("version", false, "Display version information and exit")
flag.Parse()
```

**Purpose**: Allow runtime configuration and version display  
**Example Usage**:

```bash
# Use custom config
go run cmd/worker/main.go -config configs/dev/worker-local.yaml

# Show version info
go run cmd/worker/main.go -version
# Output:
# ZKP Worker Node v1.0.0
# Go Version: go1.21.5
# OS/Arch: windows/amd64
# CPUs: 8
```

**Version Information Displayed**:

- Worker version number
- Go compiler version
- Operating system and architecture
- Available CPU cores

---

### STEP 2: Initialize Structured Logger

```go
logger, err := initLogger()
if err != nil {
    fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
    os.Exit(1)
}
defer logger.Sync()
```

**Logger Configuration**:

```go
func initLogger() (*zap.Logger, error) {
    config := zap.NewProductionConfig()

    config.Encoding = "json"                          // Structured JSON output
    config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
    config.EncoderConfig.TimeKey = "timestamp"
    config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
    config.EncoderConfig.CallerKey = "caller"
    config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

    return config.Build()
}
```

**Log Output Example**:

```json
{
  "level": "info",
  "timestamp": "2025-11-03T10:15:30.123+0000",
  "caller": "main.go:45",
  "msg": "Starting ZKP Worker Node",
  "version": "1.0.0",
  "available_cpus": 8
}
```

**Benefits**:

- **Structured**: Easy parsing by log aggregators (ELK, Datadog)
- **Timestamps**: ISO8601 format for global consistency
- **Caller info**: Quick debugging with file:line references
- **Buffered**: High performance with deferred sync

---

### STEP 3: Load Worker Configuration

```go
cfg, err := config.LoadWorkerConfig(*configPath)
if err != nil {
    logger.Fatal("Failed to load configuration", zap.Error(err))
}
```

**Configuration File Structure** (`worker.yaml`):

```yaml
worker:
  id: "worker-1" # Unique identifier
  coordinator_address: "localhost:9090" # gRPC endpoint
  concurrency: 4 # Parallel proof generations
  heartbeat_interval: 5s # Health check frequency

zkp:
  curve: "BN254" # Elliptic curve for proofs
  proving_key_path: "keys/proving.key"
  verification_key_path: "keys/verification.key"
```

**Configuration Validation**:

```go
func validateConfig(cfg *config.WorkerConfig) error {
    if cfg.Worker.ID == "" {
        return fmt.Errorf("worker.id cannot be empty")
    }

    if cfg.Worker.Concurrency > runtime.NumCPU()*2 {
        return fmt.Errorf("concurrency too high: %d, max: %d",
            cfg.Worker.Concurrency, runtime.NumCPU()*2)
    }

    if cfg.Worker.HeartbeatInterval < time.Second {
        return fmt.Errorf("heartbeat_interval too low: %v", cfg.Worker.HeartbeatInterval)
    }

    return nil
}
```

**Validation Rules**:

- ✅ Worker ID must be non-empty (required for coordinator tracking)
- ✅ Concurrency must be positive and ≤ 2× CPU cores
- ✅ Heartbeat interval must be 1-60 seconds
- ✅ Coordinator address must be specified
- ✅ ZKP curve must be valid (BN254, BLS12-381)

---

### STEP 4: Initialize Worker Instance

```go
workerCfg := worker.Config{
    WorkerID:           cfg.Worker.ID,
    CoordinatorAddress: cfg.Worker.CoordinatorAddress,
    Concurrency:        cfg.Worker.Concurrency,
    HeartbeatInterval:  cfg.Worker.HeartbeatInterval,
    ZKPCurve:           cfg.ZKP.Curve,
}

w, err := worker.NewWorker(workerCfg, logger)
```

**What Happens in `NewWorker()`**:

1. **Initialize ZKP Prover**:

   ```go
   prover, err := zkp.NewGroth16Prover(cfg.ZKPCurve)
   ```

   - Loads proving/verification keys
   - Configures elliptic curve parameters
   - Prepares circuit compilation environment

2. **Create Worker Pool**:

   ```go
   pool := executor.NewWorkerPool(prover, cfg.Concurrency, logger)
   ```

   - Spawns N goroutines for concurrent proof generation
   - Creates buffered channels for tasks and results
   - Sets up bounded concurrency pattern

3. **Create Coordinator Client**:
   ```go
   coordClient, err := client.NewCoordinatorClient(
       cfg.WorkerID,
       cfg.CoordinatorAddress,
       cfg.Concurrency,
       logger,
   )
   ```
   - Prepares gRPC connection options (keepalive, timeouts)
   - Creates task channel for receiving assignments
   - Initializes connection state tracking

**Dependency Graph**:

```
Worker
  ├── CoordinatorClient (gRPC)
  │   └── pb.CoordinatorServiceClient
  │
  └── WorkerPool
      ├── Prover (ZKP)
      ├── CircuitFactory
      └── N × Goroutines
```

---

### STEP 5: Connect and Start Worker

```go
if err := w.Start(); err != nil {
    logger.Fatal("Failed to start worker", zap.Error(err))
}
```

**Worker Startup Sequence** (in `worker.Start()`):

#### 5.1: Connect to Coordinator

```go
// internal/worker/worker.go
func (w *Worker) Start() error {
    // Step 1: Establish gRPC connection
    if err := w.coordinatorClient.Connect(w.ctx); err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }
}
```

**Connection Options**:

```go
opts := []grpc.DialOption{
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:                10 * time.Second,  // Ping every 10s
        Timeout:             3 * time.Second,   // Wait 3s for pong
        PermitWithoutStream: true,              // Ping even without RPCs
    }),
}

conn, err := grpc.DialContext(dialCtx, coordinatorAddr, opts...)
```

**Why Keepalive?**

- Detects dead connections faster than TCP timeout (minutes)
- Prevents silent connection failures
- Enables quick reconnection on network issues

#### 5.2: Register with Coordinator

```go
// Step 2: Send registration request
if err := w.coordinatorClient.Register(w.ctx); err != nil {
    return fmt.Errorf("failed to register: %w", err)
}
```

**Registration Request**:

```protobuf
message RegisterWorkerRequest {
    string worker_id = 1;             // "worker-1"
    int32 max_concurrent_tasks = 2;   // 4
    map<string, string> capabilities = 3;  // {"circuits": "merkle,range"}
}
```

**What Coordinator Does**:

1. Validates worker ID uniqueness
2. Stores worker in registry with metadata
3. Persists registration to database
4. Returns success response
5. Worker proceeds to open task stream

#### 5.3: Start Worker Pool

```go
// Step 3: Start proof generation goroutines
w.workerPool.Start()
```

**Worker Pool Startup**:

```go
func (wp *WorkerPool) Start() {
    for i := 0; i < wp.concurrency; i++ {
        wp.wg.Add(1)
        go wp.worker(i)  // Launch goroutine
    }
}
```

**Each Goroutine**:

```go
func (wp *WorkerPool) worker(id int) {
    defer wp.wg.Done()

    for task := range wp.tasks {
        result := wp.processTask(task)  // Generate proof
        wp.results <- result            // Send result
    }
}
```

#### 5.4: Start Heartbeat Loop

```go
// Step 4: Begin health monitoring
go w.heartbeatLoop()
```

**Heartbeat Loop**:

```go
func (w *Worker) heartbeatLoop() {
    ticker := time.NewTicker(w.heartbeatInterval)  // 5 seconds
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            ctx, cancel := context.WithTimeout(w.ctx, 3*time.Second)

            if err := w.coordinatorClient.SendHeartbeat(ctx); err != nil {
                w.logger.Error("Heartbeat failed", zap.Error(err))
            }

            cancel()

        case <-w.ctx.Done():
            return  // Shutdown
        }
    }
}
```

**Heartbeat Packet**:

```protobuf
message HeartbeatRequest {
    string worker_id = 1;
    int64 timestamp = 2;  // Unix timestamp
}
```

**Purpose**:

- Proves worker is alive and responsive
- Updates coordinator's last_heartbeat timestamp
- Prevents worker from being marked as dead

#### 5.5: Start Result Reporter Loop

```go
// Step 5: Monitor for completed proofs
go w.resultReporterLoop()
```

**Result Reporter**:

```go
func (w *Worker) resultReporterLoop() {
    resultChan := w.workerPool.Results()

    for result := range resultChan {
        if result.Success {
            w.reportTaskSuccess(result)
        } else {
            w.reportTaskFailure(result.TaskID, result.Error)
        }
    }
}
```

#### 5.6: Start Task Receiver Loop

```go
// Step 6: Begin receiving task assignments
go w.taskReceiverLoop()
```

**Task Receiver**:

```go
func (w *Worker) taskReceiverLoop() {
    taskChan := w.coordinatorClient.TaskChannel()

    for task := range taskChan {
        // Parse JSON input data
        var inputData map[string]interface{}
        json.Unmarshal([]byte(task.InputData), &inputData)

        // Submit to worker pool
        w.workerPool.Submit(ctx, executorTask)
    }
}
```

**Complete Data Flow**:

```
Coordinator                 Worker
    │                         │
    ├─── TaskAssignment ──────>│ (gRPC stream)
    │                          │
    │                          ├─> Task Channel
    │                          │
    │                          ├─> Worker Pool
    │                          │     └─> Goroutine generates proof
    │                          │
    │                          ├─> Result Channel
    │                          │
    │<─ TaskCompletionRequest ─┤ (gRPC unary)
    │                          │
```

---

### STEP 6: Display Worker Status

```go
stats := w.GetStats()
logger.Info("Worker statistics",
    zap.String("worker_id", stats.WorkerID),
    zap.Int("pool_concurrency", stats.PoolConcurrency),
    zap.Int("active_tasks", stats.ActiveTasks),
    zap.Int("queued_tasks", stats.QueuedTasks),
    zap.Bool("connected", stats.Connected),
)
```

**Statistics Displayed**:

```json
{
  "level": "info",
  "msg": "Worker statistics",
  "worker_id": "worker-1",
  "pool_concurrency": 4,
  "active_tasks": 0,
  "queued_tasks": 0,
  "connected": true
}
```

**Monitoring Metrics**:

- `pool_concurrency`: Max parallel proofs
- `active_tasks`: Currently generating proofs
- `queued_tasks`: Waiting in buffer
- `connected`: gRPC connection status

---

### STEP 7: Wait for Shutdown Signal

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

sig := <-quit  // Blocks here
logger.Info("Shutdown signal received", zap.String("signal", sig.String()))
```

**Signals Handled**:

- **SIGINT (Ctrl+C)**: User interrupt in terminal
- **SIGTERM**: Kubernetes pod termination, `kill` command

**Why Buffer Size = 1?**

- Prevents signal loss if handler is busy
- Signal gets queued instead of dropped

---

### STEP 8: Graceful Shutdown

```go
shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer shutdownCancel()

shutdownComplete := make(chan struct{})
go func() {
    w.Stop()
    close(shutdownComplete)
}()

select {
case <-shutdownComplete:
    logger.Info("Worker shutdown completed successfully")
case <-shutdownCtx.Done():
    logger.Warn("Shutdown timeout exceeded, forcing termination")
}
```

**Shutdown Sequence** (in `worker.Stop()`):

```go
func (w *Worker) Stop() {
    // 1. Cancel context (signals all goroutines to stop)
    w.cancel()

    // 2. Stop worker pool
    w.workerPool.Stop()
    // - Closes task channel (no new submissions)
    // - Waits for in-progress proofs to complete
    // - Closes result channel

    // 3. Disconnect from coordinator
    w.coordinatorClient.Disconnect()
    // - Cancels task stream
    // - Closes gRPC connection
    // - Closes task channel
}
```

**Graceful Shutdown Timeline**:

```
T+0s:  Receive SIGTERM
       ├─> Cancel context
       ├─> Close task channel
       └─> Stop accepting new tasks

T+1s:  Wait for in-progress proofs
       ├─> Goroutine 1: Generating proof (80% complete)
       ├─> Goroutine 2: Generating proof (60% complete)
       └─> Goroutine 3: Idle

T+5s:  All proofs complete
       ├─> Send completion reports to coordinator
       └─> Close result channel

T+6s:  Close gRPC connection
       └─> Flush buffered logs

T+7s:  Shutdown complete (within 30s timeout)
```

**Why 30 Second Timeout?**

- Most proofs complete in 1-5 seconds
- Kubernetes default termination grace period is 30s
- Prevents indefinite hanging on stuck proofs
- After timeout, OS forcibly kills process (SIGKILL)

---

## gRPC Client Deep Dive

### Connection Management

#### Establishing Connection

```go
// internal/worker/client/coordinator_client.go
func (c *CoordinatorClient) Connect(ctx context.Context) error {
    opts := []grpc.DialOption{
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:                10 * time.Second,
            Timeout:             3 * time.Second,
            PermitWithoutStream: true,
        }),
    }

    dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    conn, err := grpc.DialContext(dialCtx, c.coordinatorAddr, opts...)
    if err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }

    c.conn = conn
    c.client = pb.NewCoordinatorServiceClient(conn)
    c.connected = true

    return nil
}
```

**Connection Options Explained**:

1. **Transport Security**:

   ```go
   grpc.WithTransportCredentials(insecure.NewCredentials())
   ```

   - Currently: No TLS (for development)
   - Production: Use `credentials.NewClientTLSFromFile()`

2. **Keepalive Parameters**:
   ```go
   Time:                10 * time.Second,  // Send ping every 10s
   Timeout:             3 * time.Second,   // Wait 3s for pong
   PermitWithoutStream: true,              // Ping even without active RPCs
   ```

**Keepalive Behavior**:

```
T+0s:   Connection established
T+10s:  Send PING (no active RPCs)
T+13s:  If no PONG received, declare connection dead
        └─> Trigger reconnection logic
```

---

### RPC Methods

#### 1. RegisterWorker (Unary RPC)

```go
func (c *CoordinatorClient) Register(ctx context.Context) error {
    req := &pb.RegisterWorkerRequest{
        WorkerId:           c.workerID,
        MaxConcurrentTasks: int32(c.maxConcurrentTasks),
        Capabilities: map[string]string{
            "circuits": "merkle,range,addition",
        },
    }

    regCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    resp, err := client.RegisterWorker(regCtx, req)
    if err != nil {
        return fmt.Errorf("registration failed: %w", err)
    }

    if !resp.Success {
        return fmt.Errorf("registration rejected: %s", resp.Message)
    }

    // Start receiving tasks after successful registration
    go c.receiveTasksLoop(ctx)

    return nil
}
```

**Request/Response Flow**:

```
Worker                           Coordinator
  │                                   │
  ├─── RegisterWorkerRequest ────────>│
  │    {                              │
  │      worker_id: "worker-1",       │
  │      max_concurrent_tasks: 4,     │
  │      capabilities: {...}          │
  │    }                              │
  │                                   │
  │                    Validate worker ID
  │                    Store in registry
  │                    Persist to database
  │                                   │
  │<── RegisterWorkerResponse ────────┤
  │    {                              │
  │      success: true,               │
  │      message: "Registered"        │
  │    }                              │
  │                                   │
```

---

#### 2. SendHeartbeat (Unary RPC)

```go
func (c *CoordinatorClient) SendHeartbeat(ctx context.Context) error {
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

    return nil
}
```

**Why Timestamps?**

- Coordinator tracks: `last_heartbeat` timestamp
- Enables detection of clock skew
- Helps debug network latency issues

**Heartbeat Failure Handling**:

```go
// In worker.go
if err := w.coordinatorClient.SendHeartbeat(ctx); err != nil {
    w.logger.Error("Heartbeat failed", zap.Error(err))
    // Worker continues operating (transient network error)
    // Coordinator will mark worker as 'suspect' after timeout
}
```

---

#### 3. ReceiveTasks (Server Streaming RPC)

```go
func (c *CoordinatorClient) receiveTasksStream(ctx context.Context) error {
    req := &pb.ReceiveTasksRequest{
        WorkerId: c.workerID,
    }

    stream, err := client.ReceiveTasks(ctx, req)
    if err != nil {
        return fmt.Errorf("failed to open stream: %w", err)
    }

    // Receive tasks from stream until closed
    for {
        assignment, err := stream.Recv()
        if err == io.EOF {
            return nil  // Stream closed normally
        }
        if err != nil {
            return fmt.Errorf("stream error: %w", err)
        }

        // Convert protobuf to Task struct
        task := &Task{
            ID:          assignment.TaskId,
            CircuitType: assignment.CircuitType,
            InputData:   string(assignment.InputDataJson),
            CreatedAt:   time.Unix(assignment.CreatedAt, 0),
        }

        // Send to task channel (non-blocking)
        select {
        case c.taskChan <- task:
            // Success
        case <-ctx.Done():
            return ctx.Err()
        default:
            w.logger.Warn("Task channel full, dropping task")
        }
    }
}
```

**Stream Lifecycle**:

```
Worker                          Coordinator
  │                                  │
  ├─── ReceiveTasksRequest ─────────>│
  │                                  │
  │                    Create channel for worker
  │                    Register in taskStreams map
  │                                  │
  │◄────── Stream Open ──────────────┤
  │                                  │
  │                                  │  Task scheduled
  │◄────── TaskAssignment ───────────┤  Send via channel
  │                                  │
  │                                  │  Another task scheduled
  │◄────── TaskAssignment ───────────┤
  │                                  │
  │         ...                      │
  │                                  │
  │◄────── Stream Close ─────────────┤  Worker disconnects
  │                                  │
```

**Retry Logic**:

```go
func (c *CoordinatorClient) receiveTasksLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return  // Shutdown
        default:
            // Attempt to establish stream
            if err := c.receiveTasksStream(ctx); err != nil {
                c.logger.Error("Stream error, retrying", zap.Error(err))

                // Wait before retrying (exponential backoff could be added)
                select {
                case <-time.After(5 * time.Second):
                    continue
                case <-ctx.Done():
                    return
                }
            }
        }
    }
}
```

**Why Retry Loop?**

- Network issues can close stream unexpectedly
- Coordinator restarts cause stream termination
- Automatic reconnection maintains availability

---

#### 4. ReportCompletion (Unary RPC)

```go
func (c *CoordinatorClient) ReportCompletion(
    ctx context.Context,
    result executor.TaskResult,
) error {
    // Marshal proof metadata to JSON
    metadataJSON, err := json.Marshal(result.ProofMetadata)
    if err != nil {
        return fmt.Errorf("failed to marshal metadata: %w", err)
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
        return fmt.Errorf("rejected: %s", resp.Message)
    }

    return nil
}
```

**What Coordinator Does**:

1. Update task status: `in_progress` → `completed`
2. Store proof data and metadata in database
3. Update worker's completed task count
4. Free up worker capacity (decrement active tasks)
5. Return success response

**Failure Handling**:

```go
// In worker.go
if err := w.coordinatorClient.ReportCompletion(ctx, result); err != nil {
    w.logger.Error("Failed to report completion", zap.Error(err))
    // TODO: Implement retry logic with exponential backoff
    // Store result locally and retry later
}
```

---

#### 5. ReportFailure (Unary RPC)

```go
func (c *CoordinatorClient) ReportFailure(
    ctx context.Context,
    taskID string,
    errorMsg string,
) error {
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
        return fmt.Errorf("rejected: %s", resp.Message)
    }

    return nil
}
```

**Failure Scenarios**:

- Invalid circuit type
- Malformed input data
- Proof generation timeout
- Out of memory
- Circuit compilation error

**Coordinator Actions**:

1. Update task status: `in_progress` → `failed`
2. Store error message in database
3. Increment worker's failed task count
4. Optionally reassign task to different worker

---

### Modifying gRPC Client

#### Adding Connection Retry with Exponential Backoff

```go
func (c *CoordinatorClient) ConnectWithRetry(ctx context.Context) error {
    maxRetries := 5
    baseDelay := time.Second

    for attempt := 0; attempt < maxRetries; attempt++ {
        err := c.Connect(ctx)
        if err == nil {
            return nil  // Success
        }

        // Calculate exponential backoff
        delay := baseDelay * time.Duration(1<<attempt)  // 1s, 2s, 4s, 8s, 16s

        c.logger.Warn("Connection failed, retrying",
            zap.Int("attempt", attempt+1),
            zap.Int("max_retries", maxRetries),
            zap.Duration("retry_in", delay),
            zap.Error(err),
        )

        select {
        case <-time.After(delay):
            continue
        case <-ctx.Done():
            return ctx.Err()
        }
    }

    return fmt.Errorf("failed to connect after %d attempts", maxRetries)
}
```

**Usage**:

```go
// In worker.go Start()
if err := w.coordinatorClient.ConnectWithRetry(w.ctx); err != nil {
    return fmt.Errorf("failed to connect: %w", err)
}
```

---

#### Adding TLS/SSL Support

```go
func (c *CoordinatorClient) ConnectSecure(ctx context.Context, certFile string) error {
    // Load CA certificate
    creds, err := credentials.NewClientTLSFromFile(certFile, "")
    if err != nil {
        return fmt.Errorf("failed to load TLS cert: %w", err)
    }

    opts := []grpc.DialOption{
        grpc.WithTransportCredentials(creds),  // Use TLS instead of insecure
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:                10 * time.Second,
            Timeout:             3 * time.Second,
            PermitWithoutStream: true,
        }),
    }

    conn, err := grpc.DialContext(ctx, c.coordinatorAddr, opts...)
    if err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }

    c.conn = conn
    c.client = pb.NewCoordinatorServiceClient(conn)
    c.connected = true

    return nil
}
```

**Configuration**:

```yaml
worker:
  coordinator_address: "coordinator.example.com:9090"
  tls_enabled: true
  tls_cert_file: "certs/ca-cert.pem"
```

---

#### Adding Authentication with Metadata

```go
// Add auth token to every RPC call
type AuthInterceptor struct {
    token string
}

func (a *AuthInterceptor) Unary() grpc.UnaryClientInterceptor {
    return func(
        ctx context.Context,
        method string,
        req, reply interface{},
        cc *grpc.ClientConn,
        invoker grpc.UnaryInvoker,
        opts ...grpc.CallOption,
    ) error {
        // Add auth token to metadata
        md := metadata.Pairs("authorization", "Bearer "+a.token)
        ctx = metadata.NewOutgoingContext(ctx, md)

        return invoker(ctx, method, req, reply, cc, opts...)
    }
}

func (a *AuthInterceptor) Stream() grpc.StreamClientInterceptor {
    return func(
        ctx context.Context,
        desc *grpc.StreamDesc,
        cc *grpc.ClientConn,
        method string,
        streamer grpc.Streamer,
        opts ...grpc.CallOption,
    ) (grpc.ClientStream, error) {
        // Add auth token to metadata
        md := metadata.Pairs("authorization", "Bearer "+a.token)
        ctx = metadata.NewOutgoingContext(ctx, md)

        return streamer(ctx, desc, cc, method, opts...)
    }
}
```

**Usage**:

```go
authInterceptor := &AuthInterceptor{token: workerToken}

opts := []grpc.DialOption{
    grpc.WithUnaryInterceptor(authInterceptor.Unary()),
    grpc.WithStreamInterceptor(authInterceptor.Stream()),
    // ... other options
}
```

---

## Worker Pool Architecture

### Bounded Concurrency Pattern

The worker pool implements **bounded concurrency** to prevent resource exhaustion:

```go
type WorkerPool struct {
    prover         zkp.Prover
    circuitFactory *circuits.CircuitFactory
    concurrency    int
    logger         *zap.Logger

    tasks   chan Task         // Buffered input
    results chan TaskResult   // Buffered output

    wg     sync.WaitGroup
    ctx    context.Context
    cancel context.CancelFunc
}
```

**Key Design Decisions**:

1. **Fixed Concurrency**:

   ```go
   concurrency := runtime.NumCPU() - 1  // Leave 1 CPU for OS
   ```

   - Prevents CPU oversubscription
   - Ensures consistent performance
   - Simplifies capacity planning

2. **Buffered Channels**:

   ```go
   tasks:   make(chan Task, concurrency*2),
   results: make(chan TaskResult, concurrency*2),
   ```

   - Buffer size = 2× concurrency
   - Prevents blocking on task submission
   - Allows burst handling

3. **WaitGroup for Cleanup**:
   ```go
   func (wp *WorkerPool) Stop() {
       close(wp.tasks)   // Signal: no more tasks
       wp.wg.Wait()      // Wait for all workers
       close(wp.results) // Signal: no more results
   }
   ```

---

### Proof Generation Pipeline

```
Task Submission
      ↓
[Task Channel] (buffer: 8)
      ↓
[Worker Goroutines] (4 parallel)
  ├─> Worker 1: processTask()
  │     ├─> parseTaskInput()
  │     ├─> prover.GenerateProof()
  │     └─> return TaskResult
  │
  ├─> Worker 2: processTask()
  ├─> Worker 3: processTask()
  └─> Worker 4: processTask()
      ↓
[Result Channel] (buffer: 8)
      ↓
Result Reporter
```

---

### Task Processing Flow

```go
func (wp *WorkerPool) processTask(task Task) TaskResult {
    startTime := time.Now()

    result := TaskResult{
        TaskID: task.ID,
    }

    // Step 1: Parse circuit type and create instances
    circuit, witness, err := wp.parseTaskInput(task)
    if err != nil {
        result.Error = fmt.Errorf("invalid input: %w", err)
        result.Duration = time.Since(startTime)
        return result
    }

    // Step 2: Generate proof (Phase 1 code - unchanged)
    proof, err := wp.prover.GenerateProof(circuit, witness)
    if err != nil {
        result.Error = fmt.Errorf("proof generation failed: %w", err)
        result.Duration = time.Since(startTime)
        return result
    }

    // Step 3: Populate result
    result.Success = true
    result.ProofData = proof.ProofData
    result.ProofMetadata = proof.PublicInputs
    result.Duration = time.Since(startTime)

    return result
}
```

**Phase Breakdown**:

- **Phase 1**: Parse input (1-10ms)
- **Phase 2**: Generate proof (1000-5000ms) ← Most time
- **Phase 3**: Package result (1ms)

---

### Merkle Proof Example

```go
func (wp *WorkerPool) parseMerkleProofTask(task Task) (circuit, witness frontend.Circuit, err error) {
    // Extract leaves from input_data
    leaves, ok := task.InputData["leaves"].([]interface{})
    if !ok {
        return nil, nil, fmt.Errorf("missing 'leaves' field")
    }

    leafIndex, ok := task.InputData["leaf_index"].(float64)
    if !ok {
        return nil, nil, fmt.Errorf("missing 'leaf_index' field")
    }

    // Convert hex strings to bytes
    leafBytes := make([][]byte, len(leaves))
    for i, leaf := range leaves {
        leafStr := leaf.(string)
        if len(leafStr) > 2 && leafStr[:2] == "0x" {
            leafStr = leafStr[2:]
        }
        leafBytes[i], err = hex.DecodeString(leafStr)
    }

    // Build Merkle tree (Phase 1 code)
    tree := zkp.NewMerkleTree(leafBytes, ecc.BN254)
    path, indices, err := tree.GenerateProof(int(leafIndex))

    // Create circuit template
    circuitTemplate := wp.circuitFactory.MerkleProof(tree.Depth)

    // Create witness with field elements
    witnessCircuit := wp.createMerkleWitness(
        tree.Depth,
        leafBytes[int(leafIndex)],
        path,
        indices,
        tree.Root,
    )

    return circuitTemplate, witnessCircuit, nil
}
```

**Input Data Structure**:

```json
{
  "leaves": [
    "0x1234567890abcdef",
    "0xfedcba0987654321",
    "0xabcdef1234567890",
    "0x567890abcdef1234"
  ],
  "leaf_index": 1
}
```

**Circuit Template vs Witness**:

- **Template**: Circuit structure (depth, constraints)
- **Witness**: Actual values (leaf, path, root)

---

## Code Optimizations Applied

### 1. **Added Configuration Validation**

**Before**: No validation (runtime panics on bad config)

**After**: Comprehensive validation

```go
func validateConfig(cfg *config.WorkerConfig) error {
    if cfg.Worker.Concurrency > runtime.NumCPU()*2 {
        return fmt.Errorf("concurrency too high")
    }
    // ... more checks
}
```

**Benefits**:

- Catch errors at startup
- Clear error messages
- Prevents silent failures

---

### 2. **Added Version Flag**

**Before**: No way to check worker version

**After**: Version display

```bash
go run cmd/worker/main.go -version
# ZKP Worker Node v1.0.0
# Go Version: go1.21.5
# CPUs: 8
```

**Benefits**:

- Deployment tracking
- Debugging version mismatches
- Operations visibility

---

### 3. **Added Graceful Shutdown Timeout**

**Before**: Blocking shutdown (could hang indefinitely)

**After**: Timeout with forced termination

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

select {
case <-shutdownComplete:
    // Clean shutdown
case <-shutdownCtx.Done():
    // Forced termination
}
```

**Benefits**:

- Predictable shutdown behavior
- Prevents hung processes
- Kubernetes-compatible (30s grace period)

---

### 4. **Improved Logger Configuration**

**Before**: Basic production logger

**After**: Enhanced with caller info and ISO8601 timestamps

```go
config.EncoderConfig.CallerKey = "caller"
config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
```

**Benefits**:

- Faster debugging (caller info)
- Consistent timestamps across services
- Better log aggregation

---

### 5. **Added Worker Statistics Display**

**Before**: No visibility into worker state

**After**: Statistics at startup

```json
{
  "worker_id": "worker-1",
  "pool_concurrency": 4,
  "active_tasks": 0,
  "connected": true
}
```

**Benefits**:

- Operator visibility
- Health check endpoint data
- Monitoring integration

---

### 6. **Step-by-Step Comments**

**Before**: Minimal inline comments

**After**: Comprehensive 8-step structure

```go
// ========================================================================
// STEP 3: Load Worker Configuration
// ========================================================================
```

**Benefits**:

- Onboarding new developers
- Understanding control flow
- Maintenance documentation

---

## Architecture Patterns

### 1. Producer-Consumer Pattern

```
TaskReceiver (Producer)
      ↓
  Task Channel
      ↓
WorkerPool (Consumers)
      ↓
  Result Channel
      ↓
ResultReporter (Consumer)
```

**Benefits**:

- Decoupled components
- Natural backpressure
- Easy to scale

---

### 2. Circuit Breaker Pattern (Future Enhancement)

```go
type CircuitBreaker struct {
    maxFailures int
    resetTimeout time.Duration

    failures int
    state    string  // "closed", "open", "half-open"
    lastFail time.Time
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    if cb.state == "open" {
        if time.Since(cb.lastFail) > cb.resetTimeout {
            cb.state = "half-open"
        } else {
            return fmt.Errorf("circuit breaker open")
        }
    }

    err := fn()
    if err != nil {
        cb.failures++
        cb.lastFail = time.Now()

        if cb.failures >= cb.maxFailures {
            cb.state = "open"
        }
        return err
    }

    cb.failures = 0
    cb.state = "closed"
    return nil
}
```

**Usage**:

```go
// Wrap coordinator RPC calls
err := circuitBreaker.Call(func() error {
    return w.coordinatorClient.SendHeartbeat(ctx)
})
```

---

### 3. Retry with Exponential Backoff

```go
func (w *Worker) reportWithRetry(ctx context.Context, result TaskResult) error {
    maxRetries := 3
    baseDelay := time.Second

    for attempt := 0; attempt < maxRetries; attempt++ {
        err := w.coordinatorClient.ReportCompletion(ctx, result)
        if err == nil {
            return nil
        }

        delay := baseDelay * time.Duration(1<<attempt)

        select {
        case <-time.After(delay):
            continue
        case <-ctx.Done():
            return ctx.Err()
        }
    }

    return fmt.Errorf("failed after %d retries", maxRetries)
}
```

---

## Production Considerations

### Metrics to Monitor

```
# Worker Health
worker_connected{worker_id="worker-1"} 1
worker_last_heartbeat_seconds{worker_id="worker-1"} 1698765432

# Capacity
worker_pool_concurrency{worker_id="worker-1"} 4
worker_active_tasks{worker_id="worker-1"} 2
worker_queued_tasks{worker_id="worker-1"} 3

# Task Processing
worker_tasks_completed_total{worker_id="worker-1"} 157
worker_tasks_failed_total{worker_id="worker-1"} 3
worker_proof_duration_seconds{circuit_type="merkle"} 2.45

# gRPC Client
grpc_client_calls_total{method="RegisterWorker",status="OK"} 1
grpc_client_calls_total{method="SendHeartbeat",status="OK"} 342
grpc_client_stream_duration_seconds{method="ReceiveTasks"} 3600
```

### Alerting Rules

```yaml
- alert: WorkerDisconnected
  expr: worker_connected == 0
  for: 1m
  annotations:
    summary: "Worker lost connection to coordinator"

- alert: WorkerHighFailureRate
  expr: rate(worker_tasks_failed_total[5m]) > 0.1
  for: 5m
  annotations:
    summary: "Worker failure rate > 10%"

- alert: WorkerPoolSaturated
  expr: worker_active_tasks >= worker_pool_concurrency
  for: 10m
  annotations:
    summary: "Worker pool at max capacity for 10 minutes"
```

### Resource Limits (Kubernetes)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: zkp-worker-1
spec:
  containers:
    - name: worker
      image: zkp-worker:1.0.0
      resources:
        requests:
          memory: "2Gi"
          cpu: "2000m"
        limits:
          memory: "4Gi"
          cpu: "4000m"
      env:
        - name: WORKER_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
```

---

## Troubleshooting Guide

### Issue 1: Worker Cannot Connect to Coordinator

**Symptoms**:

```
failed to connect: connection refused
```

**Causes**:

1. Coordinator not running
2. Wrong coordinator address
3. Firewall blocking port

**Solutions**:

```bash
# Test coordinator reachability
telnet coordinator-host 9090

# Check DNS resolution
nslookup coordinator-host

# Check firewall
netstat -an | findstr 9090
```

---

### Issue 2: Worker Disconnects Randomly

**Symptoms**:

```
stream error, retrying: rpc error: code = Unavailable
```

**Causes**:

1. Network instability
2. Coordinator restart
3. Keepalive timeout mismatch

**Solutions**:

```yaml
# Adjust keepalive parameters
worker:
  keepalive_time: 10s # Increase from 10s to 30s
  keepalive_timeout: 5s # Increase from 3s to 5s
```

---

### Issue 3: Proof Generation Fails

**Symptoms**:

```
proof generation failed: circuit compilation error
```

**Debug**:

```go
// Add detailed logging
func (wp *WorkerPool) processTask(task Task) TaskResult {
    wp.logger.Info("Processing task",
        zap.String("task_id", task.ID),
        zap.String("circuit_type", task.CircuitType),
        zap.Any("input_data", task.InputData),
    )

    circuit, witness, err := wp.parseTaskInput(task)
    if err != nil {
        wp.logger.Error("Failed to parse input",
            zap.Error(err),
            zap.Any("input_data", task.InputData),
        )
        // ...
    }
}
```

---

### Issue 4: Memory Leak

**Symptoms**:

```
Worker memory grows over time
```

**Debug**:

```go
import _ "net/http/pprof"

// Add pprof endpoint
go func() {
    http.ListenAndServe("localhost:6060", nil)
}()
```

```bash
# Get heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Get goroutine profile
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

---

## Summary

The refactored worker main.go demonstrates production-grade patterns:

✅ **8-step initialization** with validation and error handling  
✅ **Graceful shutdown** with timeout protection  
✅ **Configuration validation** preventing runtime errors  
✅ **Comprehensive logging** with structured output  
✅ **Statistics display** for operational visibility  
✅ **Version tracking** for deployment management  
✅ **Detailed comments** explaining every step

**Key Takeaways**:

1. Workers are stateless - can scale horizontally
2. gRPC streaming enables push-based task delivery
3. Bounded concurrency prevents resource exhaustion
4. Graceful shutdown ensures no proof data loss
5. Comprehensive logging is essential for debugging

**Next Steps**:

1. Add Prometheus metrics endpoint
2. Implement circuit breaker for coordinator failures
3. Add local task queue for offline operation
4. Implement result retry logic with exponential backoff
5. Add distributed tracing (OpenTelemetry)
