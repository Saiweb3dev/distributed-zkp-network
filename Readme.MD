# Distributed ZKP Network

A distributed zero-knowledge proof (ZKP) computation network with Raft consensus, designed for scalable and fault-tolerant ZKP generation.

## 🚀 Quick Start

### Prerequisites
- Go 1.24+
- Docker & Docker Compose
- Make

### Development Mode (Single Node)
```bash
make dev
```

### Production Cluster (3 Coordinators, 2 Workers)
```bash
# Windows
scripts\bat\bootstrap-raft.bat
scripts\bat\start-cluster.bat

# Unix
./scripts/sh/bootstrap-raft.sh
./scripts/sh/start-cluster.sh
```

### Run Tests
```bash
# Windows
test\e2e\run-tests.bat

# Unix
./test/e2e/run-tests.sh
```

## 📚 Documentation

- **[Documentation Index](docs/README.md)** - Complete documentation catalog

### Architecture
- [Coordinator Architecture](docs/COORDINATOR_ARCHITECTURE_GUIDE.md) - Leader election, task scheduling
- [Worker Architecture](docs/WORKER_ARCHITECTURE_GUIDE.md) - Task execution, proof generation
- [gRPC Setup](docs/architecture/gRPC_Setup_Guide.md) - Protocol Buffers configuration

### Getting Started
- [Bootstrap Guide](docs/guides/BOOTSTRAP_GUIDE.md) - Initial setup and configuration
- [Raft Guide](docs/RAFT_GUIDE.md) - Consensus implementation and operations
- [Cheatsheet](docs/Cheatsheet.md) - Common commands and workflows

### Operations
- [Production Deployment](docs/PRODUCTION_DEPLOYMENT.md) - Production setup strategies
- [CI/CD Pipeline](docs/CI_CD_PIPELINE.md) - Automated testing and deployment
- [CI/CD Local Testing](docs/CI_CD_LOCAL_TESTING.md) - Run GitHub Actions locally
- [Quick CI Commands](docs/QUICK_CI_COMMANDS.md) - Fast reference for pre-push checks
- [Test Failure Analysis](docs/TEST_FAILURE_ANALYSIS.md) - Troubleshooting guide

## 🧪 Testing

Comprehensive test suite with **31 automated tests** covering:
- Container health checks (7 tests)
- Raft leader election (3 tests)
- Database connectivity (2 tests)
- API Gateway functionality (6 tests)
- Worker registration (1 test)
- **Redis Pub/Sub system (12 tests)** - New in Phase 03

See [test/README.md](test/README.md) for details.

**Test Coverage:**
- E2E tests: 100% (31/31 passing)
- Unit tests: 50%+ (CI requirement)
- Integration tests: Cluster health, failover, performance
- Redis tests: Pub/Sub, event-driven architecture

### Run Tests Locally Before Pushing
```bash
# Quick pre-push checks (2 minutes)
make pre-push

# Full CI/CD pipeline (15-20 minutes)
make ci-local
```

See [docs/QUICK_CI_COMMANDS.md](docs/QUICK_CI_COMMANDS.md) for more options.

## 🏗️ Architecture

```
┌─────────────────┐
│   API Gateway   │  ← Client requests
│   (Port 8080)   │
└────────┬────────┘
         │
    ┌────▼─────────────────┐
    │  Coordinator Cluster │
    │   (Raft Consensus)   │
    ├──────────────────────┤
    │ • Leader: Task queue │
    │ • Followers: Standby │
    │ • Ports: 8090-8092   │
    └────────┬─────────────┘
             │
    ┌────────▼────────┐
    │  Worker Pool    │
    │ • Execute proofs│
    │ • Report status │
    └─────────────────┘
```

## 🛠️ Development

### Project Structure
```
distributed-zkp-network/
├── cmd/                    ← Service entry points
│   ├── api-gateway/
│   ├── coordinator/
│   └── worker/
├── internal/               ← Application code
│   ├── api/
│   ├── coordinator/
│   ├── worker/
│   └── proto/
├── scripts/                ← Automation
│   ├── sh/                 ← Unix scripts
│   └── bat/                ← Windows scripts
├── test/                   ← Test suites
│   ├── e2e/
│   └── manual/
├── docs/                   ← Documentation
└── deployments/            ← Docker configs
```

### Common Commands

```bash
# Build all services
make build

# Run tests
make test

# Start cluster
make cluster

# Stop all services
make stop

# Clean artifacts
make clean
```

### Script Locations

**Unix/macOS:** `scripts/sh/`
- bootstrap-raft.sh
- start-cluster.sh
- stop-services.sh
- test-services.sh
- setup.sh

**Windows:** `scripts/bat/`
- bootstrap-raft.bat
- start-cluster.bat
- stop-services.bat
- test-services.bat

## 🔄 Raft Consensus

The system uses Raft for leader election and fault tolerance:

- **3 Coordinators:** High availability cluster
- **Leader Election:** Automatic failover (< 10 seconds)
- **Log Replication:** Consistent task state
- **Network Partition:** Quorum-based decisions

```bash
# Check cluster status
curl http://localhost:8090/health | python -m json.tool

# Verify leader
curl http://localhost:8090/health | grep "is_leader"
```

## 🚀 Deployment

### Docker Compose (Development)
```bash
docker-compose -f deployments/docker/docker-compose-cluster.yml up
```

### Kubernetes (Production)
See [Production Deployment Guide](docs/PRODUCTION_DEPLOYMENT.md)

### CI/CD
GitHub Actions workflow:
- Code quality checks
- Unit tests (50%+ coverage)
- Integration tests
- E2E task flow validation
- Performance benchmarks
- Automated deployment

## 📊 Monitoring

Access observability endpoints:
- **Coordinator Health:** http://localhost:8090/health
- **API Gateway:** http://localhost:8080/health
- **Metrics:** (Coming soon - Prometheus)

## 🐛 Troubleshooting

### Cluster Won't Start
```bash
# Check Docker containers
docker ps

# Check logs
docker logs zkp-coordinator-1

# Reset cluster
./scripts/sh/stop-services.sh
docker volume prune -f
./scripts/sh/start-cluster.sh
```

### No Leader Elected
```bash
# Verify network connectivity
docker network inspect docker_zkp-network

# Check Raft logs
docker logs zkp-coordinator-1 | grep -i "raft"
```

### Tests Failing
See [Test Failure Analysis](docs/TEST_FAILURE_ANALYSIS.md)

## 📖 Additional Resources

- [Documentation Index](docs/README.md) - All documentation
- [Cheatsheet](docs/Cheatsheet.md) - Quick command reference
- [CI/CD Pipeline](docs/CI_CD_PIPELINE.md) - Workflow details

## 📝 License

MIT License - See LICENSE file

## 🤝 Contributing

1. Fork the repository
2. Create feature branch
3. Run tests: `make test`
4. Submit pull request

## 📞 Support

- Issues: GitHub Issues
- Docs: [docs/README.md](docs/README.md)
- Tests: [test/README.md](test/README.md)
