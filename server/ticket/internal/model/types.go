package model

import (
	"time"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"gorm.io/plugin/optimisticlock"
)

type Stage string

type State string

type ID string

type EmailAddress string

type ActorType string

const (
	StateWaitingUser       State = "waiting_user"
	StateWaitingDependency State = "waiting_dependency"
	StateWaitingCapacity   State = "waiting_capacity"
	StateWorking           State = "working"

	ActorTypeUser  ActorType = "user"
	ActorTypeAgent ActorType = "agent"

	CompletedStage Stage = "__completed__"
)

type ActorUser struct {
	Email EmailAddress `json:"email"`
}

type ActorAgent struct {
	CellName       string `json:"cell"`
	WorkflowName   string `json:"workflow_name"`
	ExecutionID    string `json:"execution_id"`
	InvocationHash string `json:"invocation_hash"`
}

type Actor struct {
	Type  ActorType
	User  *ActorUser  `gorm:"embedded;embeddedPrefix:actor_user_"`
	Agent *ActorAgent `gorm:"embedded;embeddedPrefix:actor_agent_"`
}

type Ticket struct {
	ID          ID                     `gorm:"type:char(26);index"`
	Version     optimisticlock.Version `gorm:"column:version"`
	CellName    core.CellName
	Title       string
	Description string
	Stage       Stage
	State       State
	Creator     Actor `gorm:"embedded;embeddedPrefix:creator_"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	ValidFrom   time.Time `gorm:"primaryKey;type:timestamp"`
	ValidUntil  time.Time `gorm:"type:timestamp"`
}

type ActorPatch struct {
	Type  ActorType
	User  *ActorUser
	Agent *ActorAgent
}

type ShortIDGenerator interface {
	NewID() (string, error)
}

func (Ticket) TableName() string { return "tickets" }

var builtinStates = []State{
	StateWaitingUser,
	StateWaitingDependency,
	StateWaitingCapacity,
	StateWorking,
}

func BuiltinStates() []State {
	result := make([]State, len(builtinStates))
	copy(result, builtinStates)
	return result
}

func IsValidState(state State) bool {
	for _, s := range builtinStates {
		if s == state {
			return true
		}
	}
	return false
}
