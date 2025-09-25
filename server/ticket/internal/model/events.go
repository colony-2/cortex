package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type (
	WorkflowID      string
	WorkflowRunID   string
	TicketEventID   string
	TicketEventKind string
	TicketResetID   string
	TicketFieldName string
)

type TicketFieldChange struct {
	Field TicketFieldName `json:"field"`
	From  string          `json:"from,omitempty"`
	To    string          `json:"to,omitempty"`
}

const (
	TicketEventKindTicket      TicketEventKind = "ticket"
	TicketEventKindWorkflow    TicketEventKind = "workflow"
	TicketEventKindMarkdownDoc TicketEventKind = "markdown_doc"
	TicketEventKindChangeSet   TicketEventKind = "changeset"
)

type TicketEventPayloadType string

const (
	TicketEventPayloadTypeTicket      TicketEventPayloadType = "ticket"
	TicketEventPayloadTypeWorkflow    TicketEventPayloadType = "workflow"
	TicketEventPayloadTypeMarkdownDoc TicketEventPayloadType = "markdown_doc"
	TicketEventPayloadTypeChangeSet   TicketEventPayloadType = "changeset"
)

type TicketFieldChangeList []TicketFieldChange

func (l TicketFieldChangeList) Value() (driver.Value, error) {
	if l == nil {
		return nil, nil
	}
	data, err := json.Marshal([]TicketFieldChange(l))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (l *TicketFieldChangeList) Scan(value interface{}) error {
	if l == nil {
		return errors.New("ticket: nil TicketFieldChangeList receiver")
	}
	switch v := value.(type) {
	case nil:
		*l = nil
		return nil
	case []byte:
		if len(v) == 0 {
			*l = nil
			return nil
		}
		var decoded []TicketFieldChange
		if err := json.Unmarshal(v, &decoded); err != nil {
			return err
		}
		*l = TicketFieldChangeList(decoded)
		return nil
	case string:
		if v == "" {
			*l = nil
			return nil
		}
		var decoded []TicketFieldChange
		if err := json.Unmarshal([]byte(v), &decoded); err != nil {
			return err
		}
		*l = TicketFieldChangeList(decoded)
		return nil
	default:
		return fmt.Errorf("ticket: unsupported TicketFieldChangeList type %T", value)
	}
}

func (l TicketFieldChangeList) Clone() []TicketFieldChange {
	if len(l) == 0 {
		return nil
	}
	out := make([]TicketFieldChange, len(l))
	copy(out, l)
	return out
}

func cloneTicketFieldChanges(changes []TicketFieldChange) []TicketFieldChange {
	if len(changes) == 0 {
		return nil
	}
	out := make([]TicketFieldChange, len(changes))
	copy(out, changes)
	return out
}

type TicketEvent struct {
	ID          TicketEventID          `gorm:"primaryKey;type:char(26)"`
	TicketID    ID                     `gorm:"type:char(26);index"`
	Kind        TicketEventKind        `gorm:"type:text"`
	PayloadType TicketEventPayloadType `gorm:"column:payload_type;type:text;index"`
	Actor       Actor                  `gorm:"embedded;embeddedPrefix:actor_"`
	EventTime   time.Time              `gorm:"index"`
	CreatedAt   time.Time
	ResetID     *TicketResetID `gorm:"type:char(26);index"`

	TicketData    TicketEventPayload      `gorm:"embedded;embeddedPrefix:ticket_" json:"-"`
	TicketChanges TicketFieldChangeList   `gorm:"column:ticket_changes;type:jsonb" json:"-"`
	WorkflowData  WorkflowEventPayload    `gorm:"embedded;embeddedPrefix:workflow_" json:"-"`
	MarkdownData  MarkdownDocEventPayload `gorm:"embedded;embeddedPrefix:markdown_" json:"-"`
	ChangeSetData ChangeSetEventPayload   `gorm:"embedded;embeddedPrefix:changeset_" json:"-"`

	Payload TicketEventBody `gorm:"-"`
}

type TicketEventBody struct {
	Ticket      *TicketEventPayload      `json:"ticket,omitempty"`
	Workflow    *WorkflowEventPayload    `json:"workflow,omitempty"`
	MarkdownDoc *MarkdownDocEventPayload `json:"markdown_doc,omitempty"`
	ChangeSet   *ChangeSetEventPayload   `json:"changeset,omitempty"`
}

type TicketEventPayload struct {
	Changes []TicketFieldChange `json:"changes" gorm:"-"`
	Notes   string              `json:"notes,omitempty"`
}

type WorkflowEventType string

const (
	WorkflowEventRunning   WorkflowEventType = "running"
	WorkflowEventCompleted WorkflowEventType = "completed"
	WorkflowEventFailed    WorkflowEventType = "failed"
)

type WorkflowEventPayload struct {
	Type       WorkflowEventType `json:"type"`
	WorkflowID WorkflowID        `json:"workflow_id"`
	RunID      WorkflowRunID     `json:"run_id"`
}

type MarkdownDocEventType string

const (
	MarkdownDocAttached   MarkdownDocEventType = "attached"
	MarkdownDocOverridden MarkdownDocEventType = "overridden"
	MarkdownDocRemoved    MarkdownDocEventType = "removed"
)

type MarkdownDocEventPayload struct {
	Type MarkdownDocEventType `json:"type"`
	Name string               `json:"name"`
	Path string               `json:"path"`
}

type ChangeSetEventType string

const (
	ChangeSetAttached   ChangeSetEventType = "attached"
	ChangeSetOverridden ChangeSetEventType = "overridden"
	ChangeSetRemoved    ChangeSetEventType = "removed"
)

type ChangeSetEventPayload struct {
	Type          ChangeSetEventType `json:"type"`
	Path          string             `json:"path"`
	CommitMessage string             `json:"commit_message"`
	BaseGitHash   string             `json:"base_git_hash"`
	ParentGitHash string             `json:"parent_git_hash"`
	TipGitHash    string             `json:"tip_git_hash"`
}

type TicketReset struct {
	ID        TicketResetID `gorm:"primaryKey;type:char(26)"`
	TicketID  ID            `gorm:"type:char(26);index"`
	Actor     Actor         `gorm:"embedded;embeddedPrefix:actor_"`
	Reason    string        `gorm:"type:text"`
	CreatedAt time.Time
}

type TicketEventFilter struct {
	Kinds        []TicketEventKind
	PayloadTypes []TicketEventPayloadType
	Types        []string
	Since        *time.Time
	Until        *time.Time
	IncludeReset bool
}

func (TicketEvent) TableName() string { return "ticket_events" }

func (TicketReset) TableName() string { return "ticket_resets" }

func (e *TicketEvent) SetPayload(kind TicketEventKind, body TicketEventBody) {
	if e == nil {
		return
	}
	e.Kind = kind
	e.Payload = body
	e.PayloadType = payloadTypeFromKind(kind)

	e.TicketData = TicketEventPayload{}
	e.TicketChanges = nil
	e.WorkflowData = WorkflowEventPayload{}
	e.MarkdownData = MarkdownDocEventPayload{}
	e.ChangeSetData = ChangeSetEventPayload{}

	switch e.PayloadType {
	case TicketEventPayloadTypeTicket:
		if body.Ticket != nil {
			e.TicketData.Notes = body.Ticket.Notes
			e.TicketChanges = TicketFieldChangeList(cloneTicketFieldChanges(body.Ticket.Changes))
		}
	case TicketEventPayloadTypeWorkflow:
		if body.Workflow != nil {
			e.WorkflowData = *body.Workflow
		}
	case TicketEventPayloadTypeMarkdownDoc:
		if body.MarkdownDoc != nil {
			e.MarkdownData = *body.MarkdownDoc
		}
	case TicketEventPayloadTypeChangeSet:
		if body.ChangeSet != nil {
			e.ChangeSetData = *body.ChangeSet
		}
	}

	e.HydratePayload()
}

func (e *TicketEvent) HydratePayload() {
	if e == nil {
		return
	}
	ptype := e.PayloadType
	if ptype == "" {
		ptype = payloadTypeFromKind(e.Kind)
	}
	e.PayloadType = ptype
	e.Payload = TicketEventBody{}
	switch ptype {
	case TicketEventPayloadTypeTicket:
		payload := &TicketEventPayload{
			Notes: e.TicketData.Notes,
		}
		if len(e.TicketChanges) > 0 {
			payload.Changes = cloneTicketFieldChanges([]TicketFieldChange(e.TicketChanges))
		}
		e.Payload.Ticket = payload
	case TicketEventPayloadTypeWorkflow:
		if e.WorkflowData.Type != "" || e.WorkflowData.WorkflowID != "" || e.WorkflowData.RunID != "" {
			payload := e.WorkflowData
			e.Payload.Workflow = &payload
		}
	case TicketEventPayloadTypeMarkdownDoc:
		if e.MarkdownData.Type != "" || e.MarkdownData.Name != "" || e.MarkdownData.Path != "" {
			payload := e.MarkdownData
			e.Payload.MarkdownDoc = &payload
		}
	case TicketEventPayloadTypeChangeSet:
		if e.ChangeSetData.Type != "" || e.ChangeSetData.Path != "" || e.ChangeSetData.CommitMessage != "" || e.ChangeSetData.BaseGitHash != "" || e.ChangeSetData.ParentGitHash != "" || e.ChangeSetData.TipGitHash != "" {
			payload := e.ChangeSetData
			e.Payload.ChangeSet = &payload
		}
	}
}

func payloadTypeFromKind(kind TicketEventKind) TicketEventPayloadType {
	switch kind {
	case TicketEventKindTicket:
		return TicketEventPayloadTypeTicket
	case TicketEventKindWorkflow:
		return TicketEventPayloadTypeWorkflow
	case TicketEventKindMarkdownDoc:
		return TicketEventPayloadTypeMarkdownDoc
	case TicketEventKindChangeSet:
		return TicketEventPayloadTypeChangeSet
	default:
		return TicketEventPayloadType(kind)
	}
}
