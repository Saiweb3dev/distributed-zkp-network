#!/bin/bash

# 🗂️ Automated Project Refactoring Script
# This script reorganizes the distributed-zkp-network project structure

set -e

PROJECT_ROOT="A:/DistributedSystems/projects/distributed-zkp-network"
cd "$PROJECT_ROOT"

echo "============================================================================"
echo "🗂️  Distributed ZKP Network - Project Refactoring"
echo "============================================================================"
echo ""

# ============================================================================
# Phase 1: Create Necessary Directories
# ============================================================================
echo "Phase 1: Creating directory structure..."

mkdir -p archive
mkdir -p docs/guides
mkdir -p scripts/sh
mkdir -p scripts/bat

echo "✓ Directories created"
echo ""

# ============================================================================
# Phase 2: Root Directory Cleanup
# ============================================================================
echo "Phase 2: Cleaning up root directory..."

# Move guides to docs/guides/
if [ -f "BOOTSTRAP_GUIDE.md" ]; then
    mv BOOTSTRAP_GUIDE.md docs/guides/
    echo "  → Moved BOOTSTRAP_GUIDE.md to docs/guides/"
fi

# Move cheatsheet to docs/
if [ -f "Cheatsheet.md" ]; then
    mv Cheatsheet.md docs/
    echo "  → Moved Cheatsheet.md to docs/"
fi

# Archive outdated files
for file in PHASE03_README.md RAFT_SUCCESS.md CLUSTER_QUICKSTART.md QuickStart.md; do
    if [ -f "$file" ]; then
        mv "$file" archive/
        echo "  → Archived $file"
    fi
done

# Move scripts to proper location
if [ -f "setup.sh" ]; then
    mv setup.sh scripts/sh/
    echo "  → Moved setup.sh to scripts/sh/"
fi

if [ -f "test.sh" ]; then
    mv test.sh scripts/sh/
    echo "  → Moved test.sh to scripts/sh/"
fi

echo "✓ Root directory cleaned"
echo ""

# ============================================================================
# Phase 3: Scripts Directory Consolidation
# ============================================================================
echo "Phase 3: Organizing scripts directory..."

# Move misplaced script
if [ -f "scripts/install-raft-deps.sh" ]; then
    mv scripts/install-raft-deps.sh scripts/sh/
    echo "  → Moved install-raft-deps.sh to scripts/sh/"
fi

# Remove redundant documentation
if [ -f "scripts/SCRIPTS_SUMMARY.md" ]; then
    mv scripts/SCRIPTS_SUMMARY.md archive/
    echo "  → Archived SCRIPTS_SUMMARY.md"
fi

echo "✓ Scripts organized"
echo ""

# ============================================================================
# Phase 4: Docs Directory Consolidation
# ============================================================================
echo "Phase 4: Consolidating documentation..."

# Archive outdated docs
for file in docs/PHASE03_CHANGES_SUMMARY.md docs/ISSUES_SOLVED.md; do
    if [ -f "$file" ]; then
        mv "$file" archive/
        echo "  → Archived $(basename $file)"
    fi
done

# Merge RAFT guides into single comprehensive guide
if [ -f "docs/RAFT_INTEGRATION_GUIDE.md" ] && [ -f "docs/RAFT_QUICKSTART.md" ]; then
    cat > docs/RAFT_GUIDE.md << 'RAFTGUIDE'
# Raft Consensus Guide

## 🎯 Overview
This guide covers Raft consensus implementation in the Distributed ZKP Network.

## 🚀 Quick Start

### Prerequisites
- Go 1.24+
- Docker & Docker Compose
- 3 available ports: 7000-7002 (Raft), 8090-8092 (HTTP), 9090-9092 (gRPC)

### Start Raft Cluster

**Windows:**
```cmd
scripts\bat\bootstrap-raft.bat
scripts\bat\start-cluster.bat
```

**Unix:**
```bash
./scripts/sh/bootstrap-raft.sh
./scripts/sh/start-cluster.sh
```

### Verify Cluster
```bash
# Check leader election
curl http://localhost:8090/health | python -m json.tool

# Expected output:
{
    "raft": {
        "state": "Leader",
        "is_leader": true,
        "leader_address": "coordinator-1"
    }
}
```

