package tickets

import (
	"context"
	"errors"
	"fmt"

	"github.com/divisive-ai/vibethis/server/ticket/internal/model"
	"gorm.io/gorm"
)

type Store interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error
	Create(ctx context.Context, ticket *model.Ticket) error
	Get(ctx context.Context, id model.ID) (*model.Ticket, error)
	Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.Ticket], error)
	SearchStages(ctx context.Context, filter model.SearchFilter) (Iterator[model.Stage], error)
	Update(ctx context.Context, ticket *model.Ticket, fields ...string) error
}

type store struct {
	db *gorm.DB
}

var ErrOptimisticLock = errors.New("tickets store: optimistic lock conflict")

func New(db *gorm.DB) (Store, error) {
	if db == nil {
		return nil, errors.New("tickets store: nil db")
	}
	if err := db.AutoMigrate(&model.Ticket{}); err != nil {
		return nil, fmt.Errorf("tickets store: auto migrate: %w", err)
	}
	return &store{db: db}, nil
}

func (s *store) WithTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		nested := &store{db: tx}
		return fn(ctx, nested)
	})
}

func (s *store) Create(ctx context.Context, ticket *model.Ticket) error {
	return s.db.WithContext(ctx).Create(ticket).Error
}

func (s *store) Get(ctx context.Context, id model.ID) (*model.Ticket, error) {
	var ticket model.Ticket
	err := s.db.WithContext(ctx).First(&ticket, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (s *store) Update(ctx context.Context, ticket *model.Ticket, fields ...string) error {
	tx := s.db.WithContext(ctx)
	if len(fields) > 0 {
		tx = tx.Select(fields)
	}
	result := tx.Model(ticket).Updates(ticket)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

func (s *store) Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.Ticket], error) {
	var tickets []*model.Ticket
	query := applyFilter(s.db.WithContext(ctx), filter)
	if err := query.Order("updated_at DESC").Find(&tickets).Error; err != nil {
		return nil, err
	}
	return newSliceIterator(tickets), nil
}

func (s *store) SearchStages(ctx context.Context, filter model.SearchFilter) (Iterator[model.Stage], error) {
	var stages []model.Stage
	query := applyFilter(s.db.WithContext(ctx).Model(&model.Ticket{}), filter)
	if err := query.Select("DISTINCT stage").Order("stage ASC").Pluck("stage", &stages).Error; err != nil {
		return nil, err
	}
	return newSliceIterator(stages), nil
}

func applyFilter(db *gorm.DB, filter model.SearchFilter) *gorm.DB {
	if db == nil {
		return db
	}
	if len(filter.StageAny) > 0 {
		db = db.Where("stage IN ?", filter.StageAny)
	}
	if len(filter.StageNotIn) > 0 {
		db = db.Where("stage NOT IN ?", filter.StageNotIn)
	}
	if len(filter.States) > 0 {
		db = db.Where("state IN ?", filter.States)
	}
	if len(filter.Actors) > 0 {
		db = db.Where("creator_type IN ?", filter.Actors)
	}
	if len(filter.Cells) > 0 {
		db = db.Where("cell_name IN ?", filter.Cells)
	}
	if filter.UpdatedAfter != nil {
		db = db.Where("updated_at >= ?", *filter.UpdatedAfter)
	}
	if filter.UpdatedBefore != nil {
		db = db.Where("updated_at <= ?", *filter.UpdatedBefore)
	}
	if filter.CreatedAfter != nil {
		db = db.Where("created_at >= ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		db = db.Where("created_at <= ?", *filter.CreatedBefore)
	}
	return db
}
