# Integration Testing Specification

## Overview

This specification defines the comprehensive testing strategy for the devcontainer cell execution system, covering unit tests, integration tests, and end-to-end tests across all components.

## Testing Scope

### Components to Test

1. **Recipe Infrastructure**: Cross-Temporal workflow invocation
2. **Devcontainer Management**: Container lifecycle and mounts
3. **Workflow Execution**: Cortex to Nucleus workflow chain
4. **Mount Verification**: Read-only and read-write access
5. **Error Handling**: Failure scenarios and recovery

## Test Categories

### 1. Unit Tests

#### Recipe Infrastructure Tests

```go
// server/recipe-core/pkg/activity/temporal_workflow_test.go
func TestTemporalWorkflowActivity(t *testing.T) {
    tests := []struct {
        name    string
        config  TemporalWorkflowConfig
        input   TemporalWorkflowInput
        want    TemporalWorkflowOutput
        wantErr bool
    }{
        {
            name: "successful workflow submission",
            config: TemporalWorkflowConfig{
                Host:      "localhost:7233",
                Namespace: "test",
                TaskQueue: "test-queue",
            },
            input: TemporalWorkflowInput{
                WorkflowName: "test-workflow",
                Inputs:       map[string]interface{}{"key": "value"},
            },
            want: TemporalWorkflowOutput{
                WorkflowID: "test-123",
                Status:     "completed",
                Result:     map[string]interface{}{"output": "success"},
            },
        },
        {
            name: "connection failure",
            config: TemporalWorkflowConfig{
                Host: "invalid:7233",
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Mock Temporal client
            client := &mockTemporalClient{}
            activity := NewTemporalWorkflowActivity(client)
            
            got, err := activity.Execute(context.Background(), tt.config, tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Execute() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

#### Devcontainer Management Tests

```go
// server/activity/pkg/devcontainer/devcontainer_test.go
func TestDevcontainerUp(t *testing.T) {
    tests := []struct {
        name    string
        input   DevcontainerUpInput
        want    DevcontainerUpOutput
        wantErr bool
    }{
        {
            name: "basic container start",
            input: DevcontainerUpInput{
                WorkspaceFolder: "/test/workspace",
                ConfigPath:      ".devcontainer.json",
                Mounts: []MountConfig{
                    {Type: "bind", Source: "/src", Target: "/src", ReadOnly: true},
                },
            },
            want: DevcontainerUpOutput{
                ContainerID: "abc123",
                ContainerInfo: map[string]interface{}{
                    "State": "running",
                },
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Mock devcontainer CLI
            activity := NewDevcontainerActivity(mockCLI)
            
            got, err := activity.Execute(context.Background(), 
                DevcontainerManagementConfig{Operation: "up"}, tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Execute() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 2. Integration Tests

#### Workflow Integration Test

```yaml
# recipes/test/integration-test.yaml
name: devcontainer-integration-test
version: 1.0.0
description: Integration test for devcontainer workflow execution

workflow:
  type: sequential
  steps:
    # Setup test environment
    - id: setup_test_cell
      activity: shell
      inputs:
        run: |
          # Create test cell with devcontainer
          mkdir -p test-integration-cell
          cat > test-integration-cell/.devcontainer.json << 'EOF'
          {
            "name": "Test Cell",
            "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
            "postCreateCommand": "echo 'Test cell ready'"
          }
          EOF
          
          # Create test file
          echo "test content" > test-integration-cell/test.txt
          
          # Initialize git repo
          git init
          git add .
          git commit -m "Test commit"
          
          echo "Test cell created"
      outputs:
        setup_result: stdout

    # Execute main workflow
    - id: execute_devcontainer_workflow
      activity: temporal_workflow
      inputs:
        temporal_host: "localhost:7233"
        workflow_name: "devcontainer-launcher"
        workflow_inputs:
          cell_name: "test-integration-cell"
          inner_workflow: "cell-executor"
          cleanup_on_success: false  # Keep for inspection
      outputs:
        execution_result: result

    # Verify execution results
    - id: verify_results
      activity: shell
      inputs:
        run: |
          RESULT='{{ .Steps.execute_devcontainer_workflow.Outputs.execution_result }}'
          
          # Check container was created
          CONTAINER_ID=$(echo "$RESULT" | jq -r '.container_id')
          if docker inspect "$CONTAINER_ID" > /dev/null 2>&1; then
            echo "✓ Container created: $CONTAINER_ID"
          else
            echo "✗ Container not found"
            exit 1
          fi
          
          # Check working directory exists
          WORKING_DIR=$(echo "$RESULT" | jq -r '.working_directory')
          if [ -d "$WORKING_DIR" ]; then
            echo "✓ Working directory exists: $WORKING_DIR"
          else
            echo "✗ Working directory not found"
            exit 1
          fi
          
          # Check inner workflow completed
          INNER_STATUS=$(echo "$RESULT" | jq -r '.inner_workflow_result.execution_summary.success')
          if [ "$INNER_STATUS" = "true" ]; then
            echo "✓ Inner workflow completed successfully"
          else
            echo "✗ Inner workflow failed"
            exit 1
          fi
          
          echo "All verifications passed"
      outputs:
        verification: stdout

    # Cleanup test resources
    - id: cleanup
      activity: shell
      inputs:
        run: |
          # Stop and remove container
          CONTAINER_ID='{{ .Steps.execute_devcontainer_workflow.Outputs.execution_result.container_id }}'
          docker stop "$CONTAINER_ID" || true
          docker rm "$CONTAINER_ID" || true
          
          # Remove test cell
          rm -rf test-integration-cell
          
          echo "Cleanup completed"
        continue_on_error: true
      outputs:
        cleanup_result: stdout

  outputs:
    test_passed: true
    setup_result: "{{ .Steps.setup_test_cell.Outputs.setup_result }}"
    execution_result: "{{ .Steps.execute_devcontainer_workflow.Outputs.execution_result }}"
    verification: "{{ .Steps.verify_results.Outputs.verification }}"
```

#### Mount Verification Test

```yaml
# recipes/test/mount-verification-test.yaml
name: mount-verification-test
version: 1.0.0
description: Verify mount configuration works correctly

workflow:
  type: sequential
  steps:
    - id: create_test_structure
      activity: shell
      inputs:
        run: |
          # Create cell with subdirectories
          mkdir -p mount-test-cell/src
          echo "cell file" > mount-test-cell/cell-file.txt
          echo "source code" > mount-test-cell/src/code.js
          
          # Create other cells
          mkdir -p other-cell
          echo "other cell" > other-cell/file.txt
          
          # Create root file
          echo "root file" > root-file.txt

    - id: execute_with_mounts
      activity: temporal_workflow
      inputs:
        temporal_host: "localhost:7233"
        workflow_name: "devcontainer-launcher"
        workflow_inputs:
          cell_name: "mount-test-cell"
          inner_workflow: "mount-test-workflow"

    - id: verify_mount_behavior
      activity: shell
      inputs:
        run: |
          RESULT='{{ .Steps.execute_with_mounts.Outputs.result }}'
          
          # Extract test results
          ROOT_READONLY=$(echo "$RESULT" | jq -r '.mount_tests.root_readonly')
          CELL_READWRITE=$(echo "$RESULT" | jq -r '.mount_tests.cell_readwrite')
          OTHER_READONLY=$(echo "$RESULT" | jq -r '.mount_tests.other_readonly')
          
          # Verify expectations
          if [ "$ROOT_READONLY" = "true" ] && \
             [ "$CELL_READWRITE" = "true" ] && \
             [ "$OTHER_READONLY" = "true" ]; then
            echo "✓ Mount verification passed"
          else
            echo "✗ Mount verification failed"
            echo "  Root read-only: $ROOT_READONLY (expected: true)"
            echo "  Cell read-write: $CELL_READWRITE (expected: true)"
            echo "  Other read-only: $OTHER_READONLY (expected: true)"
            exit 1
          fi
```

### 3. End-to-End Tests

#### Complete System Test

```go
// test/e2e/devcontainer_e2e_test.go
func TestDevcontainerE2E(t *testing.T) {
    // Start test infrastructure
    ctx := context.Background()
    testEnv := setupTestEnvironment(t)
    defer testEnv.Cleanup()
    
    // Start Cortex
    cortex := testEnv.StartCortex()
    
    // Start embedded Temporal for Cortex
    cortexTemporal := testEnv.StartTemporal("cortex", 7233)
    
    // Create test cell
    cellPath := filepath.Join(testEnv.WorkDir, "test-cell")
    createTestCell(t, cellPath)
    
    // Submit workflow
    client := cortex.GetClient()
    execution, err := client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
        ID:        "e2e-test-" + uuid.New().String(),
        TaskQueue: "recipe-worker",
    }, "devcontainer-launcher", map[string]interface{}{
        "cell_name": "test-cell",
        "cleanup_on_success": true,
    })
    require.NoError(t, err)
    
    // Wait for completion
    var result map[string]interface{}
    err = execution.Get(ctx, &result)
    require.NoError(t, err)
    
    // Verify results
    assert.NotEmpty(t, result["execution_id"])
    assert.NotEmpty(t, result["container_id"])
    assert.Equal(t, "completed", result["inner_workflow_result"].(map[string]interface{})["status"])
}
```

### 4. Performance Tests

#### Load Test

```go
func TestDevcontainerLoadTest(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    ctx := context.Background()
    testEnv := setupTestEnvironment(t)
    defer testEnv.Cleanup()
    
    // Test parameters
    numCells := 10
    parallelExecutions := 5
    
    // Create test cells
    cells := make([]string, numCells)
    for i := 0; i < numCells; i++ {
        cells[i] = fmt.Sprintf("load-test-cell-%d", i)
        createTestCell(t, filepath.Join(testEnv.WorkDir, cells[i]))
    }
    
    // Execute workflows in parallel
    var wg sync.WaitGroup
    errors := make(chan error, parallelExecutions)
    durations := make(chan time.Duration, parallelExecutions)
    
    for i := 0; i < parallelExecutions; i++ {
        wg.Add(1)
        go func(index int) {
            defer wg.Done()
            
            start := time.Now()
            err := executeWorkflow(ctx, cells[index%numCells])
            duration := time.Since(start)
            
            if err != nil {
                errors <- err
            } else {
                durations <- duration
            }
        }(i)
    }
    
    wg.Wait()
    close(errors)
    close(durations)
    
    // Analyze results
    var totalDuration time.Duration
    var successCount int
    
    for err := range errors {
        t.Errorf("Workflow failed: %v", err)
    }
    
    for d := range durations {
        totalDuration += d
        successCount++
    }
    
    avgDuration := totalDuration / time.Duration(successCount)
    t.Logf("Load test results: %d/%d succeeded, avg duration: %v", 
        successCount, parallelExecutions, avgDuration)
    
    // Assert performance expectations
    assert.Equal(t, parallelExecutions, successCount, "All workflows should succeed")
    assert.Less(t, avgDuration, 5*time.Minute, "Average duration should be under 5 minutes")
}
```

## Test Data

### Test Fixtures

```
test/fixtures/
├── devcontainers/
│   ├── minimal.json
│   ├── full-featured.json
│   └── invalid.json
├── cells/
│   ├── nodejs/
│   │   ├── package.json
│   │   └── index.js
│   ├── golang/
│   │   ├── go.mod
│   │   └── main.go
│   └── python/
│       ├── requirements.txt
│       └── app.py
└── recipes/
    ├── test-workflows.yaml
    └── test-activities.yaml
```

## Error Scenario Tests

### 1. Configuration Errors

```yaml
tests:
  - name: missing_devcontainer
    expect_error: "No devcontainer.json found"
    
  - name: invalid_devcontainer_json
    expect_error: "Failed to parse devcontainer.json"
    
  - name: missing_cell_directory
    expect_error: "Cell directory not found"
```

### 2. Runtime Errors

```yaml
tests:
  - name: container_start_failure
    mock_docker_error: "insufficient resources"
    expect_error: "Failed to start container"
    
  - name: nucleus_connection_failure
    mock_network_error: true
    expect_error: "Failed to connect to Nucleus"
    
  - name: workflow_timeout
    inner_workflow_delay: "40m"
    expect_error: "Workflow execution timed out"
```

### 3. Resource Cleanup

```go
func TestResourceCleanup(t *testing.T) {
    // Execute workflow that fails
    result := executeWorkflowExpectingFailure(t, "failing-cell")
    
    // Verify cleanup happened
    containerID := result["container_id"].(string)
    _, err := docker.InspectContainer(containerID)
    assert.Error(t, err, "Container should be removed after failure")
    
    // Verify working directory cleaned up (if configured)
    workingDir := result["working_directory"].(string)
    _, err = os.Stat(workingDir)
    assert.Error(t, err, "Working directory should be removed")
}
```

## Test Execution

### CI/CD Pipeline

```yaml
# .github/workflows/integration-tests.yml
name: Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Install devcontainer CLI
        run: npm install -g @devcontainers/cli
      
      - name: Run unit tests
        run: |
          cd server
          go test ./... -v -cover
      
      - name: Start test infrastructure
        run: |
          docker-compose -f test/docker-compose.yml up -d
          ./scripts/wait-for-temporal.sh
      
      - name: Run integration tests
        run: |
          cd test
          go test ./integration/... -v
      
      - name: Run E2E tests
        run: |
          cd test
          go test ./e2e/... -v
      
      - name: Cleanup
        if: always()
        run: docker-compose -f test/docker-compose.yml down -v
```

### Local Testing

```bash
# Run all tests
make test

# Run specific test category
make test-unit
make test-integration
make test-e2e

# Run with coverage
make test-coverage

# Run load tests
make test-load

# Debug specific test
go test -v -run TestDevcontainerE2E ./test/e2e/...
```

## Test Monitoring

### Metrics to Track

1. **Test Duration**: Track how long each test category takes
2. **Flaky Tests**: Identify tests that fail intermittently
3. **Resource Usage**: Monitor Docker resource consumption
4. **Coverage Trends**: Ensure coverage doesn't decrease

### Test Reports

Generate comprehensive test reports:

```bash
# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Generate test timing report
go test -json ./... | go-test-report

# Generate load test report
go test -bench=. -benchmem -cpuprofile=cpu.prof
go tool pprof -http=:8080 cpu.prof
```

## Success Criteria

1. **Unit Test Coverage**: Minimum 80% code coverage
2. **Integration Tests**: All scenarios pass consistently
3. **E2E Tests**: Complete workflow execution in < 5 minutes
4. **Load Tests**: Handle 10 parallel executions
5. **Error Handling**: All error scenarios properly tested
6. **Resource Cleanup**: No leaked containers or files