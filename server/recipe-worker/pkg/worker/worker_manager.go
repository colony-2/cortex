package worker

import (
	"fmt"
	"sync"

	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

// WorkerManager manages the lifecycle of workers for recipes
type WorkerManager struct {
	logger               *zap.Logger
	temporalClient       client.Client
	workers              map[string]worker.Worker // key is recipe name
	mu                   sync.RWMutex
	taskQueue            string // Base task queue name
	activityRegistry     *ops.ActivityRegistry
	activityTypeRegistry *recipe.ActivityTypeRegistry
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(logger *zap.Logger, temporalClient client.Client) *WorkerManager {
	activityRegistry := ops.NewActivityRegistry()
	activityTypeRegistry := recipe.NewActivityTypeRegistry()
	return &WorkerManager{
		logger:               logger,
		temporalClient:       temporalClient,
		workers:              make(map[string]worker.Worker),
		taskQueue:            "ono-recipes", // Base task queue
		activityRegistry:     activityRegistry,
		activityTypeRegistry: activityTypeRegistry,
	}
}

// StartWorker starts a new worker for a recipe
func (m *WorkerManager) StartWorker(recipe *recipe.Recipe) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worker already exists
	if _, exists := m.workers[recipe.ID]; exists {
		return fmt.Errorf("worker already exists for recipe %q", recipe.ID)
	}

	// Create task queue name for this recipe
	taskQueue := fmt.Sprintf("%s-%s", m.taskQueue, recipe.ID)

	// Create worker options
	workerOptions := worker.Options{
		MaxConcurrentWorkflowTaskPollers: 2,
		MaxConcurrentActivityTaskPollers: 2,
	}

	// Create the worker
	if recipe.Recipe == nil {
		return fmt.Errorf("recipe is nil")
	}
	w := worker.New(m.temporalClient, taskQueue, workerOptions)
	m.activityRegistry.EnableActivitiesInWorker(w)
	fn := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteNode(ctx, m.activityRegistry, &recipe.Recipe.Node, inputs)
	}

	w.RegisterWorkflowWithOptions(
		fn,
		workflow.RegisterOptions{
			Name: recipe.ID + "-workflow",
		},
	)

	// Start the worker
	err := w.Start()
	if err != nil {
		return fmt.Errorf("failed to start worker for recipe %q: %w", recipe.ID, err)
	}

	m.workers[recipe.ID] = w
	m.logger.Info("Started worker for recipe",
		zap.String("recipe", recipe.ID),
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

// GetActivityTypeRegistry returns the activity type registry for testing
func (m *WorkerManager) GetActivityTypeRegistry() *recipe.ActivityTypeRegistry {
	return m.activityTypeRegistry
}
