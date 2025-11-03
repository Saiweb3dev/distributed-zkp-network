#!/bin/bash

# stop-services.sh
# Gracefully stops all distributed ZKP network services
# This script:
# - Finds running processes (coordinator, workers, gateway)
# - Sends SIGTERM for graceful shutdown
# - Waits for processes to complete
# - Stops PostgreSQL Docker container
#
# Usage: ./stop-services.sh

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

POSTGRES_CONTAINER="zkp-postgres"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Stopping ZKP Network Services${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Function to find and kill processes by name
stop_process() {
    local process_name=$1
    local friendly_name=$2
    
    # For Windows (Go processes)
    if pgrep -f "$process_name" > /dev/null 2>&1; then
        echo -e "${YELLOW}Stopping $friendly_name...${NC}"
        pkill -SIGTERM -f "$process_name"
        
        # Wait for graceful shutdown (max 30 seconds)
        local wait_count=0
        while pgrep -f "$process_name" > /dev/null 2>&1 && [ $wait_count -lt 30 ]; do
            sleep 1
            wait_count=$((wait_count + 1))
            echo -n "."
        done
        echo ""
        
        # Force kill if still running
        if pgrep -f "$process_name" > /dev/null 2>&1; then
            echo -e "${RED}Force killing $friendly_name (did not stop gracefully)${NC}"
            pkill -SIGKILL -f "$process_name"
        else
            echo -e "${GREEN}✓ $friendly_name stopped${NC}"
        fi
    else
        echo -e "${BLUE}$friendly_name is not running${NC}"
    fi
}

# Stop services in reverse order of startup
echo -e "${BLUE}[1/5] Stopping API Gateway...${NC}"
stop_process "cmd/api-gateway/main.go" "API Gateway"

echo -e "\n${BLUE}[2/5] Stopping Workers...${NC}"
stop_process "cmd/worker/main.go" "Workers"

echo -e "\n${BLUE}[3/5] Stopping Coordinator...${NC}"
stop_process "cmd/coordinator/main.go" "Coordinator"

echo -e "\n${BLUE}[4/5] Stopping PostgreSQL...${NC}"
if docker ps --format '{{.Names}}' | grep -q "^${POSTGRES_CONTAINER}$"; then
    echo "Stopping PostgreSQL container..."
    docker stop $POSTGRES_CONTAINER
    echo -e "${GREEN}✓ PostgreSQL stopped${NC}"
else
    echo -e "${BLUE}PostgreSQL container is not running${NC}"
fi

# Optional: Remove container and data
read -p "Remove PostgreSQL container and data? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Removing PostgreSQL container...${NC}"
    docker rm $POSTGRES_CONTAINER 2>/dev/null || true
    echo -e "${GREEN}✓ PostgreSQL container removed${NC}"
fi

# Clean up temporary config files
echo -e "\n${BLUE}[5/5] Cleaning up temporary files...${NC}"
rm -f /tmp/worker-2.yaml
echo -e "${GREEN}✓ Cleanup complete${NC}"

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  All Services Stopped${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}To restart services, run: ${YELLOW}./start-services.sh${NC}"
