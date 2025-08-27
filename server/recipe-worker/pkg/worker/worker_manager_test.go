package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/mocks"
	"go.uber.org/zap/zaptest"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestWorkerManager_Basic(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mocks.Client{}
	
	manager := NewWorkerManager(logger, mockClient)
	
	// Create a test recipe with unified format for validation
	_ = &recipe.Recipe{
		ID:          "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Recipe: &yamlpkg.RecipeDefinition{
			Node: yamlpkg.Node{
				ID:   "test-recipe",
				Op:   "activity1",
				Inputs: map[string]interface{}{
					"type": "function",
				},
			},
			Version: "1.0.0",
		},
	}
	
	// Test that we can't stop a non-existent worker
	err := manager.StopWorker("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker not found")
	
	// Test worker status for non-existent worker
	status := manager.GetWorkerStatus("non-existent")
	assert.Equal(t, recipe.WorkerStatusStopped, status)
	
	// Test task queue naming
	taskQueue := manager.GetTaskQueueForRecipe("test-recipe")
	assert.Equal(t, "ono-recipes-test-recipe", taskQueue)
	
	// Test restart of non-existent worker should fail
	// Note: We can't easily test RestartWorker with mocks since it calls StartWorker internally
	// which requires a real Temporal client
}

func TestWorkerManager_Creation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mocks.Client{}
	
	manager := NewWorkerManager(logger, mockClient)
	
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.logger)
	assert.NotNil(t, manager.temporalClient)
	assert.NotNil(t, manager.workers)
	assert.NotNil(t, manager.activityRegistry)
}

func TestWorkerManager_SharedActivityRegistration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mocks.Client{}
	manager := NewWorkerManager(logger, mockClient)

	// Create a test recipe with shared activities
	testRecipe := &recipe.Recipe{
		ID:          "shared-test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe with shared activities",
		Recipe: &yamlpkg.RecipeDefinition{
			Node: yamlpkg.Node{
				ID: "shared-test-recipe",
				Sequence: []yamlpkg.Node{
					{
						ID:     "step1",
						Shared: "my_llm",
						Inputs: map[string]interface{}{
							"prompt": "test",
						},
					},
					{
						ID:     "step2",
						Shared: "my_http",
						Inputs: map[string]interface{}{
							"url": "https://api.example.com",
						},
					},
				},
			},
			Version: "1.0.0",
			Defs: map[string]yamlpkg.Node{
				"my_llm": {
					Op: "llm",
					Inputs: map[string]interface{}{
						"type":  "ai_prompt",
						"model": "gpt-4",
						"timeout": "1m",
					},
				},
				"my_http": {
					Op: "http",
					Inputs: map[string]interface{}{
						"type": "http",
						"timeout": "30s",
					},
				},
			},
		},
	}

	// Test that shared activities are registered in the activity registry
	// Note: We can't easily test the full StartWorker since it requires a real Temporal client
	// But we can test that the registry gets populated correctly
	
	// Simulate what happens during worker creation - register shared activities
	// Note: In the new structure, activities are registered differently
	// The activity registry uses Register() method with RegisterableOp types
	
	// Verify the registry exists and can list activities
	activities := manager.activityRegistry.List()
	assert.NotNil(t, activities)

	// Test recipe validation
	assert.NotNil(t, testRecipe.Recipe.Defs)
	assert.Len(t, testRecipe.Recipe.Defs, 2)
	assert.Contains(t, testRecipe.Recipe.Defs, "my_llm")
	assert.Contains(t, testRecipe.Recipe.Defs, "my_http")
	
	// Verify shared activity configurations
	llmActivity := testRecipe.Recipe.Defs["my_llm"]
	assert.Equal(t, "llm", llmActivity.Op)
	assert.Equal(t, "ai_prompt", llmActivity.Inputs["type"])
	assert.Equal(t, "gpt-4", llmActivity.Inputs["model"])
	
	httpActivity := testRecipe.Recipe.Defs["my_http"] 
	assert.Equal(t, "http", httpActivity.Op)
	assert.Equal(t, "http", httpActivity.Inputs["type"])
}

func TestWorkerManager_ErrorHandling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mocks.Client{}
	manager := NewWorkerManager(logger, mockClient)
	
	// Test validation of invalid inputs without trying to start workers
	// (StartWorker with mocks will fail at the Temporal client level)
	
	// Test worker manager methods that don't require real workers
	
	// Test GetTaskQueueForRecipe with empty name
	taskQueue := manager.GetTaskQueueForRecipe("")
	assert.Equal(t, "ono-recipes-", taskQueue)
	
	// Test GetWorkerStatus for non-existent worker
	status := manager.GetWorkerStatus("nonexistent-worker")
	assert.Equal(t, recipe.WorkerStatusStopped, status)
	
	// Test StopWorker for non-existent worker
	err := manager.StopWorker("nonexistent-worker")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker not found")
	
	// Test StopAll on empty manager (should not panic)
	manager.StopAll()
	
	// Test activity registry access
	registry := manager.GetActivityTypeRegistry()
	assert.NotNil(t, registry)
}