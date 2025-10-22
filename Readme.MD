# Distributed Zero-Knowledge Proof Verification Network

## Project Overview
A production-grade distributed system for generating and verifying zero-knowledge proofs with horizontal scalability, consensus mechanisms, and comprehensive observability.

**Learning Focus**: Golang concurrency, distributed systems patterns, DevOps infrastructure, and ZKP cryptography.

---

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────┐
│                      Client Layer (Future)                   │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    API Gateway Layer                         │
│  (HTTP/gRPC endpoints, rate limiting, auth)                  │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                  Coordinator Node Layer                      │
│  (Raft consensus, leader election, task scheduling)          │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Worker Node Layer                         │
│  (ZKP generation, proof verification, result reporting)      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Data Layer                              │
│  (PostgreSQL, Redis, Blockchain storage)                     │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                  Observability Layer                         │
│  (Prometheus, Grafana, Jaeger, ELK Stack)                    │
└─────────────────────────────────────────────────────────────┘
```

---

## Project Structure

```
zkp-distributed-network/
│
├── cmd/                                    # Application entry points
│   ├── api-gateway/
│   │   └── main.go                         # API Gateway service
│   ├── coordinator/
│   │   └── main.go                         # Coordinator node service
│   ├── worker/
│   │   └── main.go                         # Worker node service
│   └── cli/
│       └── main.go                         # CLI tool for admin tasks
│
├── internal/                               # Private application code
│   ├── api/                                # API Gateway Layer
│   │   ├── handlers/
│   │   │   ├── proof_handler.go            # HTTP handlers for proof requests
│   │   │   ├── status_handler.go           # Status and health endpoints
│   │   │   └── middleware/
│   │   │       ├── auth.go                 # Authentication middleware
│   │   │       ├── ratelimit.go            # Rate limiting
│   │   │       ├── logging.go              # Request logging
│   │   │       └── recovery.go             # Panic recovery
│   │   ├── grpc/
│   │   │   ├── server.go                   # gRPC server setup
│   │   │   └── interceptors/
│   │   │       ├── auth.go                 # Auth interceptor
│   │   │       └── logging.go              # Logging interceptor
│   │   └── router.go                       # HTTP router configuration
│   │
│   ├── coordinator/                        # Coordinator Node Layer
│   │   ├── consensus/
│   │   │   ├── raft.go                     # Raft consensus implementation
│   │   │   ├── leader_election.go          # Leader election logic
│   │   │   ├── log_replication.go          # Log replication
│   │   │   └── state_machine.go            # FSM for coordinator state
│   │   ├── scheduler/
│   │   │   ├── task_scheduler.go           # Task distribution logic
│   │   │   ├── load_balancer.go            # Worker load balancing
│   │   │   └── priority_queue.go           # Task priority management
│   │   ├── registry/
│   │   │   ├── worker_registry.go          # Track active workers
│   │   │   └── heartbeat.go                # Worker heartbeat monitoring
│   │   └── coordinator.go                  # Main coordinator logic
│   │
│   ├── worker/                             # Worker Node Layer
│   │   ├── executor/
│   │   │   ├── proof_generator.go          # ZKP generation using gnark
│   │   │   ├── verifier.go                 # Proof verification
│   │   │   ├── worker_pool.go              # Worker pool pattern
│   │   │   └── circuit_cache.go            # Circuit compilation cache
│   │   ├── client/
│   │   │   ├── coordinator_client.go       # Communicate with coordinator
│   │   │   └── heartbeat.go                # Send heartbeats
│   │   └── worker.go                       # Main worker logic
│   │
│   ├── zkp/                                # Zero-Knowledge Proof Logic
│   │   ├── circuits/
│   │   │   ├── example_circuit.go          # Sample circuits (Merkle, range proof)
│   │   │   └── circuit_registry.go         # Circuit type registry
│   │   ├── prover.go                       # Prover interface and impl
│   │   ├── verifier.go                     # Verifier interface and impl
│   │   └── serialization.go                # Proof serialization
│   │
│   ├── storage/                            # Data Layer
│   │   ├── postgres/
│   │   │   ├── client.go                   # PostgreSQL client
│   │   │   ├── proof_repository.go         # Proof CRUD operations
│   │   │   ├── task_repository.go          # Task persistence
│   │   │   └── migrations/                 # Database migrations
│   │   │       └── *.sql
│   │   ├── redis/
│   │   │   ├── client.go                   # Redis client
│   │   │   ├── cache.go                    # Caching layer
│   │   │   └── lock.go                     # Distributed locking
│   │   ├── blockchain/
│   │   │   ├── chain.go                    # Simple blockchain implementation
│   │   │   ├── block.go                    # Block structure
│   │   │   └── persistence.go              # File-based persistence
│   │   └── repository.go                   # Repository interfaces
│   │
│   ├── observability/                      # Observability Layer
│   │   ├── metrics/
│   │   │   ├── prometheus.go               # Prometheus metrics
│   │   │   ├── collector.go                # Custom collectors
│   │   │   └── registry.go                 # Metrics registry
│   │   ├── logging/
│   │   │   ├── logger.go                   # Structured logger (zap/zerolog)
│   │   │   └── context.go                  # Context-aware logging
│   │   ├── tracing/
│   │   │   ├── tracer.go                   # Jaeger tracer setup
│   │   │   └── spans.go                    # Span helpers
│   │   └── health/
│   │       └── checker.go                  # Health check aggregator
│   │
│   ├── common/                             # Shared utilities
│   │   ├── config/
│   │   │   └── config.go                   # Configuration management
│   │   ├── errors/
│   │   │   └── errors.go                   # Custom error types
│   │   ├── patterns/
│   │   │   ├── circuitbreaker.go           # Circuit breaker pattern
│   │   │   ├── retry.go                    # Retry logic
│   │   │   └── observer.go                 # Observer pattern
│   │   └── utils/
│   │       ├── context.go                  # Context utilities
│   │       └── graceful_shutdown.go        # Graceful shutdown helper
│   │
│   └── proto/                              # Protocol Buffers
│       ├── api/
│       │   └── v1/
│       │       ├── proof.proto             # Proof request/response
│       │       └── status.proto            # Status messages
│       └── coordinator/
│           └── v1/
│               ├── task.proto              # Task messages
│               └── heartbeat.proto         # Heartbeat messages
│
├── pkg/                                    # Public libraries (reusable)
│   └── client/
│       └── zkp_client.go                   # Client library for external users
│
├── api/                                    # API specifications
│   ├── openapi/
│   │   └── api.yaml                        # OpenAPI 3.0 spec
│   └── grpc/
│       └── *.proto                         # gRPC service definitions
│
├── configs/                                # Configuration files
│   ├── api-gateway.yaml
│   ├── coordinator.yaml
│   ├── worker.yaml
│   └── dev/                                # Development configs
│       └── *.yaml
│
├── deployments/                            # Deployment configurations
│   ├── docker/
│   │   ├── Dockerfile.api-gateway
│   │   ├── Dockerfile.coordinator
│   │   ├── Dockerfile.worker
│   │   └── docker-compose.yml              # Local development setup
│   ├── kubernetes/
│   │   ├── base/
│   │   │   ├── api-gateway/
│   │   │   │   ├── deployment.yaml
│   │   │   │   ├── service.yaml
│   │   │   │   └── hpa.yaml
│   │   │   ├── coordinator/
│   │   │   │   ├── statefulset.yaml
│   │   │   │   ├── service.yaml
│   │   │   │   └── headless-service.yaml
│   │   │   └── worker/
│   │   │       ├── deployment.yaml
│   │   │       ├── service.yaml
│   │   │       └── hpa.yaml
│   │   ├── overlays/
│   │   │   ├── dev/
│   │   │   ├── staging/
│   │   │   └── prod/
│   │   └── helm/
│   │       └── zkp-network/
│   │           ├── Chart.yaml
│   │           ├── values.yaml
│   │           └── templates/
│   └── terraform/
│       ├── modules/
│       │   ├── eks/                        # EKS cluster setup
│       │   ├── rds/                        # RDS PostgreSQL
│       │   └── elasticache/                # Redis ElastiCache
│       ├── environments/
│       │   ├── dev/
│       │   ├── staging/
│       │   └── prod/
│       └── main.tf
│
├── scripts/                                # Utility scripts
│   ├── setup.sh                            # Initial setup
│   ├── generate-protos.sh                  # Compile protobuf files
│   ├── run-migrations.sh                   # Database migrations
│   └── load-test.sh                        # Load testing script
│
├── test/                                   # Tests
│   ├── unit/                               # Unit tests
│   ├── integration/                        # Integration tests
│   │   ├── api_test.go
│   │   ├── coordinator_test.go
│   │   └── worker_test.go
│   ├── e2e/                                # End-to-end tests
│   │   └── proof_generation_test.go
│   └── testdata/                           # Test fixtures
│       ├── circuits/
│       └── proofs/
│
├── docs/                                   # Documentation
│   ├── architecture/
│   │   ├── system-design.md
│   │   ├── consensus.md
│   │   └── data-flow.md
│   ├── api/
│   │   └── endpoints.md
│   └── operations/
│       ├── deployment.md
│       └── monitoring.md
│
├── monitoring/                             # Monitoring configs
│   ├── prometheus/
│   │   └── prometheus.yml
│   ├── grafana/
│   │   └── dashboards/
│   │       ├── coordinator.json
│   │       ├── worker.json
│   │       └── system.json
│   └── alerting/
│       └── rules.yml
│
├── .github/
│   └── workflows/
│       ├── ci.yml                          # CI pipeline
│       ├── cd.yml                          # CD pipeline
│       └── security-scan.yml               # Security scanning
│
├── go.mod                                  # Go module definition
├── go.sum                                  # Dependency checksums
├── Makefile                                # Build automation
├── README.md                               # This file
├── .gitignore
├── .dockerignore
└── LICENSE

