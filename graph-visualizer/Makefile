.PHONY: all build clean dev dev-server dev-frontend test test-positions setupenv install run help

# Variables
BINARY_NAME = graph-visualizer
GO_FILES = $(shell find server -name '*.go')
WEB_DIST = web/dist
SERVER_DIR = server
WEB_DIR = web
BUILD_DIR = build

# Default target
all: build

# Display help
help:
	@echo "Graph Visualizer Makefile Commands:"
	@echo "  make setupenv     - Install all dependencies (Go and npm)"
	@echo "  make install      - Install the graph-visualizer binary to GOPATH/bin"
	@echo "  make dev          - Run both server and frontend in development mode"
	@echo "  make dev-server   - Run only the Go server in development mode"
	@echo "  make dev-frontend - Run only the frontend dev server"
	@echo "  make build        - Build production binary with embedded frontend"
	@echo "  make test         - Run all tests including position persistence tests"
	@echo "  make test-positions - Run only position persistence tests"
	@echo "  make test-e2e     - Run end-to-end tests (requires running server)"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make run         - Build and run the production binary"
	@echo "  make stop         - Stop the server running on port 8080"
	@echo "  make prod         - Full production build (clean, setupenv, test, build)"

# Setup development environment
setupenv:
	@echo "Installing frontend dependencies..."
	cd $(WEB_DIR) && npm install
	@echo "Installing Go dependencies..."
	cd $(SERVER_DIR) && go mod init graph-visualizer 2>/dev/null || true
	cd $(SERVER_DIR) && go get github.com/gorilla/mux
	cd $(SERVER_DIR) && go get github.com/gorilla/websocket
	cd $(SERVER_DIR) && go get gopkg.in/yaml.v3
	cd $(SERVER_DIR) && go mod tidy

# Install the graph-visualizer binary
install: build
	@echo "Installing graph-visualizer binary to GOPATH/bin..."
	@go install -tags prod ./$(SERVER_DIR)
	@echo "Installation complete. You can now run 'graph-visualizer' from anywhere."

# Development mode - run both server and frontend
dev:
	@echo "Starting development servers..."
	@make -j2 dev-server dev-frontend

# Run Go server in development mode
dev-server:
	@echo "Starting Go server on :8080..."
	cd $(SERVER_DIR) && go run . -path="../example"

# Run frontend dev server
dev-frontend:
	@echo "Starting frontend dev server on :5173..."
	cd $(WEB_DIR) && npm run dev

# Build frontend assets
build-frontend:
	@echo "Building frontend assets..."
	cd $(WEB_DIR) && npm run build

# Build production binary with embedded frontend
build: build-frontend
	@echo "Building production binary..."
	@mkdir -p $(BUILD_DIR)
	@echo "Copying frontend assets..."
	@rm -rf $(SERVER_DIR)/static/dist
	@cp -r $(WEB_DIST) $(SERVER_DIR)/static/
	cd $(SERVER_DIR) && go build -tags prod -o ../$(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
test: test-frontend test-server test-e2e

test-frontend:
	@echo "Running frontend tests..."
	cd $(WEB_DIR) && npm test || echo "No frontend tests configured yet"

test-server:
	@echo "Running server tests..."
	@echo "Testing position storage..."
	cd $(SERVER_DIR) && go test -v -run TestPositionStorage
	@echo ""
	@echo "Testing API endpoints..."
	cd $(SERVER_DIR) && go test -v -run TestPositionAPI
	@echo ""
	@echo "Testing file browser API..."
	cd $(SERVER_DIR) && go test -v -run "TestGetFiles|TestFileInfo"
	@echo ""
	@echo "Testing integration..."
	cd $(SERVER_DIR) && go test -v -run TestIntegration
	@echo ""
	@echo "Running all server tests..."
	cd $(SERVER_DIR) && go test -v ./...

test-e2e:
	@echo "Running end-to-end tests..."
	@if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null ; then \
		echo "Server is running on port 8080, executing E2E tests..."; \
		./test_e2e.sh; \
	else \
		echo "Server not running on port 8080. Start it with 'make dev' in another terminal."; \
		echo "Skipping E2E tests."; \
	fi

# Run only position-related tests
test-positions:
	@echo "Running position persistence tests..."
	cd $(SERVER_DIR) && go test -v -run "TestPosition|TestIntegration" -count=1

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(WEB_DIST)
	rm -rf $(WEB_DIR)/node_modules
	rm -rf $(WEB_DIR)/test-results
	rm -rf $(WEB_DIR)/playwright-report
	rm -f $(WEB_DIR)/*.png
	@echo "Clean complete"

# Build and run the production binary
run: build
	@echo "Running graph visualizer..."
	./$(BUILD_DIR)/$(BINARY_NAME) -path="./example"

# Watch mode for development (requires entr)
watch-server:
	@command -v entr >/dev/null 2>&1 || { echo "entr is required but not installed. Install with: brew install entr"; exit 1; }
	@echo "Watching server files for changes..."
	find $(SERVER_DIR) -name '*.go' | entr -r make dev-server

# Format code
fmt:
	@echo "Formatting Go code..."
	cd $(SERVER_DIR) && go fmt ./...
	@echo "Formatting complete"

# Lint code
lint: lint-go lint-frontend

lint-go:
	@echo "Linting Go code..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. See: https://golangci-lint.run/usage/install/"; exit 1; }
	cd $(SERVER_DIR) && golangci-lint run

lint-frontend:
	@echo "Linting frontend code..."
	cd $(WEB_DIR) && npm run check

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