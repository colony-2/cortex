package gitstate

import (
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
	Result     map[string]interface{}
	ContextMap map[string]interface{}
	GitContext Context
}

// DetachedWorkspaceFunc represents the execution performed inside the detached workspace.
type DetachedWorkspaceFunc func(workflow.Context, map[string]interface{}) (map[string]interface{}, error)

// PlanDetachedWorkspace derives an isolated git context and input payload without performing any git operations.
func PlanDetachedWorkspace(inv coreops.Invocation, inputs map[string]interface{}, opts DetachedWorkspaceOptions) (Context, map[string]interface{}, error) {
	if inputs == nil {
		inputs = make(map[string]interface{})
	}
	baseCtx, err := ContextFromRequest(inv, inputs)
	if err != nil {
		return Context{}, nil, err
	}
	return deriveDetachedWorkspace(inv, *baseCtx, opts, inputs)
}

// WithDetachedWorkspace provisions a standalone git workspace, executes fn, and persists the resulting git context without mutating the caller's inputs.
func WithDetachedWorkspace(ctx workflow.Context, inv coreops.Invocation, inputs map[string]interface{}, opts DetachedWorkspaceOptions, fn DetachedWorkspaceFunc) (*DetachedWorkspaceResult, error) {
	if fn == nil {
		return nil, fmt.Errorf("detached workspace requires execution function")
	}
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	childCtx, childInputs, err := PlanDetachedWorkspace(inv, inputs, opts)
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

	persistOutput, err := runInlinePersistStage(ctx, childCtx)
	if err != nil {
		return nil, err
	}
	childCtx = persistOutput.Context

	outputs := map[string]interface{}{"result": result}
	InjectPersistResult(outputs, childCtx.PersistHash, childCtx)
	contextMap, _ := outputs["context"].(map[string]interface{})

	return &DetachedWorkspaceResult{
		Result:     result,
		ContextMap: cloneMap(contextMap),
		GitContext: childCtx,
	}, nil
}

func deriveDetachedWorkspace(inv coreops.Invocation, parentCtx Context, opts DetachedWorkspaceOptions, baseInputs map[string]interface{}) (Context, map[string]interface{}, error) {
	if parentCtx.WorktreePath == "" {
		return Context{}, nil, fmt.Errorf("git context missing worktree path for detached workspace")
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

	childInputs := cloneMap(baseInputs)
	if childInputs == nil {
		childInputs = make(map[string]interface{})
	}

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
		"node_path": childCtx.NodePath,
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
	if childCtx.JobID != "" {
		recipeMeta["job_id"] = string(childCtx.JobID)
	}
	contextMap["recipe"] = recipeMeta

	childInputs["context"] = contextMap
	childInputs["git_persist_hash"] = childCtx.PersistHash
	if childCtx.TicketID != "" {
		childInputs["ticket_id"] = childCtx.TicketID
	}
	if childCtx.CellName != "" {
		childInputs["cell_name"] = childCtx.CellName
	}
	if len(childInputs) == 0 {
		panic("detached workspace produced empty child inputs")
	}

	return childCtx, childInputs, nil
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