```

---

## Initial Setup Commands

```bash
# Initialize Go module
go mod init github.com/yourusername/zkp-distributed-network

# Install core dependencies
go get -u google.golang.org/grpc
go get -u google.golang.org/protobuf
go get -u github.com/consensys/gnark
go get -u github.com/gorilla/mux
go get -u github.com/lib/pq
go get -u github.com/go-redis/redis/v8
go get -u github.com/prometheus/client_golang/prometheus
go get -u go.uber.org/zap
go get -u github.com/spf13/viper
go get -u github.com/hashicorp/raft
go get -u github.com/opentracing/opentracing-go

# Development tools
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

## Key Technologies

### Core
- **Language**: Go 1.22+
- **ZKP Library**: gnark (consensys)
- **Consensus**: Raft (HashiCorp)
- **API**: gRPC + HTTP/2

### Data Layer
- **Primary DB**: PostgreSQL 15+
- **Cache**: Redis 7+
- **Blockchain**: Custom file-based chain

### Observability
- **Metrics**: Prometheus
- **Visualization**: Grafana
- **Tracing**: Jaeger
- **Logging**: Zap/Zerolog + ELK

### Infrastructure
- **Containers**: Docker
- **Orchestration**: Kubernetes
- **IaC**: Terraform
- **CI/CD**: GitHub Actions

