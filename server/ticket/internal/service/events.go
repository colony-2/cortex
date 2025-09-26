package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	eventstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/events"
	store "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/optimisticlock"
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

func (s *service) appendTicketEventInTx(ctx context.Context, st store.Store, snapshot *model.Ticket, id model.ID, actor model.Actor, changes []model.TicketFieldChange, eventTime time.Time) (*model.TicketEvent, error) {
	body := model.TicketEventBody{
		Ticket: &model.TicketEventPayload{Changes: changes},
	}
	return s.appendEventInTx(ctx, st, snapshot, id, actor, model.TicketEventKindTicket, body, eventTime)
}

func (s *service) appendEvent(ctx context.Context, id model.ID, rawActor model.Actor, kind model.TicketEventKind, body model.TicketEventBody, eventTime time.Time) (*model.TicketEvent, error) {
	snapshot, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.attachLastReset(ctx, snapshot); err != nil {
		return nil, err
	}

	var appended *model.TicketEvent
	err = s.store.WithTx(ctx, func(ctx context.Context, st store.Store) error {
		event, err := s.appendEventInTx(ctx, st, snapshot, id, rawActor, kind, body, eventTime)
		if err != nil {
			return err
		}
		appended = event
		return nil
	})
	if err != nil {
		return nil, err
	}

	return appended, nil
}

