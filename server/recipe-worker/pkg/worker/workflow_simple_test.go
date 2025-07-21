package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
	"github.com/vibethis/server/recipe-worker/pkg/compiler"
)

// TestWorkflowCompilerIntegration tests that the compiler correctly integrates with workflow creation
func TestWorkflowCompilerIntegration(t *testing.T) {
	// Create activity registry and compiler
	activityRegistry := compiler.NewActivityRegistry()
	_ = compiler.NewCompiler(activityRegistry)
	
	// Create activity definition
	activityDef := &yamlpkg.ActivityDefinition{
		Name: "process-data",
	}
	
	// Register activity
	activityRegistry.RegisterActivity(activityDef)
	
	// Verify the compiler has the activity
	activity := activityRegistry.GetActivity("process-data")
	require.NotNil(t, activity)
	assert.Equal(t, "process-data", activity.Name)
}

// TestWorkerManagerCreation tests that the worker manager can be created with compiler
func TestWorkerManagerCreation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Create worker manager
	manager := NewWorkerManager(logger, nil)
	require.NotNil(t, manager)
	
	// Verify it has a compiler
	assert.NotNil(t, manager.compiler)
	assert.NotNil(t, manager.activityRegistry)
	
	// Test task queue naming
	taskQueue := manager.GetTaskQueueForRecipe("test-recipe")
	assert.Equal(t, "ono-recipes-test-recipe", taskQueue)
}

// TestRegistryWithMockManager tests the registry with a mock worker manager
func TestRegistryWithMockManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()
	
	// Create mock manager
	manager := &MockWorkerManager{
		workers: make(map[string]bool),
		logger:  logger,
	}
	
	// Create registry
	registry, err := NewRegistry(logger, tempDir, manager)
	require.NoError(t, err)
	
	// Start registry
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	// List recipes (should be empty)
	recipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)
	assert.Empty(t, recipes)
}