---

## Development Workflow

### 1. Local Development
```bash
# Start dependencies
make dev-up

# Run API Gateway
make run-api-gateway

# Run Coordinator (3 nodes for Raft quorum)
make run-coordinator-1
make run-coordinator-2
make run-coordinator-3

# Run Workers
make run-worker
```

### 2. Testing
```bash
# Unit tests
make test-unit

# Integration tests
make test-integration

# E2E tests
make test-e2e

# Coverage
make test-coverage
```

### 3. Building
```bash
# Build all services
make build

# Build Docker images
make docker-build

# Push to registry
make docker-push
```

---

## Learning Path

### Phase 1: Foundations (Weeks 1-2)
- [ ] Project structure setup
- [ ] Basic HTTP API gateway
- [ ] Simple ZKP generation with gnark
- [ ] PostgreSQL integration
- [ ] Unit testing framework

### Phase 2: Concurrency (Weeks 3-4)
- [ ] Worker pool implementation
- [ ] Channel-based communication
- [ ] Context for cancellation
- [ ] Goroutine coordination
- [ ] Fan-out/fan-in patterns

### Phase 3: Distributed Systems (Weeks 5-7)
- [ ] Raft consensus implementation
- [ ] Leader election
- [ ] Distributed task scheduling
- [ ] Heartbeat protocol
- [ ] Failure detection and recovery

