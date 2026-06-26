package cortex

import (
	"context"
	"errors"
	"strings"

	configpkg "github.com/colony-2/c2j/pkg/config"
	"github.com/colony-2/c2j/pkg/recipejob"
	storyapi "github.com/colony-2/c2j/pkg/story/api"
	"github.com/colony-2/c2j/pkg/worker/compiler"
)

type Cell struct {
	ID               string `json:"id"`
	ProjectID        string `json:"project_id"`
	TenantID         string `json:"tenant_id"`
	Name             string `json:"name"`
	Repo             string `json:"repo"`
	RepositorySource string `json:"repository_source"`
	GitRef           string `json:"git_ref,omitempty"`
	Kind             string `json:"kind"`
}

type cellCatalog struct {
	workingDir       string
	defaultTenantID  string
	defaultProjectID string
}

func (c *cellCatalog) Cells(ctx context.Context, tenantID string) ([]Cell, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = c.defaultTenantID
	}

	cfg, err := configpkg.LoadProjectConfig(c.workingDir)
	if err != nil {
		return nil, err
	}

	out := make([]Cell, 0, 8)
	seen := map[string]bool{}
	add := func(repo string, ref string, kind string) {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			return
		}
		source, err := recipejob.NormalizeRepositorySource(repo)
		if err != nil {
			source = repo
		}
		key := source
		if seen[key] {
			return
		}
		seen[key] = true

		name, ok := cfg.CellNameFromRepo(ctx, repo)
		if !ok || strings.TrimSpace(name) == "" {
			name = compiler.RepositoryNameFromSource(repo)
		}
		if strings.TrimSpace(name) == "" {
			name = compiler.RepositoryNameFromSource(source)
		}
		out = append(out, Cell{
			ID:               source,
			ProjectID:        tenantID,
			TenantID:         tenantID,
			Name:             name,
			Repo:             repo,
			RepositorySource: source,
			GitRef:           strings.TrimSpace(ref),
			Kind:             kind,
		})
	}

	selfRepo, err := cfg.SelfRepo(ctx)
	if err != nil {
		return nil, err
	}
	selfRef, err := cfg.SelfRef(ctx)
	if err != nil {
		return nil, err
	}
	add(selfRepo, selfRef, "self")

	dependents, err := cfg.AllowedDependentRepos(ctx)
	if err != nil {
		return nil, err
	}
	for _, repo := range dependents {
		ref, err := defaultRefForRepo(ctx, cfg, repo)
		if err != nil {
			return nil, err
		}
		add(repo, ref, "dependent")
	}
	return out, nil
}

func (c *cellCatalog) SelfCell(ctx context.Context, tenantID string) (Cell, error) {
	cells, err := c.Cells(ctx, tenantID)
	if err != nil {
		return Cell{}, err
	}
	for _, cell := range cells {
		if cell.Kind == "self" {
			return cell, nil
		}
	}
	return Cell{}, storyapi.ErrCellNotFound
}

func (c *cellCatalog) GetCell(ctx context.Context, cellID string) (*storyapi.Cell, error) {
	cellID = strings.TrimSpace(cellID)
	if cellID == "" {
		return nil, storyapi.ErrCellNotFound
	}
	cells, err := c.Cells(ctx, c.defaultProjectID)
	if err != nil {
		if errors.Is(err, configpkg.ErrConfigNotFound) {
			return nil, storyapi.ErrCellNotFound
		}
		return nil, err
	}
	for _, cell := range cells {
		if cell.ID == cellID || cell.Name == cellID || cell.RepositorySource == cellID || cell.Repo == cellID {
			return toStoryCell(cell), nil
		}
	}
	return nil, storyapi.ErrCellNotFound
}

func (c *cellCatalog) ListCells(ctx context.Context, filter storyapi.SearchFilter) (storyapi.CellIterator, error) {
	projectID := c.defaultProjectID
	if len(filter.ProjectIDs) > 0 && strings.TrimSpace(filter.ProjectIDs[0]) != "" {
		projectID = strings.TrimSpace(filter.ProjectIDs[0])
	}
	cells, err := c.Cells(ctx, projectID)
	if err != nil {
		if errors.Is(err, configpkg.ErrConfigNotFound) {
			return &cellIterator{}, nil
		}
		return nil, err
	}

	names := make(map[string]bool, len(filter.Names))
	for _, name := range filter.Names {
		name = strings.TrimSpace(name)
		if name != "" {
			names[name] = true
		}
	}
	out := make([]*storyapi.Cell, 0, len(cells))
	for _, cell := range cells {
		if len(names) > 0 && !names[cell.Name] && !names[cell.ID] && !names[cell.RepositorySource] {
			continue
		}
		out = append(out, toStoryCell(cell))
	}
	return &cellIterator{cells: out}, nil
}

type cellIterator struct {
	cells []*storyapi.Cell
	index int
}

func (i *cellIterator) Next(context.Context) (*storyapi.Cell, error) {
	if i == nil || i.index >= len(i.cells) {
		return nil, storyapi.ErrCellNotFound
	}
	cell := i.cells[i.index]
	i.index++
	return cell, nil
}

func (i *cellIterator) Close(context.Context) error {
	return nil
}

func toStoryCell(cell Cell) *storyapi.Cell {
	repo := cell.RepositorySource
	ref := cell.GitRef
	return &storyapi.Cell{
		ID:          cell.ID,
		ProjectID:   cell.ProjectID,
		Name:        cell.Name,
		GitRepoName: &repo,
		GitBranch:   &ref,
	}
}

func defaultRefForRepo(ctx context.Context, cfg *configpkg.ProjectConfig, repo string) (string, error) {
	rootRepo, err := cfg.RootRepo(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rootRepo) != "" {
		equal, err := recipejob.RepositorySourcesEqual(rootRepo, repo)
		if err == nil && equal {
			return cfg.RootRef(ctx)
		}
	}

	selfRepo, err := cfg.SelfRepo(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(selfRepo) != "" {
		equal, err := recipejob.RepositorySourcesEqual(selfRepo, repo)
		if err == nil && equal {
			return cfg.SelfRef(ctx)
		}
	}

	return compiler.DefaultRecipeRef, nil
}
