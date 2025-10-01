package gitstate

// InjectPersistResult merges git persistence metadata into the activity outputs.
func InjectPersistResult(outputs map[string]interface{}, newHash string, updated Context) {
	if outputs == nil {
		return
	}

	contextMap, _ := outputs["context"].(map[string]interface{})
	if contextMap == nil {
		contextMap = make(map[string]interface{})
		outputs["context"] = contextMap
	}

	contextMap["git"] = updated.ToMap()
	contextMap["worktree"] = updated.WorktreePath
	contextMap["blobstore"] = updated.BlobStoreURI
	if updated.TicketID != "" {
		contextMap["ticketid"] = updated.TicketID
	}
	if updated.CellName != "" {
		contextMap["cellname"] = updated.CellName
	}

	recipeMap := map[string]interface{}{
		"id":        updated.RecipeID,
		"node_path": updated.RecipeNode,
	}
	if updated.WorkflowID != "" {
		recipeMap["workflow_id"] = updated.WorkflowID
	}
	if updated.WorkflowRunID != "" {
		recipeMap["workflow_run_id"] = updated.WorkflowRunID
	}
	if updated.InvocationHash != "" {
		recipeMap["invocation_hash"] = updated.InvocationHash
	}
	if updated.InvocationID != "" {
		recipeMap["invocation_id"] = updated.InvocationID
	}
	if updated.InvocationAttempt != 0 {
		recipeMap["invocation_attempt"] = updated.InvocationAttempt
	}
	contextMap["recipe"] = recipeMap

	outputs["git_persist_hash"] = updated.PersistHash
}
