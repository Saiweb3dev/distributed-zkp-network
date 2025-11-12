# Raft Integration: Issues Solved & Changes Made

## 🎯 Summary

This document explains all the issues encountered during Raft integration and how they were resolved.

---

## 🐛 Issues & Solutions

### 1. **Go Version Mismatch**

**Issue:**
```bash
go: go.mod requires go >= 1.24.0 (running go 1.21.13; GOTOOLCHAIN=local)
```

**Root Cause:**
- Project uses Go 1.24 features
- Dockerfiles specified `golang:1.21-alpine`

**Solution:**
```dockerfile
# Before
FROM golang:1.21-alpine AS builder

# After
FROM golang:1.24-alpine AS builder
```

**Files Changed:**
- `deployments/docker/Dockerfile.coordinator`
- `deployments/docker/Dockerfile.worker`
- `deployments/docker/Dockerfile.api-gateway`

---

### 2. **Database Authentication Failure**

**Issue:**
```
pq: password authentication failed for user "postgres"
```

**Root Cause:**
- Code had hardcoded defaults: `postgres/postgres`
- Docker-compose used: `zkp_user/zkp_password`
- Environment variables weren't matching Viper's expected format

**Solution:**
```yaml
# docker-compose.yml - Updated environment variables
environment:
  # Coordinator uses COORDINATOR_ prefix
  - COORDINATOR_DATABASE_HOST=postgres
  - COORDINATOR_DATABASE_USER=zkp_user
  - COORDINATOR_DATABASE_PASSWORD=zkp_password
  
  # API Gateway uses API_GATEWAY_ prefix
  - API_GATEWAY_DATABASE_HOST=postgres
  - API_GATEWAY_DATABASE_USER=zkp_user
  
  # Worker uses WORKER_ prefix
  - WORKER_WORKER_COORDINATOR_ADDRESS=coordinator-1:9090
```

**How Viper Maps Environment Variables:**
```go
// In config.go:
v.SetEnvPrefix("COORDINATOR")  // Set prefix
v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))  // Replace dots with underscores
v.AutomaticEnv()  // Enable env var override

// Example:
// YAML: database.host = "localhost"
// Env:  COORDINATOR_DATABASE_HOST=postgres
// Result: database.host = "postgres" (env overrides YAML)
```

---

### 3. **Raft Address Not Advertisable**

**Issue:**
```
failed to create transport: local bind address is not advertisable
```

**Root Cause:**
```go
// Original code (wrong):
raftAddr := fmt.Sprintf(":%d", cfg.Coordinator.Raft.RaftPort)
// Result: ":7000" (no hostname - can't advertise to cluster)
```

**Why This is a Problem:**
- Raft needs to tell other nodes "connect to me at X"
- `:7000` doesn't tell WHERE, only the port
- Other nodes can't find this coordinator

**Solution:**
```go
// Fixed code:
hostname, err := os.Hostname()
if err != nil {
    logger.Fatal("Failed to get hostname", zap.Error(err))
}
raftAddr := fmt.Sprintf("%s:%d", hostname, cfg.Coordinator.Raft.RaftPort)
// Result: "coordinator-1:7000" (fully qualified address)
```

**In Production (different machines):**
```yaml
# Config must use real IPs:
peers:
  - id: "coordinator-1"
    address: "192.168.1.10:7000"  # Must be reachable from other machines
  - id: "coordinator-2"
    address: "192.168.1.11:7000"
```

**Files Changed:**
- `cmd/coordinator/main.go` (line 135-150)

---

### 4. **Bootstrap Not Triggering**

**Issue:**
```
[WARN] raft: no known peers, aborting election
```

**Root Cause:**
- Config file had no `bootstrap` field
- Environment variable `COORDINATOR_RAFT_BOOTSTRAP=true` wasn't working
- Viper requires exact nested key structure

**Solution:**
```yaml
# Added to coordinator-cluster.yaml:
raft:
  bootstrap: true  # Explicitly set
```

