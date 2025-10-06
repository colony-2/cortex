package ops

import (
	opsrecipe "github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
	"go.temporal.io/sdk/workflow"
)

func init() {
	opsrecipe.RegisterGitStateAdapter(recipeWorkerGitStateAdapter{})
}

// recipeWorkerGitStateAdapter bridges the ops recipe package to the recipe-worker git workspace helpers.
type recipeWorkerGitStateAdapter struct{}

func (recipeWorkerGitStateAdapter) WithInlineWorkspace(ctx workflow.Context, inv coreops.Invocation, inputs map[string]interface{}, opts opsrecipe.InlineWorkspaceOptions, fn opsrecipe.InlineWorkspaceFunc) (*opsrecipe.InlineWorkspaceResult, error) {
	rwOpts := gitstate.InlineWorkspaceOptions{
		ChildID:      opts.ChildID,
		SkipFinalize: opts.SkipFinalize,
	}
	result, err := gitstate.WithInlineWorkspace(ctx, inv, inputs, rwOpts, func(inner workflow.Context, payload map[string]interface{}) (map[string]interface{}, error) {
		return fn(inner, payload)
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	gitCtx := result.GitContext
	return &opsrecipe.InlineWorkspaceResult{
		Result:     result.Result,
		ContextMap: result.ContextMap,
		GitContext: &gitCtx,
	}, nil
}

func (recipeWorkerGitStateAdapter) PlanDetachedWorkspace(inv coreops.Invocation, inputs map[string]interface{}, opts opsrecipe.DetachedWorkspaceOptions) (opsrecipe.GitStateContext, map[string]interface{}, error) {
	childCtx, childInputs, err := gitstate.PlanDetachedWorkspace(inv, inputs, gitstate.DetachedWorkspaceOptions{ChildID: opts.ChildID})
	if err != nil {
		return nil, nil, err
	}
	ctxCopy := childCtx
	return &ctxCopy, childInputs, nil
}
