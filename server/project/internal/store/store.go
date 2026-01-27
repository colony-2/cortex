package store

import (
	"context"
	"errors"
	"strings"

	"github.com/colony-2/colony2/server/project/internal/model"
	"gorm.io/gorm"
)

type Store interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error
	Create(ctx context.Context, project *model.Project) error
	Get(ctx context.Context, id model.ID) (*model.Project, error)
	Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.Project], error)
	Update(ctx context.Context, project *model.Project, fields ...string) error
	Delete(ctx context.Context, id model.ID) error
	DB() *gorm.DB
}

type store struct {
	db *gorm.DB
}

var ErrOptimisticLock = errors.New("projects store: optimistic lock conflict")

type Options struct {
	Migrate bool
}

func New(db *gorm.DB) (Store, error) {
	return NewWithOptions(db, Options{Migrate: true})
}

func NewWithOptions(db *gorm.DB, opts Options) (Store, error) {
	if db == nil {
		return nil, errors.New("projects store: nil db")
	}
	if opts.Migrate {
		if err := db.AutoMigrate(&model.Project{}); err != nil {
			return nil, err
		}
	}
	return &store{db: db}, nil
}

func (s *store) WithTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error {
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

func (s *store) Create(ctx context.Context, project *model.Project) error {
	return s.db.WithContext(ctx).Create(project).Error
}

func (s *store) Get(ctx context.Context, id model.ID) (*model.Project, error) {
	var project model.Project
	err := s.db.WithContext(ctx).First(&project, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	normalizeTimes(&project)
	return &project, nil
}

func (s *store) Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.Project], error) {
	var projects []*model.Project
	query := applyFilter(s.db.WithContext(ctx), filter)
	if err := query.Order("created_at ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	for _, project := range projects {
		normalizeTimes(project)
	}
	return newSliceIterator(projects), nil
}

func (s *store) Update(ctx context.Context, project *model.Project, fields ...string) error {
	tx := s.db.WithContext(ctx)
	if len(fields) > 0 {
		tx = tx.Select(fields)
	}
	result := tx.Model(project).Updates(project)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

func (s *store) Delete(ctx context.Context, id model.ID) error {
	result := s.db.WithContext(ctx).Delete(&model.Project{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func applyFilter(db *gorm.DB, filter model.SearchFilter) *gorm.DB {
	if db == nil {
		return db
	}
	if len(filter.IDs) > 0 {
		db = db.Where("id IN ?", filter.IDs)
	}
	if len(filter.Names) > 0 {
		db = db.Where("name IN ?", filter.Names)
	}
	if trimmed := strings.TrimSpace(filter.NameContains); trimmed != "" {
		db = db.Where("name ILIKE ?", "%"+trimmed+"%")
	}
	return db
}

func normalizeTimes(project *model.Project) {
	if project == nil {
		return
	}
	project.CreatedAt = project.CreatedAt.UTC()
	project.UpdatedAt = project.UpdatedAt.UTC()
}
