package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/cell/internal/idgen"
	"github.com/divisive-ai/vibethis/server/cell/internal/model"
	"github.com/divisive-ai/vibethis/server/cell/internal/store"
	"github.com/divisive-ai/vibethis/server/project/pkg/project"
	"gorm.io/gorm"
)

var (
	ErrEmptyName         = errors.New("cell: name is required")
	ErrEmptyWorkingPath  = errors.New("cell: working path is required")
	ErrInvalidProject    = errors.New("cell: project not found")
	ErrIDGeneration      = errors.New("cell: id generation failed")
	ErrVersionConflict   = errors.New("cell: version conflict")
	ErrNotFound          = errors.New("cell: not found")
	ErrAlreadyDeleted    = errors.New("cell: already deleted")
	ErrInvalidDependency = errors.New("cell: invalid dependency")
)

type Clock interface {
	Now() time.Time
}

type Service interface {
	CreateCell(ctx context.Context, input CreateInput) (*model.Cell, error)
	GetCell(ctx context.Context, id model.ID) (*model.Cell, error)
	ListCells(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Cell], error)
	UpdateCell(ctx context.Context, id model.ID, patch UpdateInput) (*model.Cell, error)
	MarkDeleted(ctx context.Context, id model.ID) error
	ReplaceDependencies(ctx context.Context, id model.ID, deps []model.ID) error
	SyncFromPopulator(ctx context.Context, projectID project.ID, pop Populator, opts SyncOptions) (*SyncResult, error)
}

type ServiceConfig struct {
	Store         store.Store
	Clock         Clock
	IDGen         model.ShortIDGenerator
	Projects      project.Service
	ProjectsStore project.Store
}

type Populator interface {
	Name() string
	Populate(ctx context.Context, projectID project.ID) ([]PopulatorCell, error)
}

type PopulatorCell struct {
	Name         string
	Description  string
	WorkingPath  string
	ExternalID   string
	Dependencies []string
}

type service struct {
	store     store.Store
	clock     Clock
	idGen     model.ShortIDGenerator
	projects  project.Service
	projStore project.Store
}

