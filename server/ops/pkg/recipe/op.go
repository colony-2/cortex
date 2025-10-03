package recipe

import (
	"fmt"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RecipeInput defines the input for recipe activities
type RecipeInput struct {
	Name     string                 `json:"name"`
	Inputs   map[string]interface{} `json:"inputs"`
	GitState string                 `json:"git_state,omitempty"`
	RunMode  string                 `json:"run_mode,omitempty"`
	Raw      map[string]interface{} `json:"-" mapstructure:",remain"`
}

// RecipeOutput defines the output from recipe activities
type RecipeOutput struct {
	Outputs        map[string]interface{} `json:"result"`            // Outputs from the executed recipe
	Context        map[string]interface{} `json:"context,omitempty"` // Propagated git context
	GitPersistHash string                 `json:"git_persist_hash,omitempty"`
}

// RecipeMetadata contains execution metadata
type RecipeMetadata struct {
	StartTime    string `json:"start_time"`    // RFC3339 formatted time
	EndTime      string `json:"end_time"`      // RFC3339 formatted time
	DurationMs   int64  `json:"duration_ms"`   // Duration in milliseconds
	AttemptCount int    `json:"attempt_count"` // Number of attempts
}

// RecipeActivityWrapper implements the RegisterableOp interface
type recipeInlineAdapter struct {
	isWorkflowContext bool // Indicates if running in workflow context
}

const (
	gitStateShared   = "shared"
	gitStateDiscrete = "discrete"
	runModeSync      = "sync"
	runModeAsync     = "async"
)

// NewRecipeActivity creates a new recipe activity that implements RegisterableOp
func GetOp() ops.RegisterableOp {
	return ops.NewInlineOpV2[RecipeInput, RecipeOutput](ops.OpMetadata{
		Type:           "recipe",
		Description:    "Invokes another recipe as a child workflow with automatic context propagation",
		Version:        "1.0.0",
		DefaultTimeout: 35 * time.Minute, // Default timeout, can be overridden
	}, execute)
}

// Execute runs the activity with provided configuration and inputs
func execute(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input RecipeInput) (RecipeOutput, error) {
	baseInputs := make(map[string]interface{})
	for k, v := range input.Raw {
		baseInputs[k] = v
	}

	gitState := strings.ToLower(strings.TrimSpace(input.GitState))
	if gitState == "" {
		gitState = gitStateShared
	}
	if gitState != gitStateShared && gitState != gitStateDiscrete {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid git_state: %s", input.GitState), "INVALID_GIT_STATE", nil)
	}

	runMode := strings.ToLower(strings.TrimSpace(input.RunMode))
	if runMode == "" {
		runMode = runModeSync
	}
	if runMode != runModeSync && runMode != runModeAsync {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid run_mode: %s", input.RunMode), "INVALID_RUN_MODE", nil)
	}
	if gitState == gitStateShared && runMode == runModeAsync {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError("run_mode 'async' requires git_state 'discrete'", "INVALID_COMBINATION", nil)
	}

	switch {
	case gitState == gitStateShared && runMode == runModeSync:
		return executeSharedSync(inv, ctx, timeout, retry, input, baseInputs)
	case gitState == gitStateDiscrete && runMode == runModeSync:
		return executeDiscreteSync(inv, ctx, timeout, retry, input, baseInputs)
	case gitState == gitStateDiscrete && runMode == runModeAsync:
		return executeDiscreteAsync(inv, ctx, timeout, retry, input, baseInputs)
	default:
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError("unsupported git_state/run_mode combination", "INVALID_COMBINATION", nil)
	}
}

func executeSharedSync(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input RecipeInput, baseInputs map[string]interface{}) (RecipeOutput, error) {
	workspaceResult, err := gitstate.WithInlineWorkspace(ctx, inv, baseInputs, gitstate.InlineWorkspaceOptions{}, func(inner workflow.Context, childInputs map[string]interface{}) (map[string]interface{}, error) {
		for k, v := range input.Inputs {
			childInputs[k] = v
		}
		workflow.GetLogger(inner).Info("invoking child recipe", "inputs_keys", mapKeys(childInputs))
		cwo := workflow.ChildWorkflowOptions{
			RetryPolicy: retry,
		}
		if timeout > 0 {
			cwo.WorkflowRunTimeout = timeout
			cwo.WorkflowExecutionTimeout = timeout
		}
		inner = workflow.WithChildOptions(inner, cwo)
		workflowRun := workflow.ExecuteChildWorkflow(inner, input.Name, childInputs)
		var result map[string]interface{}
		if err := workflowRun.Get(inner, &result); err != nil {
			return nil, err
		}
		if result == nil {
			result = make(map[string]interface{})
		}
		workflow.GetLogger(inner).Info("child recipe completed", "keys", mapKeys(result), "result", result)
		return result, nil
	})
	if err != nil {
		return RecipeOutput{}, err
	}

	return RecipeOutput{
		Outputs:        workspaceResult.Result,
		Context:        workspaceResult.ContextMap,
		GitPersistHash: workspaceResult.GitContext.PersistHash,
	}, nil
}

func executeDiscreteSync(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input RecipeInput, baseInputs map[string]interface{}) (RecipeOutput, error) {
	childCtx, childInputs, err := prepareDetachedChild(inv, baseInputs)
	if err != nil {
		return RecipeOutput{}, err
	}
	mergeChildInputs(childInputs, input.Inputs)
	ensureDetachedContext(childInputs, childCtx)
	ensureDetachedGitPersist(childInputs, childCtx)
	if _, ok := childInputs["context"]; !ok {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError("detached workspace missing context", "MISSING_CONTEXT", nil)
	}
	workflow.GetLogger(ctx).Info("invoking discrete child recipe", "inputs_keys", mapKeys(childInputs))
	childOptions := workflow.ChildWorkflowOptions{RetryPolicy: retry}
	if timeout > 0 {
		childOptions.WorkflowRunTimeout = timeout
		childOptions.WorkflowExecutionTimeout = timeout
	}
	childCtxWF := workflow.WithChildOptions(ctx, childOptions)
	workflowRun := workflow.ExecuteChildWorkflow(childCtxWF, input.Name, childInputs)
	var childResult map[string]interface{}
	if err := workflowRun.Get(childCtxWF, &childResult); err != nil {
		return RecipeOutput{}, err
	}
	if childResult == nil {
		childResult = make(map[string]interface{})
	}
	workflow.GetLogger(ctx).Info("discrete child recipe completed", "keys", mapKeys(childResult))
	return RecipeOutput{Outputs: childResult}, nil
}

func executeDiscreteAsync(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input RecipeInput, baseInputs map[string]interface{}) (RecipeOutput, error) {
	childCtx, childInputs, err := prepareDetachedChild(inv, baseInputs)
	if err != nil {
		return RecipeOutput{}, err
	}
	mergeChildInputs(childInputs, input.Inputs)
	ensureDetachedContext(childInputs, childCtx)
	ensureDetachedGitPersist(childInputs, childCtx)
	if _, ok := childInputs["context"]; !ok {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError("detached workspace missing context", "MISSING_CONTEXT", nil)
	}
	workflow.GetLogger(ctx).Info("scheduling async discrete child recipe", "inputs_keys", mapKeys(childInputs))
	workflow.GetLogger(ctx).Info("async discrete context snapshot", "context", childInputs["context"])

	cwo := workflow.ChildWorkflowOptions{
		RetryPolicy:           retry,
		ParentClosePolicy:     enumspb.PARENT_CLOSE_POLICY_ABANDON,
		WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
	}
	if timeout > 0 {
		cwo.WorkflowRunTimeout = timeout
		cwo.WorkflowExecutionTimeout = timeout
	}
	childCtxWF := workflow.WithChildOptions(ctx, cwo)
	childFuture := workflow.ExecuteChildWorkflow(childCtxWF, input.Name, childInputs)
	var exec workflow.Execution
	if err := childFuture.GetChildWorkflowExecution().Get(childCtxWF, &exec); err != nil {
		return RecipeOutput{}, err
	}
	handle := map[string]interface{}{
		"recipe":           input.Name,
		"workflow_id":      exec.ID,
		"run_id":           exec.RunID,
		"git_state":        gitStateDiscrete,
		"context":          childInputs["context"],
		"git_persist_hash": childInputs["git_persist_hash"],
	}
	return RecipeOutput{Outputs: map[string]interface{}{"async_handle": handle}}, nil
}

func prepareDetachedChild(inv ops.Invocation, baseInputs map[string]interface{}) (gitstate.Context, map[string]interface{}, error) {
	childCtx, childInputs, err := gitstate.PlanDetachedWorkspace(inv, baseInputs, gitstate.DetachedWorkspaceOptions{})
	if err != nil {
		return gitstate.Context{}, nil, err
	}
	ensureDetachedContext(childInputs, childCtx)
	ensureDetachedGitPersist(childInputs, childCtx)
	return childCtx, childInputs, nil
}

func ensureDetachedContext(inputs map[string]interface{}, ctx gitstate.Context) {
	if inputs == nil {
		return
	}
	contextVal, _ := inputs["context"].(map[string]interface{})
	if contextVal == nil {
		contextVal = make(map[string]interface{})
	} else {
		contextVal = copyShallow(contextVal)
	}
	gitMap, _ := contextVal["git"].(map[string]interface{})
	if gitMap == nil {
		gitMap = make(map[string]interface{})
	} else {
		gitMap = copyShallow(gitMap)
	}
	for k, v := range ctx.ToMap() {
		gitMap[k] = v
	}
	contextVal["git"] = gitMap
	if ctx.WorktreePath != "" {
		contextVal["worktree"] = ctx.WorktreePath
	}
	if ctx.BlobStoreURI != "" {
		contextVal["blobstore"] = ctx.BlobStoreURI
	}
	if ctx.TicketID != "" {
		contextVal["ticketid"] = ctx.TicketID
	}
	if ctx.CellName != "" {
		contextVal["cellname"] = ctx.CellName
	}
	inputs["context"] = contextVal
}

func ensureDetachedGitPersist(inputs map[string]interface{}, ctx gitstate.Context) {
	if inputs == nil {
		return
	}
	if _, ok := inputs["git_persist_hash"]; !ok && ctx.PersistHash != "" {
		inputs["git_persist_hash"] = ctx.PersistHash
	}
}

func mergeChildInputs(target map[string]interface{}, additions map[string]interface{}) {
	if additions == nil {
		return
	}
	for k, v := range additions {
		target[k] = v
	}
}

func copyShallow(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
