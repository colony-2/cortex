package ticketop

import (
	"time"

	"github.com/colony-2/colony2/server/ticket/internal/model"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
)

type ActionType string

const (
	ActionCreateTicket     ActionType = "create_ticket"
	ActionUpdateTicket     ActionType = "update_ticket"
	ActionAppendTicketNote ActionType = "append_ticket_note"
	ActionLinkMarkdown     ActionType = "link_markdown_doc"
	ActionOverrideMarkdown ActionType = "override_markdown_doc"
	ActionRemoveMarkdown   ActionType = "remove_markdown_doc"
	ActionAppendWorkflow   ActionType = "append_workflow_event"
	ActionResetTicket      ActionType = "reset_ticket"
)

type Action interface {
	isAction()
	ActorPayload() *ActorPayload
}

var _ Action = BaseAction{}

type BaseAction struct {
	Actor *ActorPayload `json:"actor,omitempty" yaml:"actor,omitempty"`
}

func (a BaseAction) ActorPayload() *ActorPayload {
	return a.Actor
}

func (BaseAction) isAction() {}

type CreateTicketAction struct {
	BaseAction
	Cell        string `json:"cell" yaml:"cell" validate:"required"`
	ProjectID   string `json:"project_id" yaml:"project_id" validate:"required"`
	Title       string `json:"title" yaml:"title" validate:"required"`
	Stage       string `json:"stage" yaml:"stage" validate:"required"`
	State       string `json:"state" yaml:"state" validate:"required"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type UpdateTicketAction struct {
	BaseAction
	existingTicketOp
	ExpectedVersion *int64  `json:"expected_version,omitempty" yaml:"expected_version,omitempty" validate:"omitempty,gt=0"`
	Stage           *string `json:"stage,omitempty" yaml:"stage,omitempty"`
	State           *string `json:"state,omitempty" yaml:"state,omitempty"`
	Description     *string `json:"description,omitempty" yaml:"description,omitempty"`
}

type AppendTicketNoteAction struct {
	BaseAction
	existingTicketOp
	Note      string     `json:"note" yaml:"note" validate:"required"`
	EventTime *time.Time `json:"event_time,omitempty" yaml:"event_time,omitempty"`
}

type BaseMarkdownAction struct {
	BaseAction
	existingTicketOp
	Name      string     `json:"name" yaml:"name" validate:"required"`
	Path      string     `json:"path" yaml:"path" validate:"required"`
	Reason    string     `json:"reason,omitempty" yaml:"reason,omitempty"`
	EventTime *time.Time `json:"event_time,omitempty" yaml:"event_time,omitempty"`
}

type existingTicketOp struct {
	TicketID model.ID `json:"ticket_id" validate:"required"`
}

type MarkdownLinkAction struct {
	BaseMarkdownAction `json:",inline"`
}
type MarkdownOverrideAction struct {
	BaseMarkdownAction `json:",inline"`
}
type MarkdownRemoveAction struct {
	BaseMarkdownAction `json:",inline"`
}

type AppendWorkflowAction struct {
	BaseAction
	existingTicketOp
	WorkflowID string                   `json:"workflow_id" yaml:"workflow_id" validate:"required"`
	RunID      string                   `json:"run_id" yaml:"run_id" validate:"required"`
	Status     ticket.WorkflowEventType `json:"status" yaml:"status" validate:"required"`
	EventTime  *time.Time               `json:"event_time,omitempty" yaml:"event_time,omitempty"`
}

type ResetTicketAction struct {
	BaseAction
	existingTicketOp
	Reason        string                `json:"reason" yaml:"reason" validate:"required"`
	AnchorEventID *ticket.TicketEventID `json:"anchor_event_id,omitempty" yaml:"anchor_event_id,omitempty"`
}

// ActorPayload represents action-level actor configuration.
type ActorPayload struct {
	Type  string             `json:"type" yaml:"type"`
	User  *ActorUserPayload  `json:"user,omitempty" yaml:"user,omitempty"`
	Agent *ActorAgentPayload `json:"agent,omitempty" yaml:"agent,omitempty"`
}

type ActorUserPayload struct {
	Email string `json:"email" yaml:"email"`
}

type ActorAgentPayload struct {
	CellName       string `json:"cell" yaml:"cell"`
	WorkflowName   string `json:"workflow_name" yaml:"workflow_name"`
	ExecutionID    string `json:"execution_id" yaml:"execution_id"`
	InvocationHash string `json:"invocation_hash" yaml:"invocation_hash"`
}