### Phase 4: Production Patterns (Weeks 8-9)
- [ ] Circuit breaker
- [ ] Retry with exponential backoff
- [ ] Graceful shutdown
- [ ] Configuration management
- [ ] Error handling patterns

### Phase 5: DevOps (Weeks 10-12)
- [ ] Dockerfile optimization
- [ ] Kubernetes deployment
- [ ] Helm charts
- [ ] Prometheus metrics
- [ ] Grafana dashboards
- [ ] CI/CD pipeline

---

## Configuration Example

### `configs/coordinator.yaml`
```yaml
server:
  grpc_port: 9090
  http_port: 8080
  
raft:
  node_id: "coord-1"
  bind_addr: "0.0.0.0:7000"
  peers:
    - "coord-1:7000"
    - "coord-2:7000"
    - "coord-3:7000"
  data_dir: "/var/lib/zkp/raft"
  
scheduler:
  max_concurrent_tasks: 100
  task_timeout: 300s
  
database:
  host: "postgres"
  port: 5432
  database: "zkp_network"
  user: "zkp_user"
  
redis:
  addr: "redis:6379"
  db: 0
  
observability:
  prometheus_port: 9091
  jaeger_endpoint: "http://jaeger:14268/api/traces"
  log_level: "info"
```

---

## API Examples

### Submit Proof Generation Request
```bash
curl -X POST http://localhost:8080/api/v1/proofs \
  -H "Content-Type: application/json" \
  -d '{
    "circuit_type": "merkle_proof",
    "inputs": {
      "leaf": "0x123...",
      "path": ["0x456...", "0x789..."]
    },
    "priority": "normal"
  }'
```

Response:
```json
{
  "task_id": "task_abc123",
  "status": "pending",
  "submitted_at": "2025-10-22T10:30:00Z"
}
```

### Check Proof Status
```bash
curl http://localhost:8080/api/v1/proofs/task_abc123
```

---

## Next Steps

1. **Set up the project structure** using the layout above
2. **Create initial configurations** in `configs/`
3. **Implement API Gateway** with basic handlers
4. **Build ZKP module** with gnark integration
5. **Add PostgreSQL** for task persistence
6. **Implement worker pool** with concurrency patterns
7. **Add Raft consensus** to coordinators
8. **Set up observability** stack
9. **Containerize** all services
10. **Deploy to Kubernetes**

---

## Resources

- [gnark Documentation](https://docs.gnark.consensys.net/)
- [Raft Consensus](https://raft.github.io/)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [Kubernetes Best Practices](https://kubernetes.io/docs/concepts/)
- [Prometheus Monitoring](https://prometheus.io/docs/introduction/overview/)

---

## Contributing

This is a learning project. Feel free to experiment, break things, and learn from failures!

## License

MIT License - See LICENSE file for details