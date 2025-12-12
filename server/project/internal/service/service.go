package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/project/internal/idgen"
	"github.com/divisive-ai/vibethis/server/project/internal/model"
	"github.com/divisive-ai/vibethis/server/project/internal/store"
	"gorm.io/gorm"
)

var (
	ErrEmptyName       = errors.New("project: name is required")
	ErrEmptyGitRepo    = errors.New("project: git repo path is required")
	ErrIDGeneration    = errors.New("project: id generation failed")
	ErrVersionConflict = errors.New("project: version conflict")
	ErrNotFound        = errors.New("project: not found")
)

type Clock interface {
	Now() time.Time
}

type Service interface {
	CreateProject(ctx context.Context, input CreateInput) (*model.Project, error)
	GetProject(ctx context.Context, id model.ID) (*model.Project, error)
	ListProjects(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Project], error)
	UpdateProject(ctx context.Context, id model.ID, patch UpdateInput) (*model.Project, error)
	DeleteProject(ctx context.Context, id model.ID) error
}

type ServiceConfig struct {
	Store store.Store
	Clock Clock
	IDGen model.ShortIDGenerator
}

type service struct {
	store store.Store
	clock Clock
	idGen model.ShortIDGenerator
}

func New(config ServiceConfig) (Service, error) {
	if config.Store == nil {
		return nil, errors.New("project service: store is required")
	}
	if config.Clock == nil {
		config.Clock = systemClock{}
	}
	if config.IDGen == nil {
		config.IDGen = idgen.NewKSUIDGenerator()
	}
	return &service{
		store: config.Store,
		clock: config.Clock,
		idGen: config.IDGen,
	}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type CreateInput struct {
	Name        string
	GitRepoPath string
}

type UpdateInput struct {
	Name        *string
	GitRepoPath *string
}

func (s *service) CreateProject(ctx context.Context, input CreateInput) (*model.Project, error) {
	name := strings.TrimSpace(input.Name)
	repo := strings.TrimSpace(input.GitRepoPath)
	if name == "" {
		return nil, ErrEmptyName
	}
	if repo == "" {
		return nil, ErrEmptyGitRepo
	}
	id, err := s.idGen.NewID()
	if err != nil {
		return nil, errors.Join(ErrIDGeneration, err)
	}
	now := s.clock.Now()
	project := &model.Project{
		ID:          model.ID(id),
		Name:        name,
		GitRepoPath: repo,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.Create(ctx, project); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *service) GetProject(ctx context.Context, id model.ID) (*model.Project, error) {
	project, err := s.store.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return project, err
}

func (s *service) ListProjects(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Project], error) {
	return s.store.Search(ctx, filter)
}

func (s *service) UpdateProject(ctx context.Context, id model.ID, patch UpdateInput) (*model.Project, error) {
	existing, err := s.store.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	updated := false
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return nil, ErrEmptyName
		}
		existing.Name = name
		updated = true
	}
	if patch.GitRepoPath != nil {
		repo := strings.TrimSpace(*patch.GitRepoPath)
		if repo == "" {
			return nil, ErrEmptyGitRepo
		}
		existing.GitRepoPath = repo
		updated = true
	}
	if !updated {
		return existing, nil
	}

	existing.UpdatedAt = s.clock.Now()
	err = s.store.Update(ctx, existing)
	if errors.Is(err, store.ErrOptimisticLock) {
		return nil, ErrVersionConflict
	}
	return existing, err
}

func (s *service) DeleteProject(ctx context.Context, id model.ID) error {
	err := s.store.Delete(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
