package ticketop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	yamlv3 "gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/plugin/optimisticlock"
)

const opName = "ticket.manage"

var (
	errMissingActions         = errors.New("ticket.manage: actions must not be empty")
	errTicketIDRequired       = errors.New("ticket.manage: ticket id is required")
	errCreateWithTicketID     = errors.New("ticket.manage: create_ticket must not include ticket id")
	errUpdateNoFields         = errors.New("ticket.manage: update_ticket requires at least one field to modify")
	errInvalidExpectedVersion = errors.New("ticket.manage: update_ticket expected_version must be greater than 0")
)

type (
	Input struct {
		Actions ActionList `json:"actions" yaml:"actions" validate:"required,min=1,dive"`
	}

	Output struct {
		Results []ActionResult `json:"results"`
	}
)
type ActionList []Action

func (l *ActionList) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	actions := make([]Action, 0, len(raw))
	for _, item := range raw {
		aux := struct {
			Type ActionType `json:"type"`
		}{}
		if err := json.Unmarshal(item, &aux); err != nil {
			return err
		}
		if aux.Type == "" {
			return fmt.Errorf("action type is required")
		}
		action, err := unmarshalActionPayload(aux.Type, func(v any) error {
			return json.Unmarshal(item, v)
		})
		if err != nil {
			return err
		}
		actions = append(actions, action)
	}
	*l = actions
	return nil
}

func (l *ActionList) UnmarshalYAML(node *yamlv3.Node) error {
	if node.Kind != yamlv3.SequenceNode {
		return fmt.Errorf("actions must be a sequence")
	}
	actions := make([]Action, 0, len(node.Content))
	for _, item := range node.Content {
		aux := struct {
			Type ActionType `yaml:"type"`
		}{}
		if err := item.Decode(&aux); err != nil {
			return err
		}
		if aux.Type == "" {
			return fmt.Errorf("action type is required")
		}
		action, err := unmarshalActionPayload(aux.Type, func(v any) error {
			return item.Decode(v)
		})
		if err != nil {
			return err
		}
		actions = append(actions, action)
	}
	*l = actions
	return nil
}

func unmarshalActionPayload(actionType ActionType, decode func(any) error) (Action, error) {
	switch actionType {
	case ActionCreateTicket:
		return decodeToType[CreateTicketAction](decode)
	case ActionUpdateTicket:
		return decodeToType[UpdateTicketAction](decode)
	case ActionAppendTicketNote:
		return decodeToType[AppendTicketNoteAction](decode)
	case ActionLinkMarkdown:
		return decodeToType[MarkdownLinkAction](decode)
	case ActionOverrideMarkdown:
		return decodeToType[MarkdownOverrideAction](decode)
	case ActionRemoveMarkdown:
		return decodeToType[MarkdownRemoveAction](decode)
	case ActionAppendWorkflow:
		return decodeToType[AppendWorkflowAction](decode)
	case ActionResetTicket:
		return decodeToType[ResetTicketAction](decode)
	default:
		return nil, fmt.Errorf("unsupported action type %q", actionType)
	}
}

func decodeToType[T any, P interface {
	*T
	Action
}](decode func(any) error) (P, error) {
	var payload T
	p := P(&payload)
	if err := decode(p); err != nil {
		return nil, err
	}
	return p, nil
}

var (
	serviceFactoryMu sync.RWMutex
	serviceFactory   = defaultServiceFactory
)

type serviceFactoryFunc func(inv ops.OpDependencies, db *gorm.DB) (ticket.Service, error)

func defaultServiceFactory(_ ops.OpDependencies, db *gorm.DB) (ticket.Service, error) {
	if db == nil {
		panic("ticketop: database dependency is nil")
	}
	return ticket.NewServiceFromDB(db)
}

// TestingStub allows tests to swap the service factory with a stub implementation.
type TestingStub struct {
	Service ticket.Service
	Err     error
}

func (s TestingStub) Install() func() {
	serviceFactoryMu.Lock()
	previous := serviceFactory
	serviceFactory = func(inv ops.OpDependencies, db *gorm.DB) (ticket.Service, error) {
		if s.Service != nil {
			return s.Service, s.Err
		}
		if s.Err != nil {
			return nil, s.Err
		}
		return previous(inv, db)
	}
	serviceFactoryMu.Unlock()
	return func() {
		serviceFactoryMu.Lock()
		serviceFactory = previous
		serviceFactoryMu.Unlock()
	}
}

