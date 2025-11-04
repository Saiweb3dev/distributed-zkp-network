# Testing Quick Reference Guide

## 🚀 Quick Start

### Run Automated Tests

**Linux/Mac:**
```bash
chmod +x test/e2e/run-tests.sh
./test/e2e/run-tests.sh
```

**Windows:**
```cmd
test\e2e\run-tests.bat
```

### Manual Testing

See full guide: `test/manual/TEST_SUITE.md`

---

## 📊 Common Test Commands

### 1. Check Cluster Status
```bash
# All services
docker ps --filter "name=zkp-"

# Leader status
curl -s http://localhost:8090/health | grep "is_leader"
curl -s http://localhost:8091/health | grep "is_leader"
curl -s http://localhost:8092/health | grep "is_leader"
```

### 2. Submit Test Task
```bash
# Submit proof request
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{
    "circuit_type": "test",
    "public_inputs": ["1", "2", "3"],
    "private_inputs": ["secret"]
  }'

# Response: {"task_id": "...", "status": "pending"}
```

### 3. Check Task Status
```bash
# Replace <task_id> with actual ID
curl http://localhost:8080/api/v1/proof/<task_id>
```

### 4. Test Leader Failover
```bash
# Find current leader
LEADER=$(curl -s http://localhost:8090/health | grep -o '"is_leader":true' && echo "coordinator-1" || echo "coordinator-2")

# Stop leader
docker stop zkp-$LEADER

# Wait for new election (5 seconds)
sleep 5

# Check new leader
curl http://localhost:8091/health | grep "is_leader"
curl http://localhost:8092/health | grep "is_leader"

# Restart old leader
docker start zkp-$LEADER
```

### 5. Check Logs
```bash
# Coordinator logs (last 50 lines)
docker logs zkp-coordinator-1 --tail=50

# Follow logs in real-time
docker logs -f zkp-coordinator-1

# Filter for specific events
docker logs zkp-coordinator-1 | grep -i "leader\|election\|error"

# Worker logs
docker logs zkp-worker-1-cluster --tail=50

# API Gateway logs
docker logs zkp-api-gateway-cluster --tail=50
```

### 6. Database Queries
```bash
# Connect to database
docker exec -it zkp-postgres-cluster psql -U zkp_user -d zkp_network

# List all tasks
SELECT id, status, circuit_type, created_at FROM tasks ORDER BY created_at DESC LIMIT 10;

# Count tasks by status
SELECT status, COUNT(*) FROM tasks GROUP BY status;

# Exit: \q
```

### 7. Monitor Resources
```bash
# Container stats (CPU, Memory)
docker stats --no-stream

# Continuous monitoring
docker stats

# Specific container
docker stats zkp-coordinator-1
```

### 8. Restart Cluster
```bash
# Stop all
cd a:/DistributedSystems/projects/distributed-zkp-network
docker-compose -f deployments/docker/docker-compose-cluster.yml down

# Clear Raft data (fresh start)
docker volume rm docker_coordinator-1-data docker_coordinator-2-data docker_coordinator-3-data

# Start all
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d

# Check startup
sleep 10
docker ps
```

---

## 🔍 Troubleshooting

### Issue: No Leader Elected

**Check:**
```bash
# Verify all coordinators running
docker ps --filter "name=coordinator"

# Check Raft logs
docker logs zkp-coordinator-1 | grep -i "election\|term"

# Verify unique IDs
curl -s http://localhost:8090/health | grep "coordinator_id"
curl -s http://localhost:8091/health | grep "coordinator_id"
curl -s http://localhost:8092/health | grep "coordinator_id"
```

**Fix:**
- Ensure all 3 coordinators are running
- Verify environment variables: `COORDINATOR_COORDINATOR_ID`
- Check for network connectivity between containers

---

### Issue: Workers Not Registering

**Check:**
```bash
# Worker logs
docker logs zkp-worker-1-cluster --tail=50

# Coordinator logs
docker logs zkp-coordinator-1 | grep -i "worker\|register"

# Network connectivity
docker exec zkp-worker-1-cluster ping coordinator-1
```

**Fix:**
- Verify coordinator gRPC port (9090) is accessible
- Check worker configuration: `WORKER_WORKER_COORDINATOR_ADDRESS`
- Ensure leader is elected before workers start

---

### Issue: Database Connection Failed

**Check:**
```bash
# Postgres status
docker ps --filter "name=postgres"

# Connection test
docker exec zkp-postgres-cluster psql -U zkp_user -d zkp_network -c "SELECT 1;"

# Coordinator logs
docker logs zkp-coordinator-1 | grep -i "database\|postgres"
```

**Fix:**
- Verify environment variables: `COORDINATOR_DATABASE_USER`, `COORDINATOR_DATABASE_PASSWORD`
- Check postgres is healthy: `docker ps` shows "healthy" status
- Ensure zkp_user/zkp_password credentials match

---

### Issue: High Memory Usage

