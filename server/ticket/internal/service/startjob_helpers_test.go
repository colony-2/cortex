package service

import (
	"testing"

	"github.com/colony-2/colony2/server/ticket/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestActorToContextMap_User(t *testing.T) {
	actor := model.Actor{
		Type: model.ActorTypeUser,
		User: &model.ActorUser{Email: "user@example.com"},
	}

	result := actorToCreatorContext(actor)

	assert.Equal(t, "user", result.Type)
	if assert.NotNil(t, result.User) {
		assert.Equal(t, "user@example.com", result.User.Email)
	}
	assert.Nil(t, result.Agent)
}

func TestActorToContextMap_Agent(t *testing.T) {
	actor := model.Actor{
		Type: model.ActorTypeAgent,
		Agent: &model.ActorAgent{
			CellName:       "cell-1",
			WorkflowName:   "workflow",
			ExecutionID:    "exec-1",
			InvocationHash: "hash-1",
		},
	}

	result := actorToCreatorContext(actor)

	assert.Equal(t, "agent", result.Type)
	assert.Nil(t, result.User)
	if assert.NotNil(t, result.Agent) {
		assert.Equal(t, "cell-1", result.Agent.CellName)
		assert.Equal(t, "workflow", result.Agent.WorkflowName)
		assert.Equal(t, "exec-1", result.Agent.ExecutionID)
		assert.Equal(t, "hash-1", result.Agent.InvocationHash)
	}
}
