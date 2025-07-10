#!/bin/bash

echo "Testing workflow commands..."

# Test help commands
echo "=== Testing workflow help ==="
go run main.go workflow --help

echo -e "\n=== Testing workflow list help ==="
go run main.go workflow list --help

echo -e "\n=== Testing workflow describe help ==="
go run main.go workflow describe --help

echo -e "\n=== Testing workflow history help ==="
go run main.go workflow history --help

echo -e "\n=== Testing workflow restart help ==="
go run main.go workflow restart --help

echo -e "\n=== Testing workflow activities help ==="
go run main.go workflow activities --help

echo -e "\n=== Testing workflow run help ==="
go run main.go workflow run --help

echo -e "\nAll help commands executed successfully!"