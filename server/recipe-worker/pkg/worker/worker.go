package worker

import (
	"fmt"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/ticket/pkg/database"
	ticketop "github.com/divisive-ai/vibethis/server/ticket/pkg/op"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

// Worker represents the recipe worker system that monitors directories
// and manages Temporal workers for recipes
type Worker struct {
	logger        *zap.Logger
	registry      *Registry
	workerManager *WorkerManager
	ticketCleanup func() error
}

// NewWorker creates a new recipe worker system
func NewWorker(logger *zap.Logger, recipesDir string, temporalClient client.Client, namespace string) (*Worker, error) {
	coreops.Register(ticketop.GetOp())

	ticketDB, closeTicketDB, err := database.Open(database.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open ticket database: %w", err)
	}

	// Create worker manager
	workerManager := NewWorkerManager(logger, temporalClient)
	workerManager.SetDependencies(newWorkerDependencies(temporalClient, namespace, ticketDB))

	// Create registry
	registry, err := NewRegistry(logger, recipesDir, workerManager)
	if err != nil {
		_ = closeTicketDB()
		return nil, err
	}

	return &Worker{
		logger:        logger,
		registry:      registry,
		workerManager: workerManager,
		ticketCleanup: closeTicketDB,
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

	if w.ticketCleanup != nil {
		if err := w.ticketCleanup(); err != nil {
			w.logger.Warn("Failed to close ticket database", zap.Error(err))
		}
	}

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
