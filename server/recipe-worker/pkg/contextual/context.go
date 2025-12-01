package contextual

import (
	"github.com/colony-2/swf-go/pkg/swf"
)

// InvocationContext carries invocation-scoped identifiers shared across ops, gitstate, and compiler.
// All fields are JSON tagged for consistent serialization/deserialization at API and task boundaries.
type InvocationContext struct {
	InvocationID      string    `json:"invocation_id,omitempty"`
	InvocationHash    string    `json:"invocation_hash,omitempty"`
	InvocationAttempt int       `json:"invocation_attempt,omitempty"`
	ActivityID        string    `json:"activity_id,omitempty"`
	BoxID             string    `json:"box_id,omitempty"`
	NodePath          string    `json:"node_path,omitempty"`
	RecipeID          string    `json:"recipe_id,omitempty"`
	JobID             swf.JobId `json:"job_id,omitempty"`
}

// ActorContext represents the user/cell identity associated with an invocation.
type ActorContext struct {
	TicketID   string `json:"ticket_id,omitempty"`
	ActorName  string `json:"actor_name,omitempty"`
	ActorEmail string `json:"actor_email,omitempty"`
	CellName   string `json:"cell_name,omitempty"`
}

// EnvironmentContext captures filesystem and storage locations relevant to execution.
type EnvironmentContext struct {
	WorktreePath string `json:"worktree_path,omitempty"`
	BlobStoreURI string `json:"blob_store_uri,omitempty"`
	ThinPackPath string `json:"thin_pack_path,omitempty"`
}

// GitSnapshotContext captures immutable git state information at a point in time.
type GitSnapshotContext struct {
	BaseRepo          string `json:"base_repo,omitempty"`
	BaseHash          string `json:"base_hash,omitempty"`
	PersistHash       string `json:"persist_hash,omitempty"`
	PreviousHash      string `json:"previous_hash,omitempty"`
	WorkspacePrepared bool   `json:"workspace_prepared,omitempty"`
	GitAuthor         string `json:"git_author,omitempty"`
}

// WorkflowEnvelope provides high-level workflow/session identifiers.
type WorkflowEnvelope struct {
	JobID     swf.JobId `json:"job_id,omitempty"`
	Namespace string    `json:"namespace,omitempty"`
}
