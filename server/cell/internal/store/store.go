package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/cell/internal/model"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, s Store) error) error
	Create(ctx context.Context, cell *model.Cell) error
	Get(ctx context.Context, id model.ID) (*model.Cell, error)
	GetIncludingDeleted(ctx context.Context, id model.ID) (*model.Cell, error)
	Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.Cell], error)
	Update(ctx context.Context, cell *model.Cell, fields ...string) error
	SoftDelete(ctx context.Context, id model.ID, deletedAt time.Time) error
	ReplaceDependencies(ctx context.Context, projectID project.ID, from model.ID, to []model.ID) error
	ListDependencies(ctx context.Context, projectID project.ID, from model.ID) ([]model.ID, error)
	DB() *gorm.DB
}

type store struct {
	db *gorm.DB
}

var ErrOptimisticLock = errors.New("cells store: optimistic lock conflict")

func New(db *gorm.DB) (Store, error) {
	if db == nil {
		return nil, errors.New("cells store: nil db")
	}
	if err := db.AutoMigrate(&model.Cell{}, &model.CellDependency{}); err != nil {
		return nil, err
	}
	if err := applyConstraints(db); err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) WithTx(ctx context.Context, fn func(ctx context.Context, s Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, &store{db: tx})
	})
}

func (s *store) DB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *store) Create(ctx context.Context, cell *model.Cell) error {
	return s.db.WithContext(ctx).Create(cell).Error
}

func (s *store) Get(ctx context.Context, id model.ID) (*model.Cell, error) {
	return s.get(ctx, id, false)
}

func (s *store) GetIncludingDeleted(ctx context.Context, id model.ID) (*model.Cell, error) {
	return s.get(ctx, id, true)
}

func (s *store) get(ctx context.Context, id model.ID, includeDeleted bool) (*model.Cell, error) {
	var cell model.Cell
	q := s.db.WithContext(ctx)
	if !includeDeleted {
		q = q.Where("deleted_at IS NULL")
	}
	err := q.First(&cell, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	normalizeTimes(&cell)
	return &cell, nil
}

func (s *store) Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.Cell], error) {
	var cells []*model.Cell
	query := applyFilter(s.db.WithContext(ctx), filter)
	if err := query.Order("created_at ASC").Find(&cells).Error; err != nil {
		return nil, err
	}
	for _, c := range cells {
		normalizeTimes(c)
	}
	return newSliceIterator(cells), nil
}

func (s *store) Update(ctx context.Context, cell *model.Cell, fields ...string) error {
	tx := s.db.WithContext(ctx)
	if len(fields) > 0 {
		tx = tx.Select(fields)
	}
	result := tx.Model(cell).Updates(cell)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

func (s *store) SoftDelete(ctx context.Context, id model.ID, deletedAt time.Time) error {
	result := s.db.WithContext(ctx).
		Model(&model.Cell{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{
			"deleted_at": deletedAt,
			"updated_at": deletedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) ReplaceDependencies(ctx context.Context, projectID project.ID, from model.ID, to []model.ID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ? AND from_cell_id = ?", projectID, from).
			Delete(&model.CellDependency{}).Error; err != nil {
			return err
		}
		if len(to) == 0 {
			return nil
		}
		deps := make([]model.CellDependency, 0, len(to))
		now := time.Now().UTC()
		for _, dest := range to {
			deps = append(deps, model.CellDependency{
				ProjectID:  projectID,
				FromCellID: from,
				ToCellID:   dest,
				CreatedAt:  now,
			})
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&deps).Error
	})
}

func (s *store) ListDependencies(ctx context.Context, projectID project.ID, from model.ID) ([]model.ID, error) {
	var deps []model.CellDependency
	err := s.db.WithContext(ctx).
		Where("project_id = ? AND from_cell_id = ?", projectID, from).
		Order("id ASC").
		Find(&deps).Error
	if err != nil {
		return nil, err
	}
	result := make([]model.ID, 0, len(deps))
	for _, d := range deps {
		result = append(result, d.ToCellID)
	}
	return result, nil
}

func applyFilter(db *gorm.DB, filter model.SearchFilter) *gorm.DB {
	if db == nil {
		return db
	}
	if !filter.IncludeDeleted {
		db = db.Where("deleted_at IS NULL")
	}
	if len(filter.IDs) > 0 {
		db = db.Where("id IN ?", filter.IDs)
	}
	if len(filter.ProjectIDs) > 0 {
		db = db.Where("project_id IN ?", filter.ProjectIDs)
	}
	if len(filter.Names) > 0 {
		db = db.Where("name IN ?", filter.Names)
	}
	if trimmed := strings.TrimSpace(filter.NameContains); trimmed != "" {
		db = db.Where("name ILIKE ?", "%"+trimmed+"%")
	}
	if trimmed := strings.TrimSpace(filter.PathPrefix); trimmed != "" {
		db = db.Where("working_path ILIKE ?", trimmed+"%")
	}
	if trimmed := strings.TrimSpace(filter.Populator); trimmed != "" {
		db = db.Where("populator = ?", trimmed)
	}
	if len(filter.PopulatorIDs) > 0 {
		db = db.Where("populator_id IN ?", filter.PopulatorIDs)
	}
	if len(filter.DependsOn) > 0 {
		db = db.Joins("JOIN cell_dependencies cd ON cd.from_cell_id = cells.id").
			Where("cd.to_cell_id IN ?", filter.DependsOn)
	}
	return db
}

func normalizeTimes(cell *model.Cell) {
	if cell == nil {
		return
	}
	cell.CreatedAt = cell.CreatedAt.UTC()
	cell.UpdatedAt = cell.UpdatedAt.UTC()
	if cell.DeletedAt != nil {
		t := cell.DeletedAt.UTC()
		cell.DeletedAt = &t
	}
}

func applyConstraints(db *gorm.DB) error {
	if db == nil {
		return errors.New("cells store: nil db")
	}
	constraints := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_cells_project_name_active ON cells (project_id, name) WHERE deleted_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_cells_project_populator_active ON cells (project_id, populator, populator_id) WHERE deleted_at IS NULL AND populator_id <> ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_cell_deps_unique ON cell_dependencies (project_id, from_cell_id, to_cell_id)`,
	}
	for _, stmt := range constraints {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("apply constraint failed: %w", err)
		}
	}
	return nil
}
