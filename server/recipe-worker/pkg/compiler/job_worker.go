package compiler

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/colony-2/colony2/server/core/pkg/logutil"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

type recipeWorkerImpl struct {
	activityRegistry *workerops.ActivityRegistry
	recipes          *recipeRetriever
	celProvider      template.CELOptionsProvider
}

func NewRecipeWorker(dependencies ops.ServiceDependencies2, activityRegistry *workerops.ActivityRegistry, provider ...template.CELOptionsProvider) (*swf.WorkSet, error) {
	job := &recipeWorkerImpl{
		activityRegistry: activityRegistry,
	}
	if len(provider) > 0 {
		job.celProvider = provider[0]
	}
	return swf.AsWorkSet(job, activityRegistry.GetTaskWorkers(dependencies)...)
}

func (j recipeWorkerImpl) Name() string {
	return starter.RecipeJobType
}

func (j recipeWorkerImpl) Run(ctx swf.JobContext, jobData swf.JobData) (swf.JobData, error) {
	jobKey := ctx.GetJobKey()
	logger := ctx.Logger()
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(
		"tenant_id", jobKey.TenantId,
		"job_id", jobKey.JobId,
		"job_type", starter.RecipeJobType,
	)
	artifacts, err := jobData.GetArtifacts()
	if err != nil {
		logger.Error("recipe job: failed to load artifacts",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	j.recipes = newRetriever(artifacts)

	data, err := jobData.GetData()
	if err != nil {
		logger.Error("recipe job: failed to read job data",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	input := workflowctl.StartJob{}
	err = json.Unmarshal(data, &input)
	if err != nil {
		logger.Error("recipe job: failed to unmarshal start job payload",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}

	if input.RecipeName == "" {
		err := fmt.Errorf("missing recipe name")
		logger.Warn("recipe job: invalid start payload", "error", err)
		return nil, err
	}
	logger = logger.With("recipe_name", input.RecipeName)

	r, err := j.recipes.GetRecipe(input.RecipeName)
	if err != nil {
		logger.Error("recipe job: failed to load recipe",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}

	runContext := input.JobContext
	err = ensureSentinel(&runContext.Environment.WorktreePath, contextual.WorktreePathSentinel, "worktree path")
	if err != nil {
		logger.Error("recipe job: invalid worktree sentinel",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	err = ensureSentinel(&runContext.Environment.WorkdirPath, contextual.WorkdirPathSentinel, "workdir path")
	if err != nil {
		logger.Error("recipe job: invalid workdir sentinel",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	err = ensureSentinel(&runContext.Environment.ArtifactInbox, contextual.ArtifactInboxSentinel, "artifact inbox")
	if err != nil {
		logger.Error("recipe job: invalid artifact inbox sentinel",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	err = ensureSentinel(&runContext.Environment.ArtifactOutbox, contextual.ArtifactOutboxSentinel, "artifact outbox")
	if err != nil {
		logger.Error("recipe job: invalid artifact outbox sentinel",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	err = ensureSentinel(&runContext.Workflow.JobID, contextual.JobIdSentinel, "job id")
	if err != nil {
		logger.Error("recipe job: invalid job id sentinel",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}

	wCtx := workflow.Context{JobContext: ctx}
	opts := ExecutionOptions{}
	if j.celProvider != nil {
		opts.CELOptionsProvider = j.celProvider
	}

	out, artifacts, err := ExecuteRecipe(wCtx, r, input.Inputs, runContext, contextual.GitCommitContext{ParentRef: input.GitRef}, opts)

	if err != nil {
		logger.Error("recipe execution failed",
			"git_ref", input.GitRef,
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	taskData, err := swf.NewTaskData(out, artifacts...)
	if err != nil {
		logger.Error("recipe job: failed to create task data",
			"error", err,
			"error_chain", logutil.ErrorChain(err),
			"stacktrace", logutil.Stacktrace(5),
		)
		return nil, err
	}
	logger.Info("recipe execution completed successfully")
	return swf.JobData(taskData), nil

}

func ensureSentinel(field *string, sentinel string, name string) error {
	if *field == sentinel {
		return nil
	}

	if *field != "" {
		return fmt.Errorf("unexpected %s: %s", name, *field)
	}

	*field = sentinel
	return nil
}

var _ swf.JobWorker = &recipeWorkerImpl{}
