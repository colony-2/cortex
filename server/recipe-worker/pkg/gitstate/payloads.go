package gitstate

import "encoding/json"

// WorkspacePayload carries the typed git context and any auxiliary fields from the caller.
// The RawInput field preserves the original user payload for the wrapped function to decode.
type WorkspacePayload struct {
	Context        Context         `json:"context"`
	GitPersistHash string          `json:"git_persist_hash,omitempty"`
	TicketID       string          `json:"ticket_id,omitempty"`
	CellName       string          `json:"cell_name,omitempty"`
	RawInput       json.RawMessage `json:"raw_input,omitempty"`
}

// WorkspaceResult captures the output of inline/detached workspace executions.
type WorkspaceResult struct {
	Result         json.RawMessage `json:"result,omitempty"`
	Context        Context         `json:"context"`
	GitPersistHash string          `json:"git_persist_hash,omitempty"`
}

// LegacyOutputsFromResult converts a typed workspace result into a legacy map shape for callers still expecting maps.
func LegacyOutputsFromResult(res WorkspaceResult) map[string]interface{} {
	outputs := map[string]interface{}{
		"result": res.Result,
	}

	ctx := res.Context
	contextMap := map[string]interface{}{
		"git": ctx.ToMap(),
	}
	contextMap["worktree"] = ctx.WorktreePath
	contextMap["blobstore"] = ctx.BlobStoreURI
	if ctx.TicketID != "" {
		contextMap["ticketid"] = ctx.TicketID
	}
	if ctx.CellName != "" {
		contextMap["cellname"] = ctx.CellName
	}

	recipeMap := map[string]interface{}{
		"id":        ctx.RecipeID,
		"node_path": ctx.NodePath,
	}
	if ctx.InvocationHash != "" {
		recipeMap["invocation_hash"] = ctx.InvocationHash
	}
	if ctx.InvocationID != "" {
		recipeMap["invocation_id"] = ctx.InvocationID
	}
	if ctx.InvocationAttempt != 0 {
		recipeMap["invocation_attempt"] = ctx.InvocationAttempt
	}
	if ctx.JobID != "" {
		recipeMap["job_id"] = string(ctx.JobID)
	}
	contextMap["recipe"] = recipeMap

	outputs["context"] = contextMap
	outputs["git_persist_hash"] = ctx.PersistHash
	return outputs
}