func (s *service) appendEventInTx(ctx context.Context, st store.Store, snapshot *model.Ticket, id model.ID, rawActor model.Actor, kind model.TicketEventKind, body model.TicketEventBody, eventTime time.Time) (*model.TicketEvent, error) {
	if st == nil {
		return nil, errors.New("ticket: nil transactional store")
	}
	if snapshot == nil {
		return nil, errors.New("ticket: nil snapshot for append")
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
	createdAt := s.clock.Now()

	eventID, err := s.eventIDGen.NewID()
	if err != nil {
		return nil, errors.Join(ErrIDGeneration, err)
	}

	txDB := st.DB()
	baselineResetID := snapshot.LastResetID
	baselineResetAt := snapshot.LastResetAt
	buildEvent := func(resetID *model.TicketResetID) *model.TicketEvent {
		event := &model.TicketEvent{
			ID:        model.TicketEventID(eventID),
			TicketID:  id,
			Kind:      kind,
			Actor:     actor,
			EventTime: eventTime,
			CreatedAt: createdAt,
			ResetID:   resetID,
		}
		event.SetPayload(kind, body)
		return event
	}

	if txDB == nil {
		event := buildEvent(nil)
		if err := s.events.Append(ctx, event); err != nil {
			return nil, err
		}
		return event, nil
	}

	lock := clause.Locking{Strength: "UPDATE"}
	if err := txDB.WithContext(ctx).
		Clauses(lock).
		Where("id = ? AND valid_until = ?", id, temporalInfinity()).
		First(&model.Ticket{}).Error; err != nil {
		return nil, err
	}

	txEvents, err := eventstore.NewWithDB(txDB)
	if err != nil {
		return nil, err
	}

	latestReset, err := txEvents.LatestReset(ctx, id)
	if err != nil {
		return nil, err
	}

	var resetID *model.TicketResetID
	if resetMetadataChanged(baselineResetID, baselineResetAt, latestReset) {
		idCopy := latestReset.ID
		resetID = &idCopy
	}

	event := buildEvent(resetID)

	if err := txEvents.Append(ctx, event); err != nil {
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

func (s *service) ResetTicket(ctx context.Context, id model.ID, input TicketResetInput) (*model.TicketReset, error) {
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

	var resetResult *model.TicketReset

	err = s.store.WithTx(ctx, func(ctx context.Context, st store.Store) error {
		ticketCurrent, err := st.Get(ctx, id)
		if err != nil {
			return err
		}

		txDB := st.DB()
		if txDB == nil {
			return errors.New("ticket: missing transaction db")
		}

		evtStore, err := eventstore.NewWithDB(txDB)
		if err != nil {
			return err
		}

		iter, err := evtStore.ListByTicket(ctx, id, model.TicketEventFilter{IncludeReset: true})
		if err != nil {
			return err
		}
		defer iter.Close(ctx)

		var (
			anchorEvent *model.TicketEvent
			toReset     []model.TicketEventID
		)

		for {
			evt, err := iter.Next(ctx)
			if errors.Is(err, store.ErrIteratorDone) {
				break
			}
			if err != nil {
				return err
			}
			if evt.ResetID != nil {
				continue
			}
			if sanitized.LastValidEvent != nil {
				if evt.ID == *sanitized.LastValidEvent {
					anchorEvent = evt
					continue
				}
				if anchorEvent != nil {
					toReset = append(toReset, evt.ID)
				}
				continue
			}
			toReset = append(toReset, evt.ID)
		}

		if sanitized.LastValidEvent != nil && anchorEvent == nil {
			return ErrEventNotFound
		}

		if len(toReset) == 0 {
			return ErrResetNoEvents
		}

		resetTime := s.clock.Now().UTC()
		if anchorEvent != nil {
			resetTime = anchorEvent.EventTime.UTC()
		}

		if !ticketCurrent.ValidFrom.Before(resetTime) {
			resetTime = ticketCurrent.ValidFrom.Add(time.Microsecond)
		}

		closeSlice := *ticketCurrent
		closeSlice.ValidUntil = resetTime
		closeSlice.UpdatedAt = resetTime
		if err := st.Update(ctx, &closeSlice, "ValidUntil", "UpdatedAt"); err != nil {
			if errors.Is(err, store.ErrOptimisticLock) {
				return ErrVersionConflict
			}
			return err
		}

		var sourceTicket *model.Ticket
		if anchorEvent != nil {
			historical, err := st.GetAt(ctx, id, anchorEvent.EventTime)
			if err != nil {
				return err
			}
			sourceTicket = historical
		} else {
			copyCurrent := *ticketCurrent
			sourceTicket = &copyCurrent
		}

		restored := *sourceTicket
		restored.ValidFrom = resetTime
		restored.ValidUntil = temporalInfinity()
		restored.UpdatedAt = resetTime
		restored.Version = optimisticlock.Version{Int64: ticketCurrent.Version.Int64 + 1, Valid: true}

		if err := st.Create(ctx, &restored); err != nil {
			return err
		}

		resetID, err := s.eventIDGen.NewID()
		if err != nil {
			return errors.Join(ErrIDGeneration, err)
		}

		reset := &model.TicketReset{
			ID:        model.TicketResetID(resetID),
			TicketID:  id,
			Actor:     actor,
			Reason:    sanitized.Reason,
			CreatedAt: resetTime,
		}

		if err := txDB.WithContext(ctx).Create(reset).Error; err != nil {
			return err
		}

		if len(toReset) > 0 {
			if err := txDB.WithContext(ctx).
				Model(&model.TicketEvent{}).
				Where("ticket_id = ?", id).
				Where("id IN ?", toReset).
				Where("reset_id IS NULL").
				Updates(map[string]any{"reset_id": reset.ID}).Error; err != nil {
				return err
			}
		}

		resetEventID, err := s.eventIDGen.NewID()
		if err != nil {
			return errors.Join(ErrIDGeneration, err)
		}

		var anchorEventID *model.TicketEventID
		if anchorEvent != nil {
			idCopy := anchorEvent.ID
			anchorEventID = &idCopy
		}

		resetPayload := &model.TicketResetEventPayload{
			ResetID:       reset.ID,
			AnchorEventID: anchorEventID,
			Reason:        sanitized.Reason,
		}

		resetEvent := &model.TicketEvent{
			ID:        model.TicketEventID(resetEventID),
			TicketID:  id,
			Kind:      model.TicketEventKindReset,
			Actor:     actor,
			EventTime: resetTime,
			CreatedAt: s.clock.Now(),
		}
		resetEvent.SetPayload(model.TicketEventKindReset, model.TicketEventBody{Reset: resetPayload})

		if err := txDB.WithContext(ctx).Create(resetEvent).Error; err != nil {
			return err
		}

		resetResult = reset
		return nil
	})
	if err != nil {
		return nil, err
	}

	return resetResult, nil
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
	case model.TicketEventKindReset:
		if body.Reset == nil {
			return ErrInvalidEventPayload
		}
		if body.Reset.ResetID == "" {
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

func resetMetadataChanged(baselineID *model.TicketResetID, baselineAt *time.Time, latest *model.TicketReset) bool {
	if latest == nil {
		return false
	}
	if baselineID == nil || baselineAt == nil {
		return true
	}
	if *baselineID != latest.ID {
		return true
	}
	return !latest.CreatedAt.UTC().Equal(baselineAt.UTC())
}