func GetOp() ops.RegisterableOp {
	return ops.NewOp().
		WithDefaultTimeout(2*time.Minute).
		WithType(opName).
		AddStep(opName, ops.NewStepWithDeps(execute)).
		BuildOrPanic()
}

func execute(inv ops.OpDependencies, ctx context.Context, input Input) (Output, error) {
	if len(input.Actions) == 0 {
		return Output{}, mapError(errMissingActions, nil)
	}

	svc, err := resolveService(inv)
	if err != nil {
		return Output{}, err
	}

	fallbackActor := defaultAutomationActor(inv)
	batchStart := time.Now()

	results := make([]ActionResult, 0, len(input.Actions))

	for _, action := range input.Actions {
		actor, err := resolveActor(action.ActorPayload(), fallbackActor)
		if err != nil {
			return Output{}, mapError(err, results)
		}
		result, err := executeAction(ctx, svc, action, actor)
		if err != nil {
			wrapped := mapError(err, results)
			return Output{}, wrapped
		}
		results = append(results, result)
	}

	output := Output{Results: results}
	log.Printf("ticketop: batch actions=%d status=success duration_ms=%d", len(input.Actions), time.Since(batchStart).Milliseconds())
	return output, nil
}

func resolveService(inv ops.OpDependencies) (ticket.Service, error) {
	var db *gorm.DB
	if inv != nil {
		db = inv.Database()
		if db == nil {
			return nil, workflow.NewNonRetryableApplicationError("ticket.manage: database dependency not configured")
		}
	} else {
		return nil, workflow.NewNonRetryableApplicationError("ticket.manage: dependency container missing")
	}
	serviceFactoryMu.RLock()
	factory := serviceFactory
	serviceFactoryMu.RUnlock()
	svc, err := factory(inv, db)
	if err != nil {
		return nil, err
	}
	if svc == nil {
		return nil, fmt.Errorf("ticket.manage: service factory returned nil service")
	}
	return svc, nil
}

func mapError(err error, partial []ActionResult) error {
	if err == nil {
		return nil
	}
	detail := Output{Results: partial}

	switch {
	case errors.Is(err, errMissingActions), errors.Is(err, errCreateWithTicketID), errors.Is(err, errTicketIDRequired), errors.Is(err, errUpdateNoFields), errors.Is(err, errInvalidExpectedVersion):
		return workflow.NewNonRetryableApplicationError(err.Error(), err, detail)
	case errors.Is(err, ticket.ErrVersionConflict):
		return workflow.NewNonRetryableApplicationError(err.Error(), err, detail)
	case errors.Is(err, ticket.ErrInvalidState), errors.Is(err, ticket.ErrInvalidActor), errors.Is(err, ticket.ErrEmptyTitle), errors.Is(err, ticket.ErrEmptyStage):
		return workflow.NewNonRetryableApplicationError(err.Error(), "BAD_REQUEST", err, detail)
	case errors.Is(err, ticket.ErrResetNoEvents):
		return workflow.NewNonRetryableApplicationError(err.Error(), "RESET_NOT_ALLOWED", err, detail)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return workflow.NewApplicationError(err.Error(), "TICKET_MANAGE_FAILED", err, detail)
	}
}

func executeAction(
	ctx context.Context,
	svc ticket.Service,
	action Action,
	actor ticket.Actor,
) (ActionResult, error) {

	switch payload := action.(type) {
	case *CreateTicketAction:
		return handleCreateTicket(ctx, svc, *payload, actor)
	case *UpdateTicketAction:
		return handleUpdateTicket(ctx, svc, *payload, actor)
	case *AppendTicketNoteAction:
		return handleAppendTicketNote(ctx, svc, *payload, actor)
	case *MarkdownRemoveAction:
		return handleMarkdownRemoveAction(ctx, svc, *payload, actor)
	case *MarkdownOverrideAction:
		return handleMarkdownOverrideAction(ctx, svc, *payload, actor)
	case *MarkdownLinkAction:
		return handleMarkdownLinkAction(ctx, svc, *payload, actor)
	case *AppendWorkflowAction:
		return handleWorkflowEvent(ctx, svc, *payload, actor)
	case *ResetTicketAction:
		return handleResetTicket(ctx, svc, *payload, actor)
	default:
		return nil, fmt.Errorf("unsupported action type %T", action)
	}
}

