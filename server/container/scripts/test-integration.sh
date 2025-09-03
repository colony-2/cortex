#!/bin/bash
set -e

echo "Running shai integration tests..."

# Run Go integration tests with e2e tag
echo "==> Running e2e tests..."
go test -tags=e2e -v ./...

# Run TTY-specific integration tests
echo "==> Running TTY integration tests..."
go test -v -tags=integration ./internal/devcontainer -run TestTerminal || true

# Run shai initialization tests
echo "==> Running shai initialization tests..."
go test -v ./cmd/shai -run TestShaiInitialization || true

# Test shai with a real container
echo "==> Testing shai with minimal devcontainer..."
./scripts/test-shai-container.sh

# Test mount permissions
echo "==> Testing shai mount permissions..."
go test -tags=integration -v ./pkg/shai -run TestMount

echo "✓ All integration tests completed"