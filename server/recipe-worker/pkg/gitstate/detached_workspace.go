package gitstate

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"go.temporal.io/sdk/workflow"
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
type DetachedWorkspaceFunc func(workflow.Context, WorkspacePayload) (json.RawMessage, error)

// PlanDetachedWorkspace derives an isolated git context and input payload without performing any git operations.
func PlanDetachedWorkspace(inv coreops.Invocation, payload WorkspacePayload, opts DetachedWorkspaceOptions) (Context, WorkspacePayload, error) {
	baseCtx, err := ContextFromPayload(inv, payload)
	if err != nil {
		return Context{}, WorkspacePayload{}, err
	}
	return deriveDetachedWorkspace(inv, *baseCtx, opts, payload)
}

// WithDetachedWorkspace provisions a standalone git workspace, executes fn, and persists the resulting git context without mutating the caller's inputs.
func WithDetachedWorkspace(ctx workflow.Context, inv coreops.Invocation, payload WorkspacePayload, opts DetachedWorkspaceOptions, fn DetachedWorkspaceFunc) (*DetachedWorkspaceResult, error) {
	if fn == nil {
		return nil, fmt.Errorf("detached workspace requires execution function")
	}

	childCtx, childPayload, err := PlanDetachedWorkspace(inv, payload, opts)
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

	persistOutput, err := runInlinePersistStage(ctx, childCtx)
	if err != nil {
		return nil, err
	}
	childCtx = persistOutput.Context

	return &DetachedWorkspaceResult{
		WorkspaceResult: WorkspaceResult{
			Result:         result,
			Context:        childCtx,
			GitPersistHash: childCtx.PersistHash,
		},
	}, nil
}

func deriveDetachedWorkspace(inv coreops.Invocation, parentCtx Context, opts DetachedWorkspaceOptions, basePayload WorkspacePayload) (Context, WorkspacePayload, error) {
	if parentCtx.WorktreePath == "" {
		return Context{}, WorkspacePayload{}, fmt.Errorf("git context missing worktree path for detached workspace")
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
	childCtx.WorkspacePrepared = false
	childCtx.PreviousHash = parentCtx.BaseHash
	if childCtx.PreviousHash == "" {
		childCtx.PreviousHash = parentCtx.PersistHash
	}
	if childCtx.BaseHash != "" {
		childCtx.PersistHash = childCtx.BaseHash
	}
	childCtx.InvocationID = inv.ID
	childCtx.InvocationHash = inv.Hash()
	childCtx.InvocationAttempt = inv.InvokeSeq
	childCtx.ActivityID = inv.ActivityID
	childCtx.BoxID = inv.BoxID
	if inv.NodePath != "" {
		childCtx.NodePath = inv.NodePath
	}

	if uri := strings.TrimSpace(childCtx.BlobStoreURI); uri != "" {
		childCtx.BlobStoreURI = appendBlobStoreSegment(uri, segment)
	}

	childPayload := basePayload
	childPayload.Context = childCtx
	childPayload.GitPersistHash = childCtx.PersistHash
	childPayload.TicketID = childCtx.TicketID
	childPayload.CellName = childCtx.CellName
	return childCtx, childPayload, nil
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
