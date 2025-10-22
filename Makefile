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
COLOR_RESET := \033[0m
COLOR_BOLD := \033[1m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_BLUE := \033[34m

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