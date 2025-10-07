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

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/ticket/pkg/ticket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.temporal.io/sdk/temporal"
	"gorm.io/gorm"
	"gorm.io/plugin/optimisticlock"
)

const opName = "ticket.manage"

var (
	metricInitOnce sync.Once
	actionCounter  metric.Int64Counter
	batchHistogram metric.Float64Histogram

	errMissingActions         = errors.New("ticket.manage: actions must not be empty")
	errCreateAfterTicket      = errors.New("ticket.manage: create_ticket cannot run after ticket id is assigned")
	errTicketIDRequired       = errors.New("ticket.manage: ticket id is required")
	errUpdateNoFields         = errors.New("ticket.manage: update_ticket requires at least one field to modify")
	errInvalidExpectedVersion = errors.New("ticket.manage: update_ticket expected_version must be greater than 0")
)

func initMetrics() {
	metricInitOnce.Do(func() {
		meter := otel.Meter("github.com/divisive-ai/vibethis/server/ticket/pkg/op")
		var err error
		actionCounter, err = meter.Int64Counter(
			"ticket_manage_action_total",
			metric.WithDescription("Count of ticket.manage actions by type/status"),
		)
		if err != nil {
			log.Printf("ticketop: meter Int64Counter init failed: %v", err)
		}
		batchHistogram, err = meter.Float64Histogram(
			"ticket_manage_duration_ms",
			metric.WithUnit("ms"),
			metric.WithDescription("Duration of ticket.manage batches"),
		)
		if err != nil {
			log.Printf("ticketop: meter Float64Histogram init failed: %v", err)
		}
	})
}

func recordActionMetric(ctx context.Context, action ActionType, status string) {
	initMetrics()
	if actionCounter == nil {
		return
	}
	actionCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("action", string(action)),
		attribute.String("status", status),
	))
}

func recordBatchMetric(ctx context.Context, duration time.Duration) {
	initMetrics()
	if batchHistogram == nil {
		return
	}
	batchHistogram.Record(ctx, float64(duration.Milliseconds()))
}

type (
	ActionType string

	Input struct {
		TicketID string   `json:"ticket_id,omitempty"`
		Actions  []Action `json:"actions"`
	}

	Action struct {
		Type ActionType      `json:"type"`
		Raw  json.RawMessage `json:"-"`
	}

	Output struct {
		Ticket       *ticket.Ticket `json:"ticket,omitempty"`
		Results      []ActionResult `json:"results"`
		ContextPatch map[string]any `json:"context_patch,omitempty"`
	}

	ActionResult struct {
		Type          ActionType          `json:"type"`
		Ticket        *ticket.Ticket      `json:"ticket,omitempty"`
		Event         *ticket.TicketEvent `json:"event,omitempty"`
		Reset         *ticket.TicketReset `json:"reset,omitempty"`
		EffectiveTime time.Time           `json:"effective_time"`
	}
)

const (
	ActionCreateTicket     ActionType = "create_ticket"
	ActionUpdateTicket     ActionType = "update_ticket"
	ActionAppendTicketNote ActionType = "append_ticket_note"
	ActionLinkMarkdown     ActionType = "link_markdown_doc"
	ActionOverrideMarkdown ActionType = "override_markdown_doc"
	ActionRemoveMarkdown   ActionType = "remove_markdown_doc"
	ActionAppendWorkflow   ActionType = "append_workflow_event"
	ActionResetTicket      ActionType = "reset_ticket"
)

