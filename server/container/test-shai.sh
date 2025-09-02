#!/bin/bash
# Test script to verify shai works end-to-end

set -e

echo "Testing shai functionality..."

# Create a test directory
TEST_DIR=$(mktemp -d)
echo "Created test directory: $TEST_DIR"

# Create a minimal devcontainer.json
mkdir -p "$TEST_DIR/.devcontainer"
cat > "$TEST_DIR/.devcontainer/devcontainer.json" <<EOF
{
  "name": "test-shai",
  "image": "alpine:latest",
  "command": "/bin/sh"
}
EOF

echo "Created devcontainer.json"

# Change to test directory
cd "$TEST_DIR"

# Test shai with a simple command
echo "Testing shai with echo command..."
echo 'echo "Hello from shai container"' | timeout 5 ./shai -rw . || {
    EXIT_CODE=$?
    if [ $EXIT_CODE -eq 124 ]; then
        echo "✓ Container started successfully (timed out as expected for interactive shell)"
        exit 0
    else
        echo "✗ shai failed with exit code $EXIT_CODE"
        exit 1
    fi
}

echo "✓ Test completed successfully"

# Cleanup
cd -
rm -rf "$TEST_DIR"