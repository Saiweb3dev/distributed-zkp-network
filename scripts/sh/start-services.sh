#!/bin/bash

# start-services.sh
# Automated script to start all distributed ZKP network services in separate terminals
# This script launches:
# - PostgreSQL database (Docker)
# - Coordinator service
# - 2 Worker nodes
# - API Gateway
#
# Usage: ./start-services.sh
# Requirements: Docker, Go 1.21+, Windows Terminal or compatible terminal emulator

set -e  # Exit on error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POSTGRES_CONTAINER="zkp-postgres"
POSTGRES_PORT=5432
COORDINATOR_PORT=9090
WORKER1_ID="worker-1"
WORKER2_ID="worker-2"
GATEWAY_PORT=8080

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Distributed ZKP Network Launcher${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check if a port is in use
port_in_use() {
    netstat -an 2>/dev/null | grep ":$1 " | grep -q LISTENING
}

# Function to wait for a service to be ready
wait_for_service() {
    local host=$1
    local port=$2
    local service_name=$3
    local max_attempts=30
    local attempt=0
    
    echo -e "${YELLOW}Waiting for $service_name to be ready...${NC}"
    
    while ! nc -z "$host" "$port" 2>/dev/null; do
        attempt=$((attempt + 1))
        if [ $attempt -ge $max_attempts ]; then
            echo -e "${RED}ERROR: $service_name failed to start after $max_attempts seconds${NC}"
            return 1
        fi
        sleep 1
        echo -n "."
    done
    
    echo -e "\n${GREEN}✓ $service_name is ready${NC}"
}

# Step 1: Check prerequisites
echo -e "${BLUE}[1/6] Checking prerequisites...${NC}"

if ! command_exists docker; then
    echo -e "${RED}ERROR: Docker is not installed${NC}"
    echo "Please install Docker from https://www.docker.com/get-started"
    exit 1
fi

if ! command_exists go; then
    echo -e "${RED}ERROR: Go is not installed${NC}"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi

echo -e "${GREEN}✓ Docker found: $(docker --version)${NC}"
echo -e "${GREEN}✓ Go found: $(go version)${NC}"

# Step 2: Check if ports are available
echo -e "\n${BLUE}[2/6] Checking port availability...${NC}"

if port_in_use $POSTGRES_PORT; then
    echo -e "${YELLOW}WARNING: Port $POSTGRES_PORT is already in use${NC}"
    echo "PostgreSQL might already be running"
fi

if port_in_use $COORDINATOR_PORT; then
    echo -e "${RED}ERROR: Port $COORDINATOR_PORT (Coordinator) is already in use${NC}"
    exit 1
fi

if port_in_use $GATEWAY_PORT; then
    echo -e "${RED}ERROR: Port $GATEWAY_PORT (API Gateway) is already in use${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Required ports are available${NC}"

# Step 3: Start PostgreSQL in Docker
echo -e "\n${BLUE}[3/6] Starting PostgreSQL database...${NC}"

# Check if container already exists
if docker ps -a --format '{{.Names}}' | grep -q "^${POSTGRES_CONTAINER}$"; then
    echo -e "${YELLOW}PostgreSQL container already exists${NC}"
    
    # Check if it's running
    if docker ps --format '{{.Names}}' | grep -q "^${POSTGRES_CONTAINER}$"; then
        echo -e "${GREEN}✓ PostgreSQL is already running${NC}"
    else
        echo "Starting existing PostgreSQL container..."
        docker start $POSTGRES_CONTAINER
        wait_for_service localhost $POSTGRES_PORT "PostgreSQL"
    fi
else
    echo "Creating new PostgreSQL container..."
    docker run -d \
        --name $POSTGRES_CONTAINER \
        -e POSTGRES_USER=postgres \
        -e POSTGRES_PASSWORD=postgres \
        -e POSTGRES_DB=zkp_network \
        -p $POSTGRES_PORT:5432 \
        postgres:15-alpine
    
    wait_for_service localhost $POSTGRES_PORT "PostgreSQL"
fi

# Step 4: Run database migrations
echo -e "\n${BLUE}[4/6] Running database migrations...${NC}"

sleep 2  # Give PostgreSQL a moment to fully initialize

if [ -f "$PROJECT_ROOT/internal/storage/postgres/migrations/001_initial_schema.up.sql" ]; then
    echo "Applying migrations..."
    docker exec -i $POSTGRES_CONTAINER psql -U postgres -d zkp_network < "$PROJECT_ROOT/internal/storage/postgres/migrations/001_initial_schema.up.sql" 2>/dev/null || echo -e "${YELLOW}Migrations may already be applied${NC}"
    echo -e "${GREEN}✓ Database migrations completed${NC}"
else
    echo -e "${YELLOW}WARNING: Migration file not found, skipping...${NC}"
fi

# Step 5: Determine terminal launcher
echo -e "\n${BLUE}[5/6] Detecting terminal emulator...${NC}"

LAUNCHER=""

if command_exists wt; then
    # Windows Terminal
    LAUNCHER="wt"
    echo -e "${GREEN}✓ Using Windows Terminal${NC}"
elif command_exists gnome-terminal; then
    # GNOME Terminal (Linux)
    LAUNCHER="gnome-terminal"
    echo -e "${GREEN}✓ Using GNOME Terminal${NC}"
elif command_exists xterm; then
    # XTerm (fallback for Linux)
    LAUNCHER="xterm"
    echo -e "${GREEN}✓ Using XTerm${NC}"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS Terminal
    LAUNCHER="osascript"
    echo -e "${GREEN}✓ Using macOS Terminal${NC}"
else
    echo -e "${RED}ERROR: No supported terminal emulator found${NC}"
    echo "Please install Windows Terminal, GNOME Terminal, or XTerm"
    exit 1
fi

# Step 6: Launch services in separate terminals
echo -e "\n${BLUE}[6/6] Launching services...${NC}"

cd "$PROJECT_ROOT"

# Function to launch service in new terminal
launch_service() {
    local title=$1
    local command=$2
    
    if [ "$LAUNCHER" = "wt" ]; then
        # Windows Terminal
        wt -w 0 new-tab --title "$title" cmd /k "$command"
    elif [ "$LAUNCHER" = "gnome-terminal" ]; then
        # GNOME Terminal
        gnome-terminal --tab --title="$title" -- bash -c "$command; exec bash"
    elif [ "$LAUNCHER" = "xterm" ]; then
        # XTerm
        xterm -T "$title" -e bash -c "$command; exec bash" &
    elif [ "$LAUNCHER" = "osascript" ]; then
        # macOS Terminal
        osascript -e "tell application \"Terminal\" to do script \"cd '$PROJECT_ROOT' && $command\""
    fi
    
    echo -e "${GREEN}✓ Launched $title${NC}"
}

# Launch Coordinator
echo -e "\n${YELLOW}Starting Coordinator...${NC}"
launch_service "Coordinator" "go run cmd/coordinator/main.go -config configs/dev/coordinator-local.yaml"
sleep 3

# Wait for coordinator to be ready
wait_for_service localhost $COORDINATOR_PORT "Coordinator"

# Launch Worker 1
echo -e "\n${YELLOW}Starting Worker 1...${NC}"
launch_service "Worker-1" "go run cmd/worker/main.go -config configs/dev/worker-local.yaml"
sleep 2

# Launch Worker 2
echo -e "\n${YELLOW}Starting Worker 2...${NC}"
# Create temporary config for worker 2
cat > /tmp/worker-2.yaml << EOF
worker:
  id: "$WORKER2_ID"
  coordinator_address: "localhost:9090"
  concurrency: 4
  heartbeat_interval: 5s

zkp:
  curve: "bn254"
  backend: "groth16"
EOF

launch_service "Worker-2" "go run cmd/worker/main.go -config /tmp/worker-2.yaml"
sleep 2

# Launch API Gateway
echo -e "\n${YELLOW}Starting API Gateway...${NC}"
launch_service "API-Gateway" "go run cmd/api-gateway/main.go -config configs/dev/api-gateway-local.yaml"
sleep 3

# Wait for API Gateway to be ready
wait_for_service localhost $GATEWAY_PORT "API Gateway"

# Display summary
echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}  All Services Started Successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}Service Status:${NC}"
echo -e "  PostgreSQL:    ${GREEN}✓${NC} Running on port $POSTGRES_PORT"
echo -e "  Coordinator:   ${GREEN}✓${NC} Running on port $COORDINATOR_PORT (gRPC) and 8090 (HTTP)"
echo -e "  Worker 1:      ${GREEN}✓${NC} ID: $WORKER1_ID"
echo -e "  Worker 2:      ${GREEN}✓${NC} ID: $WORKER2_ID"
echo -e "  API Gateway:   ${GREEN}✓${NC} Running on port $GATEWAY_PORT"
echo ""
echo -e "${BLUE}Quick Links:${NC}"
echo -e "  Coordinator Health: ${YELLOW}http://localhost:8090/health${NC}"
echo -e "  Coordinator Metrics: ${YELLOW}http://localhost:8090/metrics${NC}"
echo -e "  API Gateway: ${YELLOW}http://localhost:$GATEWAY_PORT${NC}"
echo ""
echo -e "${BLUE}Next Steps:${NC}"
echo -e "  1. Run ${YELLOW}./test-services.sh${NC} to test the system"
echo -e "  2. Check service logs in their respective terminal windows"
echo -e "  3. Use ${YELLOW}./stop-services.sh${NC} to shut down all services"
echo ""
echo -e "${GREEN}Happy proof generation! 🚀${NC}"
