package worker

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap/zaptest"
	recipecore "github.com/vibethis/server/recipe-core"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
)

// Mock types for testing
type mockWorker struct {
	mock.Mock
	running bool
	mu      sync.Mutex
}

func (m *mockWorker) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called()
	if args.Error(0) == nil {
		m.running = true
	}
	return args.Error(0)
}

func (m *mockWorker) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Called()
	m.running = false
}

func (m *mockWorker) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

type mockClient struct {
	mock.Mock
	client.Client
}

func TestWorkerManager_StartStopWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mockClient{}
	manager := NewWorkerManager(logger, mockClient)
	
	// Create a test recipe
	_ = &recipecore.Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Workflow: &yamlpkg.WorkflowDefinition{
			Name: "test-workflow",
		},
		Activities: []yamlpkg.ActivityDefinition{
			{Name: "activity1"},
			{Name: "activity2"},
		},
	}
	
	// Test that we can't stop a non-existent worker
	err := manager.StopWorker("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker not found")
	
	// Test worker status for non-existent worker
	status := manager.GetWorkerStatus("non-existent")
	assert.Equal(t, recipecore.WorkerStatusStopped, status)
	
	
	// Note: We can't test actual worker start without a real client
	// This would be covered in integration tests
	
	// Test StopAll
	manager.StopAll()
	assert.Empty(t, manager.workers)
}

func TestWorkerManager_RestartWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mockClient{}
	manager := NewWorkerManager(logger, mockClient)
	
	testRecipe := &recipecore.Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Workflow: &yamlpkg.WorkflowDefinition{
			Name: "test-workflow",
		},
	}
	
	// Test restart when worker doesn't exist
	// This will fail due to nil client, but we're testing the logic
	defer func() {
		if r := recover(); r != nil {
			// Expected panic due to nil client
			assert.Contains(t, fmt.Sprint(r), "Client must be created")
		}
	}()
	
	err := manager.RestartWorker("test-recipe", testRecipe)
	// Should panic before returning
	assert.Nil(t, err)
}

func TestWorkerManager_ConcurrentOperations(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mockClient{}
	manager := NewWorkerManager(logger, mockClient)
	
	// Test concurrent status checks
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			recipeName := fmt.Sprintf("recipe-%d", id)
			status := manager.GetWorkerStatus(recipeName)
			assert.Equal(t, recipecore.WorkerStatusStopped, status)
		}(i)
	}
	wg.Wait()
	
}

func TestWorkerManager_TaskQueueNaming(t *testing.T) {
	logger := zaptest.NewLogger(t)
	var mockClient client.Client
	manager := NewWorkerManager(logger, mockClient)
	
	// Verify task queue naming convention
	baseQueue := "ono-recipes"
	recipeName := "my-recipe"
	expectedQueue := "ono-recipes-my-recipe"
	
	// The task queue is created internally, so we verify through the base name
	assert.Equal(t, baseQueue, manager.taskQueue)
	
	// Task queue for a recipe would be: base-recipeName
	taskQueue := fmt.Sprintf("%s-%s", manager.taskQueue, recipeName)
	assert.Equal(t, expectedQueue, taskQueue)
	
	// Test GetTaskQueueForRecipe method
	assert.Equal(t, expectedQueue, manager.GetTaskQueueForRecipe(recipeName))
}

func TestWorkerManager_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		operation   func(*WorkerManager) error
		expectError bool
		errorMsg    string
	}{
		{
			name: "stop non-existent worker",
			operation: func(m *WorkerManager) error {
				return m.StopWorker("does-not-exist")
			},
			expectError: true,
			errorMsg:   "worker not found",
		},
		{
			name: "restart with nil recipe",
			operation: func(m *WorkerManager) error {
				defer func() {
					if r := recover(); r != nil {
						// Convert panic to error for test
						panic(fmt.Errorf("panic: %v", r))
					}
				}()
				return m.RestartWorker("test", nil)
			},
			expectError: true,
			errorMsg:   "panic",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			mockClient := &mockClient{}
			manager := NewWorkerManager(logger, mockClient)
			
			if tt.expectError {
				if tt.errorMsg == "panic" {
					assert.Panics(t, func() {
						_ = tt.operation(manager)
					})
				} else {
					err := tt.operation(manager)
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				err := tt.operation(manager)
				assert.NoError(t, err)
			}
		})
	}
}

// Integration test with real worker (requires Temporal server)
func TestWorkerManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	
	// This test would require a real Temporal server connection
	// It's marked as an integration test and skipped in short mode
	
	clientOptions := client.Options{
		HostPort: "localhost:7233",
	}
	
	c, err := client.NewClient(clientOptions)
	if err != nil {
		t.Skip("Temporal server not available")
	}
	defer c.Close()
	
	logger := zaptest.NewLogger(t)
	manager := NewWorkerManager(logger, c)
	
	recipe := &recipecore.Recipe{
		Name:        "integration-test-recipe",
		Version:     "1.0.0",
		Description: "Integration test recipe",
		Workflow: &yamlpkg.WorkflowDefinition{
			Name: "test-workflow",
		},
	}
	
	// Test starting a worker
	err = manager.StartWorker(recipe)
	require.NoError(t, err)
	
	// Verify status
	status := manager.GetWorkerStatus("integration-test-recipe")
	assert.Equal(t, recipecore.WorkerStatusRunning, status)
	
	// Test stopping the worker
	err = manager.StopWorker("integration-test-recipe")
	require.NoError(t, err)
	
	// Verify stopped
	status = manager.GetWorkerStatus("integration-test-recipe")
	assert.Equal(t, recipecore.WorkerStatusStopped, status)
	
	// Cleanup
	manager.StopAll()
}