func New(config ServiceConfig) (Service, error) {
	if config.Store == nil {
		return nil, errors.New("cell service: store is required")
	}
	if config.Clock == nil {
		config.Clock = systemClock{}
	}
	if config.IDGen == nil {
		config.IDGen = idgen.NewKSUIDGenerator()
	}
	return &service{
		store:     config.Store,
		clock:     config.Clock,
		idGen:     config.IDGen,
		projects:  config.Projects,
		projStore: config.ProjectsStore,
	}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type CreateInput struct {
	ProjectID   project.ID
	Name        string
	Description string
	WorkingPath string
	Populator   string
	PopulatorID string
}

type UpdateInput struct {
	Name        *string
	Description *string
	WorkingPath *string
}

type SyncOptions struct {
	PruneMissing bool
}

type SyncResult struct {
	Created             int
	Updated             int
	Restored            int
	Deleted             int
	Skipped             int
	DependenciesUpdated int
	AffectedIDs         []model.ID
}

func (s *service) CreateCell(ctx context.Context, input CreateInput) (*model.Cell, error) {
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	path := strings.TrimSpace(input.WorkingPath)
	if name == "" {
		return nil, ErrEmptyName
	}
	if path == "" {
		return nil, ErrEmptyWorkingPath
	}
	id, err := s.idGen.NewID()
	if err != nil {
		return nil, errors.Join(ErrIDGeneration, err)
	}
	now := s.clock.Now()
	cell := &model.Cell{
		ID:          model.ID(id),
		ProjectID:   input.ProjectID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		WorkingPath: path,
		Populator:   strings.TrimSpace(input.Populator),
		PopulatorID: strings.TrimSpace(input.PopulatorID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.Create(ctx, cell); err != nil {
		return nil, err
	}
	return cell, nil
}

func (s *service) GetCell(ctx context.Context, id model.ID) (*model.Cell, error) {
	cell, err := s.store.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return cell, err
}

func (s *service) ListCells(ctx context.Context, filter model.SearchFilter) (store.Iterator[*model.Cell], error) {
	return s.store.Search(ctx, filter)
}

func (s *service) UpdateCell(ctx context.Context, id model.ID, patch UpdateInput) (*model.Cell, error) {
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
	if patch.Description != nil {
		existing.Description = strings.TrimSpace(*patch.Description)
		updated = true
	}
	if patch.WorkingPath != nil {
		path := strings.TrimSpace(*patch.WorkingPath)
		if path == "" {
			return nil, ErrEmptyWorkingPath
		}
		existing.WorkingPath = path
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

func (s *service) MarkDeleted(ctx context.Context, id model.ID) error {
	cell, err := s.store.GetIncludingDeleted(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if cell.DeletedAt != nil {
		return nil
	}
	err = s.store.SoftDelete(ctx, id, s.clock.Now())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func (s *service) ReplaceDependencies(ctx context.Context, id model.ID, deps []model.ID) error {
	cell, err := s.store.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	unique := make(map[model.ID]struct{}, len(deps))
	ordered := make([]model.ID, 0, len(deps))
	for _, dep := range deps {
		if dep == "" || dep == id {
			continue
		}
		if _, seen := unique[dep]; seen {
			continue
		}
		target, err := s.store.Get(ctx, dep)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidDependency
		}
		if err != nil {
			return err
		}
		if target.ProjectID != cell.ProjectID {
			return ErrInvalidDependency
		}
		unique[dep] = struct{}{}
		ordered = append(ordered, dep)
	}
	return s.store.ReplaceDependencies(ctx, cell.ProjectID, id, ordered)
}

func (s *service) SyncFromPopulator(ctx context.Context, projectID project.ID, pop Populator, opts SyncOptions) (*SyncResult, error) {
	if pop == nil {
		return nil, errors.New("cell: populator is required")
	}
	if err := s.ensureProject(ctx, projectID); err != nil {
		return nil, err
	}
	popName := strings.TrimSpace(pop.Name())
	if popName == "" {
		return nil, errors.New("cell: populator name is required")
	}
	rawCells, err := pop.Populate(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("populator %s: %w", popName, err)
	}
	clean := make([]PopulatorCell, 0, len(rawCells))
	for _, pc := range rawCells {
		clean = append(clean, PopulatorCell{
			Name:         strings.TrimSpace(pc.Name),
			Description:  strings.TrimSpace(pc.Description),
			WorkingPath:  strings.TrimSpace(pc.WorkingPath),
			ExternalID:   strings.TrimSpace(pc.ExternalID),
			Dependencies: trimStrings(pc.Dependencies),
		})
	}

	result := &SyncResult{}
	err = s.store.WithTx(ctx, func(ctx context.Context, txStore store.Store) error {
		existingIt, err := txStore.Search(ctx, model.SearchFilter{
			ProjectIDs:     []project.ID{projectID},
			IncludeDeleted: true,
		})
		if err != nil {
			return err
		}
		defer existingIt.Close(ctx)

		existingByID := make(map[model.ID]*model.Cell)
		existingByName := make(map[string]*model.Cell)
		existingByPop := make(map[string]*model.Cell)
		for {
			c, err := existingIt.Next(ctx)
			if errors.Is(err, store.ErrIteratorDone) {
				break
			}
			if err != nil {
				return err
			}
			existingByID[c.ID] = c
			existingByName[c.Name] = c
			if c.Populator != "" && c.PopulatorID != "" {
				existingByPop[popKey(c.Populator, c.PopulatorID)] = c
			}
		}

		now := s.clock.Now()
		seen := make(map[model.ID]bool)
		nameToID := make(map[string]model.ID)

		for _, pc := range clean {
			if pc.Name == "" {
				return ErrEmptyName
			}
			if pc.WorkingPath == "" {
				return ErrEmptyWorkingPath
			}

			var matched *model.Cell
			if pc.ExternalID != "" {
				matched = existingByPop[popKey(popName, pc.ExternalID)]
			}
			if matched == nil {
				matched = existingByName[pc.Name]
			}

			if matched == nil {
				id, err := s.idGen.NewID()
				if err != nil {
					return errors.Join(ErrIDGeneration, err)
				}
				newCell := &model.Cell{
					ID:          model.ID(id),
					ProjectID:   projectID,
					Name:        pc.Name,
					Description: pc.Description,
					WorkingPath: pc.WorkingPath,
					Populator:   popName,
					PopulatorID: pc.ExternalID,
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := txStore.Create(ctx, newCell); err != nil {
					return err
				}
				result.Created++
				matched = newCell
				existingByID[newCell.ID] = newCell
			} else {
				wasDeleted := matched.DeletedAt != nil
				matched.Name = pc.Name
				matched.Description = pc.Description
				matched.WorkingPath = pc.WorkingPath
				matched.Populator = popName
				matched.PopulatorID = pc.ExternalID
				matched.DeletedAt = nil
				matched.UpdatedAt = now
				err := txStore.Update(ctx, matched)
				if errors.Is(err, store.ErrOptimisticLock) {
					return ErrVersionConflict
				}
				if err != nil {
					return err
				}
				if wasDeleted {
					result.Restored++
				} else {
					result.Updated++
				}
			}
			seen[matched.ID] = true
			nameToID[matched.Name] = matched.ID
		}

		if opts.PruneMissing {
			for id, cell := range existingByID {
				if cell.ProjectID != projectID || cell.Populator != popName {
					continue
				}
				if seen[id] || cell.DeletedAt != nil {
					continue
				}
				if err := txStore.SoftDelete(ctx, id, now); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				result.Deleted++
				delete(nameToID, cell.Name)
			}
		}

		for _, pc := range clean {
			cellID, ok := nameToID[pc.Name]
			if !ok {
				return fmt.Errorf("cell sync: missing resolved id for %s", pc.Name)
			}
			destSet := make(map[model.ID]struct{})
			destIDs := make([]model.ID, 0, len(pc.Dependencies))
			for _, depName := range pc.Dependencies {
				if depName == "" {
					continue
				}
				depID, ok := nameToID[depName]
				if !ok {
					if existing, found := existingByName[depName]; found && existing.ProjectID == projectID && existing.DeletedAt == nil {
						depID = existing.ID
					} else {
						return fmt.Errorf("cell sync: dependency %s not found", depName)
					}
				}
				if depID == cellID {
					continue
				}
				if _, seen := destSet[depID]; seen {
					continue
				}
				destSet[depID] = struct{}{}
				destIDs = append(destIDs, depID)
			}
			if err := txStore.ReplaceDependencies(ctx, projectID, cellID, destIDs); err != nil {
				return err
			}
			result.DependenciesUpdated++
		}

		result.AffectedIDs = make([]model.ID, 0, len(seen))
		for id := range seen {
			result.AffectedIDs = append(result.AffectedIDs, id)
		}

		return nil
	})
	if errors.Is(err, store.ErrOptimisticLock) {
		return nil, ErrVersionConflict
	}
	return result, err
}

func (s *service) ensureProject(ctx context.Context, id project.ID) error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrInvalidProject
	}
	if s.projects != nil {
		_, err := s.projects.GetProject(ctx, id)
		if errors.Is(err, project.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidProject
		}
		return err
	}
	if s.projStore != nil {
		_, err := s.projStore.Get(ctx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidProject
		}
		return err
	}
	return errors.New("cell service: project service or store is required")
}

func popKey(populator, externalID string) string {
	return populator + "|" + externalID
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
