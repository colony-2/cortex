package service

import (
	"time"

	"github.com/colony-2/c2j/pkg/core"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/internal/model"
	"gorm.io/plugin/optimisticlock"
)

type CreateInput struct {
	Cell               core.CellName
	ProjectID          project.ID `validate:"required"`
	Title              string     `validate:"required"`
	Description        string
	Stage              model.Stage `validate:"required,stage"`
	State              model.State `validate:"required,state"`
	Actor              model.Actor `validate:"required"`
	DependsOnTicketIDs []model.ID
}

type UpdateInput struct {
	ExpectedVersion optimisticlock.Version `validate:"omitempty,version"`
	Stage           *model.Stage           `validate:"omitempty,stage"`
	State           *model.State           `validate:"omitempty,state"`
	CompletedAt     *time.Time
	Actor           *model.ActorPatch
	Description     *string
}

type WorkflowEventInput struct {
	Actor     model.Actor
	Payload   model.WorkflowEventPayload
	EventTime time.Time
}

type MarkdownEventInput struct {
	Actor     model.Actor
	Payload   model.MarkdownDocEventPayload
	EventTime time.Time
}

type ChangeSetEventInput struct {
	Actor     model.Actor
	Payload   model.ChangeSetEventPayload
	EventTime time.Time
}

type TicketEventInput struct {
	Actor     model.Actor `validate:"required"`
	Notes     string      `validate:"required"`
	EventTime time.Time
}

type TicketResetInput struct {
	Actor          model.Actor `validate:"required"`
	Reason         string      `validate:"required"`
	LastValidEvent *model.TicketEventID
}
