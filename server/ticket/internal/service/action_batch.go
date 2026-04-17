package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/colony-2/c2j/pkg/core"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/internal/model"
	store "github.com/colony-2/colony2/server/ticket/internal/store/tickets"
	"gorm.io/plugin/optimisticlock"
)

func (s *service) ApplyActions(ctx context.Context, actions []model.Action, fallbackActor model.Actor) ([]model.ActionResult, error) {
	results := make([]model.ActionResult, 0, len(actions))
	err := s.store.WithTx(ctx, func(ctx context.Context, st store.Store) error {
		txSvc := *s
		txSvc.store = noNestedStore{Store: st}
		for _, action := range actions {
			actor, err := txSvc.resolveActionActor(action.ActorPayload(), fallbackActor)
			if err != nil {
				return err
			}
			result, err := txSvc.executeAction(ctx, action, actor)
			if err != nil {
				return err
			}
			results = append(results, result)
		}
		return nil
	})
	if err != nil {
		return results, err
	}
	return results, nil
}

type noNestedStore struct {
	store.Store
}

func (s noNestedStore) WithTx(ctx context.Context, fn func(ctx context.Context, st store.Store) error) error {
	return fn(ctx, s.Store)
}

func (s *service) executeAction(ctx context.Context, action model.Action, actor model.Actor) (model.ActionResult, error) {
	switch payload := action.(type) {
	case *model.CreateTicketAction:
		return s.handleCreateTicket(ctx, *payload, actor)
	case *model.UpdateTicketAction:
		return s.handleUpdateTicket(ctx, *payload, actor)
	case *model.AppendTicketNoteAction:
		return s.handleAppendTicketNote(ctx, *payload, actor)
	case *model.MarkdownRemoveAction:
		return s.handleMarkdownEvent(ctx, model.MarkdownDocRemoved, payload.BaseMarkdownAction, actor)
	case *model.MarkdownOverrideAction:
		return s.handleMarkdownEvent(ctx, model.MarkdownDocOverridden, payload.BaseMarkdownAction, actor)
	case *model.MarkdownLinkAction:
		return s.handleMarkdownEvent(ctx, model.MarkdownDocAttached, payload.BaseMarkdownAction, actor)
	case *model.AppendWorkflowAction:
		return s.handleWorkflowEvent(ctx, *payload, actor)
	case *model.ResetTicketAction:
		return s.handleResetTicket(ctx, *payload, actor)
	default:
		return nil, fmt.Errorf("unsupported action type %T", action)
	}
}

func (s *service) resolveActionActor(payload *model.ActorPayload, fallback model.Actor) (model.Actor, error) {
	if payload == nil {
		return fallback, nil
	}
	actorType := strings.ToLower(strings.TrimSpace(payload.Type))
	switch actorType {
	case "user":
		if payload.User == nil || strings.TrimSpace(payload.User.Email) == "" {
			return model.Actor{}, ErrInvalidActor
		}
		return NewUserActor(payload.User.Email), nil
	case "agent":
		if payload.Agent == nil {
			return model.Actor{}, ErrInvalidActor
		}
		agent := payload.Agent
		if strings.TrimSpace(agent.CellName) == "" || strings.TrimSpace(agent.WorkflowName) == "" || strings.TrimSpace(agent.ExecutionID) == "" || strings.TrimSpace(agent.InvocationHash) == "" {
			return model.Actor{}, ErrInvalidActor
		}
		return NewAgentActor(agent.CellName, agent.WorkflowName, agent.ExecutionID, agent.InvocationHash), nil
	default:
		return model.Actor{}, ErrInvalidActor
	}
}

func (s *service) handleCreateTicket(ctx context.Context, payload model.CreateTicketAction, actor model.Actor) (model.ActionResult, error) {
	state, err := normalizeStateValue(payload.State)
	if err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(payload.ProjectID)
	if projectID == "" {
		return nil, ErrInvalidProject
	}
	input := CreateInput{
		Cell:        core.CellName(strings.TrimSpace(payload.Cell)),
		ProjectID:   project.ID(projectID),
		Title:       strings.TrimSpace(payload.Title),
		Description: strings.TrimSpace(payload.Description),
		Stage:       normalizeStageValue(payload.Stage),
		State:       state,
		Actor:       actor,
	}
	if len(payload.DependsOnTicketIDs) > 0 {
		deps := make([]model.ID, 0, len(payload.DependsOnTicketIDs))
		for _, id := range payload.DependsOnTicketIDs {
			deps = append(deps, model.ID(id))
		}
		input.DependsOnTicketIDs = deps
	}
	created, jobID, err := s.CreateTicket(ctx, input)
	if err != nil {
		return nil, err
	}
	return &model.CreateResult{Ticket: *created, JobID: jobID}, nil
}

