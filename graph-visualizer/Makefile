.PHONY: all build clean dev dev-server dev-frontend test test-positions setupenv install run help

# Variables
BINARY_NAME = vibethis
MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
WORKSPACE_DIR = $(MAKEFILE_DIR)server-workspace
WEB_DIST = $(MAKEFILE_DIR)web/dist
WEB_DIR = $(MAKEFILE_DIR)web
BUILD_DIR = $(MAKEFILE_DIR)build
CMD_DIR = $(WORKSPACE_DIR)/cmd/vibethis

# Default target
all: build

# Display help
help:
	@echo "Graph Visualizer Makefile Commands:"
	@echo "  make setupenv     - Install all dependencies (Go and npm)"
	@echo "  make install      - Install the vibethis binary to GOPATH/bin"
	@echo "  make dev          - Run both server and frontend in development mode"
	@echo "  make dev-server   - Run only the Go server in development mode"
	@echo "  make dev-frontend - Run only the frontend dev server"
	@echo "  make build        - Build production binary with embedded frontend"
	@echo "  make test         - Run all tests"
	@echo "  make test-server  - Run only server tests"
	@echo "  make test-frontend - Run only frontend tests"
	@echo "  make test-e2e     - Run end-to-end tests (requires running server)"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make run         - Build and run the production binary"
	@echo "  make stop         - Stop the server running on port 8080"
	@echo "  make prod         - Full production build (clean, setupenv, test, build)"
	@echo "  make workspace-init - Initialize Go workspace"

# Initialize Go workspace
workspace-init:
	@echo "Initializing Go workspace..."
	cd "$(WORKSPACE_DIR)" && go work init || true
	cd "$(WORKSPACE_DIR)" && go work use ./core ./storage ./graph ./files ./git ./container ./web ./cmd/vibethis
	@echo "Installing Go module dependencies..."
	cd "$(WORKSPACE_DIR)" && go work sync

# Setup development environment
setupenv: workspace-init
	@echo "Installing frontend dependencies..."
	cd "$(WEB_DIR)" && npm install
	@echo "Installing Go workspace dependencies..."
	cd "$(WORKSPACE_DIR)/core" && go mod tidy
	cd "$(WORKSPACE_DIR)/storage" && go mod tidy
	cd "$(WORKSPACE_DIR)/graph" && go mod tidy
	cd "$(WORKSPACE_DIR)/files" && go mod tidy
	cd "$(WORKSPACE_DIR)/git" && go mod tidy
	cd "$(WORKSPACE_DIR)/container" && go mod tidy
	cd "$(WORKSPACE_DIR)/web" && go mod tidy
	cd "$(CMD_DIR)" && go mod tidy

# Install the vibethis binary
install: build
	@echo "Installing vibethis binary to GOPATH/bin..."
	@echo "Setting default port to 8081 for installed binary..."
	cd "$(CMD_DIR)" && go install -tags prod -ldflags "-X main.defaultPort=8081" .
	@echo "Installation complete. You can now run 'vibethis' from anywhere."
	@echo "Default port: 8081 (override with -p or --port flag)"

# Development mode - run both server and frontend
dev:
	@echo "Starting development servers..."
	@make -j2 dev-server dev-frontend

# Run Go server in development mode
dev-server:
	@echo "Starting Go server on :8080..."
	cd "$(CMD_DIR)" && go run . -n ../../../example

# Run frontend dev server
dev-frontend:
	@echo "Starting frontend dev server on :5173..."
	cd "$(WEB_DIR)" && npm run dev

# Build frontend assets
build-frontend:
	@echo "Building frontend assets..."
	cd "$(WEB_DIR)" && npm run build

# Build production binary with embedded frontend
build: build-frontend
	@echo "Building production binary..."
	@mkdir -p "$(BUILD_DIR)"
	@echo "Copying frontend assets..."
	@rm -rf "$(CMD_DIR)/static/dist"
	@mkdir -p "$(CMD_DIR)/static"
	@cp -r "$(WEB_DIST)" "$(CMD_DIR)/static/"
	cd "$(CMD_DIR)" && go build -tags prod -o "$(BUILD_DIR)/$(BINARY_NAME)" .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
test: test-server test-frontend test-e2e

test-frontend:
	@echo "Running frontend tests..."
	cd "$(WEB_DIR)" && npm test || echo "No frontend tests configured yet"

test-server:
	@echo "Running server tests..."
	@echo "Testing core module..."
	cd "$(WORKSPACE_DIR)/core" && go test -v ./...
	@echo ""
	@echo "Testing storage module..."
	cd "$(WORKSPACE_DIR)/storage" && go test -v ./...
	@echo ""
	@echo "Testing graph module..."
	cd "$(WORKSPACE_DIR)/graph" && go test -v ./...
	@echo ""
	@echo "Testing files module..."
	cd "$(WORKSPACE_DIR)/files" && go test -v ./...
	@echo ""
	@echo "Testing git module..."
	cd "$(WORKSPACE_DIR)/git" && go test -v ./...
	@echo ""
	@echo "Testing container module..."
	cd "$(WORKSPACE_DIR)/container" && go test -v ./...
	@echo ""
	@echo "Testing web module..."
	cd "$(WORKSPACE_DIR)/web" && go test -v ./...
	@echo ""
	@echo "Testing cmd module..."
	cd "$(CMD_DIR)" && go test -v ./...

test-e2e:
	@echo "Running end-to-end tests..."
	@if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null ; then \
		echo "Server is running on port 8080, executing E2E tests..."; \
		cd "$(WEB_DIR)" && npm run test:e2e; \
	else \
		echo "Server not running on port 8080. Start it with 'make dev' in another terminal."; \
		echo "Skipping E2E tests."; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf "$(BUILD_DIR)"
	rm -rf "$(WEB_DIST)"
	rm -rf "$(WEB_DIR)/node_modules"
	rm -rf "$(WEB_DIR)/test-results"
	rm -rf "$(WEB_DIR)/playwright-report"
	rm -f "$(WEB_DIR)"/*.png
	@echo "Clean complete"

# Build and run the production binary
run: build
	@echo "Running vibethis..."
	"$(BUILD_DIR)/$(BINARY_NAME)" -n ./example

# Watch mode for development (requires entr)
watch-server:
	@command -v entr >/dev/null 2>&1 || { echo "entr is required but not installed. Install with: brew install entr"; exit 1; }
	@echo "Watching server files for changes..."
	find "$(WORKSPACE_DIR)" -name '*.go' | entr -r make dev-server

# Format code
fmt:
	@echo "Formatting Go code..."
	cd "$(WORKSPACE_DIR)" && go fmt ./...
	@echo "Formatting complete"

# Lint code
lint: lint-go lint-frontend

lint-go:
	@echo "Linting Go code..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. See: https://golangci-lint.run/usage/install/"; exit 1; }
	cd "$(WORKSPACE_DIR)" && golangci-lint run ./...

lint-frontend:
	@echo "Linting frontend code..."
	cd "$(WEB_DIR)" && npm run check

# Docker build (optional)
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):latest .

# Production build with all optimizations
prod: clean setupenv test build
	@echo "Production build complete!"

# Stop running server
stop:
	@echo "Stopping server on port 8080..."
	@lsof -ti:8080 | xargs kill -9 2>/dev/null || true
	@echo "Server stopped"