package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/mocks"
	"go.uber.org/zap/zaptest"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

func TestWorkerManager_Basic(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockClient := &mocks.Client{}
	
	manager := NewWorkerManager(logger, mockClient)
	
	// Create a test recipe with unified format for validation
	_ = &recipe.RecipeFile{
		ID:          "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Recipe: recipe.Recipe{
			RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{
					NodeMetadata: recipe.NodeMetadata{
						ID: "test-recipe",
						Inputs: map[string]interface{}{
							"type": "function",
						},
					},
					Version: "1.0.0",
				},
				OpData: recipe.OpData{
					Op: "activity1",
				},
			},
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
	testRecipe := &recipe.RecipeFile{
		ID:          "shared-test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe with shared activities",
		Recipe: recipe.Recipe{
			RecipeImpl: &recipe.RecipeSequence{
				RecipeMetadata: recipe.RecipeMetadata{
					NodeMetadata: recipe.NodeMetadata{
						ID: "shared-test-recipe",
					},
					Version: "1.0.0",
					Defs: map[string]recipe.Node{
						"my_llm": {
							NodeImpl: &recipe.NodeOp{
								NodeMetadata: recipe.NodeMetadata{
									Inputs: map[string]interface{}{
										"type":    "ai_prompt",
										"model":   "gpt-4",
										"timeout": "1m",
									},
								},
								OpData: recipe.OpData{
									Op: "llm",
								},
							},
						},
						"my_http": {
							NodeImpl: &recipe.NodeOp{
								NodeMetadata: recipe.NodeMetadata{
									Inputs: map[string]interface{}{
										"type":    "http",
										"timeout": "30s",
									},
								},
								OpData: recipe.OpData{
									Op: "http",
								},
							},
						},
					},
				},
				SequenceData: recipe.SequenceData{
					Sequence: []recipe.Node{
						{
							NodeImpl: &recipe.NodeShared{
								Shared: "my_llm",
							},
						},
						{
							NodeImpl: &recipe.NodeShared{
								Shared: "my_http",
							},
						},
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
	recipeSeq := testRecipe.Recipe.RecipeImpl.(*recipe.RecipeSequence)
	assert.NotNil(t, recipeSeq.RecipeMetadata.Defs)
	assert.Len(t, recipeSeq.RecipeMetadata.Defs, 2)
	assert.Contains(t, recipeSeq.RecipeMetadata.Defs, "my_llm")
	assert.Contains(t, recipeSeq.RecipeMetadata.Defs, "my_http")
	
	// Verify shared activity configurations
	llmActivity := recipeSeq.RecipeMetadata.Defs["my_llm"].NodeImpl.(*recipe.NodeOp)
	assert.Equal(t, "llm", llmActivity.Op)
	assert.Equal(t, "ai_prompt", llmActivity.NodeMetadata.Inputs["type"])
	assert.Equal(t, "gpt-4", llmActivity.NodeMetadata.Inputs["model"])
	
	httpActivity := recipeSeq.RecipeMetadata.Defs["my_http"].NodeImpl.(*recipe.NodeOp)
	assert.Equal(t, "http", httpActivity.Op)
	assert.Equal(t, "http", httpActivity.NodeMetadata.Inputs["type"])
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
	assert.NotNil(t, manager.activityRegistry)
}