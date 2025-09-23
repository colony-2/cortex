package service

import "github.com/divisive-ai/vibethis/server/ticket/internal/model"

func NewUserActor(email string) model.Actor {
	return model.Actor{
		Type: model.ActorTypeUser,
		User: &model.ActorUser{Email: model.EmailAddress(email)},
	}
}

func NewAgentActor(cell, workflow, executionID, invocationHash string) model.Actor {
	return model.Actor{
		Type: model.ActorTypeAgent,
		Agent: &model.ActorAgent{
			CellName:       cell,
			WorkflowName:   workflow,
			ExecutionID:    executionID,
			InvocationHash: invocationHash,
		},
	}
}

func StagePtr(stage model.Stage) *model.Stage {
	s := stage
	return &s
}

func StatePtr(state model.State) *model.State {
	s := state
	return &s
}
