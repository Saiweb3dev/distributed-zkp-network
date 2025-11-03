# gRPC Setup Guide - Distributed ZKP Network

## Step 1: Generate Protobuf Code

First, install the necessary tools:

```bash
# Install protoc compiler
# On Ubuntu/Debian
sudo apt-get install protobuf-compiler

# On macOS
brew install protobuf

# Install Go protobuf plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Add to PATH
export PATH="$PATH:$(go env GOPATH)/bin"
```

Generate the Go code from your proto file:

```bash
# From your project root
mkdir -p internal/proto/coordinator

protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    internal/proto/coordinator/coordinator.proto
```

This generates two files:
- `coordinator.pb.go` - Message types
- `coordinator_grpc.pb.go` - gRPC service interfaces

## Step 2: Architecture Flow

### Registration & Heartbeat Flow
```
Worker                    Coordinator
  |                            |
  |--RegisterWorker()--------->| (Stores in WorkerRegistry)
  |<-------Success-------------|
  |                            |
  |--SendHeartbeat()---------->| (Updates LastHeartbeatAt)
  |<-------Success-------------|
  |  (repeats every 5s)        |
```

### Task Assignment Flow
```
Coordinator (Scheduler)      Worker (Executor)
  |                               |
  |--ReceiveTasks stream--------->| (Long-lived connection)
  |                               |
  |--TaskAssignment-------------->| (Push task)
  |                               |
  |<--ReportCompletion()---------| (Task finished)
  |                               |
```

## Step 3: Implementation Files Structure

```
internal/
├── proto/coordinator/
│   ├── coordinator.proto
│   ├── coordinator.pb.go (generated)
│   └── coordinator_grpc.pb.go (generated)
├── coordinator/
│   ├── service/
│   │   └── grpc_service.go (NEW - implements proto service)
│   ├── registry/
│   │   └── worker_registry.go (UPDATE)
│   └── scheduler/
│       └── task_scheduler.go (existing)
└── worker/
    └── client/
        └── coordinator_client.go (NEW - gRPC client)
```

## Key Concepts

### 1. Unary RPC (Request-Response)
Used for: RegisterWorker, SendHeartbeat, ReportCompletion, ReportFailure
- Client sends one request
- Server sends one response
- Connection closes after response

### 2. Server Streaming RPC
Used for: ReceiveTasks
- Client sends one request
- Server sends multiple responses over time (stream)
- Connection stays open
- Allows coordinator to push tasks to worker in real-time

## Next Steps

After generating the proto code, you'll need to:
1. Implement CoordinatorServiceServer interface (coordinator side)
2. Use CoordinatorServiceClient (worker side)
3. Wire up the implementations to existing code
4. Test the connection

See the implementation artifacts for complete code.