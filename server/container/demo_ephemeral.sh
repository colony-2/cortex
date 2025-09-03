#!/bin/bash

# Demo script for ephemeral devcontainer functionality

echo "==================================="
echo "Ephemeral DevContainer Demo"
echo "==================================="
echo ""

# Create a test workspace
TEST_DIR="/tmp/ephemeral-demo-$$"
mkdir -p "$TEST_DIR/.devcontainer"
mkdir -p "$TEST_DIR/src"
mkdir -p "$TEST_DIR/tests"
mkdir -p "$TEST_DIR/docs"

echo "Created test workspace at: $TEST_DIR"

# Create a devcontainer.json with all lifecycle commands
cat > "$TEST_DIR/.devcontainer/devcontainer.json" <<'EOF'
{
  "image": "alpine:latest",
  "remoteUser": "nobody",
  "onCreateCommand": "echo '==> Running onCreate command'",
  "updateContentCommand": "echo '==> Running updateContent command'",
  "postCreateCommand": "echo '==> Running postCreate command'",
  "postStartCommand": "echo '==> Running postStart command'",
  "postAttachCommand": "echo '==> Running postAttach command'",
  "workspaceFolder": "/workspace",
  "features": {
    "ghcr.io/devcontainers/features/git:1": {}
  }
}
EOF

echo "Created devcontainer.json with lifecycle commands"
echo ""

# Create test files
echo "package main" > "$TEST_DIR/src/main.go"
echo "# README" > "$TEST_DIR/docs/README.md"
echo "package main_test" > "$TEST_DIR/tests/main_test.go"

echo "==================================="
echo "Running ephemeral container..."
echo "==================================="
echo ""
echo "Command: shai -rw src -rw tests -ephemeral"
echo ""
echo "Expected behavior:"
echo "  1. Container starts with --rm flag (auto-cleanup)"
echo "  2. Setup script runs as root:"
echo "     - Installs features (git)"
echo "     - Runs lifecycle commands in order"
echo "     - Shows progress markers"
echo "  3. Process switches to 'nobody' user"
echo "  4. Interactive shell starts"
echo "  5. src/ and tests/ are writable"
echo "  6. docs/ is read-only"
echo "  7. Container removed on exit"
echo ""
echo "==================================="
echo ""

# Show what the setup script looks like
echo "Generated setup script preview:"
echo "-------------------------------"
cat <<'SCRIPT'
#!/bin/sh
set -e

echo "::DEVCONTAINER::INIT::START::Initializing devcontainer setup"

# Install features
echo "::DEVCONTAINER::FEATURES::START::Installing devcontainer features"
echo "::DEVCONTAINER::FEATURES::PROGRESS::Installing feature ghcr.io/devcontainers/features/git:1"
apt-get update && apt-get install -y git
echo "::DEVCONTAINER::FEATURES::COMPLETE::All features installed"

# Run lifecycle commands
echo "::DEVCONTAINER::ONCREATE::START::Executing oncreate command"
echo '==> Running onCreate command'
echo "::DEVCONTAINER::ONCREATE::COMPLETE::Completed oncreate command"

echo "::DEVCONTAINER::UPDATECONTENT::START::Executing updatecontent command"
echo '==> Running updateContent command'
echo "::DEVCONTAINER::UPDATECONTENT::COMPLETE::Completed updatecontent command"

echo "::DEVCONTAINER::POSTCREATE::START::Executing postcreate command"
echo '==> Running postCreate command'
echo "::DEVCONTAINER::POSTCREATE::COMPLETE::Completed postcreate command"

echo "::DEVCONTAINER::POSTSTART::START::Executing poststart command"
echo '==> Running postStart command'
echo "::DEVCONTAINER::POSTSTART::COMPLETE::Completed poststart command"

echo "::DEVCONTAINER::POSTATTACH::START::Executing postattach command"
echo '==> Running postAttach command'
echo "::DEVCONTAINER::POSTATTACH::COMPLETE::Completed postattach command"

echo "::DEVCONTAINER::USERSWITCH::START::Switching to user nobody"

# Replace this process with user shell
exec su - nobody -c 'exec /bin/bash --login'
SCRIPT

echo ""
echo "==================================="
echo "Progress Display:"
echo "-------------------------------"
echo "⚙️  Initializing devcontainer setup"
echo "🔧 Installing devcontainer features"
echo "  → Installing feature ghcr.io/devcontainers/features/git:1"
echo "✅ All features installed"
echo "📦 Executing oncreate command"
echo "✅ Completed oncreate command"
echo "🔄 Executing updatecontent command"
echo "✅ Completed updatecontent command"
echo "🔨 Executing postcreate command"
echo "✅ Completed postcreate command"
echo "🚀 Executing poststart command"
echo "✅ Completed poststart command"
echo "📎 Executing postattach command"
echo "✅ Completed postattach command"
echo "👤 Switching to user nobody"
echo ""
echo "==================================="
echo ""

# Clean up
echo "Test workspace location: $TEST_DIR"
echo "To clean up: rm -rf $TEST_DIR"
echo ""
echo "To run the actual container (requires Docker):"
echo "  cd $TEST_DIR"
echo "  shai -rw src -rw tests"