func defaultAutomationActor(inv ops.OpDependencies) ticket.Actor {
	return ticket.NewAgentActor("unknown", "unknown", "unknown", "unknown")
}

func resolveActor(payload *ActorPayload, fallback ticket.Actor) (ticket.Actor, error) {
	if payload == nil {
		return fallback, nil
	}
	actorType := strings.ToLower(strings.TrimSpace(payload.Type))
	switch actorType {
	case "user":
		if payload.User == nil || strings.TrimSpace(payload.User.Email) == "" {
			return ticket.Actor{}, ticket.ErrInvalidActor
		}
		return ticket.NewUserActor(payload.User.Email), nil
	case "agent":
		if payload.Agent == nil {
			return ticket.Actor{}, ticket.ErrInvalidActor
		}
		agent := payload.Agent
		if strings.TrimSpace(agent.CellName) == "" || strings.TrimSpace(agent.WorkflowName) == "" || strings.TrimSpace(agent.ExecutionID) == "" || strings.TrimSpace(agent.InvocationHash) == "" {
			return ticket.Actor{}, ticket.ErrInvalidActor
		}
		return ticket.NewAgentActor(agent.CellName, agent.WorkflowName, agent.ExecutionID, agent.InvocationHash), nil
	default:
		return ticket.Actor{}, ticket.ErrInvalidActor
	}
}

func normalizeStageValue(stage string) ticket.Stage {
	return ticket.Stage(strings.ToLower(strings.TrimSpace(stage)))
}

func normalizeStateValue(state string) (ticket.State, error) {
	normalized := ticket.State(strings.TrimSpace(state))
	if normalized == "" {
		return normalized, ticket.ErrInvalidState
	}
	for _, known := range ticket.BuiltinStates() {
		if known == normalized {
			return normalized, nil
		}
	}
	return normalized, ticket.ErrInvalidState
}

func handleCreateTicket(
	ctx context.Context,
	svc ticket.Service,
	payload CreateTicketAction,
	actor ticket.Actor,
) (ActionResult, error) {
	state, err := normalizeStateValue(payload.State)
	if err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(payload.ProjectID)
	if projectID == "" {
		return nil, ticket.ErrInvalidProject
	}
	input := ticket.CreateInput{
		Cell:        core.CellName(strings.TrimSpace(payload.Cell)),
		ProjectID:   ticket.ProjectID(projectID),
		Title:       strings.TrimSpace(payload.Title),
		Description: strings.TrimSpace(payload.Description),
		Stage:       normalizeStageValue(payload.Stage),
		State:       state,
		Actor:       actor,
	}
	created, err := svc.CreateTicket(ctx, input)
	if err != nil {
		return nil, err
	}
	return &CreateResult{Ticket: *created}, nil
}

func handleUpdateTicket(
	ctx context.Context,
	svc ticket.Service,
	payload UpdateTicketAction,
	fallback ticket.Actor,
) (ActionResult, error) {
	input := ticket.UpdateInput{}
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
		actor, err := resolveActor(payload.Actor, fallback)
		if err != nil {
			return nil, err
		}
		patchActor := toActorPatch(actor)
		input.Actor = &patchActor
		fields++
	}
	if fields == 0 {
		return nil, errUpdateNoFields
	}
	updated, err := svc.UpdateTicket(ctx, payload.TicketID, input)
	if err != nil {
		return nil, err
	}
	return &UpdateResult{Ticket: updated}, nil
}

func handleAppendTicketNote(
	ctx context.Context,
	svc ticket.Service,
	payload AppendTicketNoteAction,
	fallback ticket.Actor,
) (ActionResult, error) {
	actor, err := resolveActor(payload.Actor, fallback)
	if err != nil {
		return nil, err
	}
	eventTime := time.Now().UTC()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := svc.AppendTicketEvent(ctx, payload.TicketID, ticket.TicketEventInput{
		Actor:     actor,
		Notes:     strings.TrimSpace(payload.Note),
		EventTime: eventTime,
	})
	if err != nil {
		return nil, err
	}
	return &AppendTicketNoteResult{
		Event: event,
	}, nil
}

func handleMarkdownRemoveAction(
	ctx context.Context,
	svc ticket.Service,
	payload MarkdownRemoveAction,
	fallback ticket.Actor,
) (ActionResult, error) {
	return handleMarkdownEvent(ctx, svc, ticket.MarkdownDocRemoved, payload.BaseMarkdownAction, fallback)
}

