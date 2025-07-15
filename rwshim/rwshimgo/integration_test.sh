#!/bin/bash
# Run integration tests in Docker

set -e

echo "=== Running integration tests in Docker (golang:1.24) ==="
echo "This will build both C and Go components inside the container..."

# Get the absolute path to this script's directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Get the parent directory (rwshim)
PARENT_DIR="$(dirname "$SCRIPT_DIR")"

# Build and run in one command to avoid intermediate images
docker run --rm -v "$PARENT_DIR":/workspace -w /workspace golang:1.24 bash -c '
set -e
echo "=== Building C components ==="
cd clib
gcc -o test_app test_app.c
gcc -shared -fPIC -o intercept.so intercept.c -ldl
cd ..

echo ""
echo "=== Building Go library ==="
cd rwshimgo
go mod download
go build ./...
cd ..

echo ""
echo "=== Running integration tests ==="
cd rwshimgo
go test -tags=integration -v . -run TestIntegrationWithCShim
'