package recipeworker

import (
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"github.com/vibethis/server/recipe-worker/pkg/worker"
)

// Re-export key types
type Registry = worker.Registry
type WorkerManager = worker.WorkerManager
type Worker = worker.Worker

// Re-export constructor functions
func NewRegistry(logger *zap.Logger, recipesDir string, workerManager *WorkerManager) (*Registry, error) {
	return worker.NewRegistry(logger, recipesDir, workerManager)
}

func NewWorkerManager(temporalClient client.Client, logger *zap.Logger) *WorkerManager {
	return worker.NewWorkerManager(temporalClient, logger)
}