func handleMarkdownLinkAction(
	ctx context.Context,
	svc ticket.Service,
	payload MarkdownLinkAction,
	fallback ticket.Actor,
) (ActionResult, error) {
	return handleMarkdownEvent(ctx, svc, ticket.MarkdownDocAttached, payload.BaseMarkdownAction, fallback)
}

func handleMarkdownOverrideAction(
	ctx context.Context,
	svc ticket.Service,
	payload MarkdownOverrideAction,
	fallback ticket.Actor,
) (ActionResult, error) {
	return handleMarkdownEvent(ctx, svc, ticket.MarkdownDocOverridden, payload.BaseMarkdownAction, fallback)
}

func handleMarkdownEvent(
	ctx context.Context,
	svc ticket.Service,
	markdownEventType ticket.MarkdownDocEventType,
	payload BaseMarkdownAction,
	fallback ticket.Actor,
) (ActionResult, error) {
	actor, err := resolveActor(payload.Actor, fallback)
	if err != nil {
		return nil, err
	}
	eventTime := time.Now().UTC()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := svc.AppendMarkdownEvent(ctx, payload.TicketID, ticket.MarkdownEventInput{
		Actor: actor,
		Payload: ticket.MarkdownDocEventPayload{
			Type: markdownEventType,
			Name: strings.TrimSpace(payload.Name),
			Path: strings.TrimSpace(payload.Path),
		},
		EventTime: eventTime,
	})
	if err != nil {
		return nil, err
	}
	return &MarkdownResult{Event: event}, nil
}

func handleWorkflowEvent(
	ctx context.Context,
	svc ticket.Service,
	payload AppendWorkflowAction,
	actor ticket.Actor,
) (ActionResult, error) {
	eventTime := time.Now().UTC()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := svc.AppendWorkflowEvent(ctx, payload.TicketID, ticket.WorkflowEventInput{
		Actor: actor,
		Payload: ticket.WorkflowEventPayload{
			Type:       payload.Status,
			WorkflowID: ticket.WorkflowID(strings.TrimSpace(payload.WorkflowID)),
			RunID:      ticket.WorkflowRunID(strings.TrimSpace(payload.RunID)),
		},
		EventTime: eventTime,
	})
	if err != nil {
		return nil, err
	}
	return &WorkflowResult{Event: event}, nil
}

func handleResetTicket(
	ctx context.Context,
	svc ticket.Service,
	payload ResetTicketAction,
	actor ticket.Actor,
) (ActionResult, error) {
	input := ticket.TicketResetInput{
		Actor:  actor,
		Reason: strings.TrimSpace(payload.Reason),
	}
	input.LastValidEvent = payload.AnchorEventID

	reset, err := svc.ResetTicket(ctx, payload.TicketID, input)
	if err != nil {
		return nil, err
	}

	refreshed, err := svc.GetTicketAt(ctx, payload.TicketID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return &ResetResult{
		Reset:  reset,
		Ticket: refreshed,
	}, nil
}

func markdownEventType(action ActionType) (ticket.MarkdownDocEventType, error) {
	switch action {
	case ActionLinkMarkdown:
		return ticket.MarkdownDocAttached, nil
	case ActionOverrideMarkdown:
		return ticket.MarkdownDocOverridden, nil
	case ActionRemoveMarkdown:
		return ticket.MarkdownDocRemoved, nil
	default:
		return "", fmt.Errorf("unsupported markdown action %s", action)
	}
}

func requireTicketID(actionTicketID string) (ticket.ID, error) {
	trimmed := strings.TrimSpace(actionTicketID)
	if trimmed == "" {
		return "", errTicketIDRequired
	}
	return ticket.ID(trimmed), nil
}

func toVersion(v *int64) optimisticlock.Version {
	if v == nil {
		return optimisticlock.Version{}
	}
	return optimisticlock.Version{Int64: *v, Valid: true}
}

func toActorPatch(actor ticket.Actor) ticket.ActorPatch {
	switch actor.Type {
	case ticket.ActorTypeUser:
		return ticket.ActorPatch{Type: actor.Type, User: actor.User}
	case ticket.ActorTypeAgent:
		return ticket.ActorPatch{Type: actor.Type, Agent: actor.Agent}
	default:
		return ticket.ActorPatch{Type: actor.Type}
	}
}
