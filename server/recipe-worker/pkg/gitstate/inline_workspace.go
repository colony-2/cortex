package gitstate

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// InlineWorkspaceOptions provides knobs for deriving child workspace metadata.
type InlineWorkspaceOptions struct {
	// ChildID overrides the generated child workspace identifier used in the worktree path.
	ChildID string
	// SkipFinalize skips the persist/injection stages. Useful for async invocations where the caller cannot
	// safely wait for Git persistence without blocking on the child workflow completing first.
	SkipFinalize bool
}

// InlineWorkspaceResult packages the result of executing inline logic within a managed workspace lifecycle.
type InlineWorkspaceResult struct {
	// Result captures the raw outputs from the wrapped inline logic.
	Result map[string]interface{}
	// ContextMap mirrors the structure injected into outputs["context"].
	ContextMap map[string]interface{}
	// GitContext exposes the structured git metadata after persistence.
	GitContext Context
}

// InlineWorkspaceFunc represents the inline logic executed within a managed git workspace.
type InlineWorkspaceFunc func(workflow.Context, map[string]interface{}) (map[string]interface{}, error)

// WithInlineWorkspace orchestrates prepare/restore/persist semantics for inline operations that mutate git state.
func WithInlineWorkspace(ctx workflow.Context, inv coreops.Invocation, inputs map[string]interface{}, opts InlineWorkspaceOptions, fn InlineWorkspaceFunc) (*InlineWorkspaceResult, error) {
	if fn == nil {
		return nil, fmt.Errorf("inline workspace requires execution function")
	}
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	parentCtx, err := ContextFromRequest(inv, inputs)
	if err != nil {
		return nil, err
	}

	childCtx, childInputs, err := deriveChildWorkspace(inv, *parentCtx, opts, inputs)
	if err != nil {
		return nil, err
	}

	childCtx, err = runInlineLifecycleStage(ctx, childCtx, inlinePrepareWorkspaceActivity)
	if err != nil {
		return nil, err
	}

	childCtx, err = runInlineLifecycleStage(ctx, childCtx, inlineRestoreWorkspaceActivity)
	if err != nil {
		return nil, err
	}

	result, err := fn(ctx, childInputs)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = make(map[string]interface{})
	}

	workspaceResult := &InlineWorkspaceResult{Result: result}

	if opts.SkipFinalize {
		workspaceResult.GitContext = childCtx
		return workspaceResult, nil
	}

	persistOutput, err := runInlinePersistStage(ctx, childCtx)
	if err != nil {
		return nil, err
	}
	childCtx = persistOutput.Context

	outputs := map[string]interface{}{
		"result": result,
	}
	InjectPersistResult(outputs, childCtx.PersistHash, childCtx)

	contextMap, _ := outputs["context"].(map[string]interface{})

	workspaceResult.ContextMap = cloneMap(contextMap)
	workspaceResult.GitContext = childCtx

	return workspaceResult, nil
}

type inlineLifecycleActivity func(context.Context, Context) (Context, error)

func runInlineLifecycleStage(ctx workflow.Context, gitCtx Context, activity inlineLifecycleActivity) (Context, error) {
	laOpts := workflow.LocalActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumAttempts:    3,
		},
	}
	laCtx := workflow.WithLocalActivityOptions(ctx, laOpts)
	var updated Context
	err := workflow.ExecuteLocalActivity(laCtx, activity, gitCtx).Get(laCtx, &updated)
	if err != nil {
		return Context{}, err
	}
	return updated, nil
}

func runInlinePersistStage(ctx workflow.Context, gitCtx Context) (inlinePersistResult, error) {
	laOpts := workflow.LocalActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumAttempts:    3,
		},
	}
	laCtx := workflow.WithLocalActivityOptions(ctx, laOpts)
	var output inlinePersistResult
	err := workflow.ExecuteLocalActivity(laCtx, inlinePersistWorkspaceActivity, gitCtx).Get(laCtx, &output)
	if err != nil {
		return inlinePersistResult{}, err
	}
	return output, nil
}

type inlinePersistResult struct {
	Context Context
}

func inlinePrepareWorkspaceActivity(ctx context.Context, gitCtx Context) (Context, error) {
	controller := NewController(nil)
	if err := controller.PrepareWorkspace(ctx, &gitCtx); err != nil {
		return Context{}, err
	}
	return gitCtx, nil
}

func inlineRestoreWorkspaceActivity(ctx context.Context, gitCtx Context) (Context, error) {
	controller := NewController(nil)
	if err := controller.Restore(ctx, &gitCtx); err != nil {
		return Context{}, err
	}
	return gitCtx, nil
}

