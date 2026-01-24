package recipe

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

// RecipeInput defines the input for recipe activities
type SingleRecipe struct {
	Name             string                 `json:"name" validate:"required"`
	Cell             cell.ID                `json:"cell"`
	CellName         string                 `json:"cell_name,omitempty"`
	CellPath         string                 `json:"cell_path,omitempty"`
	Inputs           map[string]interface{} `json:"inputs"`
	Artifacts        []swf.ArtifactKey      `json:"artifacts"`
	Git              SingleRecipeGit        `json:"git"`
}

type SingleRecipeGit struct {
	BaseRepo string `json:"base_repo,omitempty"`
	BaseRef  string `json:"base_ref,omitempty"`
	BaseHash string `json:"base_hash,omitempty"`
	Author   string `json:"author,omitempty"`
}

type RecipeDefaults struct {
	CellName string          `json:"cell_name" default:"{{ context.workflow.cell }}" validate:"required"`
	CellPath string          `json:"cell_path" default:"{{ context.workflow.cell_path }}" validate:"required"`
	Git      RecipeDefaultsGit `json:"git"`
}

type RecipeDefaultsGit struct {
	BaseRepo string `json:"base_repo" default:"{{ context.git.repo }}" validate:"required"`
	BaseRef  string `json:"base_ref" default:"{{ context.git.ref }}" validate:"required"`
	BaseHash string `json:"base_hash" default:"{{ context.git.resolved_hash }}" validate:"required"`
	Author   string `json:"author" default:"{{ context.workflow.job_id }}@{{ context.workflow.cell }}"`
}

type SingleRecipeWithRef struct {
	SingleRecipe `json:",squash"`
	Defaults     RecipeDefaults `json:"defaults"`
	GitRef       string         `json:"git_ref"`
}

type MultipleRecipes struct {
	GitRef   string         `json:"git_ref"`
	Defaults RecipeDefaults `json:"defaults"`
	Recipes  []SingleRecipe `json:"recipes"`
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

	list = append(list, ops.NewOp().
		WithType("recipe.await_result").
		AddStep("wait_and_get", ops.NewStepWithDeps[StartedJob, SingleRecipeOutput](waitAndGetRecipeOutput)).
		BuildOrPanic(),
	)
	list = append(list, ops.NewOp().
		WithType("recipe.get_result").
		AddStep("get", ops.NewStepWithDeps[StartedJob, SingleRecipeOutput](getRecipeOutput)).
		BuildOrPanic(),
	)

	// single recipe sync
	list = append(list, ops.NewOp().
		WithType("recipe.run_and_get_result").
		AddStep("start", ops.NewStepWithDeps[SingleRecipeWithRef, StartedJob](startSingleJob)).
		AddStep("finish", ops.NewStepWithDeps[StartedJob, SingleRecipeOutput](waitAndGetRecipeOutput)).
		BuildOrPanic(),
	)

	list = append(list, ops.NewOp().
		WithType("recipes.run_and_wait").
		AddStep("start", ops.NewStepWithDeps[MultipleRecipes, StartedJobs](startMultipleJobs)).
		AddStep("await", ops.NewStepWithDeps[StartedJobs, StartedJobs](waitAndNoResult)).
		BuildOrPanic(),
	)

	// mutliple recipes sync
	list = append(list, ops.NewOp().
		WithType("recipes.run").
		AddStep("start", ops.NewStepWithDeps[MultipleRecipes, StartedJobs](startMultipleJobs)).
		BuildOrPanic(),
	)

	return list
}

func waitAndNoResult(deps ops.OpDependencies, ctx context.Context, input StartedJobs) (StartedJobs, error) {
	err := deps.JobTool().AwaitJobs(input.JobIDs...)
	return input, err
}

func waitAndGetRecipeOutput(deps ops.OpDependencies, ctx context.Context, input StartedJob) (SingleRecipeOutput, error) {
	err := deps.JobTool().AwaitJobs(input.JobId)
	if err != nil {
		return SingleRecipeOutput{}, err
	}
	return getRecipeOutput(deps, ctx, input)
}

func applyDefaults(recipe SingleRecipe, defaults RecipeDefaults) (SingleRecipe, error) {
	if recipe.CellName == "" {
		recipe.CellName = defaults.CellName
	}
	if recipe.CellPath == "" {
		recipe.CellPath = defaults.CellPath
	}
	if recipe.Git.BaseRepo == "" {
		recipe.Git.BaseRepo = defaults.Git.BaseRepo
	}
	if recipe.Git.BaseRef == "" {
		recipe.Git.BaseRef = defaults.Git.BaseRef
	}
	if recipe.Git.BaseHash == "" {
		recipe.Git.BaseHash = defaults.Git.BaseHash
	}
	if recipe.Git.Author == "" {
		recipe.Git.Author = defaults.Git.Author
	}

	if recipe.CellName == "" || recipe.CellPath == "" {
		return recipe, fmt.Errorf("missing cell metadata for child recipe")
	}
	if recipe.Git.BaseRepo == "" || recipe.Git.BaseRef == "" || recipe.Git.BaseHash == "" {
		return recipe, fmt.Errorf("missing git base metadata for child recipe")
	}
	return recipe, nil
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

	var raw map[string]interface{}
	if err := json.Unmarshal(jd, &raw); err != nil {
		return zero, err
	}

	outputs := map[string]interface{}{}
	if wrapped, ok := raw["output"]; ok {
		if cast, ok := wrapped.(map[string]interface{}); ok {
			outputs = cast
		}
	} else {
		outputs = raw
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

	return SingleRecipeOutput{Outputs: outputs}, nil
}

func startSingleJob(deps ops.OpDependencies, ctx context.Context, input SingleRecipeWithRef) (StartedJob, error) {
	tId := deps.JobTool().GetJobKey().TenantId
	recipe, err := applyDefaults(input.SingleRecipe, input.Defaults)
	if err != nil {
		return StartedJob{}, err
	}
	keys, err := startJobs(ctx, deps.Database(), tId, deps.WorkflowControl(), []SingleRecipe{recipe}, input.GitRef)
	if err != nil {
		return StartedJob{}, err
	}
	return StartedJob{JobId: keys[0].JobId}, nil
}

func startMultipleJobs(deps ops.OpDependencies, ctx context.Context, input MultipleRecipes) (StartedJobs, error) {
	tId := deps.JobTool().GetJobKey().TenantId
	resolved := make([]SingleRecipe, len(input.Recipes))
	for i, recipe := range input.Recipes {
		resolvedRecipe, err := applyDefaults(recipe, input.Defaults)
		if err != nil {
			return StartedJobs{}, err
		}
		resolved[i] = resolvedRecipe
	}
	keys, err := startJobs(ctx, deps.Database(), tId, deps.WorkflowControl(), resolved, input.GitRef)
	if err != nil {
		return StartedJobs{}, err
	}

	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = k.JobId
	}
	return StartedJobs{JobIDs: ids}, nil
}
