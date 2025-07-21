package worker

import recipe "github.com/vibethis/server/recipe-core/pkg/recipe"

// WorkerManagerInterface defines the interface for managing workers
type WorkerManagerInterface interface {
	StartWorker(recipe *recipe.Recipe) error
	StopWorker(recipeName string) error
	RestartWorker(recipeName string, recipe *recipe.Recipe) error
	StopAll()
	GetWorkerStatus(recipeName string) recipe.WorkerStatus
	GetTaskQueueForRecipe(recipeName string) string
}