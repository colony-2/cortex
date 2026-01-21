package contextual

import "time"

// WorktreePathSentinel is a placeholder value used during template resolution
// Templates that reference {{ environment.worktree_path }} will resolve to this sentinel
// The actual worktree path is determined at execution time on the target machine
const WorktreePathSentinel = "__COLONY_WORKTREE_PATH__"

// ActorContext represents the user/cell identity associated with an invocation.
type ActorContext struct {
	TicketID   string `json:"ticket_id,omitempty"`
	ActorName  string `json:"actor_name,omitempty"`
	ActorEmail string `json:"actor_email,omitempty"`
}

// EnvironmentContext captures filesystem and storage locations relevant to execution.
type EnvironmentContext struct {
	WorktreePath string `json:"worktree_path,omitempty"`
	// ThinPackPath removed - no longer used (moved to artifact storage)
}

// GitBaseContext captures immutable git state information at a point in time.
type GitBaseContext struct {
	BaseRepo         string `json:"repo,omitempty"`
	BaseRef          string `json:"ref,omitempty"`
	ResolvedBaseHash string `json:"resolved_hash,omitempty"`
	GitAuthor        string `json:"author,omitempty"`
}

// WorkflowContext provides high-level workflow/session identifiers.
type WorkflowContext struct {
	CellName  string `json:"cell,omitempty"`
	CellPath  string `json:"cell_path,omitempty"` // Cell relative path from repo root
	JobID     string `json:"job_id,omitempty"`
	ProjectId string `json:"project_id,omitempty"`
}

type TicketCreatorUserContext struct {
	Email string `json:"email,omitempty"`
}

type TicketCreatorAgentContext struct {
	CellName       string `json:"cell,omitempty"`
	WorkflowName   string `json:"workflow_name,omitempty"`
	ExecutionID    string `json:"execution_id,omitempty"`
	InvocationHash string `json:"invocation_hash,omitempty"`
}

type TicketCreatorContext struct {
	Type  string                     `json:"type,omitempty"`
	User  *TicketCreatorUserContext  `json:"user,omitempty"`
	Agent *TicketCreatorAgentContext `json:"agent,omitempty"`
}

// TicketContext represents ticket metadata available to recipes.
type TicketContext struct {
	ID          string               `json:"id,omitempty"`
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	Creator     TicketCreatorContext `json:"creator,omitempty"`
	CreatedAt   time.Time            `json:"created_at,omitempty"`
	UpdatedAt   time.Time            `json:"updated_at,omitempty"`
}

// ExecutionContext holds typed workflow context available to templates. It is created per task.
type JobContext struct {
	Actor       ActorContext       `json:"actor,omitempty"`
	Ticket      TicketContext      `json:"ticket,omitempty"`
	Environment EnvironmentContext `json:"environment,omitempty"`
	Workflow    WorkflowContext    `json:"workflow,omitempty"`
	GitBase     GitBaseContext     `json:"git,omitempty"`
}

type TaskContext struct {
	// GitCommit is shared across entire ResolutionContext and often updated. Thus must be a pointer.
	GitCommit  *GitCommitContext
	Invocation Invocation
}

func NewTaskExecutionContext(ctx JobContext, ctx2 TaskContext) TaskExecutionContext {
	return TaskExecutionContext{
		Actor:       ctx.Actor,
		Ticket:      ctx.Ticket,
		Environment: ctx.Environment,
		Workflow:    ctx.Workflow,
		GitBase:     ctx.GitBase,
		GitCommit:   ctx2.GitCommit,
		Invocation:  ctx2.Invocation,
	}
}

type TaskExecutionContext struct {
	// embed these directly from task and job contexts for easier resolution.
	Actor       ActorContext       `json:"actor,omitempty"`
	Ticket      TicketContext      `json:"ticket,omitempty"`
	Environment EnvironmentContext `json:"environment,omitempty"`
	Workflow    WorkflowContext    `json:"workflow,omitempty"`
	GitBase     GitBaseContext     `json:"git,omitempty"`
	GitCommit   *GitCommitContext
	Invocation  Invocation
}

func (t TaskExecutionContext) TaskContext() TaskContext {
	return TaskContext{
		GitCommit:  t.GitCommit,
		Invocation: t.Invocation,
	}
}

func (t TaskExecutionContext) JobContext() JobContext {
	return JobContext{
		Actor:       t.Actor,
		Ticket:      t.Ticket,
		Environment: t.Environment,
		Workflow:    t.Workflow,
		GitBase:     t.GitBase,
	}
}

// WorkspaceResult captures the output of inline/detached workspace executions.
type GitCommitContext struct {
	ParentRef   string `json:"parent_ref,omitempty"`  // ref carrying workspace state until a hash exists
	PersistHash string `json:"hash,omitempty"`        // materialized SHA after a commit is created
	ParentHash  string `json:"parent_hash,omitempty"` // parent SHA once materialized
}
