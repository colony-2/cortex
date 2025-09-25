package service

import (
	"context"
	"errors"
	"strings"

	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
)

func (s *service) AppendEvent(ctx context.Context, id model.ID, input TicketEventInput) (*model.TicketEvent, error) {
	if _, err := s.store.Get(ctx, id); err != nil {
		return nil, err
	}

	sanitized := input
	sanitized.Actor = sanitizeActorFields(input.Actor)

	if err := s.validate.Struct(sanitized); err != nil {
		return nil, mapValidationError(err)
	}

	actor, err := s.normalizeActor(sanitized.Actor)
	if err != nil {
		return nil, err
	}

	eventID, err := s.eventIDGen.NewID()
	if err != nil {
		return nil, errors.Join(ErrIDGeneration, err)
	}

	eventTime := sanitized.EventTime
	if eventTime.IsZero() {
		eventTime = s.clock.Now()
	}
	eventTime = eventTime.UTC()

	createdAt := s.clock.Now()

	event := &model.TicketEvent{
		ID:        model.TicketEventID(eventID),
		TicketID:  id,
		Kind:      sanitized.Kind,
		Actor:     actor,
		EventTime: eventTime,
		CreatedAt: createdAt,
	}
	event.SetPayload(sanitized.Kind, sanitized.Payload)

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
