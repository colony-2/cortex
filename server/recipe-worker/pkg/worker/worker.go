package worker

import (
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

// Worker represents the recipe worker system that monitors directories
// and manages Temporal workers for recipes
type Worker struct {
	logger        *zap.Logger
	registry      *Registry
	workerManager *WorkerManager
}

// NewWorker creates a new recipe worker system
func NewWorker(logger *zap.Logger, recipesDir string, temporalClient client.Client, namespace string) (*Worker, error) {
	// Create worker manager
	workerManager := NewWorkerManager(logger, temporalClient)
	workerManager.SetDependencies(newWorkerDependencies(temporalClient, namespace))

	// Create registry
	registry, err := NewRegistry(logger, recipesDir, workerManager)
	if err != nil {
		return nil, err
	}

	return &Worker{
		logger:        logger,
		registry:      registry,
		workerManager: workerManager,
	}, nil
}

// Start starts the recipe worker system
func (w *Worker) Start() error {
	return w.registry.Start()
}

// Stop stops the recipe worker system
func (w *Worker) Stop() error {
	// Stop registry first
	if err := w.registry.Stop(); err != nil {
		w.logger.Error("Failed to stop registry", zap.Error(err))
	}

	// Stop all workers
	w.workerManager.StopAll()

	return nil
}

// GetRegistry returns the recipe registry
func (w *Worker) GetRegistry() *Registry {
	return w.registry
}

// GetWorkerManager returns the worker manager
func (w *Worker) GetWorkerManager() *WorkerManager {
	return w.workerManager
}
