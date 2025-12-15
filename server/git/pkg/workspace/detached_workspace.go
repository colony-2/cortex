//go:build futureworkspace

package workspace

import (
	"fmt"
	"path/filepath"
	"strings"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
)

// DetachedWorkspaceOptions controls how detached workspaces are named.
type DetachedWorkspaceOptions struct {
	ChildID string
}

// DetachedWorkspaceResult mirrors the inline result but is used for isolated executions.
type DetachedWorkspaceResult struct {
	WorkspaceResult
}

// DetachedWorkspaceFunc represents the execution performed inside the detached workspace.
type DetachedWorkspaceFunc[In any, Out any] func(workflow.Context, In) (Out, error)

// PlanDetachedWorkspace derives an isolated git context and input payload without performing any git operations.
func PlanDetachedWorkspace(inv coreops.Invocation, parentCtx GitTaskContext, opts DetachedWorkspaceOptions) (GitTaskContext, error) {
	return deriveDetachedWorkspace(inv, parentCtx, opts)
}

// WithDetachedWorkspace provisions a standalone git workspace, executes fn, and persists the resulting git context without mutating the caller's inputs.
func WithDetachedWorkspace[In any, Out any](ctx workflow.Context, inv coreops.Invocation, parentCtx GitTaskContext, opts DetachedWorkspaceOptions, fn DetachedWorkspaceFunc[In, Out], input In) (Out, string, error) {
	var result Out
	if fn == nil {
		return result, "", fmt.Errorf("detached workspace requires execution function")
	}

	childCtx, err := PlanDetachedWorkspace(inv, parentCtx, opts)
	if err != nil {
		return result, "", err
	}

	childCtx, err = runInlineLifecycleStage(ctx, childCtx, inlinePrepareWorkspaceActivity)
	if err != nil {
		return result, "", err
	}

	childCtx, err = runInlineLifecycleStage(ctx, childCtx, inlineRestoreWorkspaceActivity)
	if err != nil {
		return result, "", err
	}

	result, err := fn(ctx, input)
	if err != nil {
		return result, "", err
	}

	persistOutput, err := runInlinePersistStage(ctx, childCtx)
	if err != nil {
		return result, "", err
	}

	return &DetachedWorkspaceResult{
		WorkspaceResult: WorkspaceResult{
			Result:         result,
			Context:        childCtx,
			GitPersistHash: childCtx.PersistHash,
		},
	}, nil
}

func deriveDetachedWorkspace(inv coreops.Invocation, parentCtx GitTaskContext, opts DetachedWorkspaceOptions) (GitTaskContext, error) {
	if parentCtx.WorktreePath == "" {
		return parentCtx, fmt.Errorf("git context missing worktree path for detached workspace")
	}

	runRoot := filepath.Dir(parentCtx.WorktreePath)
	segment := opts.ChildID
	if strings.TrimSpace(segment) == "" {
		segment = fmt.Sprintf("detached-%s", defaultChildSegment(inv))
	} else {
		segment = sanitizeSegment(segment)
	}

	childCtx := parentCtx
	childCtx.WorktreePath = filepath.Join(runRoot, segment, "work")
	childCtx.ThinPackPath = ""
	childCtx.PreviousHash = parentCtx.BaseHash
	if childCtx.PreviousHash == "" {
		childCtx.PreviousHash = parentCtx.PersistHash
	}
	if childCtx.BaseHash != "" {
		childCtx.PersistHash = childCtx.BaseHash
	}

	if uri := strings.TrimSpace(childCtx.BlobStoreURI); uri != "" {
		childCtx.BlobStoreURI = appendBlobStoreSegment(uri, segment)
	}

	return childCtx, nil
}

func appendBlobStoreSegment(baseURI, segment string) string {
	if segment == "" {
		return baseURI
	}
	trimmed := strings.TrimRight(baseURI, "/")
	if trimmed == "" {
		return segment
	}
	return trimmed + "/" + segment
}
