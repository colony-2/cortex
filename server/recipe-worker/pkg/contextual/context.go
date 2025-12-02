package contextual

// ActorContext represents the user/cell identity associated with an invocation.
type ActorContext struct {
	TicketID   string `json:"ticket_id,omitempty"`
	ActorName  string `json:"actor_name,omitempty"`
	ActorEmail string `json:"actor_email,omitempty"`
}

// EnvironmentContext captures filesystem and storage locations relevant to execution.
type EnvironmentContext struct {
	WorktreePath string `json:"worktree_path,omitempty"`
	BlobStoreURI string `json:"blob_store_uri,omitempty"`
	ThinPackPath string `json:"thin_pack_path,omitempty"`
}

// GitSnapshotContext captures immutable git state information at a point in time.
type GitSnapshotContext struct {
	BaseRepo     string `json:"base_repo,omitempty"`
	BaseHash     string `json:"base_hash,omitempty"`
	PersistHash  string `json:"persist_hash,omitempty"`
	PreviousHash string `json:"previous_hash,omitempty"`
	GitAuthor    string `json:"git_author,omitempty"`
}

// WorkflowEnvelope provides high-level workflow/session identifiers.
type WorkflowEnvelope struct {
	CellName string `json:"cell,omitempty"`
	JobID    string `json:"job_id,omitempty"`
}
