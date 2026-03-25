package recipe

import (
	"context"
	"fmt"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
)

func recipeToStart(ctx context.Context, tenantId string, ctl workflowctl.WorkflowControl, recipe SingleRecipe, gitRef string) workflowctl.StartJob {
	artifacts := make([]swf.Artifact, len(recipe.Artifacts))
	for i, artifact := range recipe.Artifacts {
		artifacts[i] = ctl.GetArtifactLazy(ctx, tenantId, artifact)
	}

	return workflowctl.StartJob{
		TenantId:   tenantId,
		RecipeName: recipe.Name,
		Inputs:     recipe.Inputs,
		Artifacts:  artifacts,
		JobContext: contextual.JobContext{
			Workflow: contextual.WorkflowContext{
				CellName: recipe.CellName,
				CellPath: recipe.CellPath,
			},
			GitBase: contextual.GitBaseContext{
				BaseRepo:         recipe.Git.BaseRepo,
				BaseRef:          recipe.Git.BaseRef,
				ResolvedBaseHash: recipe.Git.BaseHash,
				GitAuthor:        recipe.Git.Author,
			},
		},
		GitRef:     gitRef,
	}
}

// func(deps OpDependencies, ctx context.Context, in In)
// Execute runs the activity with provided configuration and inputs
func startJobs(ctx context.Context, tenantId string, ctl workflowctl.WorkflowControl, recipes []SingleRecipe, gitRef string) ([]swf.JobKey, error) {
	if len(recipes) == 0 {
		return nil, fmt.Errorf("no jobs to start")
	}

	jobs := make([]workflowctl.StartJob, len(recipes))
	for i, recipe := range recipes {
		jobs[i] = recipeToStart(ctx, tenantId, ctl, recipe, gitRef)
	}

	if len(jobs) == 1 {
		key, err := ctl.StartJob(ctx, jobs[0])
		if err != nil {
			return nil, err
		}
		return []swf.JobKey{key}, nil
	}

	keys := make([]swf.JobKey, len(jobs))
	for i, job := range jobs {
		key, err := ctl.StartJob(ctx, job)
		if err != nil {
			return nil, err
		}
		keys[i] = key
	}
	return keys, nil
}
