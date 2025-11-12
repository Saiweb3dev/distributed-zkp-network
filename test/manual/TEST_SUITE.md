# Manual Test Suite - Distributed ZKP Network

## 📋 Overview

This document provides comprehensive manual test cases for the distributed ZKP network with Raft consensus.

**Test Categories:**

1. Basic Health Checks
2. Raft Cluster Operations
3. Worker Management
4. Task Distribution
5. Leader Failover
6. Network Partitioning
7. Database Operations
8. API Gateway Tests
9. Performance Tests
10. Error Handling

---

## 🔧 Prerequisites

### Running Cluster

```bash
cd a:/DistributedSystems/projects/distributed-zkp-network
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d
```

### Required Tools

- `curl` - HTTP requests
- `docker` - Container management
- `python -m json.tool` or `jq` - JSON formatting
- `bash` - Script execution

---

## 1️⃣ Basic Health Checks

### Test 1.1: All Services Running

**Objective:** Verify all containers are up and healthy

```bash
# Check all containers
docker ps --filter "name=zkp-" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Expected output: 7 containers running
# - zkp-postgres-cluster
# - zkp-coordinator-1
# - zkp-coordinator-2
# - zkp-coordinator-3
# - zkp-worker-1-cluster
# - zkp-worker-2-cluster
# - zkp-api-gateway-cluster
```

**✅ Pass Criteria:**

- All 7 containers show "Up" status
- Postgres shows "healthy"
- No restart loops

---

### Test 1.2: Coordinator Health Endpoints

**Objective:** Verify HTTP health endpoints respond

```bash
# Test coordinator-1 (leader)
curl http://localhost:8090/health

# Test coordinator-2
curl http://localhost:8091/health

# Test coordinator-3
curl http://localhost:8092/health
```

**✅ Pass Criteria:**

- HTTP 200 response
- JSON with `"status": "healthy"`
- One coordinator shows `"is_leader": true`
- Two coordinators show `"is_leader": false`

---

### Test 1.3: API Gateway Health

**Objective:** Verify API gateway is accessible

```bash
# Check API gateway health
curl http://localhost:8080/health

# Check API gateway readiness
curl http://localhost:8080/ready
```

**✅ Pass Criteria:**

- HTTP 200 response
- `"status": "healthy"`
- `"database": "connected"`

---

## 2️⃣ Raft Cluster Operations

### Test 2.1: Leader Election

**Objective:** Verify one leader elected and recognized by all

```bash
# Check who is the leader
echo "=== Leader Status ==="
curl -s http://localhost:8090/health | grep -o '"is_leader":[^,]*'
curl -s http://localhost:8091/health | grep -o '"is_leader":[^,]*'
curl -s http://localhost:8092/health | grep -o '"is_leader":[^,]*'

# Get leader ID
curl -s http://localhost:8090/health | grep -o '"leader_address":"[^"]*"'
```

**✅ Pass Criteria:**

- Exactly ONE coordinator has `"is_leader": true`
- All coordinators report same `leader_address`
- Raft state: 1 Leader, 2 Followers

---

### Test 2.2: Raft Term Stability

**Objective:** Verify term number is stable (no constant elections)

```bash
# Check term at T=0
echo "Initial term:"
curl -s http://localhost:8090/health | grep -o '"term":[0-9]*'

# Wait 30 seconds
sleep 30

# Check term at T=30
echo "Term after 30 seconds:"
curl -s http://localhost:8090/health | grep -o '"term":[0-9]*'
```

**✅ Pass Criteria:**

- Term number stays the same
- No term increments (indicates stable leadership)

---

### Test 2.3: Node Identity

**Objective:** Verify each coordinator has unique ID

```bash
# Check all coordinator IDs
echo "Coordinator-1 ID:"
curl -s http://localhost:8090/health | grep -o '"coordinator_id":"[^"]*"'

echo "Coordinator-2 ID:"
curl -s http://localhost:8091/health | grep -o '"coordinator_id":"[^"]*"'

echo "Coordinator-3 ID:"
curl -s http://localhost:8092/health | grep -o '"coordinator_id":"[^"]*"'
```

