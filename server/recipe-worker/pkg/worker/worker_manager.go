package worker

import (
	"fmt"
	"sync"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
	"github.com/vibethis/server/recipe-worker/pkg/compiler"
	recipeworkflows "github.com/vibethis/server/recipe-worker/pkg/workflows"
)

// WorkerManager manages the lifecycle of workers for recipes
type WorkerManager struct {
	logger         *zap.Logger
	temporalClient client.Client
	workers        map[string]worker.Worker // key is recipe name
	mu             sync.RWMutex
	taskQueue      string // Base task queue name
	compiler       *compiler.Compiler
	activityRegistry *compiler.ActivityRegistry
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(logger *zap.Logger, temporalClient client.Client) *WorkerManager {
	activityRegistry := compiler.NewActivityRegistry()
	return &WorkerManager{
		logger:           logger,
		temporalClient:   temporalClient,
		workers:          make(map[string]worker.Worker),
		taskQueue:        "ono-recipes", // Base task queue
		activityRegistry: activityRegistry,
		compiler:         compiler.NewCompiler(activityRegistry),
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

	// Register all activities first
	for _, activityDef := range recipe.Activities {
		// Register activity definition with the registry (pass by reference)
		actDef := activityDef // Create a copy to get a stable pointer
		m.activityRegistry.RegisterActivity(&actDef)
	}
	
	// Register the workflow if it exists
	if recipe.Workflow != nil {
		// Create dynamic workflow using the compiler as executor
		workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipe.Workflow, recipe.Project, m.compiler)
		
		w.RegisterWorkflowWithOptions(
			workflowFunc,
			workflow.RegisterOptions{
				Name: recipe.Workflow.Name,
			},
		)
	}

	// Register all activities with dynamic implementations
	for _, activityDef := range recipe.Activities {
		// Create a copy to avoid closure issues
		actDef := activityDef
		
		// Create dynamic activity function
		activityFunc := recipeworkflows.CreateDynamicActivity(&actDef)
		
		w.RegisterActivityWithOptions(
			activityFunc,
			activity.RegisterOptions{
				Name: activityDef.Name,
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