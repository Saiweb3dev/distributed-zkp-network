#!/bin/bash

# Phase 03: Raft Integration - Dependency Installation Script
# This script installs required Go dependencies for Raft consensus

set -e

echo "========================================"
echo "  Raft Integration - Installing Deps"
echo "========================================"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed"
    echo "Please install Go 1.21+ from https://golang.org/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo "✅ Go version: $GO_VERSION"
echo ""

# Navigate to project root
cd "$(dirname "$0")/.."
echo "📁 Project directory: $(pwd)"
echo ""

# Install Raft dependencies
echo "📦 Installing Raft dependencies..."
echo ""

echo "  → Installing hashicorp/raft@v1.5.0"
go get github.com/hashicorp/raft@v1.5.0

echo "  → Installing hashicorp/raft-boltdb@v2.3.0"
go get github.com/hashicorp/raft-boltdb@v2.3.0

echo "  → Installing boltdb/bolt (dependency)"
go get github.com/boltdb/bolt@v1.3.1

echo ""
echo "🔧 Tidying up go.mod..."
go mod tidy

echo ""
echo "✅ Dependencies installed successfully!"
echo ""

# Verify installation
echo "📋 Verifying installation..."
if grep -q "github.com/hashicorp/raft" go.mod; then
    echo "  ✅ hashicorp/raft found in go.mod"
else
    echo "  ❌ hashicorp/raft NOT found in go.mod"
    exit 1
fi

if grep -q "github.com/hashicorp/raft-boltdb" go.mod; then
    echo "  ✅ hashicorp/raft-boltdb found in go.mod"
else
    echo "  ❌ hashicorp/raft-boltdb NOT found in go.mod"
    exit 1
fi

echo ""
echo "========================================"
echo "  Installation Complete!"
echo "========================================"
echo ""
echo "Next steps:"
echo "  1. Build the coordinator:"
echo "     go build -o bin/coordinator cmd/coordinator/main.go"
echo ""
echo "  2. Test single node:"
echo "     go run cmd/coordinator/main.go --config configs/coordinator-local.yaml"
echo ""
echo "  3. Test cluster:"
echo "     docker-compose -f deployments/docker/docker-compose-cluster.yml up"
echo ""
echo "  4. Read documentation:"
echo "     docs/RAFT_INTEGRATION_GUIDE.md"
echo "     docs/RAFT_QUICKSTART.md"
echo ""
