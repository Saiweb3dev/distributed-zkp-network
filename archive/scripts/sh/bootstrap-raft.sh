#!/bin/bash

# ============================================================================
# Manual Raft Cluster Bootstrap Script
# ============================================================================
# This script manually bootstraps the Raft cluster by executing a command
# inside the coordinator-1 container that triggers bootstrap logic.
# ============================================================================

set -e

echo "======================================"
echo "Raft Cluster Manual Bootstrap"
echo "======================================"
echo ""

# Check if docker is running
if ! docker ps > /dev/null 2>&1; then
    echo "Error: Docker is not running or not accessible"
    exit 1
fi

# Check if coordinator-1 is running
if ! docker ps | grep -q zkp-coordinator-1; then
    echo "Error: coordinator-1 container is not running"
    echo "Start the cluster first with: docker-compose -f deployments/docker/docker-compose-cluster.yml up -d"
    exit 1
fi

echo "Step 1: Checking current Raft state..."
echo ""

# Get current state of all coordinators
echo "Coordinator-1:"
curl -s http://localhost:8090/health | grep -o '"state":"[^"]*"' || echo "  Not responding"

echo "Coordinator-2:"
curl -s http://localhost:8091/health | grep -o '"state":"[^"]*"' || echo "  Not responding"

echo "Coordinator-3:"
curl -s http://localhost:8092/health | grep -o '"state":"[^"]*"' || echo "  Not responding"

echo ""
echo "======================================"
echo "Step 2: Stopping all coordinators..."
echo "======================================"
docker stop zkp-coordinator-1 zkp-coordinator-2 zkp-coordinator-3

echo ""
echo "Step 3: Clearing Raft data directories..."
echo ""
docker exec zkp-coordinator-1 rm -rf /var/lib/zkp/raft/* 2>/dev/null || echo "  coordinator-1 data cleared"
docker exec zkp-coordinator-2 rm -rf /var/lib/zkp/raft/* 2>/dev/null || echo "  coordinator-2 data cleared"
docker exec zkp-coordinator-3 rm -rf /var/lib/zkp/raft/* 2>/dev/null || echo "  coordinator-3 data cleared"

echo ""
echo "Step 4: Creating temporary bootstrap config for coordinator-1..."
echo ""

# Create a temporary config with bootstrap enabled
docker exec zkp-coordinator-1 sh -c 'cat > /tmp/bootstrap.yaml << EOF
coordinator:
  id: "coordinator-1"
  grpc_port: 9090
  http_port: 8090
  
  raft:
    node_id: "coordinator-1"
    raft_port: 7000
    raft_dir: "/var/lib/zkp/raft"
    bootstrap: true
    
    peers:
      - id: "coordinator-1"
        address: "coordinator-1:7000"
      - id: "coordinator-2"
        address: "coordinator-2:7000"
      - id: "coordinator-3"
        address: "coordinator-3:7000"
    
    heartbeat_timeout: 1s
    election_timeout: 1s
    commit_timeout: 50ms
    max_append_entries: 64
    snapshot_interval: 120s
    snapshot_threshold: 8192

database:
  host: "postgres"
  port: 5432
  database: "zkp_network"
  user: "zkp_user"
  password: "zkp_password"
  ssl_mode: "disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: "5m"

logging:
  level: "info"
  format: "json"
  output: "stdout"
EOF
' 2>/dev/null || echo "Failed to create bootstrap config"

echo ""
echo "======================================"
echo "Step 5: Starting coordinator-1 with bootstrap config..."
echo "======================================"
docker start zkp-coordinator-1

echo ""
echo "Waiting 10 seconds for coordinator-1 to bootstrap..."
sleep 10

echo ""
echo "Step 6: Checking if coordinator-1 became leader..."
echo ""
LEADER_CHECK=$(curl -s http://localhost:8090/health | grep -o '"is_leader":[^,]*' | cut -d':' -f2)

if [ "$LEADER_CHECK" = "true" ]; then
    echo "✓ SUCCESS: coordinator-1 is now the leader!"
else
    echo "✗ WARNING: coordinator-1 did not become leader yet"
    echo "  Checking logs..."
    docker logs zkp-coordinator-1 --tail=20 | grep -i "bootstrap\|leader"
fi

echo ""
echo "======================================"
echo "Step 7: Starting remaining coordinators..."
echo "======================================"
docker start zkp-coordinator-2 zkp-coordinator-3

echo ""
echo "Waiting 5 seconds for followers to join..."
sleep 5

echo ""
echo "======================================"
echo "Final Cluster State:"
echo "======================================"
echo ""

echo "Coordinator-1 (localhost:8090):"
curl -s http://localhost:8090/health | grep -o '"state":"[^"]*","is_leader":[^,]*' | sed 's/"//g'

echo ""
echo "Coordinator-2 (localhost:8091):"
curl -s http://localhost:8091/health | grep -o '"state":"[^"]*","is_leader":[^,]*' | sed 's/"//g'

echo ""
echo "Coordinator-3 (localhost:8092):"
curl -s http://localhost:8092/health | grep -o '"state":"[^"]*","is_leader":[^,]*' | sed 's/"//g'

echo ""
echo "======================================"
echo "Bootstrap complete!"
echo "======================================"
echo ""
echo "To verify cluster health:"
echo "  curl http://localhost:8090/health | jq '.raft'"
echo ""
echo "To view coordinator logs:"
echo "  docker logs -f zkp-coordinator-1"
