package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
)

func (s *service) AppendWorkflowEvent(ctx context.Context, id model.ID, input WorkflowEventInput) (*model.TicketEvent, error) {
	payload := input.Payload
	return s.appendEvent(ctx, id, input.Actor, model.TicketEventKindWorkflow, model.TicketEventBody{Workflow: &payload}, input.EventTime)
}

func (s *service) AppendMarkdownEvent(ctx context.Context, id model.ID, input MarkdownEventInput) (*model.TicketEvent, error) {
	payload := input.Payload
	return s.appendEvent(ctx, id, input.Actor, model.TicketEventKindMarkdownDoc, model.TicketEventBody{MarkdownDoc: &payload}, input.EventTime)
}

func (s *service) AppendChangeSetEvent(ctx context.Context, id model.ID, input ChangeSetEventInput) (*model.TicketEvent, error) {
	payload := input.Payload
	return s.appendEvent(ctx, id, input.Actor, model.TicketEventKindChangeSet, model.TicketEventBody{ChangeSet: &payload}, input.EventTime)
}

func (s *service) appendTicketEvent(ctx context.Context, id model.ID, actor model.Actor, changes []model.TicketFieldChange, eventTime time.Time) (*model.TicketEvent, error) {
	body := model.TicketEventBody{
		Ticket: &model.TicketEventPayload{Changes: changes},
	}
	return s.appendEvent(ctx, id, actor, model.TicketEventKindTicket, body, eventTime)
}

func (s *service) appendEvent(ctx context.Context, id model.ID, rawActor model.Actor, kind model.TicketEventKind, body model.TicketEventBody, eventTime time.Time) (*model.TicketEvent, error) {
	if _, err := s.store.Get(ctx, id); err != nil {
		return nil, err
	}

	sanitizedActor := sanitizeActorFields(rawActor)
	if err := s.validate.Struct(sanitizedActor); err != nil {
		return nil, mapValidationError(err)
	}
	actor, err := s.normalizeActor(sanitizedActor)
	if err != nil {
		return nil, err
	}

	if err := validateEventPayload(kind, body); err != nil {
		return nil, err
	}

	if eventTime.IsZero() {
		eventTime = s.clock.Now()
	}
	eventTime = eventTime.UTC()

	eventID, err := s.eventIDGen.NewID()
	if err != nil {
		return nil, errors.Join(ErrIDGeneration, err)
	}

	event := &model.TicketEvent{
		ID:        model.TicketEventID(eventID),
		TicketID:  id,
		Kind:      kind,
		Actor:     actor,
		EventTime: eventTime,
		CreatedAt: s.clock.Now(),
	}
	event.SetPayload(kind, body)

	if err := s.events.Append(ctx, event); err != nil {
		return nil, err
	}
	return event, nil
}

func (s *service) ListEvents(ctx context.Context, id model.ID, filter model.TicketEventFilter) (store.Iterator[*model.TicketEvent], error) {
	if filter.Since != nil {
		since := filter.Since.UTC()
		filter.Since = &since
	}
	if filter.Until != nil {
		until := filter.Until.UTC()
		filter.Until = &until
	}
	if filter.At != nil {
		at := filter.At.UTC()
		filter.At = &at
	}

	iter, err := s.events.ListByTicket(ctx, id, filter)
	if err != nil {
		return nil, err
	}
	return &hydrateIterator{inner: iter}, nil
}

