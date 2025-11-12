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
