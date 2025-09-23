package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestInput and TestOutput types for test activities
type TestInput struct {
	Data    string `json:"data"`
	Source  string `json:"source"`
	Type    string `json:"type"`
	Timeout string `json:"timeout"`
}

type TestOutput struct {
	Result    string `json:"result"`
	Data      string `json:"data"`
	Prepared  string `json:"prepared"`
	Processed string `json:"processed"`
}

func init() {
	// Register test activities for integration tests
	// Note: Name field is what's used for lookup when op: is specified in YAML
	processDataOp := ops.NewActivityMappedOpV2[TestInput, TestOutput](
		ops.OpMetadata{
			Type:        "process-data",
			Description: "Test activity for processing data",
			Version:     "1.0.0",
		},
		func(_ ops.Invocation, ctx context.Context, input TestInput) (TestOutput, error) {
			return TestOutput{
				Result:    "processed",
				Processed: input.Data + "_processed",
			}, nil
		},
	)
	ops.Register(processDataOp)

	prepareDataOp := ops.NewActivityMappedOpV2[TestInput, TestOutput](
		ops.OpMetadata{
			Type:        "prepare-data",
			Description: "Test activity for preparing data",
			Version:     "1.0.0",
		},
		func(_ ops.Invocation, ctx context.Context, input TestInput) (TestOutput, error) {
			return TestOutput{
				Data:     "prepared_data",
				Prepared: input.Source + "_prepared",
			}, nil
		},
	)
	ops.Register(prepareDataOp)

	transformDataOp := ops.NewActivityMappedOpV2[TestInput, TestOutput](
		ops.OpMetadata{
			Type:        "transform-data",
			Description: "Test activity for transforming data",
			Version:     "1.0.0",
		},
		func(_ ops.Invocation, ctx context.Context, input TestInput) (TestOutput, error) {
			return TestOutput{
				Result: "transformed",
				Data:   input.Data + "_transformed",
			}, nil
		},
	)
	ops.Register(transformDataOp)
}

// MockWorkerManager tracks worker operations without creating real workers
type MockWorkerManager struct {
	workers map[string]bool
	mu      sync.RWMutex
	logger  *zap.Logger
}

func (m *MockWorkerManager) StartWorker(r *recipe.RecipeFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workers[r.ID] = true
	m.logger.Info("Mock: Started worker", zap.String("recipe", r.ID))
	return nil
}

func (m *MockWorkerManager) StopWorker(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workers, name)
	m.logger.Info("Mock: Stopped worker", zap.String("recipe", name))
	return nil
}

func (m *MockWorkerManager) RestartWorker(name string, r *recipe.RecipeFile) error {
	m.StopWorker(name)
	return m.StartWorker(r)
}

func (m *MockWorkerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workers = make(map[string]bool)
}

func (m *MockWorkerManager) GetWorkerStatus(name string) recipe.WorkerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.workers[name] {
		return recipe.WorkerStatusRunning
	}
	return recipe.WorkerStatusStopped
}

func (m *MockWorkerManager) GetTaskQueueForRecipe(name string) string {
	return fmt.Sprintf("ono-recipes-%s", name)
}

func TestRegistryWorkerIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	// For integration testing of registry, we'll use a mock manager
	// that tracks calls without creating real workers
	manager := &MockWorkerManager{
		workers: make(map[string]bool),
		logger:  logger,
	}

	// Create registry with the mock manager
	registry, err := NewRegistry(logger, tempDir, manager)
	require.NoError(t, err)

	// Create a test recipe file - using unified format
	recipeContent := `
id: integration-test
version: "1.0.0"
desc: Integration test recipe

sequence:
  - id: step1
    op: process-data
    inputs:
      data: "test-data"
      type: function
      timeout: 30s
`

	recipePath := filepath.Join(tempDir, "integration-test.yaml")
	err = os.WriteFile(recipePath, []byte(recipeContent), 0644)
	require.NoError(t, err)

	// Start the registry
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Give it time to discover and start worker
	time.Sleep(500 * time.Millisecond)

	// Check if recipe was discovered
	recipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)
	assert.Len(t, recipes, 1)

	// Verify worker was started
	recipeID := recipes[0].ID
	status := manager.GetWorkerStatus(recipeID)
	assert.Equal(t, recipe.WorkerStatusRunning, status)

	// Test recipe update
	updatedContent := `
id: integration-test
version: "2.0.0"
desc: Updated integration test recipe

defs:
  process-data:
    op: process-data
    inputs:
      type: function
      timeout: 30s
  transform-data:
    op: transform-data
    inputs:
      type: function
      timeout: 30s

sequence:
  - id: step1
    shared: process-data
    inputs:
      data: "updated-test-data"
    outputs:
      result: processed
  - id: step2
    shared: transform-data
    inputs:
      input: "{{ .nodes.step1.result }}"
    outputs:
      result: transformed
`

	err = os.WriteFile(recipePath, []byte(updatedContent), 0644)
	require.NoError(t, err)

	// Wait for file watcher to pick up changes
	time.Sleep(1 * time.Second)

	// Verify recipe was updated
	updatedRecipe, err := registry.GetRecipe(recipeID)
	require.NoError(t, err)
	assert.NotEqual(t, recipes[0].Hash, updatedRecipe.Hash)

	// Test recipe removal
	err = os.Remove(recipePath)
	require.NoError(t, err)

	// Wait for file watcher
	time.Sleep(1 * time.Second)

	// Verify recipe was removed
	_, err = registry.GetRecipe(recipeID)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "not found")
	}

	// Verify worker was stopped
	status = manager.GetWorkerStatus(recipeID)
	assert.Equal(t, recipe.WorkerStatusStopped, status)

	// Cleanup
	manager.StopAll()
}