**✅ Pass Criteria:**

- coordinator-1 has ID: "coordinator-1"
- coordinator-2 has ID: "coordinator-2"
- coordinator-3 has ID: "coordinator-3"
- No duplicate IDs

---

### Test 2.4: Log Replication

**Objective:** Verify logs are replicated to followers

```bash
# Check replication in logs
docker logs zkp-coordinator-1 --tail=50 | grep "pipelining replication"
docker logs zkp-coordinator-1 --tail=50 | grep "appendEntries"
```

**✅ Pass Criteria:**

- Leader logs show "pipelining replication"
- AppendEntries sent to both followers
- No replication errors

---

## 3️⃣ Worker Management

### Test 3.1: Worker Registration

**Objective:** Verify workers can register with cluster

```bash
# Check registered workers
curl -s http://localhost:8090/health | python -m json.tool | grep -A5 "workers"

# Check worker logs
docker logs zkp-worker-1-cluster --tail=20 | grep -i "register"
docker logs zkp-worker-2-cluster --tail=20 | grep -i "register"
```

**✅ Pass Criteria:**

- `"total": 2` (both workers registered)
- `"active": 2`
- Worker logs show "registration successful"

---

### Test 3.2: Worker Heartbeats

**Objective:** Verify workers send periodic heartbeats

```bash
# Watch coordinator logs for heartbeats
docker logs -f zkp-coordinator-1 | grep -i "heartbeat"

# Let it run for 30 seconds, then Ctrl+C
```

**✅ Pass Criteria:**

- Heartbeat messages appear regularly (every 10-30 seconds)
- Worker status remains "active"
- No "suspect" or "dead" workers

---

### Test 3.3: Worker Redirect to Leader

**Objective:** Verify followers redirect workers to leader

```bash
# Check follower logs for redirects
docker logs zkp-coordinator-2 --tail=30 | grep -i "redirect"
docker logs zkp-coordinator-3 --tail=30 | grep -i "redirect"
```

**✅ Pass Criteria:**

- Follower logs show "Not leader, redirecting"
- Redirect points to correct leader
- Workers eventually connect to leader

---

### Test 3.4: Worker Capacity

**Objective:** Verify worker capacity reporting

```bash
# Check cluster capacity
curl -s http://localhost:8090/health | python -m json.tool | grep -A5 "capacity"
```

**✅ Pass Criteria:**

- `"total"`: Sum of all worker capacities
- `"available"`: Equal to total (when idle)
- `"used": 0` (no tasks running)

---

## 4️⃣ Task Distribution

### Test 4.1: Submit ZKP Proof Task

**Objective:** Submit a proof generation task via API gateway

```bash
# Submit a proof request
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{
    "circuit_type": "merkle_tree",
    "public_inputs": ["1", "2", "3"],
    "private_inputs": ["secret1", "secret2"]
  }'
```

**✅ Pass Criteria:**

- HTTP 202 Accepted
- Returns task ID
- JSON response: `{"task_id": "...", "status": "pending"}`

---

### Test 4.2: Task Status Query

**Objective:** Check status of submitted task

```bash
# Replace <task_id> with actual ID from Test 4.1
TASK_ID="<task_id>"
curl http://localhost:8080/api/v1/proof/$TASK_ID
```

**✅ Pass Criteria:**

- HTTP 200 response
- Status one of: pending, assigned, running, completed, failed
- Shows worker_id if assigned

---

### Test 4.3: Task Assignment

**Objective:** Verify coordinator assigns task to worker

```bash
# Check scheduler logs
docker logs zkp-coordinator-1 --tail=50 | grep -i "task assigned\|scheduling"
```

**✅ Pass Criteria:**

- Log shows "Task assigned to worker"
- Worker ID present
- Task transitioned from pending → assigned

