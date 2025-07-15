package recipe

import (
	"go.uber.org/zap"
	
	recipecore "github.com/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
)

// Type aliases for recipe-core types so existing ono code continues to work
type Recipe = recipecore.Recipe
type WorkerStatus = recipecore.WorkerStatus
type Job = recipecore.Job
type WorkflowExecutionInfo = recipecore.WorkflowExecutionInfo
type JobStatus = recipecore.JobStatus
type ActivityExecution = recipecore.ActivityExecution
type ActivityStatus = recipecore.ActivityStatus
type RecipeManifest = recipecore.RecipeManifest
type RecipeFilter = recipecore.RecipeFilter
type JobFilter = recipecore.JobFilter
type HashComputer = recipecore.HashComputer
type Parser = recipecore.Parser

// Re-export constants
const (
	WorkerStatusRunning  = recipecore.WorkerStatusRunning
	WorkerStatusStopped  = recipecore.WorkerStatusStopped
	WorkerStatusFailed   = recipecore.WorkerStatusFailed
	WorkerStatusStarting = recipecore.WorkerStatusStarting
)

const (
	JobStatusUnknown    = recipecore.JobStatusUnknown
	JobStatusRunning    = recipecore.JobStatusRunning
	JobStatusCompleted  = recipecore.JobStatusCompleted
	JobStatusFailed     = recipecore.JobStatusFailed
	JobStatusCanceled   = recipecore.JobStatusCanceled
	JobStatusTerminated = recipecore.JobStatusTerminated
)

const (
	ActivityStatusPending   = recipecore.ActivityStatusPending
	ActivityStatusRunning   = recipecore.ActivityStatusRunning
	ActivityStatusCompleted = recipecore.ActivityStatusCompleted
	ActivityStatusFailed    = recipecore.ActivityStatusFailed
	ActivityStatusSkipped   = recipecore.ActivityStatusSkipped
)

// Constructor functions
func NewHashComputer() *HashComputer {
	return recipecore.NewHashComputer()
}

func NewParser(logger *zap.Logger) *Parser {
	return recipecore.NewParser(logger)
}

// Additional type aliases for backwards compatibility
type ActivityDefinition = yamlpkg.ActivityDefinition