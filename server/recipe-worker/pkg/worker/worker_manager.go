package worker

import (
	"context"
	"fmt"
	"sync"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
	"github.com/vibethis/server/recipe-core/pkg/recipe"
)

// WorkerManager manages the lifecycle of workers for recipes
type WorkerManager struct {
	logger        *zap.Logger
	temporalClient client.Client
	workers       map[string]worker.Worker // key is recipe name
	mu            sync.RWMutex
	taskQueue     string // Base task queue name
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(logger *zap.Logger, temporalClient client.Client) *WorkerManager {
	return &WorkerManager{
		logger:         logger,
		temporalClient: temporalClient,
		workers:        make(map[string]worker.Worker),
		taskQueue:      "ono-recipes", // Base task queue
	}
}

// StartWorker starts a new worker for a recipe
func (m *WorkerManager) StartWorker(recipe *recipe.Recipe) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worker already exists
	if _, exists := m.workers[recipe.Name]; exists {
		return fmt.Errorf("worker already exists for recipe %q", recipe.Name)
	}

	// Create task queue name for this recipe
	taskQueue := fmt.Sprintf("%s-%s", m.taskQueue, recipe.Name)

	// Create worker options
	workerOptions := worker.Options{
		MaxConcurrentWorkflowTaskPollers: 2,
		MaxConcurrentActivityTaskPollers: 2,
	}

	// Create the worker
	w := worker.New(m.temporalClient, taskQueue, workerOptions)

	// Register the workflow if it exists
	if recipe.Workflow != nil {
		// TODO: Create dynamic workflow function
		// For now, register a placeholder workflow
		w.RegisterWorkflowWithOptions(
			func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
				return map[string]interface{}{
					"status": "completed",
					"recipe": recipe.Name,
				}, nil
			},
			workflow.RegisterOptions{
				Name: recipe.Workflow.Name,
			},
		)
	}

	// Register all activities
	for _, activityDef := range recipe.Activities {
		// Create a copy to avoid closure issues
		actDef := activityDef
		// TODO: Create dynamic activity function
		// For now, register a placeholder activity
		w.RegisterActivityWithOptions(
			func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
				return map[string]interface{}{
					"status": "completed",
					"activity": actDef.Name,
				}, nil
			},
			activity.RegisterOptions{
				Name: actDef.Name,
			},
		)
	}

	// Start the worker
	err := w.Start()
	if err != nil {
		return fmt.Errorf("failed to start worker for recipe %q: %w", recipe.Name, err)
	}

	m.workers[recipe.Name] = w
	m.logger.Info("Started worker for recipe", 
		zap.String("recipe", recipe.Name),
		zap.String("taskQueue", taskQueue))

	return nil
}

// StopWorker stops a worker for a recipe
func (m *WorkerManager) StopWorker(recipeName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.workers[recipeName]
	if !exists {
		return fmt.Errorf("worker not found for recipe %q", recipeName)
	}

	w.Stop()
	delete(m.workers, recipeName)

	m.logger.Info("Stopped worker for recipe", zap.String("recipe", recipeName))
	return nil
}

// RestartWorker restarts a worker for a recipe
func (m *WorkerManager) RestartWorker(recipeName string, recipe *recipe.Recipe) error {
	// Stop existing worker if it exists
	_ = m.StopWorker(recipeName)

	// Start new worker
	return m.StartWorker(recipe)
}

// StopAll stops all workers
func (m *WorkerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, w := range m.workers {
		w.Stop()
		m.logger.Info("Stopped worker", zap.String("recipe", name))
	}

	m.workers = make(map[string]worker.Worker)
}

// GetWorkerStatus returns the status of a worker
func (m *WorkerManager) GetWorkerStatus(recipeName string) recipe.WorkerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.workers[recipeName]
	if exists {
		// TODO: Check actual worker health
		return recipe.WorkerStatusRunning
	}

	return recipe.WorkerStatusStopped
}

// GetTaskQueueForRecipe returns the task queue name for a recipe
func (m *WorkerManager) GetTaskQueueForRecipe(recipeName string) string {
	return fmt.Sprintf("%s-%s", m.taskQueue, recipeName)
}