---

### Test 4.4: Task Execution

**Objective:** Verify worker executes the task

```bash
# Check worker logs
docker logs zkp-worker-1-cluster --tail=50 | grep -i "executing\|completed"
docker logs zkp-worker-2-cluster --tail=50 | grep -i "executing\|completed"
```

**✅ Pass Criteria:**

- Worker logs show "Executing task"
- Task completes successfully
- Proof generated (if circuit implemented)

---

### Test 4.5: Multiple Concurrent Tasks

**Objective:** Test parallel task processing

```bash
# Submit 5 tasks rapidly
for i in {1..5}; do
  curl -X POST http://localhost:8080/api/v1/proof \
    -H "Content-Type: application/json" \
    -d "{\"circuit_type\": \"test_circuit_$i\", \"public_inputs\": [\"$i\"]}" &
done
wait

# Check worker distribution
curl -s http://localhost:8090/health | python -m json.tool | grep -A3 "scheduler"
```

**✅ Pass Criteria:**

- All 5 tasks accepted
- Tasks distributed across workers
- No task assignment failures

---

## 5️⃣ Leader Failover

### Test 5.1: Leader Crash and Recovery

**Objective:** Verify new leader elected when current leader fails

```bash
# Step 1: Identify current leader
LEADER=$(curl -s http://localhost:8090/health | grep -o '"is_leader":true' && echo "coordinator-1" || \
         curl -s http://localhost:8091/health | grep -o '"is_leader":true' && echo "coordinator-2" || \
         echo "coordinator-3")
echo "Current leader: $LEADER"

# Step 2: Stop the leader
docker stop zkp-$LEADER

# Step 3: Wait for election (3-5 seconds)
sleep 5

# Step 4: Check new leader
curl -s http://localhost:8091/health | grep -o '"is_leader":[^,]*'
curl -s http://localhost:8092/health | grep -o '"is_leader":[^,]*'

# Step 5: Restart old leader
docker start zkp-$LEADER

# Step 6: Verify it becomes follower
sleep 3
curl -s http://localhost:8090/health | grep -o '"is_leader":[^,]*'
```

**✅ Pass Criteria:**

- New leader elected within 5 seconds
- New leader is one of the remaining nodes
- Old leader rejoins as follower
- No data loss

**⏱️ Expected Timing:**

- Election timeout: ~1-3 seconds
- New leader elected: ~3-5 seconds

---

### Test 5.2: Multiple Leader Failures

**Objective:** Test cluster behavior with only 1 node remaining

```bash
# Stop 2 coordinators
docker stop zkp-coordinator-2 zkp-coordinator-3

# Wait 10 seconds
sleep 10

# Check coordinator-1 status
curl -s http://localhost:8090/health | python -m json.tool

# Restart the cluster
docker start zkp-coordinator-2 zkp-coordinator-3
sleep 5
```

**✅ Pass Criteria:**

