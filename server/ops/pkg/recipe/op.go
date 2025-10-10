package recipe

import (
	"fmt"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/story"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RecipeInput defines the input for recipe activities
type RecipeInput struct {
	Name     string                 `json:"name"`
	Inputs   map[string]interface{} `json:"inputs"`
	GitState GitStateModeName       `json:"git_state,omitempty"`
	RunMode  ExecutionRunModeName   `json:"run_mode,omitempty"`
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

type GitStateModeName = string
type ExecutionRunModeName = string

const (
	gitStateSharedName   GitStateModeName     = "shared"
	gitStateDiscreteName GitStateModeName     = "discrete"
	runModeSyncName      ExecutionRunModeName = "sync"
	runModeAsyncName     ExecutionRunModeName = "async"
)

type gitStateMode int

const (
	gitStateModeUnknown gitStateMode = iota
	gitStateModeShared
	gitStateModeDiscrete
)

var gitStateModeNames = map[gitStateMode]GitStateModeName{
	gitStateModeShared:   gitStateSharedName,
	gitStateModeDiscrete: gitStateDiscreteName,
}

func (m gitStateMode) String() string {
	if name, ok := gitStateModeNames[m]; ok {
		return name
	}
	return "unknown"
}

type executionRunMode int

const (
	runModeUnknown executionRunMode = iota
	runModeSync
	runModeAsync
)

var runModeNames = map[executionRunMode]ExecutionRunModeName{
	runModeSync:  runModeSyncName,
	runModeAsync: runModeAsyncName,
}

func (m executionRunMode) String() string {
	if name, ok := runModeNames[m]; ok {
		return name
	}
	return "unknown"
}

func parseGitStateMode(raw GitStateModeName) (gitStateMode, error) {
	value := strings.ToLower(strings.TrimSpace(string(raw)))
	if value == "" {
		return gitStateModeShared, nil
	}
	switch value {
	case gitStateModeNames[gitStateModeShared]:
		return gitStateModeShared, nil
	case gitStateModeNames[gitStateModeDiscrete]:
		return gitStateModeDiscrete, nil
	default:
		return gitStateModeUnknown, fmt.Errorf("invalid git_state: %s", raw)
	}
}

func parseRunMode(raw ExecutionRunModeName) (executionRunMode, error) {
	value := strings.ToLower(strings.TrimSpace(string(raw)))
	if value == "" {
		return runModeSync, nil
	}
	switch value {
	case runModeNames[runModeSync]:
		return runModeSync, nil
	case runModeNames[runModeAsync]:
		return runModeAsync, nil
	default:
		return runModeUnknown, fmt.Errorf("invalid run_mode: %s", raw)
	}
}

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

	gitMode, err := parseGitStateMode(input.GitState)
	if err != nil {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError(err.Error(), "INVALID_GIT_STATE", nil)
	}

	rMode, err := parseRunMode(input.RunMode)
	if err != nil {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError(err.Error(), "INVALID_RUN_MODE", nil)
	}

	executor := childExecutor{
		invocation: inv,
		ctx:        ctx,
		timeout:    timeout,
		retry:      retry,
		input:      input,
		baseInputs: baseInputs,
	}

	return executor.run(gitMode, rMode)
}

type childExecutor struct {
	invocation ops.Invocation
	ctx        workflow.Context
	timeout    time.Duration
	retry      *temporal.RetryPolicy
	input      RecipeInput
	baseInputs map[string]interface{}
}

func (e *childExecutor) run(gitMode gitStateMode, runMode executionRunMode) (RecipeOutput, error) {
	if gitMode == gitStateModeUnknown || runMode == runModeUnknown {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError("unsupported git_state/run_mode combination", "INVALID_COMBINATION", nil)
	}

	switch gitMode {
	case gitStateModeShared:
		return e.runShared(runMode)
	case gitStateModeDiscrete:
		return e.runDiscrete(runMode)
	default:
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError("unsupported git_state/run_mode combination", "INVALID_COMBINATION", nil)
	}
}

func (e *childExecutor) runShared(runMode executionRunMode) (RecipeOutput, error) {
	if !hasGitContext(e.baseInputs) {
		fallbackInputs := copyShallow(e.baseInputs)
		if fallbackInputs == nil {
			fallbackInputs = make(map[string]interface{})
		}
		mergeChildInputs(fallbackInputs, e.input.Inputs)
		outputs, err := e.startChildWorkflow(e.ctx, runMode, gitStateModeShared, fallbackInputs)
		if err != nil {
			return RecipeOutput{}, err
		}
		return RecipeOutput{Outputs: outputs}, nil
	}

	options := InlineWorkspaceOptions{}
	if runMode == runModeAsync {
		options.SkipFinalize = true
	}

	workspaceResult, err := withInlineWorkspace(e.ctx, e.invocation, e.baseInputs, options, func(inner workflow.Context, childInputs map[string]interface{}) (map[string]interface{}, error) {
		mergeChildInputs(childInputs, e.input.Inputs)
		return e.startChildWorkflow(inner, runMode, gitStateModeShared, childInputs)
	})
	if err != nil {
		return RecipeOutput{}, err
	}

	outputs := map[string]interface{}{}
	if workspaceResult != nil && workspaceResult.Result != nil {
		outputs = workspaceResult.Result
	}

	result := RecipeOutput{Outputs: outputs}
	if runMode == runModeSync && workspaceResult != nil {
		result.Context = workspaceResult.ContextMap
		if workspaceResult.GitContext != nil {
			result.GitPersistHash = workspaceResult.GitContext.GetPersistHash()
		}
	}
	return result, nil
}

func (e *childExecutor) runDiscrete(runMode executionRunMode) (RecipeOutput, error) {
	childCtx, childInputs, err := planDetachedWorkspace(e.invocation, e.baseInputs, DetachedWorkspaceOptions{})
	if err != nil {
		return RecipeOutput{}, err
	}

	mergeChildInputs(childInputs, e.input.Inputs)
	ensureDetachedContext(childInputs, childCtx)
	ensureDetachedGitPersist(childInputs, childCtx)
	if _, ok := childInputs["context"]; !ok {
		return RecipeOutput{}, temporal.NewNonRetryableApplicationError("detached workspace missing context", "MISSING_CONTEXT", nil)
	}

	outputs, err := e.startChildWorkflow(e.ctx, runMode, gitStateModeDiscrete, childInputs)
	if err != nil {
		return RecipeOutput{}, err
	}
	return RecipeOutput{Outputs: outputs}, nil
}

func (e *childExecutor) startChildWorkflow(ctx workflow.Context, runMode executionRunMode, gitMode gitStateMode, childInputs map[string]interface{}) (map[string]interface{}, error) {
	future, childCtx := e.scheduleChildWorkflow(ctx, runMode, childInputs)
	logger := workflow.GetLogger(ctx)
	logger.Info("invoking child recipe", "recipe", e.input.Name, "run_mode", runMode.String(), "git_state", gitMode.String(), "inputs_keys", mapKeys(childInputs))

	var exec workflow.Execution
	if err := future.GetChildWorkflowExecution().Get(childCtx, &exec); err != nil {
		story.RecordChildRecipeResult(childCtx, e.invocation, story.ChildRecipeResultPayload{
			ChildWorkflowID: "",
			ChildRunID:      "",
			Status:          "start_failed",
			CompletedAt:     workflow.Now(childCtx),
			ErrorMessage:    err.Error(),
			RecipeName:      e.input.Name,
			Metadata: map[string]interface{}{
				"run_mode":  runMode.String(),
				"git_state": gitMode.String(),
			},
		})
		return nil, err
	}

	story.RecordChildRecipeTrigger(childCtx, e.invocation, story.ChildRecipeTriggerPayload{
		ChildWorkflowID: exec.ID,
		ChildRunID:      exec.RunID,
		TriggeredAt:     workflow.Now(childCtx),
		RecipeName:      e.input.Name,
		Inputs:          copyShallow(childInputs),
		Metadata: map[string]interface{}{
			"run_mode":  runMode.String(),
			"git_state": gitMode.String(),
		},
	})

	if runMode == runModeSync {
		var result map[string]interface{}
		if err := future.Get(childCtx, &result); err != nil {
			story.RecordChildRecipeResult(childCtx, e.invocation, story.ChildRecipeResultPayload{
				ChildWorkflowID: exec.ID,
				ChildRunID:      exec.RunID,
				Status:          "failed",
				CompletedAt:     workflow.Now(childCtx),
				ErrorMessage:    err.Error(),
				RecipeName:      e.input.Name,
				Metadata: map[string]interface{}{
					"run_mode":  runMode.String(),
					"git_state": gitMode.String(),
				},
			})
			return nil, err
		}
		if result == nil {
			result = make(map[string]interface{})
		}
		logger.Info("child recipe completed", "recipe", e.input.Name, "keys", mapKeys(result))
		story.RecordChildRecipeResult(childCtx, e.invocation, story.ChildRecipeResultPayload{
			ChildWorkflowID: exec.ID,
			ChildRunID:      exec.RunID,
			Status:          "completed",
			CompletedAt:     workflow.Now(childCtx),
			Outputs:         copyShallow(result),
			RecipeName:      e.input.Name,
			Metadata: map[string]interface{}{
				"run_mode":  runMode.String(),
				"git_state": gitMode.String(),
			},
		})
		return result, nil
	}
	handle := map[string]interface{}{
		"recipe":      e.input.Name,
		"workflow_id": exec.ID,
		"run_id":      exec.RunID,
		"git_state":   gitMode.String(),
	}
	if ctxVal, ok := childInputs["context"]; ok {
		handle["context"] = ctxVal
	}
	if hash, ok := childInputs["git_persist_hash"]; ok {
		handle["git_persist_hash"] = hash
	}
	logger.Info("async child recipe scheduled", "recipe", e.input.Name, "workflow_id", exec.ID, "run_id", exec.RunID, "git_state", gitMode.String())
	return map[string]interface{}{"async_handle": handle}, nil
}

func (e *childExecutor) scheduleChildWorkflow(ctx workflow.Context, runMode executionRunMode, childInputs map[string]interface{}) (workflow.ChildWorkflowFuture, workflow.Context) {
	childCtx := workflow.WithChildOptions(ctx, e.childWorkflowOptions(runMode))
	future := workflow.ExecuteChildWorkflow(childCtx, e.input.Name, childInputs)
	return future, childCtx
}

func (e *childExecutor) childWorkflowOptions(runMode executionRunMode) workflow.ChildWorkflowOptions {
	options := workflow.ChildWorkflowOptions{
		RetryPolicy: e.retry,
	}
	if e.timeout > 0 {
		options.WorkflowRunTimeout = e.timeout
		options.WorkflowExecutionTimeout = e.timeout
	}
	if runMode == runModeAsync {
		options.ParentClosePolicy = enumspb.PARENT_CLOSE_POLICY_ABANDON
		options.WorkflowIDReusePolicy = enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE
	}
	return options
}

func ensureDetachedContext(inputs map[string]interface{}, ctx GitStateContext) {
	if inputs == nil || ctx == nil {
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
	if path := ctx.GetWorktreePath(); path != "" {
		contextVal["worktree"] = path
	}
	if uri := ctx.GetBlobStoreURI(); uri != "" {
		contextVal["blobstore"] = uri
	}
	if ticket := ctx.GetTicketID(); ticket != "" {
		contextVal["ticketid"] = ticket
	}
	if cell := ctx.GetCellName(); cell != "" {
		contextVal["cellname"] = cell
	}
	inputs["context"] = contextVal
}

func ensureDetachedGitPersist(inputs map[string]interface{}, ctx GitStateContext) {
	if inputs == nil || ctx == nil {
		return
	}
	if _, ok := inputs["git_persist_hash"]; !ok && ctx.GetPersistHash() != "" {
		inputs["git_persist_hash"] = ctx.GetPersistHash()
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

func hasGitContext(inputs map[string]interface{}) bool {
	if inputs == nil {
		return false
	}
	if ctxVal, ok := inputs["context"].(map[string]interface{}); ok && ctxVal != nil {
		if _, hasGit := ctxVal["git"].(map[string]interface{}); hasGit {
			return true
		}
	}
	if _, ok := inputs["basegitrepo"]; ok {
		return true
	}
	return false
}
