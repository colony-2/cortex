package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
)

// CELValidator validates CEL expressions inside a recipe.
type CELValidator interface {
	ValidateCEL(ctx context.Context, projectID project.ID, rec recipe.Recipe) ([]model.ValidationError, error)
}

// CELOptionsProvider mirrors template.CELOptionsProvider for external callers.
type CELOptionsProvider = template.CELOptionsProvider

// RecipeWorkerCELValidator validates CEL expressions using recipe-worker validation mode.
type RecipeWorkerCELValidator struct {
	deps     coreops.ServiceDependencies2
	jobCtx   swf.JobContext
	provider template.CELOptionsProvider
}

// NewRecipeWorkerCELValidator creates a new validator backed by recipe-worker.
func NewRecipeWorkerCELValidator(deps coreops.ServiceDependencies2) *RecipeWorkerCELValidator {
	return NewRecipeWorkerCELValidatorWithProvider(deps, nil)
}

// NewRecipeWorkerCELValidatorWithProvider allows injecting custom CEL functions/types.
func NewRecipeWorkerCELValidatorWithProvider(deps coreops.ServiceDependencies2, provider template.CELOptionsProvider) *RecipeWorkerCELValidator {
	if deps == nil {
		deps = coreops.NewServiceDepsBuilder().Build()
	}
	return &RecipeWorkerCELValidator{
		deps:     deps,
		jobCtx:   &noopJobContext{},
		provider: provider,
	}
}

// ValidateCEL checks CEL expressions by running recipe-worker in validation mode.
func (v *RecipeWorkerCELValidator) ValidateCEL(ctx context.Context, projectID project.ID, rec recipe.Recipe) ([]model.ValidationError, error) {
	_ = ctx
	_ = projectID

	execCtx := workflow.Context{
		JobContext:           v.jobCtx,
		ServiceDependencies2: v.deps,
	}

	jobCtx := contextual.JobContext{}
	gitCtx := contextual.GitCommitContext{}

	_, _, err := compiler.ExecuteRecipe(execCtx, rec, nil, jobCtx, gitCtx, compiler.ExecutionOptions{
		Mode: compiler.ExecutionModeValidate,
		Validation: compiler.ValidationOptions{
			Mode:       compiler.ValidateAll,
			CollectAll: true,
		},
		CELOptionsProvider: v.provider,
	})
	if err == nil {
		return nil, nil
	}

	return []model.ValidationError{{
		Code:    "cel_invalid",
		Message: err.Error(),
	}}, nil
}

type noopJobContext struct{}

func (n *noopJobContext) GetJobKey() swf.JobKey            { return swf.JobKey{} }
func (n *noopJobContext) Logger() *slog.Logger             { return slog.Default() }
func (n *noopJobContext) AwaitDuration(swf.Duration) error { return nil }
func (n *noopJobContext) DoTask(swf.RunPolicy, string, swf.TaskData) (swf.TaskData, error) {
	return nil, fmt.Errorf("unexpected task invocation")
}

func (n *noopJobContext) AwaitJobs(jobIds ...string) error { return nil }

var _ swf.JobContext = &noopJobContext{}
