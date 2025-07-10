#!/bin/bash
set -e

echo "Building the server..."
moon run build

echo "Starting the server..."
./bin/ono start --db-filename ./test.db --log-level debug &
SERVER_PID=$!

echo "Server PID: $SERVER_PID"

# Give it some time to start
sleep 10

echo "Checking if server is running..."
if ps -p $SERVER_PID > /dev/null; then
    echo "Server is running"
    
    # Try to connect
    echo "Testing connection..."
    if curl -f http://127.0.0.1:7233 2>/dev/null; then
        echo "Server is responding"
    else
        echo "Server is not responding"
    fi
    
    # Kill the server
    echo "Stopping server..."
    kill $SERVER_PID
    wait $SERVER_PID 2>/dev/null || true
else
    echo "Server failed to start"
    exit 1
fi

echo "Test completed"