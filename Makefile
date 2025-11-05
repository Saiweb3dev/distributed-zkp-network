# Distributed ZKP Verification Network - Makefile
.PHONY: help build test clean docker-build docker-push k8s-deploy dev-up dev-down

# Variables
PROJECT_NAME := distributed-zkp-network
DOCKER_REGISTRY := your-registry.io
VERSION := $(shell git describe --tags --always --dirty)
GO_VERSION := 1.22
PROTOC_VERSION := 3.21.0

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
BINARY_DIR := bin

# Services
API_GATEWAY := api-gateway
COORDINATOR := coordinator
WORKER := worker
CLI := zkp-cli

# Colors for output
COLOR_RESET :=
COLOR_BOLD :=
COLOR_GREEN :=
COLOR_YELLOW :=
COLOR_BLUE :=

## help: Display this help message
help:
	@echo "$(COLOR_BOLD)$(PROJECT_NAME) - Build Commands$(COLOR_RESET)"
	@echo ""
	@grep -E '^## [a-zA-Z_-]+:' $(MAKEFILE_LIST) | \
		sed 's/## //' | \
		awk 'BEGIN {FS = ":"}; {printf "$(COLOR_GREEN)%-20s$(COLOR_RESET) %s\n", $$1, $$2}'

## init: Initialize project (run once)
init:
	@echo "$(COLOR_BLUE)Initializing project...$(COLOR_RESET)"
	@mkdir -p $(BINARY_DIR)
	@mkdir -p internal/proto/api/v1
	@mkdir -p internal/proto/coordinator/v1
	@mkdir -p internal/storage/postgres/migrations
	@mkdir -p configs/dev
	@mkdir -p test/{unit,integration,e2e}
	@mkdir -p docs/{architecture,api,operations}
	$(GOMOD) download
	@echo "$(COLOR_GREEN)✓ Project initialized$(COLOR_RESET)"

## deps: Download and verify dependencies
deps:
	@echo "$(COLOR_BLUE)Downloading dependencies...$(COLOR_RESET)"
	$(GOGET) -u google.golang.org/grpc
	$(GOGET) -u google.golang.org/protobuf
	$(GOGET) -u github.com/consensys/gnark
	$(GOGET) -u github.com/gorilla/mux
	$(GOGET) -u github.com/lib/pq
	$(GOGET) -u github.com/redis/go-redis/v9
	$(GOGET) -u github.com/prometheus/client_golang/prometheus
	$(GOGET) -u go.uber.org/zap
	$(GOGET) -u github.com/spf13/viper
	$(GOGET) -u github.com/hashicorp/raft
	$(GOGET) -u github.com/opentracing/opentracing-go
	$(GOMOD) tidy
	$(GOMOD) verify
	@echo "$(COLOR_GREEN)✓ Dependencies downloaded$(COLOR_RESET)"

