//go:build futureworkspace

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/git/pkg/gitcommit"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"go.temporal.io/sdk/temporal"
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
type InlineWorkspaceFunc[In any, Out any] func(workflow.Context, In) (Out, error)

// WithInlineWorkspace orchestrates prepare/restore/persist semantics for inline operations that mutate git state.
func WithInlineWorkspace[In any, Out any](ctx context.Context, parentCtx GitTaskContext, opts InlineWorkspaceOptions, fn InlineWorkspaceFunc[In, Out], in In) (*InlineWorkspaceResult, Out, error) {
	var result Out
	if fn == nil {
		return nil, result, fmt.Errorf("inline workspace requires execution function")
	}

	childCtx, err := deriveChildWorkspace(parentCtx, opts)
	if err != nil {
		return nil, result, err
	}

	controller := NewController(nil)
	if err := controller.Restore(ctx, gitCtx); err != nil {
		return err
	}

	err = inlineRestoreWorkspaceActivity(ctx, &childCtx)
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

func runInlinePersistStage(ctx workflow.Context, gitCtx GitTaskContext) (inlinePersistResult, error) {
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
	GitHash string `json:"git_hash"`
}

func inlineRestoreWorkspaceActivity(ctx context.Context, gitCtx *GitTaskContext) error {
	controller := NewController(nil)
	if err := controller.Restore(ctx, gitCtx); err != nil {
		return err
	}
	return nil
}

func inlinePersistWorkspaceActivity(ctx context.Context, gitCtx *GitTaskContext) (*gitcommit.PersistCommitOutput, error) {
	controller := NewController(nil)
	return controller.Persist(ctx, gitCtx)
}

func deriveChildWorkspace(parentCtx GitTaskContext, opts InlineWorkspaceOptions) (GitTaskContext, error) {
	childCtx := parentCtx
	if parentCtx.WorktreePath == "" {
		return childCtx, fmt.Errorf("parent git context missing worktree path")
	}

	runRoot := filepath.Dir(parentCtx.WorktreePath)
	childSegment := strings.TrimSpace(opts.ChildID)
	if childSegment != "" {
		childSegment = defaultChildSegment(parentCtx)
	}

	childCtx.WorktreePath = filepath.Join(runRoot, childSegment, "work")
	childCtx.ThinPackPath = ""
	childCtx.PreviousHash = parentCtx.PersistHash
	return childCtx, nil
}

var nonSegmentChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func defaultChildSegment(git GitTaskContext) string {
	base := sanitizeSegment(git.NodePath)
	if base == "" {
		base = "inline"
	}

	if len(base) > 10 {
		base = base[:40]
	}

	hash := git.GetInvokeHash()
	if len(hash) > 8 {
		hash = hash[:8]
	}
	return fmt.Sprintf("%s-%s-%d", base, hash, git.InvokeSeq)
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
