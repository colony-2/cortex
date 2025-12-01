package gitstate

import (
	"context"
	"encoding/json"
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
	WorkspaceResult
}

// InlineWorkspaceFunc represents the inline logic executed within a managed git workspace.
type InlineWorkspaceFunc func(workflow.Context, WorkspacePayload) (json.RawMessage, error)

// WithInlineWorkspace orchestrates prepare/restore/persist semantics for inline operations that mutate git state.
func WithInlineWorkspace(ctx workflow.Context, inv coreops.Invocation, payload WorkspacePayload, opts InlineWorkspaceOptions, fn InlineWorkspaceFunc) (*InlineWorkspaceResult, error) {
	if fn == nil {
		return nil, fmt.Errorf("inline workspace requires execution function")
	}

	parentCtx, err := ContextFromPayload(inv, payload)
	if err != nil {
		return nil, err
	}

	childCtx, childPayload, err := deriveChildWorkspace(inv, *parentCtx, opts, payload)
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

	result, err := fn(ctx, childPayload)
	if err != nil {
		return nil, err
	}

	workspaceResult := &InlineWorkspaceResult{}

	if opts.SkipFinalize {
		workspaceResult.Context = childCtx
		if len(result) == 0 {
			workspaceResult.Result = json.RawMessage(`{}`)
		} else {
			workspaceResult.Result = result
		}
		workspaceResult.GitPersistHash = childCtx.PersistHash
		return workspaceResult, nil
	}

	persistOutput, err := runInlinePersistStage(ctx, childCtx)
	if err != nil {
		return nil, err
	}
	childCtx = persistOutput.Context

	workspaceResult.WorkspaceResult = WorkspaceResult{
		Result:         result,
		Context:        childCtx,
		GitPersistHash: childCtx.PersistHash,
	}

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

func deriveChildWorkspace(inv coreops.Invocation, parentCtx Context, opts InlineWorkspaceOptions, basePayload WorkspacePayload) (Context, WorkspacePayload, error) {
	if parentCtx.WorktreePath == "" {
		return Context{}, WorkspacePayload{}, fmt.Errorf("parent git context missing worktree path")
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
		childCtx.NodePath = inv.NodePath
	}

	childPayload := basePayload
	childPayload.Context = childCtx
	childPayload.GitPersistHash = childCtx.PersistHash
	childPayload.TicketID = childCtx.TicketID
	childPayload.CellName = childCtx.CellName

	return childCtx, childPayload, nil
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
