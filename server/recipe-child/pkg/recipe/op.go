package recipe

import (
	"fmt"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/runmetadata"
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
	segment, remaining := e.resumeTarget(nil)
	launch, err := e.prepareChildLaunch(ctx, runMode, gitMode, childInputs, segment, remaining)
	if err != nil {
		return nil, err
	}
	return e.finalizeChildLaunch(ctx, runMode, gitMode, childInputs, launch, nil)
}

type childLaunch struct {
	exec        workflow.Execution
	metadata    map[string]interface{}
	resume      *runmetadata.Resume
	wait        func() (map[string]interface{}, error)
	asyncHandle map[string]interface{}
}

func (e *childExecutor) prepareChildLaunch(ctx workflow.Context, runMode executionRunMode, gitMode gitStateMode, childInputs map[string]interface{}, segment *runmetadata.Segment, remaining *runmetadata.Resume) (*childLaunch, error) {
	if segment != nil {
		return e.prepareResetChildLaunch(ctx, runMode, gitMode, childInputs, segment, remaining)
	}
	return e.prepareNewChildLaunch(ctx, runMode, gitMode, childInputs)
}

func (e *childExecutor) prepareNewChildLaunch(ctx workflow.Context, runMode executionRunMode, gitMode gitStateMode, childInputs map[string]interface{}) (*childLaunch, error) {
	future, childCtx := e.scheduleChildWorkflow(ctx, runMode, childInputs)
	logger := workflow.GetLogger(ctx)
	logger.Info("invoking child recipe", "recipe", e.input.Name, "run_mode", runMode.String(), "git_state", gitMode.String(), "inputs_keys", mapKeys(childInputs))

	metadata := map[string]interface{}{
		"run_mode":  runMode.String(),
		"git_state": gitMode.String(),
	}

	var exec workflow.Execution
	if err := future.GetChildWorkflowExecution().Get(childCtx, &exec); err != nil {
		story.RecordChildRecipeResult(childCtx, e.invocation, story.ChildRecipeResultPayload{
			ChildWorkflowID: "",
			ChildRunID:      "",
			Status:          "start_failed",
			CompletedAt:     workflow.Now(childCtx),
			ErrorMessage:    err.Error(),
			RecipeName:      e.input.Name,
			Metadata:        copyShallow(metadata),
		})
		return nil, err
	}

	waitFn := func() (map[string]interface{}, error) {
		var result map[string]interface{}
		if err := future.Get(childCtx, &result); err != nil {
			return nil, err
		}
		if result == nil {
			result = make(map[string]interface{})
		}
		return result, nil
	}

	return &childLaunch{
		exec:        exec,
		metadata:    metadata,
		wait:        waitFn,
		asyncHandle: e.buildAsyncHandle(childInputs, exec, gitMode),
	}, nil
}

func (e *childExecutor) prepareResetChildLaunch(ctx workflow.Context, runMode executionRunMode, gitMode gitStateMode, childInputs map[string]interface{}, segment *runmetadata.Segment, remaining *runmetadata.Resume) (*childLaunch, error) {
	if segment == nil || segment.WorkflowID == "" {
		return nil, temporal.NewNonRetryableApplicationError("resume segment missing workflow reference", "RESET_INVALID_SEGMENT", nil)
	}

	reason := ""
	if e.invocation.Deps != nil {
		if meta, ok := e.invocation.Deps.RunMetadata(); ok && meta != nil {
			reason = meta.Reason
		}
	}

	activityTimeout := e.timeout
	if activityTimeout <= 0 {
		activityTimeout = 30 * time.Minute
	}
	activityOpts := workflow.ActivityOptions{
		StartToCloseTimeout: activityTimeout,
		RetryPolicy:         e.retry,
	}
	actx := workflow.WithActivityOptions(ctx, activityOpts)
	waitForCompletion := runMode == runModeSync
	var resetResult ResetChildWorkflowActivityResult
	input := ResetChildWorkflowActivityInput{
		WorkflowID:        segment.WorkflowID,
		RunID:             segment.RunID,
		EventID:           segment.EventID,
		Reason:            reason,
		WaitForCompletion: waitForCompletion,
	}
	if err := workflow.ExecuteActivity(actx, ResetChildWorkflowActivityName, input).Get(actx, &resetResult); err != nil {
		story.RecordChildRecipeResult(ctx, e.invocation, story.ChildRecipeResultPayload{
			ChildWorkflowID: segment.WorkflowID,
			ChildRunID:      segment.RunID,
			Status:          "start_failed",
			CompletedAt:     workflow.Now(ctx),
			ErrorMessage:    err.Error(),
			RecipeName:      e.input.Name,
			Metadata: map[string]interface{}{
				"run_mode":  runMode.String(),
				"git_state": gitMode.String(),
				"reset":     true,
			},
		})
		return nil, err
	}

	exec := workflow.Execution{ID: segment.WorkflowID, RunID: resetResult.RunID}
	launch := &childLaunch{
		exec: exec,
		metadata: map[string]interface{}{
			"run_mode":  runMode.String(),
			"git_state": gitMode.String(),
			"reset":     true,
		},
		resume:      remaining,
		asyncHandle: e.buildAsyncHandle(childInputs, exec, gitMode),
	}

	if waitForCompletion {
		outputs := resetResult.Result
		if outputs == nil {
			outputs = make(map[string]interface{})
		}
		launch.wait = func() (map[string]interface{}, error) {
			return copyShallow(outputs), nil
		}
	}

	return launch, nil
}