func (s *service) ResetEvents(ctx context.Context, id model.ID, input TicketResetInput) (*model.TicketReset, error) {
	if _, err := s.store.Get(ctx, id); err != nil {
		return nil, err
	}

	sanitized := input
	sanitized.Actor = sanitizeActorFields(input.Actor)
	sanitized.Reason = strings.TrimSpace(input.Reason)

	if sanitized.Reason == "" {
		return nil, ErrInvalidEventPayload
	}

	if err := s.validate.Struct(sanitized.Actor); err != nil {
		return nil, ErrInvalidActor
	}

	actor, err := s.normalizeActor(sanitized.Actor)
	if err != nil {
		return nil, err
	}

	iter, err := s.events.ListByTicket(ctx, id, model.TicketEventFilter{IncludeReset: true})
	if err != nil {
		return nil, err
	}
	defer iter.Close(ctx)

	var (
		anchorFound bool
		toReset     []model.TicketEventID
	)

	for {
		evt, err := iter.Next(ctx)
		if errors.Is(err, store.ErrIteratorDone) {
			break
		}
		if err != nil {
			return nil, err
		}
		if sanitized.LastValidEvent != nil && evt.ID == *sanitized.LastValidEvent {
			anchorFound = true
			continue
		}
		if evt.ResetID != nil {
			continue
		}
		if sanitized.LastValidEvent == nil {
			toReset = append(toReset, evt.ID)
			continue
		}
		if anchorFound {
			toReset = append(toReset, evt.ID)
		}
	}

	if sanitized.LastValidEvent != nil && !anchorFound {
		return nil, ErrEventNotFound
	}

	if len(toReset) == 0 {
		return nil, ErrResetNoEvents
	}

	resetID, err := s.eventIDGen.NewID()
	if err != nil {
		return nil, errors.Join(ErrIDGeneration, err)
	}

	reset := &model.TicketReset{
		ID:        model.TicketResetID(resetID),
		TicketID:  id,
		Actor:     actor,
		Reason:    sanitized.Reason,
		CreatedAt: s.clock.Now(),
	}

	if err := s.events.MarkReset(ctx, id, reset, toReset); err != nil {
		return nil, err
	}

	return reset, nil
}

func validateEventPayload(kind model.TicketEventKind, body model.TicketEventBody) error {
	nonNil := 0
	if body.Ticket != nil {
		nonNil++
	}
	if body.Workflow != nil {
		nonNil++
	}
	if body.MarkdownDoc != nil {
		nonNil++
	}
	if body.ChangeSet != nil {
		nonNil++
	}
	if nonNil != 1 {
		return ErrInvalidEventPayload
	}

	switch kind {
	case model.TicketEventKindTicket:
		if body.Ticket == nil {
			return ErrInvalidEventPayload
		}
	case model.TicketEventKindWorkflow:
		if body.Workflow == nil {
			return ErrInvalidEventPayload
		}
		if body.Workflow.Type == "" || body.Workflow.WorkflowID == "" || body.Workflow.RunID == "" {
			return ErrInvalidEventPayload
		}
	case model.TicketEventKindMarkdownDoc:
		if body.MarkdownDoc == nil {
			return ErrInvalidEventPayload
		}
		if strings.TrimSpace(body.MarkdownDoc.Name) == "" || strings.TrimSpace(body.MarkdownDoc.Path) == "" || body.MarkdownDoc.Type == "" {
			return ErrInvalidEventPayload
		}
	case model.TicketEventKindChangeSet:
		if body.ChangeSet == nil {
			return ErrInvalidEventPayload
		}
		if strings.TrimSpace(body.ChangeSet.Path) == "" || body.ChangeSet.Type == "" {
			return ErrInvalidEventPayload
		}
	default:
		return ErrInvalidEventKind
	}
	return nil
}

type hydrateIterator struct {
	inner store.Iterator[*model.TicketEvent]
}

func (h *hydrateIterator) Next(ctx context.Context) (*model.TicketEvent, error) {
	if h == nil || h.inner == nil {
		return nil, store.ErrIteratorDone
	}
	evt, err := h.inner.Next(ctx)
	if err != nil {
		return evt, err
	}
	if evt != nil {
		evt.HydratePayload()
	}
	return evt, nil
}

func (h *hydrateIterator) Close(ctx context.Context) error {
	if h == nil || h.inner == nil {
		return nil
	}
	return h.inner.Close(ctx)
}
