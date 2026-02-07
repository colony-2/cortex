package store

import (
	"context"
	"errors"

	"github.com/colony-2/colony2/server/recipes/internal/model"
	"gorm.io/gorm"
)

// Store provides database access for the recipe service.
// It is intentionally thin; complex behavior lives in the service layer.
type Store interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, s Store) error) error
	DB() *gorm.DB
}

type store struct {
	db *gorm.DB
}

type Options struct {
	Migrate bool
}

// New creates a new Store instance with migrations enabled.
func New(db *gorm.DB) (Store, error) {
	return NewWithOptions(db, Options{Migrate: true})
}

func NewWithOptions(db *gorm.DB, opts Options) (Store, error) {
	if db == nil {
		return nil, errors.New("recipe store: nil db")
	}
	if opts.Migrate {
		if err := db.AutoMigrate(
			&model.RecipeBlob{},
			&model.RecipeRow{},
			&model.RecipeEvent{},
		); err != nil {
			return nil, err
		}
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
