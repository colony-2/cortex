package recipe

import (
	"context"
	"fmt"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	"gorm.io/gorm"
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
		JobContext: contextual.JobContext{},
		GitRef:     gitRef,
	}
}

// func(deps OpDependencies, ctx context.Context, in In)
// Execute runs the activity with provided configuration and inputs
func startJobs(ctx context.Context, db *gorm.DB, tenantId string, ctl workflowctl.WorkflowControl, recipes []SingleRecipe, gitRef string) ([]swf.JobKey, error) {
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

	err := db.Transaction(func(tx *gorm.DB) error {
		txctx := swf.WithTx(ctx, tx)

		for i, job := range jobs {
			key, err := ctl.StartJob(txctx, job)
			if err != nil {
				tx.Rollback()
				return err
			}
			keys[i] = key
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}