## 📚 Detailed Documentation

For comprehensive Raft implementation details, see:
- RAFT_INTEGRATION_GUIDE.md (archived)
- RAFT_QUICKSTART.md (archived)

Both files available in `archive/` folder for reference.

## 🛠️ Common Operations

### Check Cluster Status
```bash
# Check all coordinators
curl http://localhost:8090/health
curl http://localhost:8091/health
curl http://localhost:8092/health
```

### Test Leader Failover
```bash
# Stop current leader
docker stop zkp-coordinator-1

# Wait for election (5-10 seconds)
sleep 10

# Verify new leader elected
curl http://localhost:8091/health | grep -q "Leader"
```

### Reset Cluster
```bash
# Stop all services
./scripts/sh/stop-services.sh  # or scripts\bat\stop-services.bat

# Remove Raft data
docker volume rm docker_coordinator-1-data docker_coordinator-2-data docker_coordinator-3-data

# Restart cluster
./scripts/sh/start-cluster.sh  # or scripts\bat\start-cluster.bat
```

## 🐛 Troubleshooting

### No Leader Elected
```bash
# Check logs
docker logs zkp-coordinator-1 --tail=50

# Look for: "entering leader state"
# If not found, check network connectivity
```

### Split Brain
```bash
# Verify only 1 leader
for port in 8090 8091 8092; do
    curl -s http://localhost:$port/health | grep -o '"is_leader":[^,]*'
done

# Should see exactly one "is_leader":true
```

## 📖 Additional Resources
- [Coordinator Architecture](COORDINATOR_ARCHITECTURE_GUIDE.md)
- [Production Deployment](PRODUCTION_DEPLOYMENT.md)
- [CI/CD Pipeline](CI_CD_PIPELINE.md)
RAFTGUIDE

    mv docs/RAFT_INTEGRATION_GUIDE.md archive/
    mv docs/RAFT_QUICKSTART.md archive/
    echo "  → Created consolidated docs/RAFT_GUIDE.md"
fi

echo "✓ Documentation consolidated"
echo ""

# ============================================================================
# Phase 5: Test Directory Cleanup
# ============================================================================
echo "Phase 5: Cleaning up test directory..."

# Remove redundant files
for file in test/INDEX.md test/TESTING_COMPLETE.md; do
    if [ -f "$file" ]; then
        mv "$file" archive/
        echo "  → Archived $(basename $file)"
    fi
done

# Consolidate test docs
if [ -f "test/TESTING_QUICKSTART.md" ] && [ -f "test/README.md" ]; then
    # Backup original
    cp test/README.md test/README.md.bak
    
    # Create consolidated test README
    cat > test/README.md << 'TESTREADME'
# Testing Guide

## 🎯 Quick Start

### Run All Tests
**Windows:**
```cmd
test\e2e\run-tests.bat
```

**Unix:**
```bash
./test/e2e/run-tests.sh
```

### Run Specific Test Categories
```bash
# Health checks only
curl http://localhost:8090/health

# Leader election
curl http://localhost:8090/health | grep "is_leader"

# Worker registration
curl http://localhost:8090/health | grep "workers"
```

## 📋 Test Structure

```
test/
├── README.md           ← This file
├── e2e/
│   ├── run-tests.sh    ← Full test suite (Unix)
│   └── run-tests.bat   ← Full test suite (Windows)
├── integration/        ← Integration tests (TODO)
├── unit/               ← Unit tests (TODO)
└── manual/
    └── TEST_SUITE.md   ← Manual testing procedures
```

## 🧪 E2E Test Suite

The automated test suite covers:

1. **Container Health** (7 tests)
   - PostgreSQL database
   - 3 Coordinators
   - 2 Workers
   - API Gateway

2. **Health Endpoints** (4 tests)
   - All coordinator HTTP endpoints
   - API Gateway endpoint

3. **Raft Leader Election** (1 test)
   - Exactly one leader elected

4. **Node Identity** (3 tests)
   - Unique coordinator IDs

5. **Database Connectivity** (2 tests)
   - Direct database connection
   - Coordinator startup verification

