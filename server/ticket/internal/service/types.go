package service

import (
	"time"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	"gorm.io/plugin/optimisticlock"
)

type CreateInput struct {
	Cell        core.CellName
	Title       string `validate:"required"`
	Description string
	Stage       model.Stage `validate:"required,stage"`
	State       model.State `validate:"required,state"`
	Actor       model.Actor `validate:"required"`
}

type UpdateInput struct {
	ExpectedVersion optimisticlock.Version `validate:"version"`
	Stage           *model.Stage           `validate:"omitempty,stage"`
	State           *model.State           `validate:"omitempty,state"`
	CompletedAt     *time.Time
	Actor           *model.ActorPatch
}

type TicketEventInput struct {
	Kind      model.TicketEventKind `validate:"required"`
	Actor     model.Actor           `validate:"required"`
	Payload   model.TicketEventBody
	EventTime time.Time
}

type TicketResetInput struct {
	Actor          model.Actor `validate:"required"`
	Reason         string      `validate:"required"`
	LastValidEvent *model.TicketEventID
}
