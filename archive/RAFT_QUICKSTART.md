# Raft Integration - Quick Start Guide

## Prerequisites

```bash
# Install Raft dependencies
go get github.com/hashicorp/raft@v1.5.0
go get github.com/hashicorp/raft-boltdb@v2.3.0
go mod tidy
```

## Step 1: Update go.mod

Add these dependencies:

```go
require (
    github.com/hashicorp/raft v1.5.0
    github.com/hashicorp/raft-boltdb v2.3.0+incompatible
)
```

## Step 2: Create Raft Data Directories

```bash
# Local development
mkdir -p ./data/raft/coordinator-1
mkdir -p ./data/raft/coordinator-2
mkdir -p ./data/raft/coordinator-3

# Docker volumes (handled automatically)
```

## Step 3: Test Single Node (Development)

```bash
# Start PostgreSQL
docker-compose up -d postgres

# Start single coordinator (no Raft cluster)
go run cmd/coordinator/main.go --config configs/coordinator-local.yaml

# Check health
curl http://localhost:8090/health | jq
```

**Expected Output**:

```json
{
  "status": "healthy",
  "coordinator_id": "coordinator-local",
  "raft": {
    "state": "Leader",
    "is_leader": true,
    "leader_address": "coordinator-local:7000",
    "node_id": "coordinator-local"
  },
  "workers": {...},
  "scheduler": {...}
}
```

## Step 4: Test 3-Node Cluster (Production)

```bash
# Start entire cluster
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d

# Wait for leader election (3-5 seconds)
sleep 5

# Check each node
curl http://localhost:8090/health | jq '.raft'  # Coordinator-1
curl http://localhost:8091/health | jq '.raft'  # Coordinator-2
curl http://localhost:8092/health | jq '.raft'  # Coordinator-3
```

**Expected: One Leader, Two Followers**:

Coordinator-1 (Follower):

```json
{
  "state": "Follower",
  "is_leader": false,
  "leader_address": "coordinator-2:7000",
  "node_id": "coordinator-1"
}
```

Coordinator-2 (Leader):

```json
{
  "state": "Leader",
  "is_leader": true,
  "leader_address": "coordinator-2:7000",
  "node_id": "coordinator-2"
}
```

Coordinator-3 (Follower):

```json
{
  "state": "Follower",
  "is_leader": false,
  "leader_address": "coordinator-2:7000",
  "node_id": "coordinator-2"
}
```

## Step 5: Test Leader Failure

```bash
# Find current leader
LEADER=$(curl -s http://localhost:8090/health | jq -r '.raft.leader_address' | cut -d: -f1)
echo "Current leader: $LEADER"

# Kill the leader
docker-compose -f deployments/docker/docker-compose-cluster.yml stop $LEADER

# Wait for new election
sleep 5

# Check new leader (should be different)
curl http://localhost:8090/health | jq '.raft'
curl http://localhost:8091/health | jq '.raft'
curl http://localhost:8092/health | jq '.raft'

# Verify system still works
curl -X POST http://localhost:8080/api/v1/tasks/merkle \
  -H "Content-Type: application/json" \
  -d '{"leaves":["0x123..."], "leaf_index":0}'

# Restart killed leader
docker-compose -f deployments/docker/docker-compose-cluster.yml start $LEADER

# Verify it rejoins as follower
sleep 3
curl http://localhost:8090/health | jq '.raft'
```

## Step 6: Verify Task Assignment Works

```bash
# Submit task
TASK_ID=$(curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
  -H "Content-Type: application/json" \
  -d '{
    "leaves": [
      "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
      "0xfedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"
    ],
    "leaf_index": 0
  }' | jq -r '.task_id')

echo "Task ID: $TASK_ID"

# Check task status
curl http://localhost:8080/api/v1/tasks/$TASK_ID | jq

# Verify all coordinators see the same state
curl http://localhost:8090/health | jq '.workers'
curl http://localhost:8091/health | jq '.workers'
curl http://localhost:8092/health | jq '.workers'
```

## Troubleshooting

### Issue: No leader elected

```bash
# Check logs
docker logs coordinator-1 | grep -i "leader"
docker logs coordinator-2 | grep -i "leader"
docker logs coordinator-3 | grep -i "leader"

# Verify bootstrap config
docker exec coordinator-1 cat /app/configs/coordinator-cluster.yaml | grep bootstrap
```

**Solution**: Ensure only coordinator-1 has `bootstrap: true`

### Issue: Split-brain (multiple leaders)

```bash
# Check if multiple leaders
curl http://localhost:8090/health | jq '.raft.is_leader'
curl http://localhost:8091/health | jq '.raft.is_leader'
curl http://localhost:8092/health | jq '.raft.is_leader'
```

**Solution**: This should NEVER happen with Raft. If it does:

1. Check network connectivity between nodes
2. Verify peer configuration is identical
3. Restart entire cluster

### Issue: Tasks not being assigned

```bash
# Check which coordinator is leader
LEADER_PORT=$(curl -s http://localhost:8090/health | jq -r '.raft.leader_address' | cut -d: -f2)

# Check leader's scheduler metrics
curl http://localhost:809${LEADER_PORT:3:1}/metrics | grep scheduler
```

**Solution**: Only leader schedules tasks. Verify leader is healthy.

## Performance Testing

```bash
# Load test with 100 tasks
for i in {1..100}; do
  curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
    -H "Content-Type: application/json" \
    -d "{\"leaves\":[\"0x${i}${i}${i}\"],\"leaf_index\":0}" &
done

wait

# Check assignment distribution
curl http://localhost:8090/metrics | grep tasks_assigned
```

## Next Steps

1. ✅ Verify Raft integration working
2. ⏭️ Add TLS for Raft communication
3. ⏭️ Implement admin API for cluster management
4. ⏭️ Add Prometheus metrics for Raft
5. ⏭️ Setup monitoring and alerts

## Useful Commands

```bash
# View Raft logs
docker exec coordinator-1 ls -lh /var/lib/zkp/raft/

# Check Raft state
docker exec coordinator-1 cat /var/lib/zkp/raft/raft-stable.db | strings | grep term

# Follow logs in real-time
docker logs -f coordinator-1 | grep -i raft

# Restart specific coordinator
docker-compose -f deployments/docker/docker-compose-cluster.yml restart coordinator-2

# View all coordinator states
watch -n1 'curl -s http://localhost:8090/health | jq ".raft" & \
           curl -s http://localhost:8091/health | jq ".raft" & \
           curl -s http://localhost:8092/health | jq ".raft"'
```
