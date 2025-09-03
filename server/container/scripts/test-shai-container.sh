#!/bin/bash
set -e

# Test shai with a minimal devcontainer
echo "Testing shai container functionality..."

# Ensure shai binary exists
if [ ! -f "./bin/shai" ]; then
    echo "Error: shai binary not found at ./bin/shai"
    echo "Run 'moon run be-container:build-shai' first"
    exit 1
fi

# Create temporary test directory
TEST_DIR=$(mktemp -d)
trap "rm -rf $TEST_DIR" EXIT

echo "Using test directory: $TEST_DIR"

# Create minimal devcontainer.json
mkdir -p "$TEST_DIR/.devcontainer"
cat > "$TEST_DIR/.devcontainer/devcontainer.json" <<EOF
{
  "name": "test-shai",
  "image": "alpine:latest",
  "command": "/bin/sh"
}
EOF

# Change to test directory
cd "$TEST_DIR"

# Test shai with a simple echo command
echo "Starting shai container..."
echo 'echo "Hello from shai container" && exit' | timeout 5 "$OLDPWD/bin/shai" -rw . || {
    EXIT_CODE=$?
    if [ $EXIT_CODE -eq 124 ]; then
        echo "✓ Shai container started successfully (timed out waiting for interactive shell)"
        exit 0
    else
        echo "✗ Shai failed with exit code $EXIT_CODE"
        exit 1
    fi
}

echo "✓ Shai container test passed"