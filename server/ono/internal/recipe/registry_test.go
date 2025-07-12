package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestRegistry_DiscoverRecipes(t *testing.T) {
	// Create temporary directory for test recipes
	tmpDir := t.TempDir()

	// Create test recipes
	createTestRecipes(t, tmpDir)

	// Create registry without worker manager for testing
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)

	// Start registry (which triggers discovery)
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Allow time for discovery
	time.Sleep(100 * time.Millisecond)

	// List all recipes
	recipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)

	// Should find 2 recipes (one multi-file, one single-file)
	assert.Len(t, recipes, 2)

	// Check multi-file recipe
	multiRecipe, err := registry.GetRecipe("test-recipe-multi")
	require.NoError(t, err)
	assert.Equal(t, "test-recipe-multi", multiRecipe.Name)
	assert.Equal(t, "1.0.0", multiRecipe.Version)
	assert.Equal(t, "A multi-file test recipe", multiRecipe.Description)
	assert.NotEmpty(t, multiRecipe.Hash)

	// Check single-file recipe
	singleRecipe, err := registry.GetRecipe("test-recipe-single")
	require.NoError(t, err)
	assert.Equal(t, "test-recipe-single", singleRecipe.Name)
	assert.Equal(t, "1.0.0", singleRecipe.Version)
	assert.NotEmpty(t, singleRecipe.Hash)
}

func TestRegistry_FileWatching(t *testing.T) {
	// Create temporary directory for test recipes
	tmpDir := t.TempDir()

	// Create initial test recipe
	createSingleFileRecipe(t, tmpDir, "watched-recipe", "1.0.0")

	// Create registry
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)

	// Start registry
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Allow time for initial discovery
	time.Sleep(100 * time.Millisecond)

	// Get initial recipe
	recipe1, err := registry.GetRecipe("watched-recipe")
	require.NoError(t, err)
	initialHash := recipe1.Hash

	// Modify the recipe
	createSingleFileRecipe(t, tmpDir, "watched-recipe", "2.0.0")

	// Allow time for file watcher to detect change
	time.Sleep(1 * time.Second)

	// Get updated recipe
	recipe2, err := registry.GetRecipe("watched-recipe")
	require.NoError(t, err)

	// Hash should have changed
	assert.NotEqual(t, initialHash, recipe2.Hash)
	assert.Equal(t, "2.0.0", recipe2.Version)
}

func TestRegistry_RecipeRemoval(t *testing.T) {
	// Create temporary directory for test recipes
	tmpDir := t.TempDir()

	// Create test recipe
	recipePath := filepath.Join(tmpDir, "temp-recipe.yaml")
	createSingleFileRecipe(t, tmpDir, "temp-recipe", "1.0.0")

	// Create registry
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)

	// Start registry
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Allow time for discovery
	time.Sleep(100 * time.Millisecond)

	// Verify recipe exists
	_, err = registry.GetRecipe("temp-recipe")
	require.NoError(t, err)

	// Remove the recipe file
	err = os.Remove(recipePath)
	require.NoError(t, err)

	// Allow time for file watcher
	time.Sleep(1 * time.Second)

	// Recipe should still exist but with stopped status
	recipe, err := registry.GetRecipe("temp-recipe")
	require.NoError(t, err)
	assert.NotNil(t, recipe)
	assert.Equal(t, WorkerStatusStopped, recipe.WorkerStatus, "Recipe worker should be stopped")
}

// Helper functions to create test recipes

func createTestRecipes(t *testing.T, dir string) {
	// Create a multi-file recipe
	multiDir := filepath.Join(dir, "multi-recipe")
	require.NoError(t, os.MkdirAll(multiDir, 0755))

	// Recipe manifest
	manifestContent := `recipe:
  name: test-recipe-multi
  version: 1.0.0
  description: A multi-file test recipe
  files:
    workflow: workflow.yaml
    activities: activities.yaml
`
	require.NoError(t, os.WriteFile(filepath.Join(multiDir, "recipe.yaml"), []byte(manifestContent), 0644))

	// Workflow file
	workflowContent := `name: test-workflow
description: Test workflow
inputs:
  - name: input1
    type: string
outputs:
  - name: output1
    type: string
steps:
  - name: step1
    activity: test-activity
`
	require.NoError(t, os.WriteFile(filepath.Join(multiDir, "workflow.yaml"), []byte(workflowContent), 0644))

	// Activities file
	activitiesContent := `activities:
  - name: test-activity
    description: Test activity
    inputs:
      - name: input1
        type: string
    outputs:
      - name: output1
        type: string
`
	require.NoError(t, os.WriteFile(filepath.Join(multiDir, "activities.yaml"), []byte(activitiesContent), 0644))

	// Create a single-file recipe
	createSingleFileRecipe(t, dir, "test-recipe-single", "1.0.0")
}

func createSingleFileRecipe(t *testing.T, dir, name, version string) {
	content := `workflow:
  name: %s
  version: %s
  description: A single-file test recipe
  
  inputs:
    - name: input1
      type: string
      required: true
  outputs:
    - name: output1
      type: string
      
  workflow:
    type: sequential
    steps:
      - id: step1
        activity: test-activity
        inputs:
          input1: "{{ .Inputs.input1 }}"
        outputs:
          output1: result
    outputs:
      output1: "{{ .Steps.step1.outputs.result }}"

activities:
  - name: test-activity
    description: Test activity
    inputs:
      - name: input1
        type: string
        required: true
    outputs:
      - name: result
        type: string
    implementation:
      type: function
      config:
        function: testActivity
`
	filename := filepath.Join(dir, name+".yaml")
	require.NoError(t, os.WriteFile(filename, []byte(fmt.Sprintf(content, name, version)), 0644))
}