package store

import (
	"context"
	"errors"
	"strings"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"gorm.io/gorm"
)

// Store provides database persistence for published recipes.
type Store interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, s Store) error) error
	Create(ctx context.Context, recipe *model.PublishedRecipe) error
	GetByID(ctx context.Context, id model.ID) (*model.PublishedRecipe, error)
	GetByName(ctx context.Context, projectID project.ID, name string) (*model.PublishedRecipe, error)
	Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.PublishedRecipe], error)
	Update(ctx context.Context, recipe *model.PublishedRecipe) error
	Delete(ctx context.Context, id model.ID) error
	DB() *gorm.DB
}

type store struct {
	db *gorm.DB
}

var ErrOptimisticLock = errors.New("recipe store: optimistic lock conflict")

// New creates a new Store instance with the given database connection.
func New(db *gorm.DB) (Store, error) {
	if db == nil {
		return nil, errors.New("recipe store: nil db")
	}
	if err := db.AutoMigrate(&model.PublishedRecipe{}); err != nil {
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

func (s *store) Create(ctx context.Context, recipe *model.PublishedRecipe) error {
	return s.db.WithContext(ctx).Create(recipe).Error
}

func (s *store) GetByID(ctx context.Context, id model.ID) (*model.PublishedRecipe, error) {
	var recipe model.PublishedRecipe
	err := s.db.WithContext(ctx).First(&recipe, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	normalizeTimes(&recipe)
	return &recipe, nil
}

func (s *store) GetByName(ctx context.Context, projectID project.ID, name string) (*model.PublishedRecipe, error) {
	var recipe model.PublishedRecipe
	err := s.db.WithContext(ctx).
		Where("project_id = ? AND name = ?", projectID, name).
		First(&recipe).Error
	if err != nil {
		return nil, err
	}
	normalizeTimes(&recipe)
	return &recipe, nil
}

func (s *store) Search(ctx context.Context, filter model.SearchFilter) (Iterator[*model.PublishedRecipe], error) {
	var recipes []*model.PublishedRecipe
	query := applyFilter(s.db.WithContext(ctx), filter)
	if err := query.Order("name ASC").Find(&recipes).Error; err != nil {
		return nil, err
	}
	for _, r := range recipes {
		normalizeTimes(r)
	}
	return newSliceIterator(recipes), nil
}

func (s *store) Update(ctx context.Context, recipe *model.PublishedRecipe) error {
	result := s.db.WithContext(ctx).Model(recipe).Updates(recipe)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

func (s *store) Delete(ctx context.Context, id model.ID) error {
	result := s.db.WithContext(ctx).Delete(&model.PublishedRecipe{}, "id = ?", id)
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
	if len(filter.ProjectIDs) > 0 {
		db = db.Where("project_id IN ?", filter.ProjectIDs)
	}
	if len(filter.Names) > 0 {
		db = db.Where("name IN ?", filter.Names)
	}
	if trimmed := strings.TrimSpace(filter.NamePrefix); trimmed != "" {
		// Use LIKE for hierarchical prefix matching (e.g., "foo/bar/")
		db = db.Where("name LIKE ?", trimmed+"%")
	}
	if filter.Limit > 0 {
		db = db.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		db = db.Offset(filter.Offset)
	}
	return db
}

func normalizeTimes(recipe *model.PublishedRecipe) {
	if recipe == nil {
		return
	}
	recipe.CreatedAt = recipe.CreatedAt.UTC()
	recipe.UpdatedAt = recipe.UpdatedAt.UTC()
	recipe.PublishedAt = recipe.PublishedAt.UTC()
}

func applyConstraints(db *gorm.DB) error {
	// No additional constraints needed - GORM auto-migrates the uniqueIndex from struct tags
	return nil
}
