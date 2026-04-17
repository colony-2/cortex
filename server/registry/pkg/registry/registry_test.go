package registry

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryInput and TestRegistryOutput types for test activities
type TestRegistryInput struct {
	Data string `json:"data"`
	Type string `json:"type"`
}

type TestRegistryOutput struct {
	Result string `json:"result"`
}

func init() {
	// Register test-activity for registry tests
	testActivityOp := ops.NewActivityMappedOpV2[TestRegistryInput, TestRegistryOutput](
		ops.OpMetadata{
			Type:        "test-activity",
			Description: "Test activity for registry tests",
			Version:     "1.0.0",
		},
		func(_ ops.OpDependencies, ctx context.Context, input TestRegistryInput) (TestRegistryOutput, error) {
			return TestRegistryOutput{
				Result: "success",
			}, nil
		},
	)
	ops.Register(testActivityOp)
}

func TestRegistry_NewRegistry(t *testing.T) {
	logger := stdout()
	tempDir := t.TempDir()

	registry, err := NewRegistry(logger, tempDir)
	require.NoError(t, err)
	assert.NotNil(t, registry)
	// Check that recipesDir is the absolute path of tempDir
	absTempDir, _ := filepath.Abs(tempDir)
	assert.Equal(t, absTempDir, registry.recipesDir)
	assert.NotNil(t, registry.watcher)
	assert.NotNil(t, registry.ctx)
	assert.NotNil(t, registry.cancel)
}

func stdout() *slog.Logger {
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	return slog.New(h)
}

func TestRegistry_StartStop(t *testing.T) {
	logger := stdout()
	tempDir := t.TempDir()

	registry, err := NewRegistry(logger, tempDir)
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
	logger := stdout()
	tempDir := t.TempDir()

	// Create a simple recipe file
	recipeContent := `
id: test-recipe
version: "1.0.0"
desc: Test recipe

defs:
  test-activity:
    op: test-activity
    inputs:
      type: http

sequence:
  - id: step1
    shared: test-activity
`

	recipePath := filepath.Join(tempDir, "test-recipe.yaml")
	err := os.WriteFile(recipePath, []byte(recipeContent), 0644)
	require.NoError(t, err)

	// Create registry and start it
	registry, err := NewRegistry(logger, tempDir)
	require.NoError(t, err)

	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Give it time to discover
	time.Sleep(200 * time.Millisecond)

	// Check if recipe was discovered
	recipes := registry.ListRecipes()
	assert.Len(t, recipes, 1)
	assert.Equal(t, "test-recipe", recipes[0].ID)
	assert.Equal(t, "1.0.0", recipes[0].Version)

	// Test GetRecipe
	r, err := registry.GetRecipeFile("test-recipe")
	require.NoError(t, err)
	assert.Equal(t, "test-recipe", r.ID)
}

// TestRegistry_MultiFileRecipe removed - unified format doesn't support multi-file recipes

func TestRegistry_RecipeNotFound(t *testing.T) {
	logger := stdout()
	tempDir := t.TempDir()

	registry, err := NewRegistry(logger, tempDir)
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

	logger := stdout()
	tempDir := t.TempDir()

	// Create initial recipe
	recipeContent := `
id: watch-test
version: "1.0.0"
desc: Watch test recipe

sequence:
  - id: step1
    op: test-activity
`
	recipePath := filepath.Join(tempDir, "watch-test.yaml")
	err := os.WriteFile(recipePath, []byte(recipeContent), 0644)
	require.NoError(t, err)

	// Create registry without worker manager for this test
	// TODO: Create WorkerManager interface to allow proper mocking
	registry, err := NewRegistry(logger, tempDir)
	require.NoError(t, err)

	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Wait for initial discovery
	time.Sleep(200 * time.Millisecond)

	// Verify initial recipe (uses unified format name)
	recipe, err := registry.GetRecipeFile("watch-test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", recipe.Version)

	// Update recipe rapidly multiple times
	for i := 0; i < 5; i++ {
		updatedContent := `
id: watch-test
version: "1.0.` + string(rune('1'+i)) + `"
desc: Watch test recipe

sequence:
  - id: step1
    op: test-activity
`
		err = os.WriteFile(recipePath, []byte(updatedContent), 0644)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for debounce and processing
	time.Sleep(1 * time.Second)

	// Check final version (unified format)
	recipe, err = registry.GetRecipeFile("watch-test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.5", recipe.Version) // Latest version
}
