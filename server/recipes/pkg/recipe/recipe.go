package recipe

import (
	"github.com/colony-2/colony2/server/recipes/internal/idgen"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/service"
	"github.com/colony-2/colony2/server/recipes/internal/store"
	"gorm.io/gorm"
)

// Type aliases for public API
type (
	// Service types
	Service       = service.Service
	ServiceConfig = service.ServiceConfig

	// Model types
	ID            = model.ID
	CreateInput   = model.CreateInput
	UpdateInput   = model.UpdateInput
	PublishInput  = model.PublishInput
	UnpublishInput = model.UnpublishInput

	RecipeVersion     = model.RecipeVersion
	RecipeInfo        = model.RecipeInfo
	RecipeWithContent = model.RecipeWithContent
	PublishedRecipe   = model.PublishedRecipe

	RecipeFilter  = model.RecipeFilter
	PublishStatus = model.PublishStatus

	Clock             = model.Clock
	ShortIDGenerator  = model.ShortIDGenerator

	// Store types
	Store           = store.Store
	Iterator[T any] = store.Iterator[T]
)

// Constants
const (
	PublishStatusAll         = model.PublishStatusAll
	PublishStatusPublished   = model.PublishStatusPublished
	PublishStatusUnpublished = model.PublishStatusUnpublished
)

// Error variables
var (
	ErrEmptyName       = model.ErrEmptyName
	ErrInvalidName     = model.ErrInvalidName
	ErrInvalidProject  = model.ErrInvalidProject
	ErrNotFound        = model.ErrNotFound
	ErrAlreadyExists   = model.ErrAlreadyExists
	ErrNotPublished    = model.ErrNotPublished
	ErrVersionConflict = model.ErrVersionConflict
	ErrInvalidContent  = model.ErrInvalidContent
	ErrCommitNotFound  = model.ErrCommitNotFound
	ErrGitConflict     = model.ErrGitConflict
	ErrRemoteSync      = model.ErrRemoteSync
	ErrIteratorDone    = store.ErrIteratorDone // Use store error to match actual Iterator implementation
	ErrOptimisticLock  = model.ErrOptimisticLock
)

// NewService creates a new recipe service.
func NewService(config ServiceConfig) (Service, error) {
	return service.New(config)
}

// NewServiceFromDB creates a new recipe service from a database connection.
func NewServiceFromDB(db *gorm.DB, config ServiceConfig) (Service, error) {
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	config.Store = store
	return NewService(config)
}

// NewStore creates a new recipe store.
func NewStore(db *gorm.DB) (Store, error) {
	return store.New(db)
}

// NewKSUIDGenerator creates a new KSUID generator for recipe IDs.
func NewKSUIDGenerator() *idgen.KSUIDGenerator {
	return idgen.NewKSUIDGenerator()
}

// NewSystemClock creates a new system clock.
func NewSystemClock() Clock {
	return model.SystemClock{}
}
