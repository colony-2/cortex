package events

import (
	"context"
	"errors"
	"fmt"

	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	ticketstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
	"gorm.io/gorm"
)

type Store interface {
	Append(ctx context.Context, event *model.TicketEvent) error
	AppendBatch(ctx context.Context, ticketID model.ID, events []*model.TicketEvent) error
	ListByTicket(ctx context.Context, ticketID model.ID, filter model.TicketEventFilter) (ticketstore.Iterator[*model.TicketEvent], error)
	MarkReset(ctx context.Context, ticketID model.ID, reset *model.TicketReset, eventIDs []model.TicketEventID) error
	LatestReset(ctx context.Context, ticketID model.ID) (*model.TicketReset, error)
}

type store struct {
	db *gorm.DB
}

func New(db *gorm.DB) (Store, error) {
	if db == nil {
		return nil, errors.New("ticket events store: nil db")
	}
	if err := db.AutoMigrate(&model.TicketEvent{}, &model.TicketReset{}); err != nil {
		return nil, fmt.Errorf("ticket events store: auto migrate: %w", err)
	}
	return &store{db: db}, nil
}

func NewWithDB(db *gorm.DB) (Store, error) {
	if db == nil {
		return nil, errors.New("ticket events store: nil db")
	}
	return &store{db: db}, nil
}

func (s *store) Append(ctx context.Context, event *model.TicketEvent) error {
	if event == nil {
		return errors.New("ticket events store: nil event")
	}
	if event.PayloadType == "" {
		event.PayloadType = model.TicketEventPayloadType(event.Kind)
	}
	return s.db.WithContext(ctx).Create(event).Error
}

func (s *store) AppendBatch(ctx context.Context, ticketID model.ID, events []*model.TicketEvent) error {
	if len(events) == 0 {
		return nil
	}
	for _, evt := range events {
		if evt != nil && evt.TicketID == "" {
			evt.TicketID = ticketID
		}
		if evt != nil && evt.PayloadType == "" {
			evt.PayloadType = model.TicketEventPayloadType(evt.Kind)
		}
	}
	return s.db.WithContext(ctx).Create(&events).Error
}

func (s *store) ListByTicket(ctx context.Context, ticketID model.ID, filter model.TicketEventFilter) (ticketstore.Iterator[*model.TicketEvent], error) {
	var events []*model.TicketEvent
	query := s.db.WithContext(ctx).Model(&model.TicketEvent{}).Where("ticket_events.ticket_id = ?", ticketID)
	query = applyFilter(query, filter)
	if err := query.Order("event_time ASC, id ASC").Find(&events).Error; err != nil {
		return nil, err
	}
	for _, evt := range events {
		if evt != nil {
			evt.HydratePayload()
		}
	}
	return newSliceIterator(events), nil
}

func (s *store) MarkReset(ctx context.Context, ticketID model.ID, reset *model.TicketReset, eventIDs []model.TicketEventID) error {
	if reset == nil {
		return errors.New("ticket events store: nil reset")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(reset).Error; err != nil {
			return err
		}
		if len(eventIDs) == 0 {
			return nil
		}
		result := tx.Model(&model.TicketEvent{}).
			Where("ticket_id = ?", ticketID).
			Where("id IN ?", eventIDs).
			Where("reset_id IS NULL").
			Updates(map[string]any{"reset_id": reset.ID})
		return result.Error
	})
}

func (s *store) LatestReset(ctx context.Context, ticketID model.ID) (*model.TicketReset, error) {
	if s == nil {
		return nil, errors.New("ticket events store: nil store")
	}
	var reset model.TicketReset
	query := s.db.WithContext(ctx).
		Model(&model.TicketReset{}).
		Select("id", "ticket_id", "created_at").
		Where("ticket_id = ?", ticketID).
		Order("created_at DESC").
		Limit(1)
	if err := query.Take(&reset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &reset, nil
}

func applyFilter(db *gorm.DB, filter model.TicketEventFilter) *gorm.DB {
	if filter.At != nil {
		at := *filter.At
		db = db.Select("ticket_events.*").Joins("LEFT JOIN ticket_resets ON ticket_resets.id = ticket_events.reset_id")
		db = db.Where("ticket_events.event_time <= ?", at)
		db = db.Where("ticket_events.reset_id IS NULL OR ticket_resets.created_at > ?", at)
	} else if !filter.IncludeReset {
		db = db.Where("reset_id IS NULL")
	}
	if len(filter.Kinds) > 0 {
		db = db.Where("ticket_events.kind IN ?", filter.Kinds)
	}
	if len(filter.PayloadTypes) > 0 {
		db = db.Where("ticket_events.payload_type IN ?", filter.PayloadTypes)
	}
	if filter.Since != nil {
		db = db.Where("ticket_events.event_time >= ?", *filter.Since)
	}
	if filter.Until != nil {
		db = db.Where("ticket_events.event_time <= ?", *filter.Until)
	}
	if len(filter.Types) > 0 {
		db = db.Where(
			"(workflow_type <> '' AND workflow_type IN ?) OR (markdown_type <> '' AND markdown_type IN ?) OR (changeset_type <> '' AND changeset_type IN ?)",
			filter.Types, filter.Types, filter.Types,
		)
	}
	return db
}
