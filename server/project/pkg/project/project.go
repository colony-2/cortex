package project

import (
	"github.com/colony-2/colony2/server/project/internal/idgen"
	"github.com/colony-2/colony2/server/project/internal/model"
	"github.com/colony-2/colony2/server/project/internal/service"
	"github.com/colony-2/colony2/server/project/internal/store"
	"gorm.io/gorm"
)

type (
	ID              = model.ID
	Project         = model.Project
	SearchFilter    = model.SearchFilter
	Service         = service.Service
	ServiceConfig   = service.ServiceConfig
	CreateInput     = service.CreateInput
	UpdateInput     = service.UpdateInput
	Clock           = service.Clock
	Store           = store.Store
	Iterator[T any] = store.Iterator[T]
)

var (
	ErrEmptyName       = service.ErrEmptyName
	ErrEmptyGitRepo    = service.ErrEmptyGitRepo
	ErrIDGeneration    = service.ErrIDGeneration
	ErrVersionConflict = service.ErrVersionConflict
	ErrNotFound        = service.ErrNotFound
	ErrIteratorDone    = store.ErrIteratorDone
)

func NewService(config ServiceConfig) (Service, error) {
	return service.New(config)
}

func NewServiceFromDB(db *gorm.DB) (Service, error) {
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	return NewService(ServiceConfig{Store: store})
}

func NewStore(db *gorm.DB) (Store, error) {
	return store.New(db)
}

func NewKSUIDGenerator() *idgen.KSUIDGenerator { return idgen.NewKSUIDGenerator() }