func (s *service) handleUpdateTicket(ctx context.Context, payload model.UpdateTicketAction, fallback model.Actor) (model.ActionResult, error) {
	input := UpdateInput{}
	input.ExpectedVersion = toVersion(payload.ExpectedVersion)
	var fields int
	if payload.Stage != nil {
		stage := normalizeStageValue(*payload.Stage)
		input.Stage = &stage
		fields++
	}
	if payload.State != nil {
		state, err := normalizeStateValue(*payload.State)
		if err != nil {
			return nil, err
		}
		input.State = &state
		fields++
	}
	if payload.Description != nil {
		desc := strings.TrimSpace(*payload.Description)
		input.Description = &desc
		fields++
	}
	if payload.Actor != nil {
		actor, err := s.resolveActionActor(payload.Actor, fallback)
		if err != nil {
			return nil, err
		}
		patchActor := toActorPatch(actor)
		input.Actor = &patchActor
		fields++
	}
	if fields == 0 {
		return nil, ErrUpdateNoFields
	}
	updated, err := s.UpdateTicket(ctx, payload.TicketID, input)
	if err != nil {
		return nil, err
	}
	return &model.UpdateResult{Ticket: updated}, nil
}

func (s *service) handleAppendTicketNote(ctx context.Context, payload model.AppendTicketNoteAction, actor model.Actor) (model.ActionResult, error) {
	eventTime := s.clock.Now()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := s.AppendTicketEvent(ctx, payload.TicketID, TicketEventInput{
		Actor:     actor,
		Notes:     strings.TrimSpace(payload.Note),
		EventTime: eventTime,
	})
	if err != nil {
		return nil, err
	}
	return &model.AppendTicketNoteResult{Event: event}, nil
}

func (s *service) handleMarkdownEvent(
	ctx context.Context,
	markdownEventType model.MarkdownDocEventType,
	payload model.BaseMarkdownAction,
	actor model.Actor,
) (model.ActionResult, error) {
	eventTime := s.clock.Now()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := s.AppendMarkdownEvent(ctx, payload.TicketID, MarkdownEventInput{
		Actor: actor,
		Payload: model.MarkdownDocEventPayload{
			Type: markdownEventType,
			Name: strings.TrimSpace(payload.Name),
			Path: strings.TrimSpace(payload.Path),
		},
		EventTime: eventTime,
	})
	if err != nil {
		return nil, err
	}
	return &model.MarkdownResult{Event: event}, nil
}

func (s *service) handleWorkflowEvent(ctx context.Context, payload model.AppendWorkflowAction, actor model.Actor) (model.ActionResult, error) {
	eventTime := s.clock.Now()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := s.AppendWorkflowEvent(ctx, payload.TicketID, WorkflowEventInput{
		Actor: actor,
		Payload: model.WorkflowEventPayload{
			Type:       payload.Status,
			WorkflowID: model.WorkflowID(strings.TrimSpace(payload.WorkflowID)),
			RunID:      model.WorkflowRunID(strings.TrimSpace(payload.RunID)),
		},
		EventTime: eventTime,
	})
	if err != nil {
		return nil, err
	}
	return &model.WorkflowResult{Event: event}, nil
}

func (s *service) handleResetTicket(ctx context.Context, payload model.ResetTicketAction, actor model.Actor) (model.ActionResult, error) {
	input := TicketResetInput{
		Actor:  actor,
		Reason: strings.TrimSpace(payload.Reason),
	}
	input.LastValidEvent = payload.AnchorEventID
	reset, err := s.ResetTicket(ctx, payload.TicketID, input)
	if err != nil {
		return nil, err
	}
	refreshed, err := s.GetTicketAt(ctx, payload.TicketID, s.clock.Now())
	if err != nil {
		return nil, err
	}
	return &model.ResetResult{Reset: reset, Ticket: refreshed}, nil
}

func normalizeStageValue(stage string) model.Stage {
	return normalizeStage(model.Stage(stage))
}

func normalizeStateValue(state string) (model.State, error) {
	normalized := model.State(strings.TrimSpace(state))
	if normalized == "" || !model.IsValidState(normalized) {
		return normalized, ErrInvalidState
	}
	return normalized, nil
}

func toVersion(v *int64) optimisticlock.Version {
	if v == nil {
		return optimisticlock.Version{}
	}
	return optimisticlock.Version{Int64: *v, Valid: true}
}

func toActorPatch(actor model.Actor) model.ActorPatch {
	switch actor.Type {
	case model.ActorTypeUser:
		return model.ActorPatch{Type: actor.Type, User: actor.User}
	case model.ActorTypeAgent:
		return model.ActorPatch{Type: actor.Type, Agent: actor.Agent}
	default:
		return model.ActorPatch{Type: actor.Type}
	}
}
