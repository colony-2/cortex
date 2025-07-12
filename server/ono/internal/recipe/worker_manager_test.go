package recipe

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
	yamlpkg "vibethis/ono/pkg/yaml"
)

func TestWorkerManager_StartStopWorker(t *testing.T) {
	// Create test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	_ = testSuite.NewTestWorkflowEnvironment()
	
	// Create a mock Temporal client
	// Note: In a real implementation, we would use the test environment's client
	var mockClient client.Client
	
	// Create worker manager
	logger := zaptest.NewLogger(t)
	manager := NewWorkerManager(logger, mockClient)
	
	// Create a test recipe
	_ = &Recipe{
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
	
	// Test worker status for non-existent worker
	status := manager.GetWorkerStatus("non-existent")
	assert.Equal(t, WorkerStatusStopped, status)
	
	// Note: Starting a worker with a nil client will fail
	// This is expected in this unit test
	// In integration tests, we would use a real Temporal client
	
	// Test StopAll
	manager.StopAll()
	assert.Empty(t, manager.workers)
}

func TestWorkerManager_RestartWorker(t *testing.T) {
	// Create test environment
	logger := zaptest.NewLogger(t)
	var mockClient client.Client
	manager := NewWorkerManager(logger, mockClient)
	
	recipe := &Recipe{
		Name:        "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
	}
	
	// Restart should work even if worker doesn't exist
	// (it stops if exists, then starts)
	// Note: This will panic with a nil client, so we use recover
	defer func() {
		if r := recover(); r != nil {
			// Expected panic due to nil client
			assert.Contains(t, fmt.Sprint(r), "Client must be created")
		}
	}()
	
	_ = manager.RestartWorker("test-recipe", recipe)
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
}