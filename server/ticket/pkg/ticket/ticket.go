package ticket

import (
	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	internalservice "github.com/divisive-ai/vibethis/server/ticket/internal/service"
	eventstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/events"
	internalstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"gorm.io/gorm"
)

type (
	Stage                   = model.Stage
	State                   = model.State
	ID                      = model.ID
	EmailAddress            = model.EmailAddress
	ActorType               = model.ActorType
	Actor                   = model.Actor
	ActorUser               = model.ActorUser
	ActorAgent              = model.ActorAgent
	ActorPatch              = model.ActorPatch
	TicketFieldName         = model.TicketFieldName
	TicketFieldChange       = model.TicketFieldChange
	Ticket                  = model.Ticket
	TicketEventID           = model.TicketEventID
	TicketEventKind         = model.TicketEventKind
	TicketEventPayloadType  = model.TicketEventPayloadType
	TicketEvent             = model.TicketEvent
	TicketEventBody         = model.TicketEventBody
	TicketEventPayload      = model.TicketEventPayload
	WorkflowID              = model.WorkflowID
	WorkflowRunID           = model.WorkflowRunID
	WorkflowEventType       = model.WorkflowEventType
	WorkflowEventPayload    = model.WorkflowEventPayload
	MarkdownDocEventType    = model.MarkdownDocEventType
	MarkdownDocEventPayload = model.MarkdownDocEventPayload
	ChangeSetEventType      = model.ChangeSetEventType
	ChangeSetEventPayload   = model.ChangeSetEventPayload
	TicketReset             = model.TicketReset
	TicketEventFilter       = model.TicketEventFilter
	SearchFilter            = model.SearchFilter
	Service                 = internalservice.Service
	ServiceConfig           = internalservice.ServiceConfig
	Clock                   = internalservice.Clock
	CreateInput             = internalservice.CreateInput
	UpdateInput             = internalservice.UpdateInput
	TicketEventInput        = internalservice.TicketEventInput
	TicketResetInput        = internalservice.TicketResetInput
	Iterator[T any]         = internalstore.Iterator[T]
	Store                   = internalstore.Store
	EventStore              = eventstore.Store
)

const (
	StateWaitingUser                  = model.StateWaitingUser
	StateWaitingDependency            = model.StateWaitingDependency
	StateWaitingCapacity              = model.StateWaitingCapacity
	StateWorking                      = model.StateWorking
	ActorTypeUser                     = model.ActorTypeUser
	ActorTypeAgent                    = model.ActorTypeAgent
	CompletedStage                    = model.CompletedStage
	TicketEventKindTicket             = model.TicketEventKindTicket
	TicketEventKindWorkflow           = model.TicketEventKindWorkflow
	TicketEventKindMarkdownDoc        = model.TicketEventKindMarkdownDoc
	TicketEventKindChangeSet          = model.TicketEventKindChangeSet
	TicketEventPayloadTypeTicket      = model.TicketEventPayloadTypeTicket
	TicketEventPayloadTypeWorkflow    = model.TicketEventPayloadTypeWorkflow
	TicketEventPayloadTypeMarkdownDoc = model.TicketEventPayloadTypeMarkdownDoc
	TicketEventPayloadTypeChangeSet   = model.TicketEventPayloadTypeChangeSet
	WorkflowEventRunning              = model.WorkflowEventRunning
	WorkflowEventCompleted            = model.WorkflowEventCompleted
	WorkflowEventFailed               = model.WorkflowEventFailed
	MarkdownDocAttached               = model.MarkdownDocAttached
	MarkdownDocOverridden             = model.MarkdownDocOverridden
	MarkdownDocRemoved                = model.MarkdownDocRemoved
	ChangeSetAttached                 = model.ChangeSetAttached
	ChangeSetOverridden               = model.ChangeSetOverridden
	ChangeSetRemoved                  = model.ChangeSetRemoved
)

var (
	ErrInvalidState        = internalservice.ErrInvalidState
	ErrInvalidActor        = internalservice.ErrInvalidActor
	ErrEmptyTitle          = internalservice.ErrEmptyTitle
	ErrEmptyStage          = internalservice.ErrEmptyStage
	ErrIDGeneration        = internalservice.ErrIDGeneration
	ErrVersionConflict     = internalservice.ErrVersionConflict
	ErrInvalidEventKind    = internalservice.ErrInvalidEventKind
	ErrInvalidEventPayload = internalservice.ErrInvalidEventPayload
	ErrEventNotFound       = internalservice.ErrEventNotFound
	ErrResetNoEvents       = internalservice.ErrResetNoEvents
	ErrIteratorDone        = internalstore.ErrIteratorDone
)

func NewService(config ServiceConfig) (Service, error) {
	return internalservice.New(config)
}

func NewStore(db *gorm.DB) (Store, error) {
	return internalstore.New(db)
}

func NewEventStore(db *gorm.DB) (EventStore, error) {
	return eventstore.New(db)
}

func NewUserActor(email string) Actor {
	return internalservice.NewUserActor(email)
}

func NewAgentActor(cell, workflow, executionID, invocationHash string) Actor {
	return internalservice.NewAgentActor(cell, workflow, executionID, invocationHash)
}

func StagePtr(stage Stage) *Stage {
	return internalservice.StagePtr(stage)
}

func StatePtr(state State) *State {
	return internalservice.StatePtr(state)
}

func BuiltinStates() []State {
	return model.BuiltinStates()
}

type ShortIDGenerator = model.ShortIDGenerator
