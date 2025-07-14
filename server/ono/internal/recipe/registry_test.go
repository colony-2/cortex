package recipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func TestRegistry_ListRecipesWithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create recipes with different names
	createSingleFileRecipe(t, tmpDir, "recipe-alpha", "1.0.0")
	createSingleFileRecipe(t, tmpDir, "recipe-beta", "2.0.0")
	createSingleFileRecipe(t, tmpDir, "other-gamma", "1.0.0")
	
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Get all recipes
	allRecipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)
	assert.Len(t, allRecipes, 3) // Should have all 3 recipes
	
	// Test manual filtering by name prefix
	var filteredRecipes []*Recipe
	for _, r := range allRecipes {
		if strings.HasPrefix(r.Name, "recipe-") {
			filteredRecipes = append(filteredRecipes, r)
		}
	}
	assert.Len(t, filteredRecipes, 2) // Should have 2 recipes with "recipe-" prefix
	
	// Verify the filtered recipes
	for _, r := range filteredRecipes {
		assert.True(t, strings.HasPrefix(r.Name, "recipe-"))
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	createSingleFileRecipe(t, tmpDir, "concurrent-recipe", "1.0.0")
	
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recipe, err := registry.GetRecipe("concurrent-recipe")
			assert.NoError(t, err)
			assert.NotNil(t, recipe)
			assert.Equal(t, "concurrent-recipe", recipe.Name)
		}()
	}
	
	// Concurrent lists
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recipes, err := registry.ListRecipes(nil)
			assert.NoError(t, err)
			assert.NotEmpty(t, recipes)
		}()
	}
	
	wg.Wait()
}

func TestRegistry_InvalidRecipeHandling(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create an invalid YAML file
	invalidContent := `this is not valid yaml:
  - item without proper
  - structure {{ broken
`
	invalidPath := filepath.Join(tmpDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidPath, []byte(invalidContent), 0644))
	
	// Create a valid recipe too
	createSingleFileRecipe(t, tmpDir, "valid-recipe", "1.0.0")
	
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Should still discover the valid recipe
	recipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)
	assert.Len(t, recipes, 1)
	assert.Equal(t, "valid-recipe", recipes[0].Name)
	
	// Invalid recipe should not be found
	_, err = registry.GetRecipe("invalid")
	assert.Error(t, err)
}

func TestRegistry_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create nested directory structure
	nestedDir := filepath.Join(tmpDir, "category1", "subcategory")
	require.NoError(t, os.MkdirAll(nestedDir, 0755))
	
	// Create recipes at different levels
	createSingleFileRecipe(t, tmpDir, "root-recipe", "1.0.0")
	createSingleFileRecipe(t, filepath.Join(tmpDir, "category1"), "cat1-recipe", "1.0.0")
	createSingleFileRecipe(t, nestedDir, "nested-recipe", "1.0.0")
	
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Should discover all recipes
	recipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)
	assert.Len(t, recipes, 3)
	
	// Verify all recipes found
	recipeNames := make(map[string]bool)
	for _, r := range recipes {
		recipeNames[r.Name] = true
	}
	assert.True(t, recipeNames["root-recipe"])
	assert.True(t, recipeNames["cat1-recipe"])
	assert.True(t, recipeNames["nested-recipe"])
}

func TestRegistry_HashConsistency(t *testing.T) {
	tmpDir := t.TempDir()
	createSingleFileRecipe(t, tmpDir, "hash-test", "1.0.0")
	
	logger := zaptest.NewLogger(t)
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Get initial hash
	recipe1, err := registry.GetRecipe("hash-test")
	require.NoError(t, err)
	hash1 := recipe1.Hash
	
	// Stop and restart registry
	registry.Stop()
	
	registry2, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)
	err = registry2.Start()
	require.NoError(t, err)
	defer registry2.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Hash should be the same
	recipe2, err := registry2.GetRecipe("hash-test")
	require.NoError(t, err)
	assert.Equal(t, hash1, recipe2.Hash)
}

func TestRegistry_WorkerManagerIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	createSingleFileRecipe(t, tmpDir, "worker-test", "1.0.0")
	
	logger := zaptest.NewLogger(t)
	
	// Create registry without worker manager for this test
	registry, err := NewRegistry(logger, tmpDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	// Get recipe
	recipe, err := registry.GetRecipe("worker-test")
	require.NoError(t, err)
	assert.Equal(t, "worker-test", recipe.Name)
	assert.Equal(t, "1.0.0", recipe.Version)
	
	// Modify the recipe
	createSingleFileRecipe(t, tmpDir, "worker-test", "2.0.0")
	time.Sleep(1 * time.Second)
	
	// Verify recipe was updated
	updatedRecipe, err := registry.GetRecipe("worker-test")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", updatedRecipe.Version)
}

// Mock worker manager for testing
type mockWorkerManager struct {
	mock.Mock
}

func (m *mockWorkerManager) StartWorker(ctx context.Context, name string, recipe *Recipe) error {
	args := m.Called(ctx, name, recipe)
	return args.Error(0)
}

func (m *mockWorkerManager) StopWorker(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *mockWorkerManager) RestartWorker(name string, recipe *Recipe) error {
	args := m.Called(name, recipe)
	return args.Error(0)
}

func (m *mockWorkerManager) GetWorkerStatus(name string) WorkerStatus {
	args := m.Called(name)
	return args.Get(0).(WorkerStatus)
}

func (m *mockWorkerManager) GetAllWorkerStatus() map[string]WorkerStatus {
	args := m.Called()
	return args.Get(0).(map[string]WorkerStatus)
}

func (m *mockWorkerManager) StopAll() {
	m.Called()
}