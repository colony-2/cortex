#!/bin/bash

# Development helper script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_color() {
    color=$1
    message=$2
    echo -e "${color}${message}${NC}"
}

# Check if dependencies are installed
check_deps() {
    print_color $YELLOW "Checking dependencies..."
    
    if ! command -v go &> /dev/null; then
        print_color $RED "Go is not installed. Please install Go first."
        exit 1
    fi
    
    if ! command -v node &> /dev/null; then
        print_color $RED "Node.js is not installed. Please install Node.js first."
        exit 1
    fi
    
    print_color $GREEN "All dependencies found!"
}

# Main menu
show_menu() {
    echo
    print_color $GREEN "Graph Visualizer Development Menu"
    echo "================================="
    echo "1. Install dependencies"
    echo "2. Run development servers (frontend + backend)"
    echo "3. Run frontend only"
    echo "4. Run backend only"
    echo "5. Build production binary"
    echo "6. Run tests"
    echo "7. Clean build artifacts"
    echo "8. Exit"
    echo
}

# Handle user choice
handle_choice() {
    case $1 in
        1)
            print_color $YELLOW "Installing dependencies..."
            make install
            ;;
        2)
            print_color $YELLOW "Starting development servers..."
            make dev
            ;;
        3)
            print_color $YELLOW "Starting frontend dev server..."
            make dev-frontend
            ;;
        4)
            print_color $YELLOW "Starting backend server..."
            make dev-server
            ;;
        5)
            print_color $YELLOW "Building production binary..."
            make build
            ;;
        6)
            print_color $YELLOW "Running tests..."
            make test
            ;;
        7)
            print_color $YELLOW "Cleaning build artifacts..."
            make clean
            ;;
        8)
            print_color $GREEN "Goodbye!"
            exit 0
            ;;
        *)
            print_color $RED "Invalid option. Please try again."
            ;;
    esac
}

# Main script
check_deps

if [ $# -eq 0 ]; then
    # Interactive mode
    while true; do
        show_menu
        read -p "Select an option: " choice
        handle_choice $choice
        read -p "Press Enter to continue..."
    done
else
    # Command line mode
    case $1 in
        install) make install ;;
        dev) make dev ;;
        frontend) make dev-frontend ;;
        backend) make dev-server ;;
        build) make build ;;
        test) make test ;;
        clean) make clean ;;
        *) 
            print_color $RED "Unknown command: $1"
            echo "Usage: $0 [install|dev|frontend|backend|build|test|clean]"
            exit 1
            ;;
    esac
fi