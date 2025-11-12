# 🚀 Quick Start: Running the Raft Cluster

## Prerequisites

✅ Docker & Docker Compose installed  
✅ Go 1.21+ installed  
✅ curl and jq (for testing)

## Option 1: Automated Startup (Recommended)

### Windows:
```bash
scripts\bat\start-cluster.bat
```

### Linux/Mac:
```bash
chmod +x scripts/sh/start-cluster.sh
./scripts/sh/start-cluster.sh
```

This script will:
1. Install Go dependencies (Raft libraries)
2. Build Docker images
3. Start 3-node coordinator cluster + workers + API gateway
4. Display health status

## Option 2: Manual Startup

### Step 1: Install Dependencies
```bash
go get github.com/hashicorp/raft@v1.5.0
go get github.com/hashicorp/raft-boltdb@v2.3.0
go mod tidy
```

### Step 2: Build & Start
```bash
# Build images
docker-compose -f deployments/docker/docker-compose-cluster.yml build

# Start cluster
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d
```

### Step 3: Wait for Leader Election
```bash
# Wait 10 seconds for Raft to elect a leader
sleep 10
```

## Verify Cluster Health

### Check All Coordinators:
```bash
# Coordinator-1
curl http://localhost:8090/health | jq '.'

# Coordinator-2
curl http://localhost:8091/health | jq '.'

# Coordinator-3
curl http://localhost:8092/health | jq '.'
```

### Find the Leader:
```bash
# Check which node is leader
curl http://localhost:8090/health | jq '.raft.state'
curl http://localhost:8091/health | jq '.raft.state'
curl http://localhost:8092/health | jq '.raft.state'

# Output: "Leader", "Follower", or "Candidate"
```

## Service Endpoints

| Service | HTTP | gRPC | Raft | Container |
|---------|------|------|------|-----------|
| Coordinator-1 | 8090 | 9090 | 7000 | zkp-coordinator-1 |
| Coordinator-2 | 8091 | 9091 | 7001 | zkp-coordinator-2 |
| Coordinator-3 | 8092 | 9092 | 7002 | zkp-coordinator-3 |
| API Gateway | 8080 | - | - | zkp-api-gateway-cluster |
| PostgreSQL | 5432 | - | - | zkp-postgres-cluster |
| Worker-1 | - | - | - | zkp-worker-1-cluster |
| Worker-2 | - | - | - | zkp-worker-2-cluster |

## Test Basic Operation

### 1. Submit a Proof Request:
```bash
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{
    "circuit_type": "merkle_tree",
    "input_data": {
      "leaf": "0x1234567890abcdef",
      "path": ["0xabc", "0xdef"],
      "root": "0x9876543210fedcba"
    }
  }'
```

### 2. Check Task Status:
```bash
# Replace TASK_ID with ID from previous response
curl http://localhost:8080/api/v1/proof/TASK_ID
```

## Test Failover (High Availability)

### 1. Identify Leader:
```bash
# Check logs to find leader
docker-compose -f deployments/docker/docker-compose-cluster.yml logs coordinator-1 | grep "became leader"
docker-compose -f deployments/docker/docker-compose-cluster.yml logs coordinator-2 | grep "became leader"
docker-compose -f deployments/docker/docker-compose-cluster.yml logs coordinator-3 | grep "became leader"
```

### 2. Kill Leader:
```bash
# If coordinator-1 is leader:
docker stop zkp-coordinator-1
```

### 3. Watch Re-election:
```bash
# Should see new leader elected within 1-3 seconds
docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f coordinator-2
docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f coordinator-3
```

### 4. Verify Service Continues:
```bash
# Submit another proof request - should still work!
curl -X POST http://localhost:8080/api/v1/proof \
  -H "Content-Type: application/json" \
  -d '{
    "circuit_type": "simple_hash",
    "input_data": {"value": "42"}
  }'
```

### 5. Restore Leader:
```bash
docker start zkp-coordinator-1

# Watch it rejoin as follower
docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f coordinator-1
```

## View Logs

### All Services:
```bash
docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f
```

### Specific Service:
```bash
docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f coordinator-1
docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f worker-1
docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f api-gateway
```

### Filter Raft Logs:
```bash
docker-compose -f deployments/docker/docker-compose-cluster.yml logs coordinator-1 | grep -i raft
```

## Stop Cluster

```bash
# Stop and remove containers (preserves volumes)
docker-compose -f deployments/docker/docker-compose-cluster.yml down

# Stop and remove everything including data
docker-compose -f deployments/docker/docker-compose-cluster.yml down -v
```

## Troubleshooting

### Issue: Services won't start
```bash
# Check for port conflicts
netstat -ano | findstr "8090"
netstat -ano | findstr "9090"
netstat -ano | findstr "7000"

# Solution: Kill conflicting processes or change ports
```

### Issue: No leader elected
```bash
# Check coordinator logs
docker-compose -f deployments/docker/docker-compose-cluster.yml logs coordinator-1 | grep -i "election\|leader\|raft"

# Common causes:
# - Network issues between containers
# - Time synchronization problems
# - Raft data corruption (solution: docker-compose down -v)
```

### Issue: Workers can't connect
```bash
# Check worker logs
docker-compose -f deployments/docker/docker-compose-cluster.yml logs worker-1

# Verify coordinator is reachable
docker exec zkp-worker-1-cluster ping coordinator-1
```

### Issue: Database connection failed
```bash
# Check postgres health
docker exec zkp-postgres-cluster pg_isready -U zkp_user

# Check coordinator database logs
docker-compose -f deployments/docker/docker-compose-cluster.yml logs postgres
```

## Next Steps

📖 **Read Full Documentation:**
- `docs/RAFT_INTEGRATION_GUIDE.md` - Complete architecture & design
- `docs/RAFT_QUICKSTART.md` - Detailed testing scenarios
- `docs/PHASE03_CHANGES_SUMMARY.md` - Code changes explained

🔍 **Monitor Cluster:**
- Set up Prometheus + Grafana (see `deployments/docker/prometheus.yml`)
- Add custom Raft metrics (leader changes, log replication lag)

🧪 **Advanced Testing:**
- Chaos engineering (kill random nodes)
- Network partitions (use Docker network manipulation)
- Performance benchmarks (1000+ concurrent proof requests)

## Expected Behavior

✅ **Normal Operation:**
- One coordinator is leader (handles all writes)
- Two coordinators are followers (replicate logs)
- Workers connect to any coordinator
- API Gateway load balances across coordinators
- Task scheduling happens only on leader

✅ **Leader Failure:**
- New leader elected within 1-3 seconds
- No task loss (committed tasks in Raft log)
- Workers automatically reconnect
- <2 seconds downtime

✅ **Follower Failure:**
- Cluster continues normally
- Failed node can rejoin and catch up
- No service interruption

❌ **Majority Failure (2+ nodes down):**
- Cluster loses quorum
- No writes possible (reads may work)
- Manual intervention required

## Support

🐛 **Found a bug?** Check `docs/RAFT_INTEGRATION_GUIDE.md` Troubleshooting section  
📚 **Need help?** Review architecture diagrams in documentation  
🚀 **Want to contribute?** See `PHASE03_CHANGES_SUMMARY.md` for code structure