func TestMultiFileRecipeWorkerCreation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	// Use mock manager for testing
	manager := &MockWorkerManager{
		workers: make(map[string]bool),
		logger:  logger,
	}

	// Create registry
	registry, err := NewRegistry(logger, tempDir, manager)
	require.NoError(t, err)

	// Create a multi-file recipe structure
	recipeDir := filepath.Join(tempDir, "multi-recipe")
	err = os.MkdirAll(recipeDir, 0755)
	require.NoError(t, err)

	// Create recipe.yaml with unified format reference
	recipeManifest := `
recipe:
  name: multi-file-test
  version: "1.0.0"
  description: Multi-file test recipe
  files:
    workflow: workflow.yaml
    activities: activities.yaml
`
	err = os.WriteFile(filepath.Join(recipeDir, "recipe.yaml"), []byte(recipeManifest), 0644)
	require.NoError(t, err)

	// Create workflow.yaml using unified format
	workflowContent := `
id: multi-file-test
version: "1.0.0"
desc: Multi-file test recipe

sequence:
  - id: prepare
    op: prepare-data
    inputs:
      source: "test"
      type: function
      timeout: 30s
  - id: process
    op: process-data
    inputs:
      data: "{{ .nodes.prepare.data }}"
      type: function
      timeout: 1m
`
	err = os.WriteFile(filepath.Join(recipeDir, "workflow.yaml"), []byte(workflowContent), 0644)
	require.NoError(t, err)

	// Create activities.yaml (can be empty since shared activities are in workflow.yaml)
	activitiesContent := `
# Additional activities can be defined here if needed
# The main activities are defined in workflow.yaml shared section
`
	err = os.WriteFile(filepath.Join(recipeDir, "activities.yaml"), []byte(activitiesContent), 0644)
	require.NoError(t, err)

	// Start the registry
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Give it time to discover
	time.Sleep(500 * time.Millisecond)

	// Check if recipe was discovered
	foundRecipe, err := registry.GetRecipe("multi-file-test")
	require.NoError(t, err)
	assert.Equal(t, "multi-file-test", foundRecipe.ID)
	assert.Equal(t, "1.0.0", foundRecipe.Version)
	assert.NotNil(t, foundRecipe.Recipe)
	if foundRecipe.Recipe.RecipeImpl != nil {
		if recipeSeq, ok := foundRecipe.Recipe.RecipeImpl.(*recipe.RecipeSequence); ok {
			assert.NotEmpty(t, recipeSeq.Sequence)
		}
	}

	// Verify worker was started
	status := manager.GetWorkerStatus("multi-file-test")
	assert.Equal(t, recipe.WorkerStatusRunning, status)

	// Verify task queue
	taskQueue := manager.GetTaskQueueForRecipe("multi-file-test")
	assert.Equal(t, "ono-recipes-multi-file-test", taskQueue)

	// Cleanup
	manager.StopAll()
}

func TestConcurrentRecipeDiscovery(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	// Use mock manager for testing
	manager := &MockWorkerManager{
		workers: make(map[string]bool),
		logger:  logger,
	}

	// Create registry
	registry, err := NewRegistry(logger, tempDir, manager)
	require.NoError(t, err)

	// Create multiple recipe files concurrently
	recipeCount := 5
	done := make(chan bool, recipeCount)

	for i := 0; i < recipeCount; i++ {
		go func(index int) {
			recipeContent := fmt.Sprintf(`
id: concurrent-test-%d
version: "1.0.0"
desc: Concurrent test recipe %d

sequence:
  - id: step1
    op: process-data
    inputs:
      data: "test-%d"
      type: function
      timeout: 30s
`, index, index, index)

			recipePath := filepath.Join(tempDir, fmt.Sprintf("%d-concurrent.yaml", index))
			err := os.WriteFile(recipePath, []byte(recipeContent), 0644)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all files to be created
	for i := 0; i < recipeCount; i++ {
		<-done
	}

	// Start the registry
	err = registry.Start()
	require.NoError(t, err)
	defer registry.Stop()

	// Give it time to discover all recipes
	time.Sleep(1 * time.Second)

	// Check if all recipes were discovered
	recipes, err := registry.ListRecipes(nil)
	require.NoError(t, err)
	assert.Len(t, recipes, recipeCount)

	// Verify all workers were started
	runningCount := 0
	for _, r := range recipes {
		if manager.GetWorkerStatus(r.ID) == recipe.WorkerStatusRunning {
			runningCount++
		}
	}
	assert.Equal(t, recipeCount, runningCount)

	// Cleanup
	manager.StopAll()
}
