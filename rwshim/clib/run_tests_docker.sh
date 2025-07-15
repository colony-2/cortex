#!/bin/bash
# run_tests_docker.sh - Run tests inside a Docker container

set -e

echo "Running tests in Docker container..."

docker run --rm -v "/Users/jnadeau/src/vibethis/c/rwshim":/workspace -w /workspace gcc:latest bash -c '
    # Build everything
    echo "=== Building test components ==="
    gcc -shared -fPIC -o intercept.so intercept.c -ldl
    gcc -o test_app test_app.c
    gcc -o mock_monitor mock_monitor.c
    
    # Run the tests
    bash test_runner_linux.sh
'