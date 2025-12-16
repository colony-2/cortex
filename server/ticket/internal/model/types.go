package model

import (
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/project/pkg/project"
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
	Type  ActorType   `json:"type"`
	User  *ActorUser  `json:"user,omitempty" gorm:"embedded;embeddedPrefix:actor_user_"`
	Agent *ActorAgent `json:"agent,omitempty" gorm:"embedded;embeddedPrefix:actor_agent_"`
}

type Ticket struct {
	ID          ID                     `json:"id" gorm:"column:id;type:char(27);index"`
	Version     optimisticlock.Version `json:"-" gorm:"column:version"`
	ProjectID   project.ID             `json:"project_id" gorm:"column:project_id;type:char(27);index;not null;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	CellID      cell.ID                `json:"cell_id" gorm:"column:cell_id;type:char(27);index;not null;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	CellName    core.CellName          `json:"cell_name" gorm:"column:cell_name;index;not null"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Stage       Stage                  `json:"stage"`
	State       State                  `json:"state"`
	Creator     Actor                  `json:"creator" gorm:"embedded;embeddedPrefix:creator_"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	ValidFrom   time.Time              `json:"valid_from" gorm:"primaryKey;type:timestamp"`
	ValidUntil  time.Time              `json:"valid_until" gorm:"type:timestamp"`
	LastResetID *TicketResetID         `json:"last_reset_id,omitempty" gorm:"-"`
	LastResetAt *time.Time             `json:"last_reset_at,omitempty" gorm:"-"`
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