**Why Environment Variables Failed:**
```bash
# What we tried:
COORDINATOR_RAFT_BOOTSTRAP=true

# What Viper expects:
# For nested key raft.bootstrap, the env var should be:
# COORDINATOR_RAFT_BOOTSTRAP

# But Viper couldn't map it because the key didn't exist in YAML
# Solution: Add it to YAML, then env var can override
```

**Files Changed:**
- `configs/coordinator-cluster.yaml`

---

### 5. **All Coordinators Have Same ID**

**Issue:**
```json
{
  "coordinator_id": "coordinator-1",  // All 3 nodes!
  "raft": { "node_id": "coordinator-1" }
}
```

**Root Cause:**
- All containers mount the SAME config file: `coordinator-cluster.yaml`
- Environment variable override for `coordinator.id` didn't work

**Why This Breaks Raft:**
- Raft requires unique IDs for each node
- With duplicate IDs, election fails (nodes can't distinguish each other)
- Logs show: "pre-vote denied" (nodes reject votes from "themselves")

**Solution 1: Separate Config Files (Production)**
```
configs/prod/coordinator-1.yaml  # id: "coordinator-1"
configs/prod/coordinator-2.yaml  # id: "coordinator-2"
configs/prod/coordinator-3.yaml  # id: "coordinator-3"
```

**Solution 2: Environment Variable Override (Docker Compose)**
```yaml
# In docker-compose-cluster.yml:
coordinator-1:
  environment:
    - COORDINATOR_COORDINATOR_ID=coordinator-1  # Override
    - COORDINATOR_RAFT_NODE_ID=coordinator-1

coordinator-2:
  environment:
    - COORDINATOR_COORDINATOR_ID=coordinator-2
    - COORDINATOR_RAFT_NODE_ID=coordinator-2
```

**Current Status:**
- ⚠️ Still needs fixing in local setup
- ✅ Production configs created with unique IDs

---

## 🔧 Docker Changes Summary

### Before (Broken)
```yaml
# docker-compose-cluster.yml
services:
  coordinator-1:
    build:
      context: .  # WRONG: builds from docker/ directory
      dockerfile: Dockerfile.coordinator
    environment:
      - DATABASE_HOST=postgres  # WRONG: doesn't match Viper prefix
      - RAFT_ADDR=0.0.0.0:7000  # WRONG: not advertisable

  worker-1:
    environment:
      - WORKER_ID=worker-1  # WRONG: should be WORKER_WORKER_ID
```

### After (Fixed)
```yaml
services:
  coordinator-1:
    build:
      context: ../..  # CORRECT: builds from project root
      dockerfile: deployments/docker/Dockerfile.coordinator
    environment:
      - COORDINATOR_DATABASE_HOST=postgres  # CORRECT: matches Viper
      - COORDINATOR_DATABASE_USER=zkp_user
      - COORDINATOR_DATABASE_PASSWORD=zkp_password
      - COORDINATOR_RAFT_NODE_ID=coordinator-1
      - COORDINATOR_RAFT_RAFT_PORT=7000  # Uses hostname resolution
      - COORDINATOR_RAFT_BOOTSTRAP=true

  worker-1:
    environment:
      - WORKER_WORKER_ID=worker-1-cluster  # CORRECT: nested key
      - WORKER_WORKER_COORDINATOR_ADDRESS=coordinator-1:9090
```

---

## 🌐 Local vs Production: Key Differences

### Networking

**Local (Docker Compose):**
```yaml
# Uses Docker's internal DNS
peers:
  - id: "coordinator-1"
    address: "coordinator-1:7000"  # Resolved by Docker
  - id: "coordinator-2"
    address: "coordinator-2:7000"
```

**Production (Different Machines):**
```yaml
# Uses real IP addresses
peers:
  - id: "coordinator-1"
    address: "192.168.1.10:7000"  # Must be reachable
  - id: "coordinator-2"
    address: "192.168.1.11:7000"
  - id: "coordinator-3"
    address: "192.168.1.12:7000"
```

### Configuration

**Local:**
```bash
# Single config file, environment variables differentiate nodes
docker-compose up
# Each container gets:
# - Same config file: coordinator-cluster.yaml
# - Different env vars: COORDINATOR_ID, RAFT_NODE_ID
```

**Production:**
```bash
# Separate config files per machine
# Machine 1:
./coordinator --config /etc/zkp/coordinator-1.yaml

# Machine 2:
./coordinator --config /etc/zkp/coordinator-2.yaml

# Machine 3:
./coordinator --config /etc/zkp/coordinator-3.yaml
```

### Security

**Local:**
```yaml
# No TLS needed (private Docker network)
database:
  ssl_mode: "disable"
```

**Production:**
```yaml
# TLS REQUIRED (public network)
database:
  ssl_mode: "require"

# Also add to Raft transport:
raft:
  tls_enabled: true
  cert_file: "/etc/zkp/tls/cert.pem"
  key_file: "/etc/zkp/tls/key.pem"
```

---

## 🚀 How to Run Bootstrap

### Local (Docker Compose):

**Windows:**
```cmd
cd A:\DistributedSystems\projects\distributed-zkp-network
scripts\bat\bootstrap-raft.bat
```

**Linux/Mac:**
```bash
chmod +x scripts/sh/bootstrap-raft.sh
./scripts/sh/bootstrap-raft.sh
```

**What it does:**
1. Stops all coordinators
2. Clears Raft data (`docker volume rm`)
3. Starts coordinator-1 (bootstraps cluster)
4. Waits 10 seconds
5. Starts coordinator-2 and coordinator-3 (join as followers)

### Production (Different Machines):

**Step 1: Start Machine 1 (bootstrap node)**
```bash
# On 192.168.1.10
docker run -d \
  --name zkp-coordinator-1 \
  -v /etc/zkp/coordinator-1.yaml:/app/config.yaml \
  zkp-coordinator:latest --config /app/config.yaml

# Wait for bootstrap
docker logs -f zkp-coordinator-1
# Look for: "Raft cluster bootstrapped successfully"
```

**Step 2: Start Machine 2 (follower)**
```bash
# On 192.168.1.11
docker run -d \
  --name zkp-coordinator-2 \
  -v /etc/zkp/coordinator-2.yaml:/app/config.yaml \
  zkp-coordinator:latest --config /app/config.yaml

# Check it joins
docker logs -f zkp-coordinator-2
# Look for: "entering follower state"
```

**Step 3: Start Machine 3 (follower)**
```bash
# On 192.168.1.12
docker run -d \
  --name zkp-coordinator-3 \
  -v /etc/zkp/coordinator-3.yaml:/app/config.yaml \
  zkp-coordinator:latest --config /app/config.yaml
```

**Step 4: Verify Cluster**
```bash
# From any machine
curl http://192.168.1.10:8090/health | jq '.raft'
curl http://192.168.1.11:8090/health | jq '.raft'
curl http://192.168.1.12:8090/health | jq '.raft'

# Should see:
# One "Leader", two "Follower"
```

---

## 🔗 How Coordinators Communicate (Production)

### 1. Raft Heartbeats
```
Every 2 seconds:
  Leader (192.168.1.10) → Follower (192.168.1.11) : AppendEntries RPC on port 7000
  Leader (192.168.1.10) → Follower (192.168.1.12) : AppendEntries RPC on port 7000
```

### 2. Log Replication
```
When new task arrives:
  1. Leader receives task
  2. Leader writes to Raft log
  3. Leader sends AppendEntries to followers (port 7000)
  4. Followers acknowledge
  5. Once 2/3 ack, Leader commits to database
  6. Leader notifies followers to commit
```

### 3. Leader Election
```
If leader dies (192.168.1.10 crashes):
  1. Followers stop receiving heartbeats
  2. After 3 seconds, followers start election
  3. Follower with most up-to-date log wins
  4. New leader elected (e.g., 192.168.1.11)
  5. New leader sends heartbeats to 192.168.1.12
  6. When 192.168.1.10 comes back, it becomes follower
```

### 4. Worker Connection
```
Workers can connect to ANY coordinator:
  Worker → 192.168.1.10:9090 (gRPC)
  
If not leader:
  Response: "Not leader, connect to: 192.168.1.11:9090"
  Worker → 192.168.1.11:9090 (retry)
```

### 5. Database Access
```
All coordinators connect to SAME database:
  Coordinator-1 → postgres.example.com:5432
  Coordinator-2 → postgres.example.com:5432
  Coordinator-3 → postgres.example.com:5432

BUT only leader writes:
  if raftNode.IsLeader() {
      db.Exec("INSERT INTO tasks ...")
  }
```

---

## 📊 Files Modified

### Core Changes:
1. `cmd/coordinator/main.go` - Added Raft initialization (STEP 6)
2. `internal/coordinator/raft/fsm.go` - Fixed duplicate functions
3. `internal/coordinator/raft/raft_node.go` - Added helper methods
4. `internal/common/config/config.go` - Added RaftConfig struct

### Docker Changes:
5. `deployments/docker/Dockerfile.coordinator` - Go 1.24
6. `deployments/docker/Dockerfile.worker` - Go 1.24
7. `deployments/docker/Dockerfile.api-gateway` - Go 1.24
8. `deployments/docker/docker-compose-cluster.yml` - Fixed env vars

### Configuration:
9. `configs/coordinator-cluster.yaml` - Added bootstrap field
10. `configs/prod/coordinator-1.yaml` - Production config (NEW)
11. `configs/prod/coordinator-2.yaml` - Production config (NEW)
12. `configs/prod/coordinator-3.yaml` - Production config (NEW)

### Documentation:
13. `docs/RAFT_INTEGRATION_GUIDE.md` - 1,200+ lines
14. `docs/RAFT_QUICKSTART.md` - 300+ lines
15. `docs/PHASE03_CHANGES_SUMMARY.md` - 800+ lines
16. `docs/PRODUCTION_DEPLOYMENT.md` - Production guide (NEW)
17. `BOOTSTRAP_GUIDE.md` - Bootstrap instructions
18. `CLUSTER_QUICKSTART.md` - Quick start guide

### Scripts:
19. `scripts/sh/bootstrap-raft.sh` - Linux/Mac bootstrap
20. `scripts/bat/bootstrap-raft.bat` - Windows bootstrap
21. `scripts/sh/start-cluster.sh` - Automated startup
22. `scripts/bat/start-cluster.bat` - Windows startup

---

## ✅ What's Working Now

1. ✅ All services build successfully (Go 1.24)
2. ✅ Database connections work (correct credentials)
3. ✅ Raft nodes initialize (correct addresses)
4. ✅ Bootstrap logic works (cluster formation)
5. ✅ Nodes communicate (pre-vote/election RPCs working)
6. ✅ Documentation complete (3,300+ lines)
7. ✅ Production configs created

## ⚠️ What Still Needs Work

1. ⚠️ Local setup: All nodes have same ID (need unique IDs)
2. ⚠️ Leader election: Blocked by duplicate IDs
3. ⚠️ TLS: Not implemented (needed for production)

## 🎯 Next Steps

1. Fix local coordinator IDs (use env vars properly)
2. Test full cluster with unique IDs
3. Implement TLS for Raft transport
4. Add load balancer configuration
5. Set up monitoring (Prometheus/Grafana)

---

## 📚 References

- **Raft Consensus Algorithm**: https://raft.github.io/
- **HashiCorp Raft Library**: https://github.com/hashicorp/raft
- **Production Deployment Guide**: `docs/PRODUCTION_DEPLOYMENT.md`
- **Quick Start**: `CLUSTER_QUICKSTART.md`
- **Bootstrap Guide**: `BOOTSTRAP_GUIDE.md`