func (a *Action) UnmarshalJSON(data []byte) error {
	type alias Action
	aux := struct {
		Type ActionType `json:"type"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Type == "" {
		return fmt.Errorf("action type is required")
	}
	a.Type = aux.Type
	a.Raw = append(a.Raw[:0], data...)
	return nil
}

// actorPayload represents action-level actor configuration.
type actorPayload struct {
	Type  string             `json:"type"`
	User  *actorUserPayload  `json:"user,omitempty"`
	Agent *actorAgentPayload `json:"agent,omitempty"`
}

type actorUserPayload struct {
	Email string `json:"email"`
}

type actorAgentPayload struct {
	CellName       string `json:"cell"`
	WorkflowName   string `json:"workflow_name"`
	ExecutionID    string `json:"execution_id"`
	InvocationHash string `json:"invocation_hash"`
}

type createTicketAction struct {
	Cell        string        `json:"cell"`
	Title       string        `json:"title"`
	Stage       string        `json:"stage"`
	State       string        `json:"state"`
	Description string        `json:"description,omitempty"`
	Actor       *actorPayload `json:"actor,omitempty"`
}

type updateTicketAction struct {
	ExpectedVersion int64         `json:"expected_version"`
	Stage           *string       `json:"stage,omitempty"`
	State           *string       `json:"state,omitempty"`
	Description     *string       `json:"description,omitempty"`
	Actor           *actorPayload `json:"actor,omitempty"`
}

type appendTicketNoteAction struct {
	Note      string        `json:"note"`
	Actor     *actorPayload `json:"actor,omitempty"`
	EventTime *time.Time    `json:"event_time,omitempty"`
}

type markdownAction struct {
	Name      string        `json:"name"`
	Path      string        `json:"path"`
	Reason    string        `json:"reason,omitempty"`
	Actor     *actorPayload `json:"actor,omitempty"`
	EventTime *time.Time    `json:"event_time,omitempty"`
}

type appendWorkflowAction struct {
	WorkflowID string                   `json:"workflow_id"`
	RunID      string                   `json:"run_id"`
	Status     ticket.WorkflowEventType `json:"status"`
	Actor      *actorPayload            `json:"actor,omitempty"`
	EventTime  *time.Time               `json:"event_time,omitempty"`
}

type resetTicketAction struct {
	Reason        string                `json:"reason"`
	AnchorEventID *ticket.TicketEventID `json:"anchor_event_id,omitempty"`
	Actor         *actorPayload         `json:"actor,omitempty"`
}

var (
	serviceFactoryMu sync.RWMutex
	serviceFactory   = defaultServiceFactory
)

type serviceFactoryFunc func(inv ops.Invocation, db *gorm.DB) (ticket.Service, error)

func defaultServiceFactory(_ ops.Invocation, db *gorm.DB) (ticket.Service, error) {
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
	serviceFactory = func(inv ops.Invocation, db *gorm.DB) (ticket.Service, error) {
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
	return ops.NewActivityMappedOpV2[Input, Output](
		ops.OpMetadata{
			Type:           opName,
			Description:    "Manage ticket lifecycle actions as a batch",
			Version:        "1.0.0",
			DefaultTimeout: 2 * time.Minute,
		},
		execute,
	)
}

func execute(inv ops.Invocation, ctx context.Context, input Input) (Output, error) {
	if len(input.Actions) == 0 {
		return Output{}, mapError(errMissingActions, nil, nil)
	}

	svc, err := resolveService(inv)
	if err != nil {
		return Output{}, err
	}

	fallbackActor := defaultAutomationActor(inv)
	batchStart := time.Now()
	defer func(start time.Time) {
		recordBatchMetric(ctx, time.Since(start))
	}(batchStart)

	results := make([]ActionResult, 0, len(input.Actions))
	contextPatch := make(map[string]any)

	ticketID := strings.TrimSpace(input.TicketID)
	if ticketID != "" {
		contextPatch["ticket.id"] = ticketID
	}

	var currentTicket *ticket.Ticket

	for idx, action := range input.Actions {
		actionStart := time.Now()
		result, err := executeAction(ctx, inv, svc, action, fallbackActor, &currentTicket, &ticketID, contextPatch)
		status := "success"
		if err != nil {
			status = "error"
		}
		recordActionMetric(ctx, action.Type, status)
		logLine := fmt.Sprintf("ticketop: action=%s ticket=%s status=%s index=%d duration_ms=%d", action.Type, ticketID, status, idx, time.Since(actionStart).Milliseconds())
		if err != nil {
			wrapped := mapError(err, results, contextPatch)
			log.Printf("%s error=%v", logLine, err)
			return Output{}, wrapped
		}
		log.Printf("%s", logLine)
		results = append(results, result)
	}

	output := Output{Results: results}
	if currentTicket != nil {
		output.Ticket = currentTicket
	}
	if len(contextPatch) > 0 {
		output.ContextPatch = contextPatch
	}
	log.Printf("ticketop: batch ticket=%s actions=%d status=success duration_ms=%d", ticketID, len(input.Actions), time.Since(batchStart).Milliseconds())
	return output, nil
}

func resolveService(inv ops.Invocation) (ticket.Service, error) {
	var db *gorm.DB
	if inv.Deps != nil {
		var ok bool
		db, ok = inv.Deps.Database()
		if !ok || db == nil {
			return nil, temporal.NewNonRetryableApplicationError("ticket.manage: database dependency not configured", "MISSING_DATABASE", nil)
		}
	} else {
		return nil, temporal.NewNonRetryableApplicationError("ticket.manage: dependency container missing", "MISSING_DATABASE", nil)
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

func mapError(err error, partial []ActionResult, patch map[string]any) error {
	if err == nil {
		return nil
	}
	detail := Output{Results: partial}
	if len(patch) > 0 {
		detail.ContextPatch = patch
	}
	switch {
	case errors.Is(err, errMissingActions), errors.Is(err, errCreateAfterTicket), errors.Is(err, errTicketIDRequired), errors.Is(err, errUpdateNoFields), errors.Is(err, errInvalidExpectedVersion):
		return temporal.NewNonRetryableApplicationError(err.Error(), "BAD_REQUEST", err, detail)
	case errors.Is(err, ticket.ErrVersionConflict):
		return temporal.NewNonRetryableApplicationError(err.Error(), "VERSION_CONFLICT", err, detail)
	case errors.Is(err, ticket.ErrInvalidState), errors.Is(err, ticket.ErrInvalidActor), errors.Is(err, ticket.ErrEmptyTitle), errors.Is(err, ticket.ErrEmptyStage):
		return temporal.NewNonRetryableApplicationError(err.Error(), "BAD_REQUEST", err, detail)
	case errors.Is(err, ticket.ErrResetNoEvents):
		return temporal.NewNonRetryableApplicationError(err.Error(), "RESET_NOT_ALLOWED", err, detail)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return temporal.NewApplicationError(err.Error(), "TICKET_MANAGE_FAILED", err, detail)
	}
}

func executeAction(
	ctx context.Context,
	inv ops.Invocation,
	svc ticket.Service,
	action Action,
	fallback ticket.Actor,
	currentTicket **ticket.Ticket,
	ticketID *string,
	contextPatch map[string]any,
) (ActionResult, error) {
	switch action.Type {
	case ActionCreateTicket:
		return handleCreateTicket(ctx, svc, action.Raw, fallback, currentTicket, ticketID, contextPatch)
	case ActionUpdateTicket:
		return handleUpdateTicket(ctx, svc, action.Raw, fallback, currentTicket, ticketID, contextPatch)
	case ActionAppendTicketNote:
		return handleAppendTicketNote(ctx, svc, action.Raw, fallback, currentTicket, *ticketID, contextPatch)
	case ActionLinkMarkdown, ActionOverrideMarkdown, ActionRemoveMarkdown:
		return handleMarkdownEvent(ctx, svc, action.Type, action.Raw, fallback, *ticketID, contextPatch)
	case ActionAppendWorkflow:
		return handleWorkflowEvent(ctx, svc, action.Raw, fallback, *ticketID, contextPatch)
	case ActionResetTicket:
		return handleResetTicket(ctx, svc, action.Raw, fallback, currentTicket, *ticketID, contextPatch)
	default:
		return ActionResult{}, fmt.Errorf("unsupported action type %q", action.Type)
	}
}

func defaultAutomationActor(inv ops.Invocation) ticket.Actor {
	cell := strings.TrimSpace(inv.BoxID)
	if cell == "" {
		cell = "unknown"
	}
	workflow := strings.TrimSpace(inv.RecipeID)
	if workflow == "" {
		workflow = "recipe"
	}
	execID := strings.TrimSpace(inv.ActivityID)
	if execID == "" {
		execID = fmt.Sprintf("%s:%d", inv.NodePath, inv.InvokeSeq)
	}
	hash := inv.Hash()
	if hash == "" {
		hash = fmt.Sprintf("%s:%d", inv.NodePath, inv.InvokeSeq)
	}
	return ticket.NewAgentActor(cell, workflow, execID, hash)
}

func resolveActor(payload *actorPayload, fallback ticket.Actor) (ticket.Actor, error) {
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
	raw json.RawMessage,
	fallback ticket.Actor,
	currentTicket **ticket.Ticket,
	ticketID *string,
	patch map[string]any,
) (ActionResult, error) {
	var payload createTicketAction
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ActionResult{}, err
	}
	if *ticketID != "" {
		return ActionResult{}, errCreateAfterTicket
	}
	actor, err := resolveActor(payload.Actor, fallback)
	if err != nil {
		return ActionResult{}, err
	}
	state, err := normalizeStateValue(payload.State)
	if err != nil {
		return ActionResult{}, err
	}
	input := ticket.CreateInput{
		Cell:        core.CellName(strings.TrimSpace(payload.Cell)),
		Title:       strings.TrimSpace(payload.Title),
		Description: strings.TrimSpace(payload.Description),
		Stage:       normalizeStageValue(payload.Stage),
		State:       state,
		Actor:       actor,
	}
	created, err := svc.CreateTicket(ctx, input)
	if err != nil {
		return ActionResult{}, err
	}
	result := ActionResult{
		Type:          ActionCreateTicket,
		Ticket:        created,
		EffectiveTime: created.UpdatedAt,
	}
	*ticketID = string(created.ID)
	if *ticketID != "" {
		patch["ticket.id"] = *ticketID
	}
	*currentTicket = created
	applyTicketContext(patch, created)
	return result, nil
}

func handleUpdateTicket(
	ctx context.Context,
	svc ticket.Service,
	raw json.RawMessage,
	fallback ticket.Actor,
	currentTicket **ticket.Ticket,
	ticketID *string,
	patch map[string]any,
) (ActionResult, error) {
	if *ticketID == "" {
		return ActionResult{}, errTicketIDRequired
	}
	var payload updateTicketAction
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ActionResult{}, err
	}
	if payload.ExpectedVersion <= 0 {
		return ActionResult{}, errInvalidExpectedVersion
	}
	input := ticket.UpdateInput{
		ExpectedVersion: toVersion(payload.ExpectedVersion),
	}
	var fields int
	if payload.Stage != nil {
		stage := normalizeStageValue(*payload.Stage)
		input.Stage = &stage
		fields++
	}
	if payload.State != nil {
		state, err := normalizeStateValue(*payload.State)
		if err != nil {
			return ActionResult{}, err
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
			return ActionResult{}, err
		}
		patchActor := toActorPatch(actor)
		input.Actor = &patchActor
		fields++
	}
	if fields == 0 {
		return ActionResult{}, errUpdateNoFields
	}
	updated, err := svc.UpdateTicket(ctx, ticket.ID(*ticketID), input)
	if err != nil {
		return ActionResult{}, err
	}
	*ticketID = string(updated.ID)
	*currentTicket = updated
	applyTicketContext(patch, updated)
	return ActionResult{
		Type:          ActionUpdateTicket,
		Ticket:        updated,
		EffectiveTime: updated.UpdatedAt,
	}, nil
}

func handleAppendTicketNote(
	ctx context.Context,
	svc ticket.Service,
	raw json.RawMessage,
	fallback ticket.Actor,
	currentTicket **ticket.Ticket,
	ticketID string,
	patch map[string]any,
) (ActionResult, error) {
	if ticketID == "" {
		return ActionResult{}, errTicketIDRequired
	}
	var payload appendTicketNoteAction
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ActionResult{}, err
	}
	actor, err := resolveActor(payload.Actor, fallback)
	if err != nil {
		return ActionResult{}, err
	}
	eventTime := time.Now().UTC()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := svc.AppendTicketEvent(ctx, ticket.ID(ticketID), ticket.TicketEventInput{
		Actor:     actor,
		Notes:     strings.TrimSpace(payload.Note),
		EventTime: eventTime,
	})
	if err != nil {
		return ActionResult{}, err
	}
	applyEventContext(patch, event)
	return ActionResult{
		Type:          ActionAppendTicketNote,
		Event:         event,
		EffectiveTime: event.EventTime,
	}, nil
}

func handleMarkdownEvent(
	ctx context.Context,
	svc ticket.Service,
	actionType ActionType,
	raw json.RawMessage,
	fallback ticket.Actor,
	ticketID string,
	patch map[string]any,
) (ActionResult, error) {
	if ticketID == "" {
		return ActionResult{}, errTicketIDRequired
	}
	var payload markdownAction
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ActionResult{}, err
	}
	actor, err := resolveActor(payload.Actor, fallback)
	if err != nil {
		return ActionResult{}, err
	}
	eventTime := time.Now().UTC()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	eventType, err := markdownEventType(actionType)
	if err != nil {
		return ActionResult{}, err
	}
	if reason := strings.TrimSpace(payload.Reason); reason != "" {
		log.Printf("ticketop: markdown action=%s ticket=%s reason=%s", actionType, ticketID, reason)
	}
	event, err := svc.AppendMarkdownEvent(ctx, ticket.ID(ticketID), ticket.MarkdownEventInput{
		Actor: actor,
		Payload: ticket.MarkdownDocEventPayload{
			Type: eventType,
			Name: strings.TrimSpace(payload.Name),
			Path: strings.TrimSpace(payload.Path),
		},
		EventTime: eventTime,
	})
	if err != nil {
		return ActionResult{}, err
	}
	applyEventContext(patch, event)
	return ActionResult{
		Type:          actionType,
		Event:         event,
		EffectiveTime: event.EventTime,
	}, nil
}

func handleWorkflowEvent(
	ctx context.Context,
	svc ticket.Service,
	raw json.RawMessage,
	fallback ticket.Actor,
	ticketID string,
	patch map[string]any,
) (ActionResult, error) {
	if ticketID == "" {
		return ActionResult{}, errTicketIDRequired
	}
	var payload appendWorkflowAction
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ActionResult{}, err
	}
	actor, err := resolveActor(payload.Actor, fallback)
	if err != nil {
		return ActionResult{}, err
	}
	eventTime := time.Now().UTC()
	if payload.EventTime != nil {
		eventTime = payload.EventTime.UTC()
	}
	event, err := svc.AppendWorkflowEvent(ctx, ticket.ID(ticketID), ticket.WorkflowEventInput{
		Actor: actor,
		Payload: ticket.WorkflowEventPayload{
			Type:       payload.Status,
			WorkflowID: ticket.WorkflowID(strings.TrimSpace(payload.WorkflowID)),
			RunID:      ticket.WorkflowRunID(strings.TrimSpace(payload.RunID)),
		},
		EventTime: eventTime,
	})
	if err != nil {
		return ActionResult{}, err
	}
	applyEventContext(patch, event)
	if event.Payload.Workflow != nil {
		patch["ticket.workflow_id"] = string(event.Payload.Workflow.WorkflowID)
		patch["ticket.workflow_run_id"] = string(event.Payload.Workflow.RunID)
	}
	return ActionResult{
		Type:          ActionAppendWorkflow,
		Event:         event,
		EffectiveTime: event.EventTime,
	}, nil
}

func handleResetTicket(
	ctx context.Context,
	svc ticket.Service,
	raw json.RawMessage,
	fallback ticket.Actor,
	currentTicket **ticket.Ticket,
	ticketID string,
	patch map[string]any,
) (ActionResult, error) {
	if ticketID == "" {
		return ActionResult{}, errTicketIDRequired
	}
	var payload resetTicketAction
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ActionResult{}, err
	}
	actor, err := resolveActor(payload.Actor, fallback)
	if err != nil {
		return ActionResult{}, err
	}
	input := ticket.TicketResetInput{
		Actor:  actor,
		Reason: strings.TrimSpace(payload.Reason),
	}
	input.LastValidEvent = payload.AnchorEventID

	reset, err := svc.ResetTicket(ctx, ticket.ID(ticketID), input)
	if err != nil {
		return ActionResult{}, err
	}
	applyResetContext(patch, reset)

	refreshed, err := svc.GetTicketAt(ctx, ticket.ID(ticketID), time.Now().UTC())
	if err != nil {
		return ActionResult{}, err
	}
	*currentTicket = refreshed
	applyTicketContext(patch, refreshed)
	return ActionResult{
		Type:          ActionResetTicket,
		Reset:         reset,
		EffectiveTime: reset.CreatedAt,
		Ticket:        refreshed,
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

func applyTicketContext(patch map[string]any, tkt *ticket.Ticket) {
	if patch == nil || tkt == nil {
		return
	}
	patch["ticket.id"] = string(tkt.ID)
	patch["ticket.stage"] = string(tkt.Stage)
	patch["ticket.state"] = string(tkt.State)
	if tkt.Version.Valid {
		patch["ticket.version"] = tkt.Version.Int64
	}
	patch["ticket.updated_at"] = tkt.UpdatedAt.UTC().Format(time.RFC3339Nano)
	if tkt.LastResetID != nil {
		patch["ticket.last_reset_id"] = string(*tkt.LastResetID)
	}
	if tkt.LastResetAt != nil {
		patch["ticket.last_reset_at"] = tkt.LastResetAt.UTC().Format(time.RFC3339Nano)
	}
}

func applyEventContext(patch map[string]any, evt *ticket.TicketEvent) {
	if patch == nil || evt == nil {
		return
	}
	patch["ticket.last_event_id"] = string(evt.ID)
	patch["ticket.last_event_kind"] = string(evt.Kind)
	patch["ticket.last_event_time"] = evt.EventTime.UTC().Format(time.RFC3339Nano)
}

func applyResetContext(patch map[string]any, reset *ticket.TicketReset) {
	if patch == nil || reset == nil {
		return
	}
	patch["ticket.last_reset_id"] = string(reset.ID)
	patch["ticket.last_reset_reason"] = reset.Reason
	patch["ticket.last_reset_at"] = reset.CreatedAt.UTC().Format(time.RFC3339Nano)
}

func toVersion(v int64) optimisticlock.Version {
	return optimisticlock.Version{Int64: v, Valid: true}
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
