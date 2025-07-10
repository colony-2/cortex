#!/bin/bash

echo "Diagnosing ono start issues"
echo "=========================="
echo ""

# Check if ono is installed
if ! command -v ono &> /dev/null; then
    echo "❌ ono command not found. Run: moon run install"
    exit 1
fi

echo "✓ ono command found at: $(which ono)"
echo ""

# Check ports
echo "Checking default ports..."
if lsof -Pi :7233 -sTCP:LISTEN -t >/dev/null ; then
    echo "❌ Port 7233 is already in use (Temporal frontend)"
    echo "   Try: ono start --port 17233"
else
    echo "✓ Port 7233 is available"
fi

if lsof -Pi :8233 -sTCP:LISTEN -t >/dev/null ; then
    echo "❌ Port 8233 is already in use (Temporal UI)"
    echo "   Try: ono start --ui-port 18233"
else
    echo "✓ Port 8233 is available"
fi

echo ""
echo "Checking file permissions..."
if [ -f "./ono.db" ]; then
    if [ -w "./ono.db" ]; then
        echo "✓ Database file ./ono.db exists and is writable"
    else
        echo "❌ Database file ./ono.db exists but is not writable"
    fi
else
    echo "✓ Database file ./ono.db will be created"
fi

echo ""
echo "Testing with in-memory database..."
echo "Running: ono start --in-memory --port 17233 --ui-port 18233"
echo "(Press Ctrl+C to stop)"
echo ""

# Try to start with non-standard ports and in-memory DB
ono start --in-memory --port 17233 --ui-port 18233