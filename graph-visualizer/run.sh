#!/bin/bash

echo "Starting Graph Visualizer..."
echo "Usage: ./run.sh [path-to-scan]"
echo ""

PATH_TO_SCAN=${1:-../example}

cd server
go run main.go -path="$PATH_TO_SCAN"
