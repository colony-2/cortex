package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	workspaceRootEnvKey  = "VIBETHIS_WORKSPACE_ROOT"
	blobStoreBaseEnvKey  = "VIBETHIS_BLOBSTORE_BASE"
	defaultWorkspaceRoot = "/tmp/vibethis/workflows"
	defaultBlobStoreBase = "file:///tmp/vibethis/blobstore"
)

// executionContextKey is used to stash the execution context inside the workflow context.
type executionContextKey struct{}

// executionContext aggregates execution metadata that needs to travel with the workflow.
type executionContext struct {
	Git       GitContext
	Worktree  string
	BlobStore string
	TicketID  string
	CellName  string
	Recipe    ExecutionRecipeContext
}

// GitContext captures core git metadata shared across the recipe lifecycle.
type GitContext struct {
	BaseRepo    string `json:"base_repo"`
	BaseHash    string `json:"base_hash"`
	PersistHash string `json:"persist_hash"`
}

type ExecutionRecipeContext struct {
	ID                string
	Version           string
	WorkflowID        string
	NodePath          string
	InvocationHash    string
	InvocationID      string
	InvocationAttempt int
}

func (rc ExecutionRecipeContext) toMap() map[string]interface{} {
	m := map[string]interface{}{
		"id":          rc.ID,
		"version":     rc.Version,
		"workflow_id": rc.WorkflowID,
		"node_path":   rc.NodePath,
	}
	if rc.InvocationHash != "" {
		m["invocation_hash"] = rc.InvocationHash
	}
	if rc.InvocationID != "" {
		m["invocation_id"] = rc.InvocationID
	}
	if rc.InvocationAttempt != 0 {
		m["invocation_attempt"] = rc.InvocationAttempt
	}
	return m
}

// toMap converts the git context to a map for injection into recipe inputs.
func (gc GitContext) toMap() map[string]interface{} {
	return map[string]interface{}{
		"base_repo":    gc.BaseRepo,
		"base_hash":    gc.BaseHash,
		"persist_hash": gc.PersistHash,
	}
}

// initializeExecutionContext validates inputs, computes workspace/blob paths, and injects context.git.
func initializeExecutionContext(ctx workflow.Context, r recipe.Recipe, inputs map[string]interface{}) (workflow.Context, map[string]interface{}, error) {
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	baseRepo, err := requireStringInput(inputs, "basegitrepo")
	if err != nil {
		return ctx, nil, err
	}

	baseHash, err := requireStringInput(inputs, "basegithash")
	if err != nil {
		return ctx, nil, err
	}

	ticketID, err := requireStringInput(inputs, "ticketid")
	if err != nil {
		return ctx, nil, err
	}

	cellName, err := requireStringInput(inputs, "cellname")
	if err != nil {
		return ctx, nil, err
	}

	workspaceRoot, err := sideEffectEnvDefault(ctx, workspaceRootEnvKey, defaultWorkspaceRoot)
	if err != nil {
		return ctx, nil, err
	}

	blobStoreBase, err := sideEffectEnvDefault(ctx, blobStoreBaseEnvKey, defaultBlobStoreBase)
	if err != nil {
		return ctx, nil, err
	}

	workflowInfo := workflow.GetInfo(ctx)
	worktreePath := filepath.Join(workspaceRoot, workflowInfo.WorkflowExecution.RunID, "work")
	blobStoreURI := fmt.Sprintf("%s/%s/%s", strings.TrimRight(blobStoreBase, "/"), cellName, ticketID)

	recipeMetadata := r.GetMetdata()
	execCtx := &executionContext{
		Git: GitContext{
			BaseRepo:    baseRepo,
			BaseHash:    baseHash,
			PersistHash: baseHash,
		},
		Worktree:  worktreePath,
		BlobStore: blobStoreURI,
		TicketID:  ticketID,
		CellName:  cellName,
		Recipe: ExecutionRecipeContext{
			ID:         recipeMetadata.ID,
			Version:    recipeMetadata.Version,
			WorkflowID: workflowInfo.WorkflowExecution.ID,
			NodePath:   recipeMetadata.NodeMetadata.ID,
		},
	}

	ctx = workflow.WithValue(ctx, executionContextKey{}, execCtx)

	contextMap, _ := inputs["context"].(map[string]interface{})
	if contextMap == nil {
		contextMap = make(map[string]interface{})
	}
	contextMap["git"] = execCtx.Git.toMap()
	contextMap["worktree"] = execCtx.Worktree
	contextMap["blobstore"] = execCtx.BlobStore
	contextMap["ticketid"] = execCtx.TicketID
	contextMap["cellname"] = execCtx.CellName
	contextMap["recipe"] = execCtx.Recipe.toMap()

	inputs["context"] = contextMap
	inputs["ticket_id"] = execCtx.TicketID
	inputs["cell_name"] = execCtx.CellName

	return ctx, inputs, nil
}

func requireStringInput(inputs map[string]interface{}, key string) (string, error) {
	val, ok := inputs[key]
	if !ok {
		return "", temporal.NewNonRetryableApplicationError(fmt.Sprintf("missing required input: %s", key), "MISSING_INPUT", nil)
	}

	str, ok := val.(string)
	if !ok || strings.TrimSpace(str) == "" {
		return "", temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid value for input %s", key), "INVALID_INPUT", nil)
	}

	return str, nil
}

func sideEffectEnvDefault(ctx workflow.Context, key, fallback string) (string, error) {
	future := workflow.SideEffect(ctx, func(workflow.Context) interface{} {
		if v := os.Getenv(key); strings.TrimSpace(v) != "" {
			return v
		}
		return fallback
	})

	var value string
	if err := future.Get(&value); err != nil {
		return "", err
	}
	return value, nil
}
