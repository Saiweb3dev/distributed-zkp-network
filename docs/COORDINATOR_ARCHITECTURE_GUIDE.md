# Coordinator Main.go - Complete Architecture Guide

## 📋 Table of Contents
1. [Overview](#overview)
2. [Step-by-Step Execution Flow](#step-by-step-execution-flow)
3. [gRPC Deep Dive](#grpc-deep-dive)
4. [Code Optimizations Applied](#code-optimizations-applied)
5. [Architecture Patterns](#architecture-patterns)
6. [Production Considerations](#production-considerations)
7. [Troubleshooting Guide](#troubleshooting-guide)

---

## Overview

The Coordinator is the **brain** of the distributed ZKP network. It orchestrates task distribution, manages worker health, and ensures reliable proof generation across a cluster of worker nodes.

### Key Responsibilities
- **Worker Management**: Register, track, and monitor worker health
- **Task Scheduling**: Assign pending tasks to available workers
- **gRPC Communication**: Maintain persistent connections with workers
- **Health Monitoring**: Expose metrics and health status via HTTP
- **Fault Tolerance**: Handle worker failures and task reassignment

---

## Step-by-Step Execution Flow

### STEP 1: Parse Command-Line Flags
```go
configPath := flag.String("config", "configs/coordinator.yaml", "Path to configuration file")
flag.Parse()
```

**Purpose**: Allow runtime configuration file specification  
**Default**: `configs/coordinator.yaml`  
**Example**:
```bash
go run cmd/coordinator/main.go -config configs/dev/coordinator-local.yaml
```

---

### STEP 2: Initialize Structured Logger
```go
logger, err := initLogger()
if err != nil {
    fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
    os.Exit(1)
}
defer logger.Sync() // Flush buffered logs on exit
```

**Purpose**: Create production-ready structured logging  
**Logger Features**:
- **Structured**: JSON format for machine parsing
- **Levels**: INFO, WARN, ERROR, FATAL
- **ISO8601 timestamps**: `2025-11-02T18:16:37.902+0530`
- **Performance**: Buffered writes with sync on shutdown

**Why `defer logger.Sync()`?**  
Ensures all buffered log entries are written before program exits, preventing log loss during crashes.

---

### STEP 3: Load Configuration
```go
cfg, err := config.LoadCoordinatorConfig(*configPath)
if err != nil {
    logger.Fatal("Failed to load configuration", zap.Error(err))
}
```

**Configuration Sources** (priority order):
1. YAML file (`coordinator-local.yaml`)
2. Environment variables (`COORDINATOR_GRPC_PORT=9090`)
3. Default values

**Key Configuration Parameters**:
```yaml
coordinator:
  id: "coordinator-local"
  grpc_port: 9090              # Worker connections
  http_port: 8090              # Health/metrics
  poll_interval: 2s            # Task polling frequency
  heartbeat_timeout: 30s       # Worker health threshold
  stale_task_timeout: 5m       # Task reassignment threshold
  cleanup_interval: 1m         # Dead worker cleanup frequency
```

---

### STEP 4: Establish Database Connection
```go
dbConfig := &postgres.DatabaseConfig{
    MaxOpenConns:    25,               // Total connection pool size
    MaxIdleConns:    5,                // Idle connections kept alive
    ConnMaxLifetime: 5 * time.Minute,  // Max connection age
}

db, err := postgres.ConnectPostgreSQL(cfg.GetDatabaseConnectionString(), dbConfig)
```

**Connection String Format**:
```
host=localhost port=5432 user=postgres password=postgres dbname=zkp_network sslmode=disable
```

**Connection Pool Tuning**:
- **MaxOpenConns (25)**: Prevents database overload
- **MaxIdleConns (5)**: Balances reuse vs resources
- **ConnMaxLifetime (5min)**: Forces reconnect to handle network issues

**Why Connection Pooling?**
- Reuses existing connections (faster than creating new ones)
- Limits concurrent database load
- Handles connection failures gracefully

---

### STEP 5: Initialize Repositories
```go
taskRepo := postgres.NewTaskRepository(db)
workerRepo := postgres.NewWorkerRepository(db)
```

**Repository Pattern Benefits**:
- **Abstraction**: Business logic doesn't know about SQL
- **Testability**: Easy to mock for unit tests
- **Consistency**: Centralized data access logic

**TaskRepository Operations**:
- `CreateTask()` - Insert new task
- `GetPendingTasks()` - Fetch unassigned tasks
- `AssignTask()` - Atomically assign task to worker
- `CompleteTask()` - Mark task as done with proof data

**WorkerRepository Operations**:
- `CreateWorker()` - Register new worker
- `GetActiveWorkers()` - Fetch healthy workers
- `UpdateHeartbeat()` - Record worker heartbeat
- `MarkWorkerDead()` - Mark worker as failed

---

### STEP 6: Initialize Worker Registry
```go
workerRegistry := registry.NewWorkerRegistry(
    workerRepo,
    cfg.Coordinator.HeartbeatTimeout,  // 30 seconds
    logger,
    nil,  // gRPC service set later (circular dependency)
)

if err := workerRegistry.Start(); err != nil {
    logger.Fatal("Failed to start worker registry", zap.Error(err))
}
defer workerRegistry.Stop()
```

**Worker Registry Responsibilities**:
1. **Registration**: Accept worker registrations via gRPC
2. **Health Tracking**: Monitor heartbeats every 5 seconds
3. **Status Management**: Track worker states (active, suspect, dead)
4. **Capacity Tracking**: Know how many tasks each worker can handle

**Worker State Machine**:
```
         RegisterWorker()
              ↓
    [ACTIVE] ←─────────┐
      ↓                 │
      │ no heartbeat    │ heartbeat received
      │ for 30s         │
      ↓                 │
   [SUSPECT] ──────────┘
      ↓
      │ no heartbeat
      │ for 90s (3x timeout)
      ↓
    [DEAD]
```

**Why `nil` for gRPC Service?**  
Circular dependency: Registry needs Service to push tasks, Service needs Registry to manage workers. Resolved by setting later with `SetGRPCService()`.

---

### STEP 7: Initialize Task Scheduler
```go
taskScheduler := scheduler.NewTaskScheduler(
    taskRepo,
    workerRegistry,
    cfg.Coordinator.PollInterval,  // 2 seconds
    logger,
)

taskScheduler.Start()  // Starts background polling loop
defer taskScheduler.Stop()
```

**Scheduler Loop** (runs every 2 seconds):
```
1. SELECT * FROM tasks WHERE status='pending' LIMIT 10
2. Get available workers from registry
3. For each task:
   - Select worker (round-robin)
   - UPDATE tasks SET status='assigned', worker_id=...
   - Increment worker's task count
   - Push task via gRPC stream
4. Sleep for poll_interval
5. Repeat
```

**Scheduling Strategies**:
- **Round-Robin** (current): Fair distribution
- **Least-Loaded**: Always pick worker with most capacity
- **Priority-Based**: High-priority tasks to powerful workers

---

### STEP 8: Configure and Start gRPC Server

#### 8.1: Configure gRPC Options
```go
grpcOpts := []grpc.ServerOption{
    // Keepalive settings
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionIdle:     15 * time.Minute,
        MaxConnectionAge:      30 * time.Minute,
        MaxConnectionAgeGrace: 5 * time.Minute,
        Time:                  5 * time.Minute,
        Timeout:               20 * time.Second,
    }),
    
    // Keepalive enforcement
    grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
        MinTime:             5 * time.Minute,
        PermitWithoutStream: true,
    }),
    
    // Message size limits
    grpc.MaxRecvMsgSize(10 * 1024 * 1024),  // 10MB
    grpc.MaxSendMsgSize(10 * 1024 * 1024),  // 10MB
}
```

**Keepalive Parameters Explained**:
- **MaxConnectionIdle (15min)**: Close connection if no activity
- **MaxConnectionAge (30min)**: Force reconnect every 30 minutes (prevents stale connections)
- **Time (5min)**: Send ping every 5 minutes to check if connection alive
- **Timeout (20s)**: Wait 20s for ping response before declaring connection dead

**Why Keepalive?**
- Detects dead TCP connections (firewalls, network issues)
- Prevents resource leaks from zombie connections
- Ensures workers reconnect periodically

**Message Size Limits (10MB)**:
- Proof data can be large (several MB)
- Default gRPC limit is 4MB (too small)
- Prevents memory exhaustion from malicious clients

#### 8.2: Create gRPC Service
```go
grpcService := service.NewCoordinatorGRPCService(
    workerRegistry,
    taskRepo,
    logger,
)
```

**gRPC Service Methods**:
1. `RegisterWorker(req)` - Worker registers on startup
2. `SendHeartbeat(req)` - Worker sends ping every 5s
3. `ReportCompletion(req)` - Worker reports successful proof generation
4. `ReportFailure(req)` - Worker reports task failure
5. `ReceiveTasks(stream)` - Worker opens persistent stream for task delivery

#### 8.3: Resolve Circular Dependency
```go
workerRegistry.SetGRPCService(grpcService)
```

**Why This Is Necessary**:
```
WorkerRegistry                CoordinatorGRPCService
      ↓                               ↓
needs to push tasks          needs to query workers
      ↓                               ↓
requires gRPC service         requires worker registry
      ↓                               ↓
    [CIRCULAR DEPENDENCY]
```

**Solution**: Use dependency injection
1. Create WorkerRegistry with `nil` service
2. Create gRPC Service with registry
3. Inject service into registry with `SetGRPCService()`

#### 8.4: Register and Start Server
```go
pb.RegisterCoordinatorServiceServer(grpcServer, grpcService)

lis, err := net.Listen("tcp", ":9090")
if err != nil {
    logger.Fatal("Failed to listen on gRPC port", zap.Error(err))
}

go func() {
    logger.Info("gRPC server started", zap.String("address", ":9090"))
    if err := grpcServer.Serve(lis); err != nil {
        logger.Error("gRPC server terminated", zap.Error(err))
    }
}()
```

**What Happens Here**:
1. Register service implementation with protobuf-generated server
2. Start TCP listener on port 9090
3. Launch server in background goroutine
4. Main thread continues to start other services

---

### STEP 9: Configure and Start HTTP Server
```go
httpMux := http.NewServeMux()
httpMux.HandleFunc("/health", createHealthHandler(cfg, workerRegistry, taskScheduler))
httpMux.HandleFunc("/metrics", createMetricsHandler(workerRegistry))
httpMux.HandleFunc("/", createRootHandler(cfg, version))

httpServer := &http.Server{
    Addr:         ":8090",
    Handler:      httpMux,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

**HTTP Endpoints**:

#### `/health` - Health Check
```json
{
  "status": "healthy",
  "coordinator_id": "coordinator-local",
  "workers": {
    "total": 3,
    "active": 2,
    "suspect": 1,
    "dead": 0
  },
  "capacity": {
    "total": 6,
    "used": 3,
    "available": 3
  },
  "scheduler": {
    "tasks_assigned": 157,
    "assignments_failed": 2,
    "poll_cycles": 1024
  }
}
```

**Use Cases**:
- Kubernetes liveness/readiness probes
- Load balancer health checks
- Monitoring dashboards

#### `/metrics` - Prometheus Metrics
```
# HELP coordinator_workers_total Total number of registered workers
# TYPE coordinator_workers_total gauge
coordinator_workers_total 3
# HELP coordinator_workers_active Number of active workers
# TYPE coordinator_workers_active gauge
coordinator_workers_active 2
```

**Use Cases**:
- Prometheus scraping
- Grafana dashboards
- Alerting (PagerDuty, Slack)

#### `/` - Service Info
```json
{
  "service": "zkp-coordinator",
  "version": "1.0.0",
  "coordinator_id": "coordinator-local",
  "endpoints": {
    "health": "/health",
    "metrics": "/metrics"
  }
}
```

**HTTP Server Timeouts**:
- **ReadTimeout (5s)**: Max time to read request headers/body
- **WriteTimeout (10s)**: Max time to write response
- **IdleTimeout (60s)**: Max idle time between requests (keepalive)

---

### STEP 10: Start Background Task Cleanup
```go
go startStaleTaskCleanup(cfg, taskScheduler, logger)
```

**Stale Task Detection**:
```sql
SELECT * FROM tasks 
WHERE status = 'in_progress' 
AND started_at < NOW() - INTERVAL '5 minutes';
```

**Recovery Process**:
1. Identify tasks stuck in `in_progress` for > 5 minutes
2. Mark associated worker as `suspect`
3. Reset task to `pending` status
4. Task gets reassigned on next scheduler cycle

**Why This Matters**:
- Workers can crash without reporting task failure
- Network issues can prevent task completion
- Ensures tasks eventually complete even with worker failures

---

### STEP 11: Signal Ready
```go
logger.Info("Coordinator initialization complete",
    zap.String("status", "READY"),
)
```

**Ready Means**:
- Database connected
- gRPC server listening
- HTTP server listening
- Scheduler polling
- Worker registry active

---

### STEP 12: Wait for Shutdown Signal
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit  // Blocks until signal received
```

**Signals Handled**:
- **SIGINT**: Ctrl+C in terminal
- **SIGTERM**: Kubernetes pod termination, `kill` command

---

### STEP 13: Graceful Shutdown
```go
shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer shutdownCancel()

// Stop services in reverse order of startup
taskScheduler.Stop()        // Stop accepting new tasks
grpcServer.GracefulStop()   // Drain existing gRPC connections
httpServer.Shutdown(shutdownCtx)  // Drain HTTP requests
workerRegistry.Stop()       // Close worker connections
db.Close()                  // Close database connections
```

**Graceful Shutdown Steps**:
1. Stop scheduler → No new task assignments
2. Stop gRPC → Wait for in-flight RPCs to complete (max 30s)
3. Stop HTTP → Wait for in-flight requests to complete
4. Stop registry → Close worker connections
5. Close database → Release connection pool

**Why 30 Second Timeout?**
- Balances safety (complete work) vs speed (quick shutdown)
- Kubernetes default termination grace period is 30s
- Long enough for proof generation to finish

---

## gRPC Deep Dive

### gRPC vs REST: Why gRPC?

| Feature | gRPC | REST |
|---------|------|------|
| **Protocol** | HTTP/2 (binary) | HTTP/1.1 (text) |
| **Payload** | Protobuf (compact) | JSON (verbose) |
| **Streaming** | Bidirectional | Server-sent events only |
| **Performance** | 7-10x faster | Baseline |
| **Type Safety** | Strongly typed | Loosely typed |
| **Code Generation** | Auto-generated | Manual |

**For This System**: gRPC is ideal because:
- **Server streaming**: Coordinator pushes tasks to workers
- **Performance**: Binary encoding reduces bandwidth
- **Type safety**: Protobuf prevents serialization bugs
- **Bidirectional**: Workers can call coordinator and vice versa

---

### gRPC Communication Patterns

#### 1. Unary RPC (Request-Response)
```protobuf
rpc RegisterWorker(RegisterWorkerRequest) returns (RegisterWorkerResponse);
```

**Flow**:
```
Worker                    Coordinator
  │                           │
  ├─ RegisterWorkerRequest ──>│
  │                           │
  │<── RegisterWorkerResponse─┤
  │                           │
```

**Used For**: Worker registration, heartbeats, result reporting

---

#### 2. Server Streaming RPC (Persistent Connection)
```protobuf
rpc ReceiveTasks(ReceiveTasksRequest) returns (stream TaskAssignment);
```

**Flow**:
```
Worker                    Coordinator
  │                           │
  ├─ ReceiveTasksRequest ────>│
  │                           │  (Connection stays open)
  │<──── TaskAssignment ──────┤
  │<──── TaskAssignment ──────┤
  │<──── TaskAssignment ──────┤
  │         ...                │
```

**Used For**: Task delivery from coordinator to workers

**Key Advantage**: Coordinator can push tasks immediately without polling

---

### gRPC Service Implementation

#### Method 1: RegisterWorker
```go
func (s *CoordinatorGRPCService) RegisterWorker(
    ctx context.Context,
    req *pb.RegisterWorkerRequest,
) (*pb.RegisterWorkerResponse, error) {
    // 1. Extract worker metadata from request
    workerID := req.WorkerId
    maxConcurrent := int(req.MaxConcurrentTasks)
    
    // 2. Get worker's IP address from gRPC context
    peerInfo, _ := peer.FromContext(ctx)
    workerAddress := peerInfo.Addr.String()
    
    // 3. Register in worker registry
    err := s.workerRegistry.RegisterWorker(
        ctx, workerID, workerAddress, maxConcurrent, capabilities,
    )
    
    // 4. Return success/failure
    return &pb.RegisterWorkerResponse{
        Success: true,
        Message: "Worker registered successfully",
    }, nil
}
```

**What Happens**:
1. Worker connects and sends registration request
2. Coordinator extracts worker metadata
3. Coordinator stores worker in registry and database
4. Coordinator responds with success
5. Connection closes (unary RPC)

---

#### Method 2: ReceiveTasks (Server Streaming)
```go
func (s *CoordinatorGRPCService) ReceiveTasks(
    req *pb.ReceiveTasksRequest,
    stream pb.CoordinatorService_ReceiveTasksServer,
) error {
    workerID := req.WorkerId
    
    // Create channel for this worker's tasks
    taskChan := make(chan *pb.TaskAssignment, 10)
    
    // Register stream in map
    s.taskStreams[workerID] = taskChan
    
    // Clean up on disconnect
    defer func() {
        delete(s.taskStreams, workerID)
        close(taskChan)
    }()
    
    // Stream tasks until connection closes
    for {
        select {
        case task := <-taskChan:
            // Send task to worker via stream
            if err := stream.Send(task); err != nil {
                return err
            }
            
        case <-stream.Context().Done():
            // Client disconnected
            return nil
        }
    }
}
```

**What Happens**:
1. Worker opens persistent stream connection
2. Coordinator creates channel for worker
3. Coordinator registers channel in `taskStreams` map
4. When scheduler assigns task:
   - Scheduler calls `PushTaskToWorker(workerID, task)`
   - Coordinator sends task via channel
   - gRPC stream delivers task to worker
5. Connection stays open until worker disconnects

**Key Design Decision**: Use channels to decouple scheduler from gRPC

```
Scheduler              Channel            gRPC Stream            Worker
    │                     │                    │                   │
    ├─ PushTask ─────────>│                    │                   │
    │                     ├─── task ──────────>│                   │
    │                     │                    ├─── Send ─────────>│
    │                     │                    │                   │
```

---

### gRPC Error Handling

**Status Codes Used**:
```go
import "google.golang.org/grpc/codes"
import "google.golang.org/grpc/status"

// Unknown worker
if !exists {
    return status.Error(codes.NotFound, "worker not found")
}

// Database error
if err := db.Save(); err != nil {
    return status.Error(codes.Internal, "database error")
}

// Invalid input
if req.WorkerId == "" {
    return status.Error(codes.InvalidArgument, "worker_id required")
}
```

**gRPC Status Codes**:
- `OK` (0): Success
- `InvalidArgument` (3): Client error
- `NotFound` (5): Resource not found
- `DeadlineExceeded` (4): Timeout
- `Internal` (13): Server error
- `Unavailable` (14): Service unavailable

---

### Modifying gRPC Implementation

#### Adding a New RPC Method

**1. Update Protobuf Definition**
```protobuf
// internal/proto/coordinator/v1/coordinator.proto
service CoordinatorService {
    // ... existing methods ...
    
    // NEW: Get worker statistics
    rpc GetWorkerStats(GetWorkerStatsRequest) returns (GetWorkerStatsResponse);
}

message GetWorkerStatsRequest {
    string worker_id = 1;
}

message GetWorkerStatsResponse {
    int32 tasks_completed = 1;
    int64 total_duration_ms = 2;
    double average_duration_ms = 3;
}
```

**2. Regenerate Go Code**
```bash
protoc --go_out=. --go-grpc_out=. internal/proto/coordinator/v1/coordinator.proto
move generated files to internal/proto/coordinator/
```

**3. Implement Method in Service**
```go
// internal/coordinator/service/grpc_service.go
func (s *CoordinatorGRPCService) GetWorkerStats(
    ctx context.Context,
    req *pb.GetWorkerStatsRequest,
) (*pb.GetWorkerStatsResponse, error) {
    // Query database for worker stats
    stats, err := s.taskRepo.GetWorkerStats(ctx, req.WorkerId)
    if err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }
    
    return &pb.GetWorkerStatsResponse{
        TasksCompleted:     int32(stats.TasksCompleted),
        TotalDurationMs:    stats.TotalDurationMs,
        AverageDurationMs:  stats.AverageDurationMs,
    }, nil
}
```

**4. Use in Worker Client**
```go
// internal/worker/client/coordinator_client.go
func (c *CoordinatorClient) GetMyStats(ctx context.Context) (*Stats, error) {
    req := &pb.GetWorkerStatsRequest{
        WorkerId: c.workerID,
    }
    
    resp, err := c.client.GetWorkerStats(ctx, req)
    if err != nil {
        return nil, err
    }
    
    return &Stats{
        TasksCompleted:    int(resp.TasksCompleted),
        AverageDurationMs: resp.AverageDurationMs,
    }, nil
}
```

---

#### Adding TLS/SSL Encryption

**1. Generate Certificates**
```bash
# Generate CA private key
openssl genrsa -out ca-key.pem 4096

# Generate CA certificate
openssl req -new -x509 -days 365 -key ca-key.pem -out ca-cert.pem

# Generate server private key
openssl genrsa -out server-key.pem 4096

# Generate server certificate signing request
openssl req -new -key server-key.pem -out server-csr.pem

# Sign server certificate with CA
openssl x509 -req -days 365 -in server-csr.pem \
    -CA ca-cert.pem -CAkey ca-key.pem -set_serial 01 \
    -out server-cert.pem
```

**2. Update Coordinator to Use TLS**
```go
// Load TLS credentials
creds, err := credentials.NewServerTLSFromFile(
    "certs/server-cert.pem",
    "certs/server-key.pem",
)
if err != nil {
    logger.Fatal("Failed to load TLS credentials", zap.Error(err))
}

// Add TLS to gRPC options
grpcOpts := []grpc.ServerOption{
    grpc.Creds(creds),  // Enable TLS
    // ... other options ...
}

grpcServer := grpc.NewServer(grpcOpts...)
```

**3. Update Worker to Verify TLS**
```go
// Load CA certificate
caCert, err := ioutil.ReadFile("certs/ca-cert.pem")
if err != nil {
    return err
}

certPool := x509.NewCertPool()
certPool.AppendCertsFromPEM(caCert)

creds := credentials.NewClientTLSFromCert(certPool, "")

// Connect with TLS
conn, err := grpc.Dial(
    coordinatorAddr,
    grpc.WithTransportCredentials(creds),
)
```

---

#### Adding Authentication

**1. Define Auth Interceptor**
```go
// internal/coordinator/service/auth.go
func AuthInterceptor(
    ctx context.Context,
    req interface{},
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (interface{}, error) {
    // Extract metadata from context
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return nil, status.Error(codes.Unauthenticated, "missing metadata")
    }
    
    // Verify auth token
    tokens := md.Get("authorization")
    if len(tokens) == 0 {
        return nil, status.Error(codes.Unauthenticated, "missing token")
    }
    
    token := tokens[0]
    if !verifyToken(token) {
        return nil, status.Error(codes.PermissionDenied, "invalid token")
    }
    
    // Call handler if authenticated
    return handler(ctx, req)
}
```

**2. Register Interceptor**
```go
grpcOpts := []grpc.ServerOption{
    grpc.UnaryInterceptor(AuthInterceptor),
    // ... other options ...
}
```

**3. Worker Sends Auth Token**
```go
// Worker adds metadata to every request
md := metadata.Pairs("authorization", workerToken)
ctx := metadata.NewOutgoingContext(context.Background(), md)

// Now make RPC call with authenticated context
resp, err := client.RegisterWorker(ctx, req)
```

---

## Code Optimizations Applied

### 1. **Extracted Handler Functions**
**Before**: Inline anonymous functions (hard to test)
```go
httpMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    // 50 lines of inline code...
})
```

**After**: Named functions (testable, reusable)
```go
httpMux.HandleFunc("/health", createHealthHandler(cfg, workerRegistry, taskScheduler))
```

**Benefits**:
- Unit testable
- Reusable across coordinators
- Cleaner main function

---

### 2. **Added gRPC Keepalive Settings**
**Before**: No keepalive (connections could hang indefinitely)

**After**: Production-grade keepalive
```go
grpc.KeepaliveParams(keepalive.ServerParameters{
    MaxConnectionIdle: 15 * time.Minute,
    Time:              5 * time.Minute,
    Timeout:           20 * time.Second,
})
```

**Benefits**:
- Detects dead connections faster
- Prevents resource leaks
- Forces periodic reconnection (good for load balancing)

---

### 3. **Added HTTP Server Timeouts**
**Before**: No timeouts (vulnerable to slowloris attacks)

**After**: Defensive timeouts
```go
httpServer := &http.Server{
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

**Benefits**:
- Prevents resource exhaustion
- Limits attack surface
- Forces clients to reconnect periodically

---

### 4. **Added Message Size Limits**
**Before**: Default 4MB limit (too small for proofs)

**After**: 10MB limit
```go
grpc.MaxRecvMsgSize(10 * 1024 * 1024),
grpc.MaxSendMsgSize(10 * 1024 * 1024),
```

**Benefits**:
- Supports large proof data
- Prevents memory exhaustion from malicious clients

---

### 5. **Improved Logger Configuration**
**Before**: Basic logger
```go
config := zap.NewProductionConfig()
config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
```

**After**: Enhanced logger with timestamps
```go
config.EncoderConfig.TimeKey = "timestamp"
config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
```

**Benefits**:
- Consistent timestamp format
- Easier log parsing
- Better correlation across services

---

### 6. **Extracted Background Service Function**
**Before**: Inline goroutine
```go
go func() {
    ticker := time.NewTicker(cfg.Coordinator.CleanupInterval)
    // 20 lines of cleanup logic...
}()
```

**After**: Named function
```go
go startStaleTaskCleanup(cfg, taskScheduler, logger)
```

**Benefits**:
- Testable in isolation
- Clear responsibility
- Reusable

---

### 7. **Added Comprehensive Comments**
**Before**: Minimal comments

**After**: Step-by-step documentation
```go
// ========================================================================
// STEP 8: Configure and Start gRPC Server (Worker Communication)
// ========================================================================
```

**Benefits**:
- Onboarding new developers
- Understanding code flow
- Maintenance documentation

---

### 8. **Improved Graceful Shutdown**
**Before**: Simple stop calls

**After**: Ordered shutdown with database close
```go
taskScheduler.Stop()        // 1. Stop new work
grpcServer.GracefulStop()   // 2. Drain connections
httpServer.Shutdown(ctx)    // 3. Drain HTTP
workerRegistry.Stop()       // 4. Close workers
db.Close()                  // 5. Close database
```

**Benefits**:
- No data loss
- Clean resource cleanup
- Proper connection draining

---

## Architecture Patterns

### 1. Dependency Injection
```go
workerRegistry := registry.NewWorkerRegistry(
    workerRepo,      // Injected dependency
    heartbeatTimeout,
    logger,
    nil,             // Injected later
)
```

**Benefits**:
- Testability (mock dependencies)
- Flexibility (swap implementations)
- Loose coupling

---

### 2. Repository Pattern
```
[Business Logic] → [Repository Interface] → [Database Implementation]
```

**Benefits**:
- Database agnostic
- Easy to switch from PostgreSQL to MySQL
- Centralized data access

---

### 3. Factory Pattern
```go
func createHealthHandler(...) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Handler logic with captured dependencies
    }
}
```

**Benefits**:
- Encapsulates handler creation
- Dependency injection via closure
- Clean separation of concerns

---

### 4. Observer Pattern (Event-Driven)
```
TaskScheduler ────> PushTask ────> gRPC Stream ────> Worker
                       │
                       └─> (Async notification)
```

**Benefits**:
- Decoupled components
- Non-blocking task assignment
- Scalable architecture

---

## Production Considerations

### Metrics to Monitor
```
# Worker Health
coordinator_workers_active
coordinator_workers_dead

# Capacity
coordinator_capacity_available
coordinator_capacity_used

# Task Throughput
coordinator_tasks_assigned_total
coordinator_tasks_failed_total
coordinator_task_duration_seconds

# gRPC Metrics
grpc_server_handled_total
grpc_server_handling_seconds
```

### Alerting Rules
```yaml
- alert: NoAvailableWorkers
  expr: coordinator_workers_active == 0
  for: 1m
  annotations:
    summary: "No workers available for task processing"

- alert: HighTaskFailureRate
  expr: rate(coordinator_tasks_failed_total[5m]) > 0.1
  for: 5m
  annotations:
    summary: "Task failure rate > 10%"

- alert: HighGRPCLatency
  expr: grpc_server_handling_seconds > 1.0
  for: 5m
  annotations:
    summary: "gRPC latency > 1 second"
```

### Load Testing
```bash
# Install grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Register 100 workers
for i in {1..100}; do
  grpcurl -plaintext -d @ localhost:9090 \
    coordinator.CoordinatorService/RegisterWorker <<EOF
{
  "worker_id": "worker-$i",
  "max_concurrent_tasks": 2
}
EOF
done

# Submit 1000 tasks via API Gateway
for i in {1..1000}; do
  curl -X POST http://localhost:8080/api/v1/tasks/merkle \
    -H "Content-Type: application/json" \
    -d '{"leaves":["0x'$i'"],"leaf_index":0}' &
done
```

---

## Troubleshooting Guide

### Issue 1: gRPC Connection Refused
**Symptom**:
```
failed to connect: connection refused
```

**Causes**:
1. Coordinator not running
2. Firewall blocking port 9090
3. Wrong address (localhost vs 0.0.0.0)

**Solutions**:
```bash
# Check if port is listening
netstat -an | grep 9090

# Check firewall
sudo ufw status
sudo ufw allow 9090/tcp

# Test connection
telnet localhost 9090
```

---

### Issue 2: Database Connection Pool Exhausted
**Symptom**:
```
pq: too many clients already
```

**Causes**:
1. MaxOpenConns too high
2. Connections not being released
3. Database max_connections too low

**Solutions**:
```yaml
# Reduce connection pool
database:
  max_open_conns: 10  # Was: 25
  max_idle_conns: 2   # Was: 5

# OR increase database limit
# In postgresql.conf:
max_connections = 100  # Default: 100
```

---

### Issue 3: Tasks Not Being Assigned
**Symptom**:
```
scheduler.poll_cycles increasing but tasks_assigned = 0
```

**Debug Steps**:
```bash
# 1. Check pending tasks exist
curl http://localhost:8090/health | jq '.scheduler'

# 2. Check workers are available
curl http://localhost:8090/health | jq '.capacity'

# 3. Check database
psql -d zkp_network -c "SELECT status, COUNT(*) FROM tasks GROUP BY status;"

# 4. Check scheduler logs
grep "Found pending tasks" logs/coordinator.log
grep "No available workers" logs/coordinator.log
```

---

### Issue 4: Memory Leak
**Symptom**:
```
Coordinator memory usage grows over time
```

**Causes**:
1. Goroutines not being cleaned up
2. gRPC streams not closing
3. Channel buffers growing

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

The refactored coordinator main.go demonstrates production-grade Go service architecture with:

✅ **13-step initialization** with clear separation of concerns  
✅ **gRPC optimizations** for reliability and performance  
✅ **Comprehensive error handling** and graceful shutdown  
✅ **Observability** through structured logging and metrics  
✅ **Testability** through dependency injection and factory functions  
✅ **Documentation** with inline comments explaining every step  

**Key Takeaways**:
1. Structure matters - separate initialization, operation, and shutdown
2. gRPC requires careful configuration for production
3. Graceful shutdown prevents data loss
4. Dependency injection enables testing
5. Comprehensive logging is essential for debugging

---

**Next Steps**:
1. Add distributed tracing (OpenTelemetry)
2. Implement Raft consensus for leader election
3. Add circuit breakers for failure isolation
4. Implement rate limiting
5. Add request authentication and authorization
