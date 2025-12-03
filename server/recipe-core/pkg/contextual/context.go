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

// GitBaseContext captures immutable git state information at a point in time.
type GitBaseContext struct {
	BaseRepo  string `json:"repo,omitempty"`
	BaseHash  string `json:"hash,omitempty"`
	GitAuthor string `json:"author,omitempty"`
}

// WorkflowContext provides high-level workflow/session identifiers.
type WorkflowContext struct {
	CellName string `json:"cell,omitempty"`
	JobID    string `json:"job_id,omitempty"`
}

// ExecutionContext holds typed workflow context available to templates. It is created per task.
type JobContext struct {
	Actor       ActorContext       `json:"actor,omitempty"`
	Environment EnvironmentContext `json:"environment,omitempty"`
	Workflow    WorkflowContext    `json:"workflow,omitempty"`
	GitBase     GitBaseContext     `json:"git,omitempty"`
}

type TaskContext struct {
	// GitCommit is shared across entire ResolutionContext and often updated. Thus must be a pointer.
	GitCommit  *GitCommitContext
	Invocation Invocation
}

type TaskExecutionContext struct {
	JobContext
	TaskContext
}

// WorkspaceResult captures the output of inline/detached workspace executions.
type GitCommitContext struct {
	PersistHash string `json:"commit"` // SHA-1 hash of created commit
	ParentHash  string `json:"parent"` // SHA-1 hash of parent commit
}