func inlinePersistWorkspaceActivity(ctx context.Context, gitCtx Context) (inlinePersistResult, error) {
	controller := NewController(nil)
	_, updated, err := controller.Persist(ctx, &gitCtx)
	if err != nil {
		return inlinePersistResult{}, err
	}
	return inlinePersistResult{Context: updated}, nil
}

func deriveChildWorkspace(inv coreops.Invocation, parentCtx Context, opts InlineWorkspaceOptions, baseInputs map[string]interface{}) (Context, map[string]interface{}, error) {
	if parentCtx.WorktreePath == "" {
		return Context{}, nil, fmt.Errorf("parent git context missing worktree path")
	}

	runRoot := filepath.Dir(parentCtx.WorktreePath)
	childSegment := opts.ChildID
	if strings.TrimSpace(childSegment) == "" {
		childSegment = defaultChildSegment(inv)
	} else {
		childSegment = sanitizeSegment(childSegment)
		if childSegment == "" {
			childSegment = defaultChildSegment(inv)
		}
	}

	childCtx := parentCtx
	childCtx.WorktreePath = filepath.Join(runRoot, childSegment, "work")
	childCtx.ThinPackPath = ""
	childCtx.WorkspacePrepared = false
	childCtx.PreviousHash = parentCtx.PersistHash
	childCtx.InvocationID = inv.ID
	childCtx.InvocationHash = inv.Hash()
	childCtx.InvocationAttempt = inv.InvokeSeq
	childCtx.ActivityID = inv.ActivityID
	childCtx.BoxID = inv.BoxID
	if inv.NodePath != "" {
		childCtx.RecipeNode = inv.NodePath
	}

	childInputs := cloneMap(baseInputs)

	contextMap, ok := childInputs["context"].(map[string]interface{})
	if !ok {
		contextMap = make(map[string]interface{})
	} else {
		contextMap = cloneMap(contextMap)
	}

	gitMap := childCtx.ToMap()
	gitMap["worktree_path"] = childCtx.WorktreePath
	contextMap["git"] = gitMap
	contextMap["worktree"] = childCtx.WorktreePath
	contextMap["blobstore"] = childCtx.BlobStoreURI
	if childCtx.TicketID != "" {
		contextMap["ticketid"] = childCtx.TicketID
	}
	if childCtx.CellName != "" {
		contextMap["cellname"] = childCtx.CellName
	}

	recipeMeta := map[string]interface{}{
		"id":        childCtx.RecipeID,
		"node_path": childCtx.RecipeNode,
	}
	if childCtx.InvocationHash != "" {
		recipeMeta["invocation_hash"] = childCtx.InvocationHash
	}
	if childCtx.InvocationID != "" {
		recipeMeta["invocation_id"] = childCtx.InvocationID
	}
	if childCtx.InvocationAttempt != 0 {
		recipeMeta["invocation_attempt"] = childCtx.InvocationAttempt
	}
	if childCtx.WorkflowID != "" {
		recipeMeta["workflow_id"] = childCtx.WorkflowID
	}
	if childCtx.WorkflowRunID != "" {
		recipeMeta["workflow_run_id"] = childCtx.WorkflowRunID
	}
	contextMap["recipe"] = recipeMeta

	childInputs["context"] = contextMap
	childInputs["git_persist_hash"] = childCtx.PersistHash
	childInputs["ticket_id"] = childCtx.TicketID
	childInputs["cell_name"] = childCtx.CellName

	return childCtx, childInputs, nil
}

var nonSegmentChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func defaultChildSegment(inv coreops.Invocation) string {
	base := sanitizeSegment(inv.NodePath)
	if base == "" {
		base = "inline"
	}
	hash := inv.Hash()
	if len(hash) > 8 {
		hash = hash[:8]
	}
	return fmt.Sprintf("%s-%s-%d", base, hash, inv.InvokeSeq)
}

func sanitizeSegment(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, string(filepath.Separator), "-")
	raw = strings.ReplaceAll(raw, " ", "-")
	raw = nonSegmentChars.ReplaceAllString(raw, "-")
	raw = strings.Trim(raw, "-")
	raw = strings.ToLower(raw)
	return raw
}

func cloneMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for k, v := range input {
		switch typed := v.(type) {
		case map[string]interface{}:
			out[k] = cloneMap(typed)
		case []interface{}:
			out[k] = cloneSlice(typed)
		default:
			out[k] = typed
		}
	}
	return out
}

func cloneSlice(input []interface{}) []interface{} {
	if input == nil {
		return nil
	}
	out := make([]interface{}, len(input))
	for i, v := range input {
		switch typed := v.(type) {
		case map[string]interface{}:
			out[i] = cloneMap(typed)
		case []interface{}:
			out[i] = cloneSlice(typed)
		default:
			out[i] = typed
		}
	}
	return out
}
