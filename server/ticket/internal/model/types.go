package model

import (
	"time"

	"github.com/colony-2/c2j/pkg/core"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/lib/pq"
	"gorm.io/plugin/optimisticlock"
)

type Stage string

type State string

type ID string

type EmailAddress string

type ActorType string

const (
	StageOpen      Stage = "open"
	StageCompleted Stage = "completed"
	StageCancelled Stage = "cancelled"

	StateWaitingUser       State = "waiting_user"
	StateWaitingDependency State = "waiting_dependency"
	StateWaitingCapacity   State = "waiting_capacity"
	StateWorking           State = "working"

	ActorTypeUser  ActorType = "user"
	ActorTypeAgent ActorType = "agent"

	CompletedStage Stage = StageCompleted
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
	ID                 ID                     `json:"id" gorm:"column:id;type:char(27);index;not null"`
	Version            optimisticlock.Version `json:"-" gorm:"column:version"`
	PrimaryJobID       *string                `json:"primary_job_id,omitempty" gorm:"column:primary_job_id;type:char(27);index"`
	DependsOnTicketIDs pq.StringArray         `json:"depends_on_ticket_ids,omitempty" gorm:"column:depends_on_ticket_ids;type:text[]"`
	ProjectID          project.ID             `json:"project_id" gorm:"column:project_id;type:char(27);index;not null;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	CellID             cell.ID                `json:"cell_id" gorm:"column:cell_id;type:char(27);index;not null;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
	CellName           core.CellName          `json:"cell_name" gorm:"column:cell_name;index;not null"`
	Title              string                 `json:"title"`
	Description        string                 `json:"description"`
	Stage              Stage                  `json:"stage"`
	State              State                  `json:"state"`
	Creator            Actor                  `json:"creator" gorm:"embedded;embeddedPrefix:creator_"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	CompletedAt        *time.Time             `json:"completed_at,omitempty"`
	ValidFrom          time.Time              `json:"valid_from" gorm:"primaryKey;type:timestamp"`
	ValidUntil         time.Time              `json:"valid_until" gorm:"type:timestamp;not null"`
	LastResetID        *TicketResetID         `json:"last_reset_id,omitempty" gorm:"column:last_reset_id;type:char(27);index"`
	LastResetAt        *time.Time             `json:"last_reset_at,omitempty" gorm:"column:last_reset_at"`
}

type ActorPatch struct {
	Type  ActorType
	User  *ActorUser
	Agent *ActorAgent
}

type ShortIDGenerator interface {
	NewID() (string, error)
}

var builtinStates = []State{
	StateWaitingUser,
	StateWaitingDependency,
	StateWaitingCapacity,
	StateWorking,
}

var builtinStages = []Stage{
	StageOpen,
	StageCompleted,
	StageCancelled,
}

func BuiltinStates() []State {
	result := make([]State, len(builtinStates))
	copy(result, builtinStates)
	return result
}

func BuiltinStages() []Stage {
	result := make([]Stage, len(builtinStages))
	copy(result, builtinStages)
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

func IsValidStage(stage Stage) bool {
	for _, s := range builtinStages {
		if s == stage {
			return true
		}
	}
	return false
}