func (e *childExecutor) finalizeChildLaunch(ctx workflow.Context, runMode executionRunMode, gitMode gitStateMode, childInputs map[string]interface{}, launch *childLaunch, recipeSetIndex *int) (map[string]interface{}, error) {
	if launch == nil {
		return nil, temporal.NewNonRetryableApplicationError("child launch data missing", "INVALID_CHILD_LAUNCH", nil)
	}
	metadata := copyShallow(launch.metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	if recipeSetIndex != nil {
		metadata["index"] = *recipeSetIndex
	}

	if err := SignalChildRunMetadata(ctx, e.invocation, launch.exec, launch.resume, recipeSetIndex); err != nil {
		story.RecordChildRecipeResult(ctx, e.invocation, story.ChildRecipeResultPayload{
			ChildWorkflowID: launch.exec.ID,
			ChildRunID:      launch.exec.RunID,
			Status:          "start_failed",
			CompletedAt:     workflow.Now(ctx),
			ErrorMessage:    err.Error(),
			RecipeName:      e.input.Name,
			Metadata:        copyShallow(metadata),
		})
		return nil, err
	}

	story.RecordChildRecipeTrigger(ctx, e.invocation, story.ChildRecipeTriggerPayload{
		ChildWorkflowID: launch.exec.ID,
		ChildRunID:      launch.exec.RunID,
		TriggeredAt:     workflow.Now(ctx),
		RecipeName:      e.input.Name,
		Inputs:          copyShallow(childInputs),
		Metadata:        copyShallow(metadata),
	})

	logger := workflow.GetLogger(ctx)
	if runMode == runModeSync {
		if launch.wait == nil {
			return nil, temporal.NewNonRetryableApplicationError("synchronous child missing wait handler", "INVALID_CHILD_WAIT", nil)
		}
		result, err := launch.wait()
		if err != nil {
			story.RecordChildRecipeResult(ctx, e.invocation, story.ChildRecipeResultPayload{
				ChildWorkflowID: launch.exec.ID,
				ChildRunID:      launch.exec.RunID,
				Status:          "failed",
				CompletedAt:     workflow.Now(ctx),
				ErrorMessage:    err.Error(),
				RecipeName:      e.input.Name,
				Metadata:        copyShallow(metadata),
			})
			return nil, err
		}
		story.RecordChildRecipeResult(ctx, e.invocation, story.ChildRecipeResultPayload{
			ChildWorkflowID: launch.exec.ID,
			ChildRunID:      launch.exec.RunID,
			Status:          "completed",
			CompletedAt:     workflow.Now(ctx),
			Outputs:         copyShallow(result),
			RecipeName:      e.input.Name,
			Metadata:        copyShallow(metadata),
		})
		logger.Info("child recipe completed", "recipe", e.input.Name, "workflow_id", launch.exec.ID, "run_id", launch.exec.RunID, "keys", mapKeys(result))
		return result, nil
	}

	handle := launch.asyncHandle
	if handle == nil {
		handle = e.buildAsyncHandle(childInputs, launch.exec, gitMode)
	}
	logger.Info("async child recipe scheduled", "recipe", e.input.Name, "workflow_id", launch.exec.ID, "run_id", launch.exec.RunID, "git_state", gitMode.String())
	return map[string]interface{}{"async_handle": handle}, nil
}

func (e *childExecutor) buildAsyncHandle(childInputs map[string]interface{}, exec workflow.Execution, gitMode gitStateMode) map[string]interface{} {
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
	return handle
}

func (e *childExecutor) resumeTarget(recipeSetIndex *int) (*runmetadata.Segment, *runmetadata.Resume) {
	if e.invocation.Deps == nil {
		return nil, nil
	}
	resume, ok := e.invocation.Deps.ResumeMetadata()
	if !ok || resume == nil {
		return nil, nil
	}
	if len(resume.ExecutionPath) == 0 {
		return nil, nil
	}
	segment := resume.ExecutionPath[0]
	if segment.InvocationHash != invocationHash(e.invocation) {
		return nil, nil
	}
	if recipeSetIndex != nil {
		if segment.RecipeSetIndex == nil || *segment.RecipeSetIndex != *recipeSetIndex {
			return nil, nil
		}
	}
	segCopy := segment
	remaining := cloneResumeWithoutHead(resume)
	return &segCopy, remaining
}

func cloneResumeWithoutHead(resume *runmetadata.Resume) *runmetadata.Resume {
	if resume == nil || len(resume.ExecutionPath) <= 1 {
		return nil
	}
	cloned := make([]runmetadata.Segment, len(resume.ExecutionPath)-1)
	copy(cloned, resume.ExecutionPath[1:])
	return &runmetadata.Resume{ExecutionPath: cloned}
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
