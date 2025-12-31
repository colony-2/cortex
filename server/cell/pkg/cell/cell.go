package cell

import (
	"github.com/colony-2/colony2/server/cell/internal/idgen"
	"github.com/colony-2/colony2/server/cell/internal/model"
	"github.com/colony-2/colony2/server/cell/internal/service"
	"github.com/colony-2/colony2/server/cell/internal/store"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"gorm.io/gorm"
)

type (
	ID               = model.ID
	Cell             = model.Cell
	Dependency       = model.CellDependency
	SearchFilter     = model.SearchFilter
	Service          = service.Service
	ServiceConfig    = service.ServiceConfig
	CreateInput      = service.CreateInput
	UpdateInput      = service.UpdateInput
	Clock            = service.Clock
	Store            = store.Store
	Iterator[T any]  = store.Iterator[T]
	Populator        = service.Populator
	PopulatorCell    = service.PopulatorCell
	SyncOptions      = service.SyncOptions
	SyncResult       = service.SyncResult
	ShortIDGenerator = model.ShortIDGenerator
)

var (
	ErrEmptyName         = service.ErrEmptyName
	ErrEmptyWorkingPath  = service.ErrEmptyWorkingPath
	ErrInvalidProject    = service.ErrInvalidProject
	ErrIDGeneration      = service.ErrIDGeneration
	ErrVersionConflict   = service.ErrVersionConflict
	ErrNotFound          = service.ErrNotFound
	ErrAlreadyDeleted    = service.ErrAlreadyDeleted
	ErrInvalidDependency = service.ErrInvalidDependency
	ErrIteratorDone      = store.ErrIteratorDone
)

func NewService(config ServiceConfig) (Service, error) {
	return service.New(config)
}

func NewServiceFromDB(db *gorm.DB) (Service, error) {
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	projectStore, err := project.NewStore(db)
	if err != nil {
		return nil, err
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		return nil, err
	}
	return NewService(ServiceConfig{Store: store, Projects: projectSvc})
}

func NewStore(db *gorm.DB) (Store, error) {
	return store.New(db)
}

func NewKSUIDGenerator() *idgen.KSUIDGenerator { return idgen.NewKSUIDGenerator() }
