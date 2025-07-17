package recipecore

import (
	"go.uber.org/zap"
	"github.com/vibethis/server/recipe-core/pkg/recipe"
	"github.com/vibethis/server/recipe-core/pkg/yaml"
)

// Re-export all types from pkg/recipe at the root level
type Recipe = recipe.Recipe
type WorkerStatus = recipe.WorkerStatus
type Job = recipe.Job
type WorkflowExecutionInfo = recipe.WorkflowExecutionInfo
type JobStatus = recipe.JobStatus
type ActivityExecution = recipe.ActivityExecution
type ActivityStatus = recipe.ActivityStatus
type RecipeManifest = recipe.RecipeManifest
type RecipeFilter = recipe.RecipeFilter
type JobFilter = recipe.JobFilter
type HashComputer = recipe.HashComputer
type Parser = recipe.Parser

// Re-export constants
const (
	WorkerStatusRunning  = recipe.WorkerStatusRunning
	WorkerStatusStopped  = recipe.WorkerStatusStopped
	WorkerStatusFailed   = recipe.WorkerStatusFailed
	WorkerStatusStarting = recipe.WorkerStatusStarting
)

const (
	JobStatusUnknown    = recipe.JobStatusUnknown
	JobStatusRunning    = recipe.JobStatusRunning
	JobStatusCompleted  = recipe.JobStatusCompleted
	JobStatusFailed     = recipe.JobStatusFailed
	JobStatusCanceled   = recipe.JobStatusCanceled
	JobStatusTerminated = recipe.JobStatusTerminated
)

const (
	ActivityStatusPending   = recipe.ActivityStatusPending
	ActivityStatusRunning   = recipe.ActivityStatusRunning
	ActivityStatusCompleted = recipe.ActivityStatusCompleted
	ActivityStatusFailed    = recipe.ActivityStatusFailed
	ActivityStatusSkipped   = recipe.ActivityStatusSkipped
)

// Re-export constructor functions
func NewHashComputer() *HashComputer {
	return recipe.NewHashComputer()
}

func NewParser(logger *zap.Logger) *Parser {
	return recipe.NewParser(logger)
}

// Re-export yaml package types
type WorkflowDefinition = yaml.WorkflowDefinition
type InputDefinition = yaml.InputDefinition
type OutputDefinition = yaml.OutputDefinition
type WorkflowSpec = yaml.WorkflowSpec
type RetryPolicy = yaml.RetryPolicy
type Step = yaml.Step
type ActivityDefinition = yaml.ActivityDefinition
type ActivityRetryPolicy = yaml.ActivityRetryPolicy
type ActivityImplementation = yaml.ActivityImplementation
type ProjectManifest = yaml.ProjectManifest
type ActivitiesFile = yaml.ActivitiesFile
type AgentDefinition = yaml.AgentDefinition
type AgentsFile = yaml.AgentsFile
type Project = yaml.Project
type YamlParser = yaml.Parser

// Re-export yaml package constructor
func NewYamlParser() *YamlParser {
	return yaml.NewParser()
}