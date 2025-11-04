# Manual Raft Cluster Bootstrap Guide

## Quick Bootstrap (Simplest Method)

Since the cluster is already running but not bootstrapped, follow these steps:

### Option 1: Restart with Fresh Raft Data

```bash
# 1. Stop all coordinators
docker stop zkp-coordinator-1 zkp-coordinator-2 zkp-coordinator-3

# 2. Clear Raft data (forces fresh bootstrap)
docker volume rm docker_coordinator-1-data docker_coordinator-2-data docker_coordinator-3-data

# 3. Restart coordinators (coordinator-1 will bootstrap automatically)
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d coordinator-1
sleep 10  # Wait for bootstrap

docker-compose -f deployments/docker/docker-compose-cluster.yml up -d coordinator-2 coordinator-3
sleep 5   # Wait for followers to join

# 4. Verify cluster state
curl http://localhost:8090/health | grep -o '"state":"[^"]*","is_leader":[^,]*'
curl http://localhost:8091/health | grep -o '"state":"[^"]*","is_leader":[^,]*'
curl http://localhost:8092/health | grep -o '"state":"[^"]*","is_leader":[^,]*'
```

### Option 2: Use Bootstrap Script

We created a bootstrap script that automates this:

**Linux/Mac:**
```bash
chmod +x scripts/sh/bootstrap-raft.sh
./scripts/sh/bootstrap-raft.sh
```

**Windows:**
```cmd
scripts\bat\bootstrap-raft.bat
```

## What the Bootstrap Does

1. **Stops all coordinators** - Prevents conflicts during bootstrap
2. **Clears Raft data** - Removes old state that might prevent bootstrap
3. **Starts coordinator-1** - With `bootstrap: true` in config, it will:
   - Create initial Raft cluster configuration
   - Add all 3 peers to the cluster
   - Become the leader
4. **Starts remaining coordinators** - They join as followers
5. **Verifies cluster** - Checks that one leader and two followers exist

## Expected Output After Bootstrap

```json
{
  "status": "healthy",
  "coordinator_id": "coordinator-1",
  "raft": {
    "state": "Leader",        // ← coordinator-1 should be leader
    "is_leader": true,
    "leader_address": "coordinator-1:7000",
    "node_id": "coordinator-1"
  }
}
```

Coordinator-2 and Coordinator-3 should show:
```json
{
  "raft": {
    "state": "Follower",
    "is_leader": false,
    "leader_address": "coordinator-1:7000"
  }
}
```

## Troubleshooting

### Issue: "cluster has already been bootstrapped"

If you see this error, the cluster was already bootstrapped once. You have two options:

**A) Clear data and re-bootstrap:**
```bash
docker stop zkp-coordinator-1 zkp-coordinator-2 zkp-coordinator-3
docker volume rm docker_coordinator-1-data docker_coordinator-2-data docker_coordinator-3-data
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d
```

**B) Just restart the coordinators:**
```bash
docker restart zkp-coordinator-1 zkp-coordinator-2 zkp-coordinator-3
sleep 10
```

The cluster might elect a leader automatically if bootstrap data exists.

### Issue: No leader after bootstrap

```bash
# Check Raft logs
docker logs zkp-coordinator-1 | grep -i "raft\|leader\|election"

# Check network connectivity between coordinators
docker exec zkp-coordinator-1 ping -c 3 coordinator-2
docker exec zkp-coordinator-1 ping -c 3 coordinator-3

# Verify ports are accessible
docker exec zkp-coordinator-1 nc -zv coordinator-2 7000
docker exec zkp-coordinator-1 nc -zv coordinator-3 7000
```

### Issue: All nodes are followers

This means bootstrap didn't run. Check:

1. Config file has `bootstrap: true`:
   ```bash
   docker exec zkp-coordinator-1 cat /app/configs/coordinator-cluster.yaml | grep bootstrap
   ```

2. Raft data directory is empty (required for first bootstrap):
   ```bash
   docker exec zkp-coordinator-1 ls -la /var/lib/zkp/raft/
   ```

3. Rebuild coordinator-1 image if config changed:
   ```bash
   docker-compose -f deployments/docker/docker-compose-cluster.yml build coordinator-1
   docker-compose -f deployments/docker/docker-compose-cluster.yml up -d coordinator-1
   ```

## Manual Verification Commands

### Check Raft State
```bash
# All coordinators
for port in 8090 8091 8092; do
  echo "Coordinator on port $port:"
  curl -s http://localhost:$port/health | jq '.raft'
done
```

### Check Logs
```bash
docker logs zkp-coordinator-1 --tail=50 | grep -E "Bootstrap|Leader|Follower"
docker logs zkp-coordinator-2 --tail=50 | grep -E "Bootstrap|Leader|Follower"
docker logs zkp-coordinator-3 --tail=50 | grep -E "Bootstrap|Leader|Follower"
```

### Test Leader Failover
```bash
# 1. Identify current leader
LEADER_PORT=$(curl -s http://localhost:8090/health | jq -r 'if .raft.is_leader then "8090" else "" end')
if [ -z "$LEADER_PORT" ]; then
  LEADER_PORT=$(curl -s http://localhost:8091/health | jq -r 'if .raft.is_leader then "8091" else "8092" end')
fi

echo "Current leader on port: $LEADER_PORT"

# 2. Stop the leader
if [ "$LEADER_PORT" = "8090" ]; then
  docker stop zkp-coordinator-1
elif [ "$LEADER_PORT" = "8091" ]; then
  docker stop zkp-coordinator-2
else
  docker stop zkp-coordinator-3
fi

# 3. Wait for new election
sleep 5

# 4. Check new leader
curl -s http://localhost:8090/health | jq '.raft' 2>/dev/null || echo "Node 1 down"
curl -s http://localhost:8091/health | jq '.raft' 2>/dev/null || echo "Node 2 down"
curl -s http://localhost:8092/health | jq '.raft' 2>/dev/null || echo "Node 3 down"

# 5. Restart stopped node
docker start zkp-coordinator-1 zkp-coordinator-2 zkp-coordinator-3
```

## Quick Reference

| Command | Purpose |
|---------|---------|
| `docker logs zkp-coordinator-1 -f` | Follow coordinator logs |
| `curl http://localhost:8090/health` | Check coordinator-1 health |
| `docker exec zkp-coordinator-1 ls /var/lib/zkp/raft/` | View Raft data files |
| `docker restart zkp-coordinator-1` | Restart coordinator |
| `docker-compose -f deployments/docker/docker-compose-cluster.yml ps` | View all services |

## Next Steps After Bootstrap

Once you have a leader:

1. **Test worker registration:**
   ```bash
   docker logs zkp-worker-1-cluster --tail=20
   ```

2. **Submit a test proof:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/proof \
     -H "Content-Type: application/json" \
     -d '{
       "circuit_type": "simple_hash",
       "input_data": {"value": "42"}
     }'
   ```

3. **Test failover:**
   - Kill the leader
   - Verify new leader is elected
   - Verify service continues

4. **Monitor cluster:**
   - Check `docs/RAFT_QUICKSTART.md` for detailed testing
   - Review `docs/RAFT_INTEGRATION_GUIDE.md` for architecture