**Check:**
```bash
# Memory stats
docker stats --no-stream --format "table {{.Name}}\t{{.MemUsage}}"

# Container logs for leaks
docker logs zkp-coordinator-1 | grep -i "memory\|oom"
```

**Fix:**
- Restart container: `docker restart zkp-coordinator-1`
- Check for large task backlogs
- Monitor over time to identify leaks

---

### Issue: API Gateway Not Responding

**Check:**
```bash
# Container status
docker ps --filter "name=api-gateway"

# Logs
docker logs zkp-api-gateway-cluster --tail=50

# Port availability
curl http://localhost:8080/health
```

**Fix:**
- Verify port 8080 not in use by another process
- Check database connection (API gateway needs DB)
- Restart: `docker restart zkp-api-gateway-cluster`

---

## 📈 Performance Benchmarks

### Expected Values

| Metric | Good | Acceptable | Poor |
|--------|------|------------|------|
| Health endpoint response | < 50ms | < 200ms | > 500ms |
| Leader election time | 2-3s | 3-5s | > 10s |
| Task submission rate | > 100/s | > 50/s | < 20/s |
| Worker registration time | < 5s | < 10s | > 20s |
| Memory per coordinator | < 300MB | < 500MB | > 1GB |
| Raft term stability | No change for 1hr | < 5 changes/hr | Constant |

### Run Performance Tests

**Response Time:**
```bash
# 100 samples
for i in {1..100}; do
  curl -s -o /dev/null -w "%{time_total}\n" http://localhost:8090/health
done | awk '{sum+=$1; count++} END {print "Average:", sum/count, "seconds"}'
```

**Throughput:**
```bash
# Submit 1000 tasks
time for i in {1..1000}; do
  curl -s -X POST http://localhost:8080/api/v1/proof \
    -H "Content-Type: application/json" \
    -d '{"circuit_type":"test","public_inputs":["'$i'"]}' > /dev/null
done
```

**Load Test:**
```bash
# Concurrent requests (requires 'ab' tool)
ab -n 1000 -c 10 http://localhost:8090/health
```

---

## 🎯 Test Scenarios

### Scenario 1: Basic Smoke Test
```bash
# 1. Check all services running
docker ps --filter "name=zkp-"

# 2. Verify leader elected
curl http://localhost:8090/health | grep "is_leader"

# 3. Submit test task
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type":"smoke_test","public_inputs":["test"]}'

# 4. Check workers registered
curl http://localhost:8090/health | grep "workers"

# ✓ Pass if: All containers up, 1 leader, task accepted, 2 workers
```

### Scenario 2: Failover Test
```bash
# 1. Submit task
TASK_ID=$(curl -s -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type":"failover_test","public_inputs":["1"]}' | grep -o '"task_id":"[^"]*"' | cut -d'"' -f4)

# 2. Kill leader
docker stop zkp-coordinator-1

# 3. Wait for election
sleep 5

# 4. Check task status (should still work)
curl http://localhost:8080/api/v1/proof/$TASK_ID

# 5. Restart leader
docker start zkp-coordinator-1

# ✓ Pass if: New leader elected, task still tracked, old leader rejoins
```

### Scenario 3: Scale Test
```bash
# 1. Submit 100 tasks rapidly
for i in {1..100}; do
  curl -s -X POST http://localhost:8080/api/v1/proof \
    -H "Content-Type: application/json" \
    -d '{"circuit_type":"scale_test","public_inputs":["'$i'"]}' > /dev/null &
done
wait

# 2. Check cluster health
curl http://localhost:8090/health

# 3. Monitor task distribution
docker logs zkp-coordinator-1 --tail=50 | grep "assigned"

# ✓ Pass if: All tasks accepted, distributed across workers, no errors
```

---

## 📝 Test Checklist

Before releasing to production:

- [ ] All automated tests pass (`run-tests.sh`)
- [ ] Leader election working (Test 2.1)
- [ ] Failover working (Test 5.1)
- [ ] Workers registering (Test 3.1)
- [ ] Tasks executing (Test 4.1-4.4)
- [ ] Database persistence (Test 7.2)
- [ ] No memory leaks (Test 9.3)
- [ ] Response time acceptable (Test 9.2)
- [ ] Logs clean (no errors/warnings)
- [ ] Documentation updated
- [ ] Performance benchmarks meet targets
- [ ] Security review completed (TLS, auth)
- [ ] Monitoring configured (Prometheus/Grafana)
- [ ] Backup strategy defined

---

## 🔗 Resources

- **Full Test Suite**: `test/manual/TEST_SUITE.md`
- **Automated Tests**: `test/e2e/run-tests.sh`
- **Raft Guide**: `docs/RAFT_INTEGRATION_GUIDE.md`
- **Production Guide**: `docs/PRODUCTION_DEPLOYMENT.md`
- **Issues Solved**: `docs/ISSUES_SOLVED.md`
- **Success Summary**: `RAFT_SUCCESS.md`
