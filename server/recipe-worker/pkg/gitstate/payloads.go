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
