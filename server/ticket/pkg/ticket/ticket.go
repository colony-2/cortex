package ticket

import (
	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	internalservice "github.com/divisive-ai/vibethis/server/ticket/internal/service"
	internalstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"gorm.io/gorm"
)

type (
	Stage           = model.Stage
	State           = model.State
	ID              = model.ID
	EmailAddress    = model.EmailAddress
	ActorType       = model.ActorType
	Actor           = model.Actor
	ActorUser       = model.ActorUser
	ActorAgent      = model.ActorAgent
	ActorPatch      = model.ActorPatch
	Ticket          = model.Ticket
	SearchFilter    = model.SearchFilter
	Service         = internalservice.Service
	ServiceConfig   = internalservice.ServiceConfig
	Clock           = internalservice.Clock
	CreateInput     = internalservice.CreateInput
	UpdateInput     = internalservice.UpdateInput
	Iterator[T any] = internalstore.Iterator[T]
	Store           = internalstore.Store
)

const (
	StateWaitingUser       = model.StateWaitingUser
	StateWaitingDependency = model.StateWaitingDependency
	StateWaitingCapacity   = model.StateWaitingCapacity
	StateWorking           = model.StateWorking
	ActorTypeUser          = model.ActorTypeUser
	ActorTypeAgent         = model.ActorTypeAgent
	CompletedStage         = model.CompletedStage
)

var (
	ErrInvalidState    = internalservice.ErrInvalidState
	ErrInvalidActor    = internalservice.ErrInvalidActor
	ErrEmptyTitle      = internalservice.ErrEmptyTitle
	ErrEmptyStage      = internalservice.ErrEmptyStage
	ErrIDGeneration    = internalservice.ErrIDGeneration
	ErrVersionConflict = internalservice.ErrVersionConflict
	ErrIteratorDone    = internalstore.ErrIteratorDone
)

func NewService(config ServiceConfig) (Service, error) {
	return internalservice.New(config)
}

func NewStore(db *gorm.DB) (Store, error) {
	return internalstore.New(db)
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