- With 1 node: No leader (can't reach quorum 2/3)
- Coordinator-1 stays as follower or candidate
- When others restart: Leader re-elected
- Cluster recovers automatically

---

### Test 5.3: Worker Tasks During Failover

**Objective:** Verify tasks continue during leader change

```bash
# Step 1: Submit long-running task
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type": "large_circuit", "public_inputs": ["test"]}'

# Step 2: Kill leader during task execution
docker stop zkp-coordinator-1

# Step 3: Wait for failover
sleep 5

# Step 4: Check task status via API gateway
curl http://localhost:8080/api/v1/proof/<task_id>

# Step 5: Restart leader
docker start zkp-coordinator-1
```

**✅ Pass Criteria:**

- Task continues executing on worker
- New leader takes over task tracking
- Task completes successfully
- No task data corruption

---

## 6️⃣ Network Partitioning

### Test 6.1: Isolate Follower

**Objective:** Test behavior when follower is network isolated

```bash
# Disconnect coordinator-3 from network
docker network disconnect docker_zkp-cluster-network zkp-coordinator-3

# Wait 30 seconds
sleep 30

# Check cluster status
curl -s http://localhost:8090/health | python -m json.tool | grep -A5 "raft"

# Reconnect
docker network connect docker_zkp-cluster-network zkp-coordinator-3

# Wait for rejoin
sleep 5
```

**✅ Pass Criteria:**

- Leader and other follower continue operating
- Isolated node becomes "candidate" (can't reach leader)
- On reconnect: Node rejoins as follower
- Logs catch up via replication

---

### Test 6.2: Split Brain Prevention

**Objective:** Verify cluster prevents split brain with 2 partitions

```bash
# This requires advanced network manipulation
# Conceptual test only (use production testing tools)

# Scenario: Network partition creates:
# - Partition A: coordinator-1, coordinator-2
# - Partition B: coordinator-3

# Expected: Partition A keeps leader (has quorum 2/3)
#           Partition B has no leader (can't reach quorum)
```

**✅ Pass Criteria:**

- Only one partition has a leader
- Partition with majority (2/3) continues operating
- Minority partition cannot elect new leader
- On network heal: Cluster converges to one leader

---

## 7️⃣ Database Operations

### Test 7.1: Database Connectivity

**Objective:** Verify all coordinators connect to database

```bash
# Check coordinator logs for DB connection
docker logs zkp-coordinator-1 --tail=50 | grep -i "database\|postgres"
docker logs zkp-coordinator-2 --tail=50 | grep -i "database\|postgres"
docker logs zkp-coordinator-3 --tail=50 | grep -i "database\|postgres"
```

**✅ Pass Criteria:**

- All coordinators show "Database connected"
- No connection errors
- Connection pool initialized

---

### Test 7.2: Task Persistence

**Objective:** Verify tasks persisted in database

```bash
# Submit a task
TASK_ID=$(curl -s -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type": "test", "public_inputs": ["1"]}' | grep -o '"task_id":"[^"]*"' | cut -d'"' -f4)

# Query database directly
docker exec zkp-postgres-cluster psql -U zkp_user -d zkp_network \
  -c "SELECT id, status, circuit_type FROM tasks WHERE id = '$TASK_ID';"
```

**✅ Pass Criteria:**

- Task exists in database
- Status matches API response
- All fields populated correctly

---

### Test 7.3: Database Failover

**Objective:** Test behavior when database is unavailable

```bash
# Stop postgres
docker stop zkp-postgres-cluster

# Try to submit task
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type": "test", "public_inputs": ["1"]}'

# Restart postgres
docker start zkp-postgres-cluster

# Wait for reconnection
sleep 5

# Submit task again
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type": "test", "public_inputs": ["1"]}'
```

**✅ Pass Criteria:**

- During outage: HTTP 503 or error response
- After restart: Coordinators reconnect automatically
- New tasks succeed
- No data corruption

---

## 8️⃣ API Gateway Tests

### Test 8.1: CORS Headers

**Objective:** Verify CORS configuration

```bash
# OPTIONS request
curl -X OPTIONS http://localhost:8080/api/v1/proof \
  -H "Origin: http://example.com" \
  -H "Access-Control-Request-Method: POST" \
  -v
```

**✅ Pass Criteria:**

- Headers include `Access-Control-Allow-Origin`
- `Access-Control-Allow-Methods` includes POST, GET
- No CORS errors

---

### Test 8.2: Rate Limiting

**Objective:** Test rate limit enforcement

```bash
# Send 100 requests rapidly
for i in {1..100}; do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/v1/health
done | sort | uniq -c
```

**✅ Pass Criteria:**

- First N requests: HTTP 200
- After limit: HTTP 429 (Too Many Requests)
- Rate limit resets after time window

---

### Test 8.3: Invalid Request Handling

**Objective:** Verify proper error responses

```bash
# Missing required fields
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{}'

# Invalid JSON
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d 'invalid json'

# Non-existent endpoint
curl http://localhost:8080/api/v1/nonexistent
```

**✅ Pass Criteria:**

- HTTP 400 for invalid input
- JSON error response with message
- HTTP 404 for non-existent routes

---

## 9️⃣ Performance Tests

### Test 9.1: Throughput Test

**Objective:** Measure maximum task submission rate

```bash
# Submit 1000 tasks and measure time
time for i in {1..1000}; do
  curl -s -X POST http://localhost:8080/api/v1/proof \
    -H "Content-Type: application/json" \
    -d "{\"circuit_type\": \"perf_test\", \"public_inputs\": [\"$i\"]}" > /dev/null
done
```

**✅ Pass Criteria:**

- All 1000 tasks accepted
- No errors
- Measure: tasks/second

**📊 Expected Performance:**

- Target: >100 tasks/second
- Acceptable: >50 tasks/second

---

### Test 9.2: Response Time

**Objective:** Measure API response latency

```bash
# Measure health endpoint latency
for i in {1..100}; do
  curl -s -o /dev/null -w "%{time_total}\n" http://localhost:8090/health
done | awk '{sum+=$1; count++} END {print "Average:", sum/count, "seconds"}'
```

**✅ Pass Criteria:**

- Average response time < 100ms
- 95th percentile < 200ms
- No timeouts

---

### Test 9.3: Memory Usage

**Objective:** Monitor memory consumption

```bash
# Check container memory usage
docker stats --no-stream --format "table {{.Name}}\t{{.MemUsage}}\t{{.MemPerc}}"
```

**✅ Pass Criteria:**

- Coordinators: < 500MB each
- Workers: < 1GB each
- Postgres: < 2GB
- No memory leaks over time

---

## 🔟 Error Handling

### Test 10.1: Coordinator Crash Recovery

**Objective:** Verify coordinator recovers from crash

```bash
# Kill coordinator forcefully
docker kill zkp-coordinator-2

# Restart
docker start zkp-coordinator-2

# Check logs for recovery
sleep 5
docker logs zkp-coordinator-2 --tail=30
```

**✅ Pass Criteria:**

- Coordinator restarts successfully
- Rejoins cluster automatically
- Logs show "entering follower state"
- No manual intervention needed

---

### Test 10.2: Worker Crash Recovery

**Objective:** Verify worker recovery and re-registration

```bash
# Kill worker
docker kill zkp-worker-1-cluster

# Check coordinator detects dead worker
sleep 35  # Wait for heartbeat timeout (30s)
curl -s http://localhost:8090/health | grep -A5 "workers"

# Restart worker
docker start zkp-worker-1-cluster

# Verify re-registration
sleep 5
docker logs zkp-worker-1-cluster --tail=20 | grep -i "register"
```

**✅ Pass Criteria:**

- Coordinator marks worker as "dead" after timeout
- Worker re-registers on restart
- Tasks reassigned if needed
- No data loss

---

### Test 10.3: Stale Task Cleanup

**Objective:** Verify stale tasks are reassigned

```bash
# Submit task
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{"circuit_type": "test", "public_inputs": ["1"]}'

# Kill worker immediately (task becomes stale)
docker kill zkp-worker-1-cluster

# Wait for stale task timeout (5 minutes)
sleep 300

# Check if task reassigned
docker logs zkp-coordinator-1 --tail=50 | grep -i "stale\|reassign"
```

**✅ Pass Criteria:**

- Coordinator detects stale task
- Task marked as "pending" again
- Reassigned to healthy worker
- Logged in coordinator

---

## 📊 Test Summary Template

Use this template to track test execution:

```
Date: ___________
Tester: ___________
Version: ___________

| Test ID | Test Name              | Result | Notes |
|---------|------------------------|--------|-------|
| 1.1     | Services Running       | ☐ PASS ☐ FAIL |       |
| 1.2     | Health Endpoints       | ☐ PASS ☐ FAIL |       |
| 1.3     | API Gateway Health     | ☐ PASS ☐ FAIL |       |
| 2.1     | Leader Election        | ☐ PASS ☐ FAIL |       |
| 2.2     | Term Stability         | ☐ PASS ☐ FAIL |       |
| 2.3     | Node Identity          | ☐ PASS ☐ FAIL |       |
| 2.4     | Log Replication        | ☐ PASS ☐ FAIL |       |
| 3.1     | Worker Registration    | ☐ PASS ☐ FAIL |       |
| 3.2     | Worker Heartbeats      | ☐ PASS ☐ FAIL |       |
| 3.3     | Worker Redirect        | ☐ PASS ☐ FAIL |       |
| 3.4     | Worker Capacity        | ☐ PASS ☐ FAIL |       |
| 4.1     | Submit Task            | ☐ PASS ☐ FAIL |       |
| 4.2     | Task Status            | ☐ PASS ☐ FAIL |       |
| 4.3     | Task Assignment        | ☐ PASS ☐ FAIL |       |
| 4.4     | Task Execution         | ☐ PASS ☐ FAIL |       |
| 4.5     | Concurrent Tasks       | ☐ PASS ☐ FAIL |       |
| 5.1     | Leader Failover        | ☐ PASS ☐ FAIL |       |
| 5.2     | Multiple Failures      | ☐ PASS ☐ FAIL |       |
| 5.3     | Tasks During Failover  | ☐ PASS ☐ FAIL |       |
| 6.1     | Isolate Follower       | ☐ PASS ☐ FAIL |       |
| 7.1     | Database Connectivity  | ☐ PASS ☐ FAIL |       |
| 7.2     | Task Persistence       | ☐ PASS ☐ FAIL |       |
| 7.3     | Database Failover      | ☐ PASS ☐ FAIL |       |
| 8.1     | CORS Headers           | ☐ PASS ☐ FAIL |       |
| 8.2     | Rate Limiting          | ☐ PASS ☐ FAIL |       |
| 8.3     | Invalid Requests       | ☐ PASS ☐ FAIL |       |
| 9.1     | Throughput             | ☐ PASS ☐ FAIL |       |
| 9.2     | Response Time          | ☐ PASS ☐ FAIL |       |
| 9.3     | Memory Usage           | ☐ PASS ☐ FAIL |       |
| 10.1    | Coordinator Recovery   | ☐ PASS ☐ FAIL |       |
| 10.2    | Worker Recovery        | ☐ PASS ☐ FAIL |       |
| 10.3    | Stale Task Cleanup     | ☐ PASS ☐ FAIL |       |

Overall Result: ☐ PASS ☐ FAIL
Pass Rate: _____ / 33 tests
```

---

## 🔍 Troubleshooting

### Common Issues

**Issue: Containers not starting**

```bash
# Check logs
docker logs zkp-coordinator-1 --tail=50

# Common causes:
# - Port conflicts (8090, 9090, 7000 already in use)
# - Database not ready
# - Config file errors
```

**Issue: No leader elected**

```bash
# Check Raft logs
docker logs zkp-coordinator-1 | grep -i "election\|leader\|term"

# Common causes:
# - Duplicate node IDs
# - Network issues between nodes
# - Not enough nodes running (need 2/3 quorum)
```

**Issue: Workers not registering**

```bash
# Check worker logs
docker logs zkp-worker-1-cluster --tail=50

# Common causes:
# - Coordinator address incorrect
# - Network connectivity
# - gRPC port not accessible
```

---

## 📚 Additional Resources

- **Cluster Quick Start**: `CLUSTER_QUICKSTART.md`
- **Raft Integration Guide**: `docs/RAFT_INTEGRATION_GUIDE.md`
- **Production Deployment**: `docs/PRODUCTION_DEPLOYMENT.md`
- **Issues Solved**: `docs/ISSUES_SOLVED.md`
- **Success Summary**: `RAFT_SUCCESS.md`
