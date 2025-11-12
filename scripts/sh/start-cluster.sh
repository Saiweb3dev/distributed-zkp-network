#!/bin/bash

# ============================================================================
# Start 3-Node Raft Coordinator Cluster
# ============================================================================

set -e

echo "======================================"
echo "ZKP Network Cluster Startup"
echo "======================================"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Step 1: Check dependencies
echo -e "\n${YELLOW}[1/5] Checking dependencies...${NC}"
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: docker is not installed${NC}"
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo -e "${RED}Error: docker-compose is not installed${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Docker and docker-compose are installed${NC}"

# Step 2: Install Go dependencies
echo -e "\n${YELLOW}[2/5] Installing Go dependencies...${NC}"
cd "$(dirname "$0")/../.."
go get github.com/hashicorp/raft@v1.5.0
go get github.com/hashicorp/raft-boltdb@v2.3.0
go mod tidy
echo -e "${GREEN}✓ Dependencies installed${NC}"

# Step 3: Stop existing containers
echo -e "\n${YELLOW}[3/5] Stopping existing containers...${NC}"
docker-compose -f deployments/docker/docker-compose-cluster.yml down -v 2>/dev/null || true
echo -e "${GREEN}✓ Existing containers stopped${NC}"

# Step 4: Build images
echo -e "\n${YELLOW}[4/5] Building Docker images...${NC}"
docker-compose -f deployments/docker/docker-compose-cluster.yml build
echo -e "${GREEN}✓ Images built successfully${NC}"

# Step 5: Start cluster
echo -e "\n${YELLOW}[5/5] Starting cluster...${NC}"
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d
echo -e "${GREEN}✓ Cluster started${NC}"

# Wait for services to be ready
echo -e "\n${YELLOW}Waiting for services to start...${NC}"
sleep 10

# Check health
echo -e "\n======================================"
echo -e "${YELLOW}Cluster Status:${NC}"
echo -e "======================================\n"

echo -e "${YELLOW}Coordinator-1 (localhost:8090):${NC}"
curl -s http://localhost:8090/health | jq '.' || echo "Not ready yet"

echo -e "\n${YELLOW}Coordinator-2 (localhost:8091):${NC}"
curl -s http://localhost:8091/health | jq '.' || echo "Not ready yet"

echo -e "\n${YELLOW}Coordinator-3 (localhost:8092):${NC}"
curl -s http://localhost:8092/health | jq '.' || echo "Not ready yet"

echo -e "\n======================================"
echo -e "${GREEN}Cluster Started Successfully!${NC}"
echo -e "======================================\n"

echo "Services:"
echo "  - API Gateway: http://localhost:8080"
echo "  - Coordinator-1: http://localhost:8090 (gRPC: 9090, Raft: 7000)"
echo "  - Coordinator-2: http://localhost:8091 (gRPC: 9091, Raft: 7001)"
echo "  - Coordinator-3: http://localhost:8092 (gRPC: 9092, Raft: 7002)"
echo "  - PostgreSQL: localhost:5432"
echo ""
echo "Commands:"
echo "  - View logs: docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f"
echo "  - Stop cluster: docker-compose -f deployments/docker/docker-compose-cluster.yml down"
echo "  - Check leader: curl http://localhost:8090/health | jq '.raft'"
echo ""
echo "Next: See docs/RAFT_QUICKSTART.md for testing instructions"
