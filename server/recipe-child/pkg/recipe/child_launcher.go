package recipe

import (
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ChildLaunchResult captures the scheduled child workflow future and resolved inputs.
type ChildLaunchResult struct {
	Future   workflow.ChildWorkflowFuture
	Ctx      workflow.Context
	Inputs   map[string]interface{}
	GitState GitStateContext
}

// LaunchDiscreteAsyncChild schedules a child recipe using a detached workspace and async run mode.
// The caller is responsible for awaiting the returned future.
func LaunchDiscreteAsyncChild(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, baseInputs map[string]interface{}, recipeName string, childInputs map[string]interface{}) (*ChildLaunchResult, error) {
	exec := childExecutor{
		invocation: inv,
		ctx:        ctx,
		timeout:    timeout,
		retry:      retry,
		input: RecipeInput{
			Name:   recipeName,
			Inputs: map[string]interface{}{},
		},
		baseInputs: copyShallow(baseInputs),
	}

	gitCtx, detachedInputs, err := planDetachedWorkspace(inv, exec.baseInputs, DetachedWorkspaceOptions{})
	if err != nil {
		return nil, err
	}

	mergeChildInputs(detachedInputs, childInputs)
	ensureDetachedContext(detachedInputs, gitCtx)
	ensureDetachedGitPersist(detachedInputs, gitCtx)
	if _, ok := detachedInputs["context"]; !ok {
		return nil, temporal.NewNonRetryableApplicationError("detached workspace missing context", "MISSING_CONTEXT", nil)
	}

	future, childCtx := exec.scheduleChildWorkflow(ctx, runModeAsync, detachedInputs)
	return &ChildLaunchResult{
		Future:   future,
		Ctx:      childCtx,
		Inputs:   detachedInputs,
		GitState: gitCtx,
	}, nil
}
