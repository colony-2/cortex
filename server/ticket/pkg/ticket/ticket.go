package ticket

import (
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/internal/model"
	internalservice "github.com/colony-2/colony2/server/ticket/internal/service"
	eventstore "github.com/colony-2/colony2/server/ticket/internal/store/events"
	internalstore "github.com/colony-2/colony2/server/ticket/internal/store/tickets"
	"gorm.io/gorm"
)

type (
	Stage                   = model.Stage
	State                   = model.State
	ID                      = model.ID
	ProjectID               = project.ID
	CellID                  = cell.ID
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
	TicketEventInput        = internalservice.TicketEventInput
	WorkflowID              = model.WorkflowID
	WorkflowRunID           = model.WorkflowRunID
	WorkflowEventType       = model.WorkflowEventType
	WorkflowEventPayload    = model.WorkflowEventPayload
	MarkdownDocEventType    = model.MarkdownDocEventType
	MarkdownDocEventPayload = model.MarkdownDocEventPayload
	ChangeSetEventType      = model.ChangeSetEventType
	ChangeSetEventPayload   = model.ChangeSetEventPayload
	TicketReset             = model.TicketReset
	TicketResetID           = model.TicketResetID
	TicketResetEventPayload = model.TicketResetEventPayload
	TicketEventFilter       = model.TicketEventFilter
	SearchFilter            = model.SearchFilter
	Action                  = model.Action
	ActionResult            = model.ActionResult
	ActionType              = model.ActionType
	BaseAction              = model.BaseAction
	CreateTicketAction      = model.CreateTicketAction
	UpdateTicketAction      = model.UpdateTicketAction
	AppendTicketNoteAction  = model.AppendTicketNoteAction
	BaseMarkdownAction      = model.BaseMarkdownAction
	MarkdownLinkAction      = model.MarkdownLinkAction
	MarkdownOverrideAction  = model.MarkdownOverrideAction
	MarkdownRemoveAction    = model.MarkdownRemoveAction
	AppendWorkflowAction    = model.AppendWorkflowAction
	ResetTicketAction       = model.ResetTicketAction
	Service                 = internalservice.Service
	ServiceConfig           = internalservice.ServiceConfig
	Clock                   = internalservice.Clock
	CreateInput             = internalservice.CreateInput
	UpdateInput             = internalservice.UpdateInput
	WorkflowEventInput      = internalservice.WorkflowEventInput
	MarkdownEventInput      = internalservice.MarkdownEventInput
	ChangeSetEventInput     = internalservice.ChangeSetEventInput
	TicketResetInput        = internalservice.TicketResetInput
	Iterator[T any]         = internalstore.Iterator[T]
	Store                   = internalstore.Store
	EventStore              = eventstore.Store
	StoreOptions            = internalstore.Options
	EventStoreOptions       = eventstore.Options
	RecipeProjectProvider   = internalservice.RecipeProjectProvider
)

const (
	StageOpen                         = model.StageOpen
	StageCompleted                    = model.StageCompleted
	StageCancelled                    = model.StageCancelled
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
	TicketEventKindReset              = model.TicketEventKindReset
	TicketEventPayloadTypeTicket      = model.TicketEventPayloadTypeTicket
	TicketEventPayloadTypeWorkflow    = model.TicketEventPayloadTypeWorkflow
	TicketEventPayloadTypeMarkdownDoc = model.TicketEventPayloadTypeMarkdownDoc
	TicketEventPayloadTypeChangeSet   = model.TicketEventPayloadTypeChangeSet
	TicketEventPayloadTypeReset       = model.TicketEventPayloadTypeReset
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
	ErrInvalidProject      = internalservice.ErrInvalidProject
	ErrInvalidCell         = internalservice.ErrInvalidCell
	ErrIDGeneration        = internalservice.ErrIDGeneration
	ErrVersionConflict     = internalservice.ErrVersionConflict
	ErrInvalidEventKind    = internalservice.ErrInvalidEventKind
	ErrInvalidEventPayload = internalservice.ErrInvalidEventPayload
	ErrEventNotFound       = internalservice.ErrEventNotFound
	ErrResetNoEvents       = internalservice.ErrResetNoEvents
	ErrUpdateNoFields      = internalservice.ErrUpdateNoFields
	ErrIteratorDone        = internalstore.ErrIteratorDone
)

func NewService(config ServiceConfig) (Service, error) {
	return internalservice.New(config)
}

func NewServiceFromDB(db *gorm.DB) (Service, error) {
	projectStore, err := project.NewStore(db)
	if err != nil {
		return nil, err
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		return nil, err
	}
	cellStore, err := cell.NewStore(db)
	if err != nil {
		return nil, err
	}
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	if err != nil {
		return nil, err
	}
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	eventStore, err := NewEventStore(db)
	if err != nil {
		return nil, err
	}
	return NewService(ServiceConfig{Store: store, EventStore: eventStore, Projects: projectSvc, Cells: cellSvc})
}

func NewStore(db *gorm.DB) (Store, error) {
	return internalstore.New(db)
}

func NewStoreWithOptions(db *gorm.DB, opts StoreOptions) (Store, error) {
	return internalstore.NewWithOptions(db, opts)
}

func NewEventStore(db *gorm.DB) (EventStore, error) {
	return eventstore.New(db)
}

func NewEventStoreWithOptions(db *gorm.DB, opts EventStoreOptions) (EventStore, error) {
	return eventstore.NewWithOptions(db, opts)
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

func BuiltinStages() []Stage {
	return model.BuiltinStages()
}

type ShortIDGenerator = model.ShortIDGenerator
