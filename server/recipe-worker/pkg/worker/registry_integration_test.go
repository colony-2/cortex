package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
)

// MockWorkerManager tracks worker operations without creating real workers
type MockWorkerManager struct {
	workers map[string]bool
	mu      sync.RWMutex
	logger  *zap.Logger
}

func (m *MockWorkerManager) StartWorker(r *recipe.Recipe) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workers[r.Name] = true
	m.logger.Info("Mock: Started worker", zap.String("recipe", r.Name))
	return nil
}

func (m *MockWorkerManager) StopWorker(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workers, name)
	m.logger.Info("Mock: Stopped worker", zap.String("recipe", name))
	return nil
}

func (m *MockWorkerManager) RestartWorker(name string, r *recipe.Recipe) error {
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
	
	// Create a test recipe file - using correct nested structure
	recipeContent := `
workflow:
  name: integration-test
  version: "1.0.0"
  description: Integration test recipe
  
  workflow:
    type: sequential
    steps:
      - id: step1
        activity: process-data
        inputs:
          data: "test-data"
        outputs:
          result: processed
    outputs:
      final_result: "{{ .Steps.step1.outputs.result }}"
      
activities:
  - name: process-data
    description: Process data activity
    timeout: 30s
    implementation:
      type: function
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
	recipeName := recipes[0].Name
	status := manager.GetWorkerStatus(recipeName)
	assert.Equal(t, recipe.WorkerStatusRunning, status)
	
	// Test recipe update
	updatedContent := `
workflow:
  name: integration-test  
  version: "2.0.0"
  description: Updated integration test recipe
  
  workflow:
    type: sequential
    steps:
      - id: step1
        activity: process-data
        inputs:
          data: "updated-test-data"
        outputs:
          result: processed
      - id: step2
        activity: transform-data
        inputs:
          input: "{{ .Steps.step1.outputs.result }}"
        outputs:
          result: transformed
    outputs:
      final_result: "{{ .Steps.step2.outputs.result }}"
      
activities:
  - name: process-data
    description: Process data activity
    timeout: 30s
    implementation:
      type: function
  - name: transform-data
    description: Transform data activity
    timeout: 30s
    implementation:
      type: function
`
	
	err = os.WriteFile(recipePath, []byte(updatedContent), 0644)
	require.NoError(t, err)
	
	// Wait for file watcher to pick up changes
	time.Sleep(1 * time.Second)
	
	// Verify recipe was updated
	updatedRecipe, err := registry.GetRecipe(recipeName)
	require.NoError(t, err)
	assert.NotEqual(t, recipes[0].Hash, updatedRecipe.Hash)
	
	// Test recipe removal
	err = os.Remove(recipePath)
	require.NoError(t, err)
	
	// Wait for file watcher
	time.Sleep(1 * time.Second)
	
	// Verify recipe was removed
	_, err = registry.GetRecipe(recipeName)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "not found")
	}
	
	// Verify worker was stopped
	status = manager.GetWorkerStatus(recipeName)
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
	
	// Create recipe.yaml
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
	
	// Create workflow.yaml
	workflowContent := `
name: multi-workflow
version: "1.0.0"
workflow:
  type: sequential
  steps:
    - id: prepare
      activity: prepare-data
      inputs:
        source: "test"
      outputs:
        data: prepared
    - id: process
      activity: process-data
      inputs:
        data: "{{ .Steps.prepare.outputs.data }}"
      outputs:
        result: processed
  outputs:
    result: "{{ .Steps.process.outputs.result }}"
`
	err = os.WriteFile(filepath.Join(recipeDir, "workflow.yaml"), []byte(workflowContent), 0644)
	require.NoError(t, err)
	
	// Create activities.yaml
	activitiesContent := `
activities:
  - name: prepare-data
    description: Prepare data for processing
    timeout: 30s
    implementation:
      type: function
  - name: process-data
    description: Process the prepared data
    timeout: 1m
    implementation:
      type: function
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
	assert.Equal(t, "multi-file-test", foundRecipe.Name)
	assert.Equal(t, "1.0.0", foundRecipe.Version)
	assert.NotNil(t, foundRecipe.Workflow)
	assert.Len(t, foundRecipe.Activities, 2)
	
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
workflow:
  name: concurrent-test-%d
  version: "1.0.0"
  
  workflow:
    type: sequential
    steps:
      - id: step1
        activity: activity-%d
        
activities:
  - name: activity-%d
    timeout: 30s
    implementation:
      type: function
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
		if manager.GetWorkerStatus(r.Name) == recipe.WorkerStatusRunning {
			runningCount++
		}
	}
	assert.Equal(t, recipeCount, runningCount)
	
	// Cleanup
	manager.StopAll()
}