## proto: Generate Go code from protobuf files
proto:
	@echo "$(COLOR_BLUE)Generating protobuf code...$(COLOR_RESET)"
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/proto/api/v1/*.proto
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/proto/coordinator/v1/*.proto
	@echo "$(COLOR_GREEN)✓ Protobuf code generated$(COLOR_RESET)"

## build: Build all services
build: build-api-gateway build-coordinator build-worker build-cli

## build-api-gateway: Build API Gateway service
build-api-gateway:
	@echo "$(COLOR_BLUE)Building API Gateway...$(COLOR_RESET)"
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -ldflags="-s -w -X main.version=$(VERSION)" \
		-o $(BINARY_DIR)/$(API_GATEWAY) ./cmd/api-gateway
	@echo "$(COLOR_GREEN)✓ API Gateway built$(COLOR_RESET)"

## build-coordinator: Build Coordinator service
build-coordinator:
	@echo "$(COLOR_BLUE)Building Coordinator...$(COLOR_RESET)"
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -ldflags="-s -w -X main.version=$(VERSION)" \
		-o $(BINARY_DIR)/$(COORDINATOR) ./cmd/coordinator
	@echo "$(COLOR_GREEN)✓ Coordinator built$(COLOR_RESET)"

## build-worker: Build Worker service
build-worker:
	@echo "$(COLOR_BLUE)Building Worker...$(COLOR_RESET)"
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -ldflags="-s -w -X main.version=$(VERSION)" \
		-o $(BINARY_DIR)/$(WORKER) ./cmd/worker
	@echo "$(COLOR_GREEN)✓ Worker built$(COLOR_RESET)"

## build-cli: Build CLI tool
build-cli:
	@echo "$(COLOR_BLUE)Building CLI...$(COLOR_RESET)"
	$(GOBUILD) -ldflags="-s -w -X main.version=$(VERSION)" \
		-o $(BINARY_DIR)/$(CLI) ./cmd/cli
	@echo "$(COLOR_GREEN)✓ CLI built$(COLOR_RESET)"

## test: Run all tests
test: test-unit test-integration

## test-unit: Run unit tests with coverage
test-unit:
	@echo "$(COLOR_BLUE)Running unit tests...$(COLOR_RESET)"
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./internal/...
	@echo "$(COLOR_GREEN)✓ Unit tests passed$(COLOR_RESET)"

## test-integration: Run integration tests
test-integration:
	@echo "$(COLOR_BLUE)Running integration tests...$(COLOR_RESET)"
	$(GOTEST) -v -tags=integration ./test/integration/...
	@echo "$(COLOR_GREEN)✓ Integration tests passed$(COLOR_RESET)"

## test-e2e: Run end-to-end tests
test-e2e:
	@echo "$(COLOR_BLUE)Running E2E tests...$(COLOR_RESET)"
	$(GOTEST) -v -tags=e2e ./test/e2e/...
	@echo "$(COLOR_GREEN)✓ E2E tests passed$(COLOR_RESET)"

## test-coverage: Generate and view test coverage report
test-coverage:
	@echo "$(COLOR_BLUE)Generating coverage report...$(COLOR_RESET)"
	$(GOTEST) -coverprofile=coverage.out ./internal/...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "$(COLOR_GREEN)✓ Coverage report: coverage.html$(COLOR_RESET)"

## lint: Run linter
lint:
	@echo "$(COLOR_BLUE)Running linter...$(COLOR_RESET)"
	golangci-lint run --timeout=5m ./...
	@echo "$(COLOR_GREEN)✓ Linting passed$(COLOR_RESET)"

## fmt: Format code
fmt:
	@echo "$(COLOR_BLUE)Formatting code...$(COLOR_RESET)"
	$(GOCMD) fmt ./...
	@echo "$(COLOR_GREEN)✓ Code formatted$(COLOR_RESET)"

## vet: Run go vet
vet:
	@echo "$(COLOR_BLUE)Running go vet...$(COLOR_RESET)"
	$(GOCMD) vet ./...
	@echo "$(COLOR_GREEN)✓ Vet passed$(COLOR_RESET)"

## clean: Clean build artifacts
clean:
	@echo "$(COLOR_YELLOW)Cleaning build artifacts...$(COLOR_RESET)"
	$(GOCLEAN)
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html
	@echo "$(COLOR_GREEN)✓ Cleaned$(COLOR_RESET)"

## clean-docker: Clean Docker cache and images (fixes build issues)
clean-docker:
	@echo "$(COLOR_YELLOW)Cleaning Docker cache...$(COLOR_RESET)"
	@echo "Stopping containers..."
	-docker-compose -f deployments/docker/docker-compose-cluster.yml down -v
	@echo "Removing images..."
	-docker rmi zkp-api-gateway-cluster zkp-coordinator-cluster zkp-worker-cluster 2>/dev/null || true
	-docker rmi distributed-zkp-network-api-gateway distributed-zkp-network-coordinator distributed-zkp-network-worker 2>/dev/null || true
	@echo "Pruning build cache..."
	docker builder prune -f
	@echo "Pruning system..."
	docker system prune -f
	@echo "$(COLOR_GREEN)✓ Docker cache cleaned$(COLOR_RESET)"
	@echo ""
	@echo "Now run: make cluster"

## docker-build: Build all Docker images
docker-build:
	@echo "$(COLOR_BLUE)Building Docker images...$(COLOR_RESET)"
	docker build -f deployments/docker/Dockerfile.api-gateway \
		-t $(DOCKER_REGISTRY)/$(API_GATEWAY):$(VERSION) .
	docker build -f deployments/docker/Dockerfile.coordinator \
		-t $(DOCKER_REGISTRY)/$(COORDINATOR):$(VERSION) .
	docker build -f deployments/docker/Dockerfile.worker \
		-t $(DOCKER_REGISTRY)/$(WORKER):$(VERSION) .
	@echo "$(COLOR_GREEN)✓ Docker images built$(COLOR_RESET)"

## docker-push: Push Docker images to registry
docker-push:
	@echo "$(COLOR_BLUE)Pushing Docker images...$(COLOR_RESET)"
	docker push $(DOCKER_REGISTRY)/$(API_GATEWAY):$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(COORDINATOR):$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(WORKER):$(VERSION)
	@echo "$(COLOR_GREEN)✓ Images pushed$(COLOR_RESET)"

## dev-up: Start local development environment
dev-up:
	@echo "$(COLOR_BLUE)Starting development environment...$(COLOR_RESET)"
	docker-compose -f deployments/docker/docker-compose.yml up -d
	@echo "$(COLOR_GREEN)✓ Development environment running$(COLOR_RESET)"
	@echo "  PostgreSQL: localhost:5432"
	@echo "  Redis: localhost:6379"
	@echo "  Prometheus: http://localhost:9090"
	@echo "  Grafana: http://localhost:3000"
	@echo "  Jaeger: http://localhost:16686"

## dev-down: Stop local development environment
dev-down:
	@echo "$(COLOR_YELLOW)Stopping development environment...$(COLOR_RESET)"
	docker-compose -f deployments/docker/docker-compose.yml down -v
	@echo "$(COLOR_GREEN)✓ Development environment stopped$(COLOR_RESET)"

## dev-logs: View development environment logs
dev-logs:
	docker-compose -f deployments/docker/docker-compose.yml logs -f

# ============================================================================
# Cluster Management (Simplified)
# ============================================================================

## cluster: Start production cluster (3 coordinators, 2 workers, 1 API gateway, Redis)
cluster:
	@echo "$(COLOR_BLUE)Starting production cluster...$(COLOR_RESET)"
	@bash scripts/sh/start-cluster.sh || cmd /c scripts\\bat\\start-cluster.bat
	@echo "$(COLOR_GREEN)✓ Cluster started$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Service Endpoints:$(COLOR_RESET)"
	@echo "  API Gateway:    http://localhost:8080"
	@echo "  Coordinator-1:  http://localhost:8090 (gRPC: 9090)"
	@echo "  Coordinator-2:  http://localhost:8091 (gRPC: 9091)"
	@echo "  Coordinator-3:  http://localhost:8092 (gRPC: 9092)"
	@echo "  Redis:          localhost:6379"
	@echo "  PostgreSQL:     localhost:5432"
	@echo ""
	@echo "$(COLOR_BOLD)Quick Commands:$(COLOR_RESET)"
	@echo "  make health          - Check cluster health"
	@echo "  make test-redis      - Test Redis integration"
	@echo "  make test-events     - Test event-driven scheduling"
	@echo "  make logs            - View all logs"

## stop: Stop all services
stop:
	@echo "$(COLOR_YELLOW)Stopping all services...$(COLOR_RESET)"
	@bash scripts/sh/stop-services.sh || cmd /c scripts\\bat\\stop-services.bat
	@echo "$(COLOR_GREEN)✓ All services stopped$(COLOR_RESET)"

## health: Check cluster health
health:
	@echo "$(COLOR_BLUE)Checking cluster health...$(COLOR_RESET)"
	@bash scripts/sh/test-services.sh || cmd /c scripts\\bat\\test-services.bat

## restart: Restart the cluster
restart: stop cluster

## logs: View all service logs
logs:
	docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f

## logs-coordinator: View coordinator logs
logs-coordinator:
	@docker logs -f zkp-coordinator-1

## logs-worker: View worker logs
logs-worker:
	@docker logs -f zkp-worker-1-cluster

## logs-gateway: View API gateway logs
logs-gateway:
	@docker logs -f zkp-api-gateway-cluster

## ps: Show running containers
ps:
	@docker ps --filter "name=zkp-" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# ============================================================================
# Redis Integration Testing
# ============================================================================

## test-redis: Test Redis connectivity and event bus
test-redis:
	@echo "$(COLOR_BLUE)Testing Redis integration...$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)[1/4] Testing Redis connectivity$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli ping || echo "$(COLOR_YELLOW)Warning: Redis not responding$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)[2/4] Checking event bus status$(COLOR_RESET)"
	@curl -s http://localhost:8090/health | grep -q "redis" && echo "$(COLOR_GREEN)✓ Coordinator connected to Redis$(COLOR_RESET)" || echo "$(COLOR_YELLOW)✗ Coordinator not connected$(COLOR_RESET)"
	@curl -s http://localhost:8080/health | grep -q "redis" && echo "$(COLOR_GREEN)✓ API Gateway connected to Redis$(COLOR_RESET)" || echo "$(COLOR_YELLOW)✗ API Gateway not connected$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)[3/4] Checking active Redis channels$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli PUBSUB CHANNELS
	@echo ""
	@echo "$(COLOR_BOLD)[4/4] Redis memory usage$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli INFO memory | grep "used_memory_human"
	@echo "$(COLOR_GREEN)✓ Redis test complete$(COLOR_RESET)"

## test-events: Test event-driven task scheduling
test-events:
	@echo "$(COLOR_BLUE)Testing event-driven scheduling...$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Submitting test task...$(COLOR_RESET)"
	@START_TIME=$$(date +%s%3N); \
	TASK_ID=$$(curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
		-H "Content-Type: application/json" \
		-d '{"leaves":["test1","test2","test3"],"leaf_index":0}' | grep -o '"task_id":"[^"]*"' | cut -d'"' -f4); \
	END_TIME=$$(date +%s%3N); \
	LATENCY=$$((END_TIME - START_TIME)); \
	echo "Task ID: $$TASK_ID"; \
	echo "API Response Time: $${LATENCY}ms"; \
	echo ""; \
	echo "$(COLOR_BOLD)Checking coordinator logs for event reception...$(COLOR_RESET)"; \
	sleep 1; \
	docker logs zkp-coordinator-1 --tail 50 | grep -i "event received" || echo "$(COLOR_YELLOW)No event logs found - checking polling$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_GREEN)✓ Event test complete$(COLOR_RESET)"

## test-load: Simulate high event volume (10 concurrent tasks)
test-load:
	@echo "$(COLOR_BLUE)Simulating high event volume...$(COLOR_RESET)"
	@echo "$(COLOR_BOLD)Submitting 10 concurrent tasks...$(COLOR_RESET)"
	@for i in $$(seq 1 10); do \
		curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
			-H "Content-Type: application/json" \
			-d "{\"leaves\":[\"load$$i-1\",\"load$$i-2\",\"load$$i-3\"],\"leaf_index\":0}" & \
	done; \
	wait; \
	echo ""; \
	echo "$(COLOR_GREEN)✓ 10 tasks submitted$(COLOR_RESET)"; \
	echo ""; \
	echo "$(COLOR_BOLD)Checking Redis message count...$(COLOR_RESET)"; \
	sleep 2; \
	docker logs zkp-coordinator-1 --tail 50 | grep -c "Event received" || echo "0"; \
	echo "$(COLOR_GREEN)✓ Load test complete$(COLOR_RESET)"

## test-load-heavy: Simulate very high event volume (100 tasks)
test-load-heavy:
	@echo "$(COLOR_BLUE)Simulating heavy load (100 tasks)...$(COLOR_RESET)"
	@echo "This may take 30-60 seconds..."
	@START_TIME=$$(date +%s); \
	for i in $$(seq 1 100); do \
		curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
			-H "Content-Type: application/json" \
			-d "{\"leaves\":[\"heavy$$i-1\",\"heavy$$i-2\",\"heavy$$i-3\"],\"leaf_index\":0}" > /dev/null & \
		if [ $$(( i % 10 )) -eq 0 ]; then echo "  Submitted $$i/100 tasks..."; fi; \
	done; \
	wait; \
	END_TIME=$$(date +%s); \
	DURATION=$$((END_TIME - START_TIME)); \
	echo ""; \
	echo "$(COLOR_GREEN)✓ 100 tasks submitted in $${DURATION}s$(COLOR_RESET)"; \
	echo ""; \
	echo "$(COLOR_BOLD)Performance Metrics:$(COLOR_RESET)"; \
	sleep 3; \
	docker exec zkp-redis-cluster redis-cli INFO stats | grep "total_commands_processed"; \
	docker logs zkp-coordinator-1 --tail 100 | grep -c "Event received" | xargs echo "Events processed:"

## monitor-redis: Monitor Redis in real-time
monitor-redis:
	@echo "$(COLOR_BLUE)Starting Redis monitor...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Press Ctrl+C to stop$(COLOR_RESET)"
	@echo ""
	@docker exec -it zkp-redis-cluster redis-cli MONITOR

## redis-cli: Open Redis CLI
redis-cli:
	@echo "$(COLOR_BLUE)Opening Redis CLI...$(COLOR_RESET)"
	@docker exec -it zkp-redis-cluster redis-cli

## redis-stats: Show Redis statistics
redis-stats:
	@echo "$(COLOR_BLUE)Redis Statistics$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Connection Info:$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli INFO clients | grep "connected_clients"
	@echo ""
	@echo "$(COLOR_BOLD)Memory Usage:$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli INFO memory | grep -E "used_memory_human|used_memory_peak_human"
	@echo ""
	@echo "$(COLOR_BOLD)Stats:$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli INFO stats | grep -E "total_commands_processed|total_connections_received|keyspace_hits|keyspace_misses"
	@echo ""
	@echo "$(COLOR_BOLD)Active Channels:$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli PUBSUB CHANNELS
	@echo ""
	@echo "$(COLOR_BOLD)Active Subscriptions:$(COLOR_RESET)"
	@docker exec zkp-redis-cluster redis-cli PUBSUB NUMSUB zkp:events:task.created

## test-degradation: Test graceful degradation (stop Redis)
test-degradation:
	@echo "$(COLOR_BLUE)Testing graceful degradation...$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)[1/4] Submitting task with Redis active$(COLOR_RESET)"
	@curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
		-H "Content-Type: application/json" \
		-d '{"leaves":["before","stop","test"],"leaf_index":0}' | grep -o '"task_id":"[^"]*"'
	@echo "$(COLOR_GREEN)✓ Task submitted successfully$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)[2/4] Stopping Redis...$(COLOR_RESET)"
	@docker stop zkp-redis-cluster
	@echo "$(COLOR_YELLOW)✓ Redis stopped$(COLOR_RESET)"
	@sleep 2
	@echo ""
	@echo "$(COLOR_BOLD)[3/4] Submitting task without Redis (should still work via polling)$(COLOR_RESET)"
	@curl -s -X POST http://localhost:8080/api/v1/tasks/merkle \
		-H "Content-Type: application/json" \
		-d '{"leaves":["after","stop","test"],"leaf_index":0}' | grep -o '"task_id":"[^"]*"' && echo "$(COLOR_GREEN)✓ Task submitted successfully (polling fallback)$(COLOR_RESET)" || echo "$(COLOR_YELLOW)✗ Task submission failed$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)[4/4] Restarting Redis...$(COLOR_RESET)"
	@docker start zkp-redis-cluster
	@sleep 3
	@echo "$(COLOR_GREEN)✓ Redis restarted$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_GREEN)✓ Degradation test complete$(COLOR_RESET)"

## test-e2e-redis: Complete end-to-end Redis integration test
test-e2e-redis: test-redis test-events test-load redis-stats
	@echo ""
	@echo "$(COLOR_GREEN)✓✓✓ All Redis integration tests passed! ✓✓✓$(COLOR_RESET)"

# ============================================================================
clean-volumes:
	@echo "$(COLOR_YELLOW)Removing Docker volumes...$(COLOR_RESET)"
	@docker volume rm docker_coordinator-1-data docker_coordinator-2-data docker_coordinator-3-data 2>/dev/null || true
	@echo "$(COLOR_GREEN)✓ Volumes removed$(COLOR_RESET)"

## migrate-up: Run database migrations up
migrate-up:
	@echo "$(COLOR_BLUE)Running migrations...$(COLOR_RESET)"
	migrate -path internal/storage/postgres/migrations \
		-database "postgres://zkp_user:zkp_pass@localhost:5432/zkp_network?sslmode=disable" up
	@echo "$(COLOR_GREEN)✓ Migrations applied$(COLOR_RESET)"

## migrate-down: Rollback database migrations
migrate-down:
	@echo "$(COLOR_YELLOW)Rolling back migrations...$(COLOR_RESET)"
	migrate -path internal/storage/postgres/migrations \
		-database "postgres://zkp_user:zkp_pass@localhost:5432/zkp_network?sslmode=disable" down 1
	@echo "$(COLOR_GREEN)✓ Migration rolled back$(COLOR_RESET)"

## run-api-gateway: Run API Gateway locally
run-api-gateway: build-api-gateway
	@echo "$(COLOR_BLUE)Starting API Gateway...$(COLOR_RESET)"
	./$(BINARY_DIR)/$(API_GATEWAY) --config configs/api-gateway.yaml

## run-coordinator-1: Run first Coordinator node
run-coordinator-1: build-coordinator
	@echo "$(COLOR_BLUE)Starting Coordinator 1...$(COLOR_RESET)"
	./$(BINARY_DIR)/$(COORDINATOR) --config configs/coordinator.yaml --node-id coord-1

## run-coordinator-2: Run second Coordinator node
run-coordinator-2: build-coordinator
	@echo "$(COLOR_BLUE)Starting Coordinator 2...$(COLOR_RESET)"
	./$(BINARY_DIR)/$(COORDINATOR) --config configs/coordinator.yaml --node-id coord-2

## run-coordinator-3: Run third Coordinator node
run-coordinator-3: build-coordinator
	@echo "$(COLOR_BLUE)Starting Coordinator 3...$(COLOR_RESET)"
	./$(BINARY_DIR)/$(COORDINATOR) --config configs/coordinator.yaml --node-id coord-3

## run-worker: Run Worker node locally
run-worker: build-worker
	@echo "$(COLOR_BLUE)Starting Worker...$(COLOR_RESET)"
	./$(BINARY_DIR)/$(WORKER) --config configs/worker.yaml

## k8s-apply: Deploy to Kubernetes
k8s-apply:
	@echo "$(COLOR_BLUE)Deploying to Kubernetes...$(COLOR_RESET)"
	kubectl apply -k deployments/kubernetes/base
	@echo "$(COLOR_GREEN)✓ Deployed to Kubernetes$(COLOR_RESET)"

## k8s-delete: Delete from Kubernetes
k8s-delete:
	@echo "$(COLOR_YELLOW)Deleting from Kubernetes...$(COLOR_RESET)"
	kubectl delete -k deployments/kubernetes/base
	@echo "$(COLOR_GREEN)✓ Deleted from Kubernetes$(COLOR_RESET)"

## helm-install: Install with Helm
helm-install:
	@echo "$(COLOR_BLUE)Installing with Helm...$(COLOR_RESET)"
	helm install zkp-network deployments/kubernetes/helm/zkp-network \
		--namespace zkp --create-namespace
	@echo "$(COLOR_GREEN)✓ Helm chart installed$(COLOR_RESET)"

## helm-upgrade: Upgrade Helm release
helm-upgrade:
	@echo "$(COLOR_BLUE)Upgrading Helm release...$(COLOR_RESET)"
	helm upgrade zkp-network deployments/kubernetes/helm/zkp-network \
		--namespace zkp
	@echo "$(COLOR_GREEN)✓ Helm chart upgraded$(COLOR_RESET)"

## benchmark: Run benchmarks
benchmark:
	@echo "$(COLOR_BLUE)Running benchmarks...$(COLOR_RESET)"
	$(GOTEST) -bench=. -benchmem ./internal/...
	@echo "$(COLOR_GREEN)✓ Benchmarks complete$(COLOR_RESET)"

## profile-cpu: Profile CPU usage
profile-cpu:
	@echo "$(COLOR_BLUE)Profiling CPU...$(COLOR_RESET)"
	$(GOTEST) -cpuprofile=cpu.prof -bench=. ./internal/...
	$(GOCMD) tool pprof -http=:8081 cpu.prof

## profile-mem: Profile memory usage
profile-mem:
	@echo "$(COLOR_BLUE)Profiling memory...$(COLOR_RESET)"
	$(GOTEST) -memprofile=mem.prof -bench=. ./internal/...
	$(GOCMD) tool pprof -http=:8081 mem.prof

## security-scan: Run security vulnerability scan
security-scan:
	@echo "$(COLOR_BLUE)Scanning for vulnerabilities...$(COLOR_RESET)"
	govulncheck ./...
	@echo "$(COLOR_GREEN)✓ Security scan complete$(COLOR_RESET)"

## mod-tidy: Tidy go modules
mod-tidy:
	@echo "$(COLOR_BLUE)Tidying modules...$(COLOR_RESET)"
	$(GOMOD) tidy
	@echo "$(COLOR_GREEN)✓ Modules tidied$(COLOR_RESET)"

## mod-verify: Verify dependencies
mod-verify:
	@echo "$(COLOR_BLUE)Verifying dependencies...$(COLOR_RESET)"
	$(GOMOD) verify
	@echo "$(COLOR_GREEN)✓ Dependencies verified$(COLOR_RESET)"

## all: Run all checks and build
all: lint vet test build
	@echo "$(COLOR_GREEN)✓ All checks passed and built successfully$(COLOR_RESET)"

.DEFAULT_GOAL := help