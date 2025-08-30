package worker

import recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"

// WorkerManagerInterface defines the interface for managing workers
type WorkerManagerInterface interface {
	StartWorker(recipe *recipe.RecipeFile) error
	StopWorker(recipeName string) error
	RestartWorker(recipeName string, recipe *recipe.RecipeFile) error
	StopAll()
	GetWorkerStatus(recipeName string) recipe.WorkerStatus
	GetTaskQueueForRecipe(recipeName string) string
}