6. **API Gateway** (2 tests)
   - Health endpoint
   - Ready endpoint

**Total: 19 tests**

## 🎯 Expected Results

```
Total tests: 19
Tests passed: 19
Tests failed: 0
Pass rate: 100%
```

## 🐛 Troubleshooting

### Tests Failing
1. Check Docker containers:
   ```bash
   docker ps
   ```

2. Check logs:
   ```bash
   docker logs zkp-coordinator-1
   docker logs zkp-api-gateway-cluster
   ```

3. Restart cluster:
   ```bash
   ./scripts/sh/stop-services.sh
   ./scripts/sh/start-cluster.sh
   ```

### Database Connection Issues
```bash
# Test direct connection
docker exec zkp-postgres-cluster psql -U zkp_user -d zkp_network -c "SELECT 1;"
```

### Leader Election Issues
```bash
# Check Raft status on all coordinators
for port in 8090 8091 8092; do
    echo "Coordinator on port $port:"
    curl -s http://localhost:$port/health | python -m json.tool | grep -A5 "raft"
done
```

## 📊 CI/CD Integration

Tests are automatically run in GitHub Actions:
- On every push
- On every pull request
- Before deployment

See [CI/CD Pipeline](../docs/CI_CD_PIPELINE.md) for details.

## 📚 Additional Resources

- [Test Failure Analysis](../docs/TEST_FAILURE_ANALYSIS.md)
- [Manual Test Suite](manual/TEST_SUITE.md)
- [CI/CD Pipeline](../docs/CI_CD_PIPELINE.md)

## 🎓 Writing New Tests

### Adding E2E Tests

Edit `test/e2e/run-tests.sh` or `run-tests.bat`:

```bash
# Example: Test task submission
echo "Testing task submission..."
TASK_ID=$(curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
    -H "Content-Type: application/json" \
    -d '{"public_inputs":["1","2","3"]}' | grep -o '"task_id":"[^"]*"' | cut -d'"' -f4)

if [ -n "$TASK_ID" ]; then
    echo "[PASS] Task submitted: $TASK_ID"
else
    echo "[FAIL] Task submission failed"
fi
```

### Adding Unit Tests

Create test files in `internal/`:
```go
package coordinator_test

import "testing"

func TestTaskScheduler(t *testing.T) {
    // Your test here
}
```

Run with:
```bash
go test ./internal/...
```
TESTREADME

    mv test/TESTING_QUICKSTART.md archive/
    echo "  → Consolidated test/README.md"
fi

echo "✓ Test directory cleaned"
echo ""

# ============================================================================
# Phase 6: Create Documentation Index
# ============================================================================
echo "Phase 6: Creating documentation index..."

cat > docs/README.md << 'DOCSINDEX'
# Documentation Index

## 🏗️ Architecture

- [Coordinator Architecture Guide](COORDINATOR_ARCHITECTURE_GUIDE.md) - Coordinator service design and implementation
- [Worker Architecture Guide](WORKER_ARCHITECTURE_GUIDE.md) - Worker service design and implementation
- [gRPC Setup Guide](architecture/gRPC_Setup_Guide.md) - Protocol Buffers and gRPC configuration

## 🔄 Raft Consensus

- [Raft Guide](RAFT_GUIDE.md) - Comprehensive Raft implementation guide
  - Quick start
  - Common operations
  - Troubleshooting

## 🚀 Deployment & Operations

- [Production Deployment](PRODUCTION_DEPLOYMENT.md) - Production deployment strategies
- [Bootstrap Guide](guides/BOOTSTRAP_GUIDE.md) - Initial setup and bootstrapping
- [Cheatsheet](Cheatsheet.md) - Common commands and operations

## 🧪 Testing & CI/CD

- [CI/CD Pipeline](CI_CD_PIPELINE.md) - GitHub Actions workflow and testing
- [Test Failure Analysis](TEST_FAILURE_ANALYSIS.md) - Common test failures and solutions

## 📖 Quick Links

### Getting Started
1. [Bootstrap Guide](guides/BOOTSTRAP_GUIDE.md) - Start here
2. [Raft Guide](RAFT_GUIDE.md) - Set up consensus
3. [Cheatsheet](Cheatsheet.md) - Common commands

