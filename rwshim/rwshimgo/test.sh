#!/bin/bash
# Script to run tests in Docker

echo "Building test container..."
docker build -f Dockerfile.test -t rwshimgo-test .

echo "Running tests in container..."
docker run --rm rwshimgo-test