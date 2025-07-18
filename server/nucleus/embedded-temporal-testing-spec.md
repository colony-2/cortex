# Embedded Temporal Testing Integration Specification

## Overview
Enhance the recipe-watcher CLI test coverage by integrating the embeddedtemporal package to enable full end-to-end testing without requiring an external Temporal server.

## Current State
- Unit test coverage: 62.6% overall
  - Config module: 90.9% (excellent)
  - Client module: 80.0% (good)
  - Main module: 45.9% (low due to integration dependencies)
- Untested areas:
  - Main execution flow (`run()` function)
  - Temporal client connection
  - Recipe-worker integration
  - Signal handling and graceful shutdown
  - Worker lifecycle management

## Goals
1. Achieve >80% test coverage across all modules
2. Enable integration testing without external dependencies
3. Test the complete workflow from CLI invocation to worker shutdown
4. Validate error handling in realistic scenarios

## Implementation Plan

### 1. Add Embedded Temporal Dependency

Update `go.mod`:
```go
require (
    github.com/vibethis/server/embeddedtemporal v0.0.0
)

replace (
    github.com/vibethis/server/embeddedtemporal => ../embeddedtemporal
    // existing replaces...
)
```

### 2. Create Integration Test Package

Create `cmd/recipe-watcher/integration_test.go`:
```go
//go:build integration
// +build integration

package main

import (
    "testing"
    embeddedtemporal "github.com/vibethis/server/embeddedtemporal/pkg/temporal"
)
```

### 3. Test Scenarios to Implement

#### A. Full Lifecycle Test
```go
func TestRecipeWatcherFullLifecycle(t *testing.T) {
    // 1. Start embedded Temporal
    // 2. Create test recipes directory
    // 3. Run recipe-watcher
    // 4. Verify worker starts
    // 5. Send shutdown signal
    // 6. Verify graceful shutdown
}
```

#### B. Configuration Tests
```go
func TestRecipeWatcherWithEmbeddedTemporal(t *testing.T) {
    tests := []struct {
        name string
        args []string
        wantErr bool
    }{
        {"valid config", []string{"--name", "test", "--recipes-path", "./recipes"}, false},
        {"custom namespace", []string{"--name", "test", "--recipes-path", "./recipes", "--namespace", "custom"}, false},
        {"debug mode", []string{"--name", "test", "--recipes-path", "./recipes", "--debug"}, false},
    }
}
```

#### C. Error Handling Tests
```go
func TestRecipeWatcherErrorScenarios(t *testing.T) {
    // Test Temporal connection failures
    // Test recipe-worker initialization errors
    // Test shutdown during startup
}
```

#### D. Recipe Processing Tests
```go
func TestRecipeWatcherProcessesRecipes(t *testing.T) {
    // Create sample recipe files
    // Verify workers are created for each recipe
    // Test recipe hot-reload
    // Verify task queue naming
}
```

### 4. Test Utilities

Create `internal/testutil/embedded.go`:
```go
package testutil

import (
    "testing"
    embeddedtemporal "github.com/vibethis/server/embeddedtemporal/pkg/temporal"
)

type TestServer struct {
    server *embeddedtemporal.Server
    client client.Client
}

func StartTestServer(t *testing.T) *TestServer {
    // Initialize and start embedded Temporal
    // Return test server instance
}

func (ts *TestServer) Cleanup() {
    // Stop server and cleanup resources
}
```

### 5. Mock Recipe Worker (if needed)

Create `internal/testutil/mock_worker.go`:
```go
package testutil

type MockRecipeWorker struct {
    StartCalled bool
    StopCalled  bool
    // Add other tracking fields
}

func NewMockRecipeWorker() *MockRecipeWorker {
    // Return configured mock
}
```

### 6. Signal Handling Tests

```go
func TestSignalHandling(t *testing.T) {
    // Start recipe-watcher in goroutine
    // Send SIGTERM
    // Verify graceful shutdown
    // Test timeout scenarios
}
```

### 7. Concurrent Testing

```go
func TestMultipleWorkers(t *testing.T) {
    // Start multiple recipe-watchers
    // Verify isolation
    // Test concurrent shutdown
}
```

## Test Structure

```
cmd/recipe-watcher/
├── main.go
├── main_test.go          # Existing unit tests
├── integration_test.go   # New integration tests
└── testdata/            # Test fixtures
    └── recipes/
        ├── valid-recipe.yaml
        └── invalid-recipe.yaml

internal/testutil/
├── embedded.go          # Embedded Temporal utilities
├── mock_worker.go       # Mock recipe worker
└── fixtures.go          # Test data generators
```

## Makefile Updates

```makefile
test-integration:
	@echo "Running integration tests with embedded Temporal..."
	go test -v -tags=integration -race ./...

test-all: test test-integration

coverage-full:
	@echo "Generating full coverage report..."
	go test -v -tags=integration -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
```

## Expected Outcomes

1. **Coverage Improvement**:
   - Main module: 45.9% → >80%
   - Overall: 62.6% → >85%

2. **Test Reliability**:
   - No external dependencies required
   - Consistent test environment
   - Faster test execution

3. **Better Error Detection**:
   - Edge cases in Temporal integration
   - Worker lifecycle issues
   - Resource cleanup problems

## Implementation Steps

1. [ ] Add embeddedtemporal dependency to go.mod
2. [ ] Create testutil package with embedded Temporal helpers
3. [ ] Write integration tests for main execution flow
4. [ ] Add signal handling tests
5. [ ] Create test fixtures (sample recipes)
6. [ ] Update Makefile with new test targets
7. [ ] Run tests and verify coverage improvement
8. [ ] Document any limitations or special considerations

## Notes

- Integration tests should be tagged to run separately from unit tests
- Embedded Temporal may have limitations compared to full server
- Consider test execution time and parallelize where possible
- Ensure proper cleanup to avoid resource leaks
- May need to mock some recipe-worker behavior if it has external dependencies