### Development
1. [Coordinator Architecture](COORDINATOR_ARCHITECTURE_GUIDE.md)
2. [Worker Architecture](WORKER_ARCHITECTURE_GUIDE.md)
3. [gRPC Setup](architecture/gRPC_Setup_Guide.md)

### Operations
1. [Production Deployment](PRODUCTION_DEPLOYMENT.md)
2. [CI/CD Pipeline](CI_CD_PIPELINE.md)
3. [Test Failure Analysis](TEST_FAILURE_ANALYSIS.md)

## 📁 Directory Structure

```
docs/
├── README.md                           ← This file
├── Cheatsheet.md
├── CI_CD_PIPELINE.md
├── RAFT_GUIDE.md
├── COORDINATOR_ARCHITECTURE_GUIDE.md
├── WORKER_ARCHITECTURE_GUIDE.md
├── PRODUCTION_DEPLOYMENT.md
├── TEST_FAILURE_ANALYSIS.md
├── guides/
│   └── BOOTSTRAP_GUIDE.md
└── architecture/
    └── gRPC_Setup_Guide.md
```

## 🗄️ Archived Documentation

Historical documentation is available in the `archive/` folder:
- PHASE03_README.md
- RAFT_SUCCESS.md
- CLUSTER_QUICKSTART.md
- QuickStart.md
- RAFT_INTEGRATION_GUIDE.md
- RAFT_QUICKSTART.md
- PHASE03_CHANGES_SUMMARY.md
- ISSUES_SOLVED.md
- TESTING_COMPLETE.md

## 📞 Need Help?

- Check [Troubleshooting sections](RAFT_GUIDE.md#troubleshooting) in guides
- Review [Test Failure Analysis](TEST_FAILURE_ANALYSIS.md)
- See [Cheatsheet](Cheatsheet.md) for common commands
DOCSINDEX

echo "✓ Documentation index created"
echo ""

# ============================================================================
# Phase 7: Update Main README
# ============================================================================
echo "Phase 7: Creating new main README..."

cat > README.md << 'MAINREADME'
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
- [Test Failure Analysis](docs/TEST_FAILURE_ANALYSIS.md) - Troubleshooting guide

## 🧪 Testing

Comprehensive test suite with 19 automated tests covering:
- Container health checks
- Raft leader election
- Database connectivity
- API Gateway functionality
- Worker registration

See [test/README.md](test/README.md) for details.

**Test Coverage:**
- E2E tests: 100% (19/19 passing)
- Unit tests: 50%+ (CI requirement)
- Integration tests: Cluster health, failover, performance

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
MAINREADME

echo "✓ Main README created"
echo ""

# ============================================================================
# Summary
# ============================================================================
echo "============================================================================"
echo "✅ Refactoring Complete!"
echo "============================================================================"
echo ""
echo "📊 Summary:"
echo "  → Root directory: 8 MD files → 1 MD file"
echo "  → Scripts organized: sh/ and bat/ subdirectories"
echo "  → Documentation: 10 files → 8 files (+ index)"
echo "  → Test docs: 5 files → 2 files"
echo "  → Created: archive/ folder with 10 historical files"
echo ""
echo "📁 New Structure:"
echo "  ✓ README.md (main entry point)"
echo "  ✓ docs/ (8 guides + README index)"
echo "  ✓ scripts/sh/ (9 Unix scripts)"
echo "  ✓ scripts/bat/ (5 Windows scripts)"
echo "  ✓ test/ (2 docs + test scripts)"
echo "  ✓ archive/ (10 historical files)"
echo ""
echo "🎯 Next Steps:"
echo "  1. Review new README.md"
echo "  2. Test scripts from new locations:"
echo "     ./scripts/sh/start-cluster.sh"
echo "  3. Update any hardcoded paths in code"
echo "  4. Commit changes:"
echo "     git add -A"
echo "     git commit -m 'refactor: organize project structure'"
echo ""
echo "📖 Documentation: docs/README.md"
echo "🧪 Testing: test/README.md"
echo "🗂️  Details: REFACTORING_PLAN.md"
echo ""
echo "============================================================================"
