#!/bin/bash
# setup.sh - Initial project setup for ZKP Distributed Network

set -e

PROJECT_NAME="zkp-distributed-network"
COLOR_GREEN='\033[0;32m'
COLOR_BLUE='\033[0;34m'
COLOR_YELLOW='\033[1;33m'
COLOR_RESET='\033[0m'

echo -e "${COLOR_BLUE}Setting up $PROJECT_NAME...${COLOR_RESET}"

# 1. Create directory structure
echo -e "${COLOR_BLUE}Creating directory structure...${COLOR_RESET}"
mkdir -p cmd/api-gateway
mkdir -p internal/{common/{config,errors,utils},zkp/{circuits,types},api/{handlers,middleware,router},observability/{logging,metrics}}
mkdir -p configs
mkdir -p deployments/docker
mkdir -p test/integration
mkdir -p bin

# 2. Initialize Go module
if [ ! -f "go.mod" ]; then
    echo -e "${COLOR_BLUE}Initializing Go module...${COLOR_RESET}"
    go mod init github.com/yourusername/$PROJECT_NAME
fi

# 3. Install dependencies
echo -e "${COLOR_BLUE}Installing dependencies...${COLOR_RESET}"
go get -u github.com/consensys/gnark@v0.10.0
go get -u github.com/consensys/gnark-crypto@v0.12.1
go get -u github.com/gorilla/mux@v1.8.1
go get -u github.com/spf13/viper@v1.18.2
go get -u go.uber.org/zap@v1.26.0
go get -u github.com/prometheus/client_golang@v1.18.0
go get -u github.com/stretchr/testify@v1.8.4

# 4. Create .gitignore
cat > .gitignore << 'EOF'
# Binaries
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, built with `go test -c`
*.test

# Output of the go coverage tool
*.out
*.prof

# Dependency directories
vendor/

# Go workspace file
go.work

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# OS
.DS_Store

# Config overrides
configs/local.yaml
configs/dev/*.local.yaml

# Logs
*.log
logs/

# Temporary files
tmp/
temp/
EOF

# 5. Create .env.example for local development
cat > .env.example << 'EOF'
# API Gateway Configuration
API_GATEWAY_PORT=8080
API_GATEWAY_HOST=0.0.0.0

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Metrics
METRICS_ENABLED=true
METRICS_PORT=9090
EOF

# 6. Create docker-compose.yml for local development
cat > deployments/docker/docker-compose.yml << 'EOF'
version: '3.8'

services:
  # =========================================================================
  # API Gateway - Our ZKP service
  # =========================================================================
  api-gateway:
    build:
      context: ../..
      dockerfile: deployments/docker/Dockerfile.api-gateway
    container_name: zkp-api-gateway
    ports:
      - "8080:8080"  # HTTP API
      - "9090:9090"  # Metrics
    environment:
      API_GATEWAY_SERVER_HOST: "0.0.0.0"
      API_GATEWAY_SERVER_PORT: "8080"
      API_GATEWAY_LOGGING_LEVEL: "info"
    volumes:
      - ../../configs:/app/configs:ro
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 10s
      timeout: 3s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  # =========================================================================
  # Prometheus - Metrics collection
  # =========================================================================
  prometheus:
    image: prom/prometheus:latest
    container_name: zkp-prometheus
    ports:
      - "9091:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/usr/share/prometheus/console_libraries'
      - '--web.console.templates=/usr/share/prometheus/consoles'
    restart: unless-stopped

  # =========================================================================
  # Grafana - Metrics visualization
  # =========================================================================
  grafana:
    image: grafana/grafana:latest
    container_name: zkp-grafana
    ports:
      - "3000:3000"
    environment:
      GF_SECURITY_ADMIN_USER: admin
      GF_SECURITY_ADMIN_PASSWORD: admin
      GF_USERS_ALLOW_SIGN_UP: "false"
    volumes:
      - grafana_data:/var/lib/grafana
    depends_on:
      - prometheus
    restart: unless-stopped

volumes:
  postgres_data:
  redis_data:
  prometheus_data:
  grafana_data:

# ============================================================================
# DOCKER COMPOSE USAGE
# ============================================================================

# Start all services:
#   docker-compose -f deployments/docker/docker-compose.yml up -d

# View logs:
#   docker-compose -f deployments/docker/docker-compose.yml logs -f api-gateway

# Stop all services:
#   docker-compose -f deployments/docker/docker-compose.yml down

# Rebuild and restart API Gateway:
#   docker-compose -f deployments/docker/docker-compose.yml up -d --build api-gateway

# Clean everything (including data):
#   docker-compose -f deployments/docker/docker-compose.yml down -v
EOF unless-stopped

  # =========================================================================
  # PostgreSQL - Task and proof storage
  # =========================================================================
  postgres:
    image: postgres:15-alpine
    container_name: zkp-postgres
    environment:
      POSTGRES_DB: zkp_network
      POSTGRES_USER: zkp_user
      POSTGRES_PASSWORD: zkp_pass
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U zkp_user"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  # =========================================================================
  # Redis - Caching and distributed locks
  # =========================================================================
  redis:
    image: redis:7-alpine
    container_name: zkp-redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 3s
      retries: 5
    restart:

# 7. Create prometheus config
cat > deployments/docker/prometheus.yml << 'EOF'
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'api-gateway'
    static_configs:
      - targets: ['host.docker.internal:9090']
    
  - job_name: 'coordinator'
    static_configs:
      - targets: ['host.docker.internal:9091']
    
  - job_name: 'worker'
    static_configs:
      - targets: ['host.docker.internal:9092']
EOF

# 8. Create initial config file
cat > configs/api-gateway.yaml << 'EOF'
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

logging:
  level: "info"
  format: "json"
  output: "stdout"

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

zkp:
  curve: "bn254"
  backend: "groth16"
  compile_cache_size: 100
  max_proof_size_mb: 10
  
rate_limit:
  enabled: true
  requests_per_second: 100
  burst: 200

cors:
  enabled: true
  allowed_origins:
    - "http://localhost:3000"
    - "http://localhost:8080"
  allowed_methods:
    - "GET"
    - "POST"
    - "PUT"
    - "DELETE"
  allowed_headers:
    - "Content-Type"
    - "Authorization"
EOF

echo -e "${COLOR_GREEN}✓ Project structure created${COLOR_RESET}"
echo -e "${COLOR_GREEN}✓ Dependencies installed${COLOR_RESET}"
echo -e "${COLOR_GREEN}✓ Configuration files created${COLOR_RESET}"
echo ""
echo -e "${COLOR_YELLOW}Next steps:${COLOR_RESET}"
echo "1. Run: docker-compose -f deployments/docker/docker-compose.yml up -d"
echo "2. Copy .env.example to .env and adjust if needed"
echo "3. Start implementing the API!"
echo ""
echo -e "${COLOR_GREEN}Happy coding! 🚀${COLOR_RESET}"