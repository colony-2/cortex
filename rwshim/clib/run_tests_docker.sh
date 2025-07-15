#!/bin/bash
# run_tests_docker.sh - Run tests inside a Docker container

set -e

get_script_dir() {
  local source="${BASH_SOURCE[0]}"
  while [ -L "$source" ]; do
    local target
    target=$(readlink "$source")
    if [[ "$target" == /* ]]; then
      source="$target"
    else
      source="$(cd -P "$(dirname "$source")" && pwd)/$target"
    fi
  done
  cd -P "$(dirname "$source")" >/dev/null 2>&1 && pwd
}

echo "Running tests in Docker container..."
SCRIPT_DIR="$(get_script_dir)"
echo $SCRIPT_DIR
docker run --rm -v "$SCRIPT_DIR":/workspace -w /workspace gcc:latest bash -c '
    # Build everything
    echo "=== Building test components ==="
    gcc -shared -fPIC -o intercept.so intercept.c -ldl
    gcc -o test_app test_app.c
    gcc -o mock_monitor mock_monitor.c
    
    # Run the tests
    bash test_runner_linux.sh
'
