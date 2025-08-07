package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

func TestRegistry_NewRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()
	
	registry, err := NewRegistry(logger, tempDir, nil)
	require.NoError(t, err)
	assert.NotNil(t, registry)
	// Check that recipesDir is the absolute path of tempDir
	absTempDir, _ := filepath.Abs(tempDir)
	assert.Equal(t, absTempDir, registry.recipesDir)
	assert.NotNil(t, registry.watcher)
	assert.NotNil(t, registry.ctx)
	assert.NotNil(t, registry.cancel)
}

func TestRegistry_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()
	
	registry, err := NewRegistry(logger, tempDir, nil)
	require.NoError(t, err)
	
	// Start registry
	err = registry.Start()
	require.NoError(t, err)
	
	// Give it a moment to start watching
	time.Sleep(100 * time.Millisecond)
	
	// Stop registry
	err = registry.Stop()
	assert.NoError(t, err)
}

func TestRegistry_RecipeDiscovery(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()
	
	// Create a simple recipe file
	recipeContent := `
name: test-recipe
version: "1.0.0"
description: Test recipe

steps:
  - id: step1
    uses: test-activity

shared:
  test-activity:
    uses: test-activity
    config:
      type: http
`
	
	recipePath := filepath.Join(tempDir, "test-recipe.yaml")
	err := os.WriteFile(recipePath, []byte(recipeContent), 0644)
	require.NoError(t, err)
	
	// Create registry and start it
	registry, err := NewRegistry(logger, tempDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	// Give it time to discover
	time.Sleep(200 * time.Millisecond)
	
	// Check if recipe was discovered
	recipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)
	assert.Len(t, recipes, 1)
	assert.Equal(t, "test-recipe", recipes[0].Name)
	assert.Equal(t, "1.0.0", recipes[0].Version)
	
	// Test GetRecipe
	recipe, err := registry.GetRecipe("test-recipe")
	require.NoError(t, err)
	assert.Equal(t, "test-recipe", recipe.Name)
}

// TestRegistry_MultiFileRecipe removed - unified format doesn't support multi-file recipes

func TestRegistry_RecipeNotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()
	
	registry, err := NewRegistry(logger, tempDir, nil)
	require.NoError(t, err)
	
	// Try to get non-existent recipe
	_, err = registry.GetRecipe("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_FileWatchingDebounce(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watching test")
	}
	
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()
	
	// Create initial recipe
	recipeContent := `
name: watch-test
version: "1.0.0"

steps:
  - id: step1
    uses: test-activity
`
	recipePath := filepath.Join(tempDir, "watch-test.yaml")
	err := os.WriteFile(recipePath, []byte(recipeContent), 0644)
	require.NoError(t, err)
	
	// Create registry without worker manager for this test
	// TODO: Create WorkerManager interface to allow proper mocking
	registry, err := NewRegistry(logger, tempDir, nil)
	require.NoError(t, err)
	
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()
	
	// Wait for initial discovery
	time.Sleep(200 * time.Millisecond)
	
	// Verify initial recipe (uses unified format name)
	recipe, err := registry.GetRecipe("watch-test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", recipe.Version)
	
	// Update recipe rapidly multiple times
	for i := 0; i < 5; i++ {
		updatedContent := `
name: watch-test
version: "1.0.` + string(rune('1'+i)) + `"

steps:
  - id: step1
    uses: test-activity
`
		err = os.WriteFile(recipePath, []byte(updatedContent), 0644)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}
	
	// Wait for debounce and processing
	time.Sleep(1 * time.Second)
	
	// Check final version (unified format)
	recipe, err = registry.GetRecipe("watch-test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.5", recipe.Version) // Latest version
}

func TestRegistry_ListRecipesWithFilter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()
	
	registry, err := NewRegistry(logger, tempDir, nil)
	require.NoError(t, err)
	
	// Add some test recipes manually
	registry.recipes["recipe1"] = &recipe.Recipe{
		Name:         "recipe1",
		WorkerStatus: recipe.WorkerStatusRunning,
	}
	registry.recipes["recipe2"] = &recipe.Recipe{
		Name:         "recipe2",
		WorkerStatus: recipe.WorkerStatusStopped,
	}
	registry.recipes["recipe3"] = &recipe.Recipe{
		Name:         "recipe3",
		WorkerStatus: recipe.WorkerStatusRunning,
	}
	
	// Test filter by status
	runningStatus := recipe.WorkerStatusRunning
	filter := &recipe.RecipeFilter{
		Status: &runningStatus,
	}
	
	recipes, err := registry.ListRecipes(filter)
	require.NoError(t, err)
	assert.Len(t, recipes, 2)
	
	// Verify all returned recipes have running status
	for _, r := range recipes {
		assert.Equal(t, recipe.WorkerStatusRunning, r.WorkerStatus)
	}
}

