package recipe

import (
	"context"
	"encoding/json"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	ops2 "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

// RecipeInput defines the input for recipe activities
type SingleRecipe struct {
	Name      string                 `json:"name"`
	Cell      cell.ID                `json:"cell"`
	Inputs    map[string]interface{} `json:"inputs"`
	Artifacts []swf.ArtifactKey      `json:"artifacts"`
}

type SingleRecipeWithRef struct {
	SingleRecipe `json:",inline"`
	GitRef       string `json:"git_ref"`
}

type MultipleRecipes struct {
	GitRef  string         `json:"git_ref"`
	Recipes []SingleRecipe `json:"recipes"`
}

type StartedJob struct {
	JobId string `json:"job_id"`
}

type SingleRecipeOutput struct {
	Outputs   map[string]interface{}      `json:"outputs"`
	GitResult contextual.GitCommitContext `json:"git,omitempty"`
}

type MultipleRecipeOutput struct {
	Outputs []SingleRecipeOutput `json:"outputs"`
}

type StartedJobs struct {
	JobIDs []string `json:"job_ids"`
}

func GetOps() []ops.RegisterableOp {
	list := []ops.RegisterableOp{}

	list = appendE(list, ops.NewOp().
		WithType("recipe.wait_and_get_result").
		AddStep("recipe.wait_and_get_result", ops.NewStepWithDeps[StartedJob, SingleRecipeOutput](waitAndGetRecipeOutput)),
	)
	list = appendE(list, ops.NewOp().
		WithType("recipe.get_result").
		AddStep("recipe.get_result", ops.NewStepWithDeps[StartedJob, SingleRecipeOutput](getRecipeOutput)),
	)

	// single recipe sync
	list = appendE(list, ops.NewOp().
		WithType("recipe.run_and_wait.start").
		AddStep("recipe.run_and_wait.start", ops.NewStepWithDeps[SingleRecipeWithRef, StartedJob](startSingleJob)).
		AddStep("recipe.run_and_wait.finish", ops.NewStepWithDeps[StartedJob, SingleRecipeOutput](waitAndGetRecipeOutput)),
	)

	// mutliple recipes sync
	list = appendE(list, ops.NewOp().
		WithType("recipes.start").
		AddStep("recipes.start", ops.NewStepWithDeps[MultipleRecipes, StartedJobs](startMultipleJobs)),
	)

	return list
}

func appendE(list []ops.RegisterableOp, builder ops.OpBuilder) []ops.RegisterableOp {
	o, err := builder.Build()
	if err != nil {
		panic(err)
	}
	return append(list, o)
}

func waitAndGetRecipeOutput(deps ops.OpDependencies, ctx context.Context, input StartedJob) (SingleRecipeOutput, error) {
	err := deps.JobTool().AwaitJobs(input.JobId)
	if err != nil {
		return SingleRecipeOutput{}, err
	}
	return getRecipeOutput(deps, ctx, input)
}

func getRecipeOutput(deps ops.OpDependencies, ctx context.Context, input StartedJob) (SingleRecipeOutput, error) {
	var zero SingleRecipeOutput
	data, err := deps.WorkflowControl().JobResult(ctx, swf.JobKey{deps.JobTool().GetJobKey().TenantId, input.JobId})
	if err != nil {
		return zero, err
	}

	jd, err := data.GetData()
	if err != nil {
		return zero, err
	}

	out := ops2.ActivityInvocationOutput{}
	err = json.Unmarshal(jd, &out)
	if err != nil {
		return zero, err
	}

	artifacts, err := data.GetArtifacts()
	if err != nil {
		return zero, err
	}

	for _, a := range artifacts {
		err = deps.AddOutputArtifact(a)
		if err != nil {
			return zero, err
		}
	}

	return SingleRecipeOutput{
		Outputs: out.OpOutput,
	}, nil
}

func startSingleJob(deps ops.OpDependencies, ctx context.Context, input SingleRecipeWithRef) (StartedJob, error) {
	tId := deps.JobTool().GetJobKey().TenantId
	keys, err := startJobs(ctx, deps.Database(), tId, deps.WorkflowControl(), []SingleRecipe{input.SingleRecipe}, input.GitRef)
	if err != nil {
		return StartedJob{}, err
	}
	return StartedJob{JobId: keys[0].JobId}, nil
}

func startMultipleJobs(deps ops.OpDependencies, ctx context.Context, input MultipleRecipes) (StartedJobs, error) {
	tId := deps.JobTool().GetJobKey().TenantId
	keys, err := startJobs(ctx, deps.Database(), tId, deps.WorkflowControl(), input.Recipes, input.GitRef)
	if err != nil {
		return StartedJobs{}, err
	}

	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = k.JobId
	}
	return StartedJobs{JobIDs: ids}, nil
}
