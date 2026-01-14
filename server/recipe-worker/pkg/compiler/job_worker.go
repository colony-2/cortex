package compiler

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

type recipeWorkerImpl struct {
	activityRegistry *workerops.ActivityRegistry
	recipes          *recipeRetriever
}

func NewRecipeWorker(dependencies ops.ServiceDependencies2, activityRegistry *workerops.ActivityRegistry) (*swf.WorkSet, error) {
	job := &recipeWorkerImpl{
		activityRegistry: activityRegistry,
	}
	return swf.AsWorkSet(job, activityRegistry.GetTaskWorkers(dependencies)...)
}

func (j recipeWorkerImpl) Name() string {
	return starter.RecipeJobType
}

func (j recipeWorkerImpl) Run(ctx swf.JobContext, jobData swf.JobData) (swf.JobData, error) {
	artifacts, err := jobData.GetArtifacts()
	if err != nil {
		return nil, err
	}
	j.recipes = newRetriever(artifacts)

	data, err := jobData.GetData()
	if err != nil {
		return nil, err
	}
	input := workflowctl.StartJob{}
	err = json.Unmarshal(data, &input)
	if err != nil {
		return nil, err
	}

	if input.RecipeName == "" {
		return nil, fmt.Errorf("missing recipe name")
	}

	r, err := j.recipes.GetRecipe(input.RecipeName)
	if err != nil {
		return nil, err
	}

	runContext := input.JobContext
	var cleanupWorktree func()
	if runContext.Environment.WorktreePath == "" {
		working, err := os.MkdirTemp("", "recipe-git-artifacts")
		if err != nil {
			return nil, err
		}
		runContext.Environment.WorktreePath = working
		cleanupWorktree = func() { os.RemoveAll(working) }
	}
	// Cleanup worktree on job completion
	if cleanupWorktree != nil {
		defer cleanupWorktree()
	}

	wCtx := workflow.Context{JobContext: ctx}
	out, err := ExecuteRecipe(wCtx, r, input.Inputs, runContext, contextual.GitCommitContext{ParentRef: input.GitRef})
	if err != nil {
		return nil, err
	}
	taskData, err := swf.NewTaskData(out)
	if err != nil {
		return nil, err
	}
	return swf.JobData(taskData), nil

}

var _ swf.JobWorker = &recipeWorkerImpl{}
