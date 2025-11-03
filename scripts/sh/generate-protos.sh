#!/bin/bash
# scripts/generate-protos.sh
# Compiles protobuf definitions to Go code

set -e

echo "Generating gRPC code from protobuf..."

# Install dependencies if needed
if ! command -v protoc &> /dev/null; then
    echo "protoc not found. Install from: https://grpc.io/docs/protoc-installation/"
    exit 1
fi

if ! command -v protoc-gen-go &> /dev/null; then
    echo "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Installing protoc-gen-go-grpc..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

# Generate Go code
protoc \
    --go_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_out=. \
    --go-grpc_opt=paths=source_relative \
    internal/proto/coordinator/coordinator.proto

echo "✓ Generated: internal/proto/coordinator/coordinator.pb.go"
echo "✓ Generated: internal/proto/coordinator/coordinator_grpc.pb.go"
echo ""
echo "Now update coordinator_client.go to use the generated types!" 