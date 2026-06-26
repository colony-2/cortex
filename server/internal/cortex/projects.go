package cortex

import (
	"context"
	"errors"
	"strings"

	configpkg "github.com/colony-2/c2j/pkg/config"
	storyapi "github.com/colony-2/c2j/pkg/story/api"
)

type Project struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
}

type tenantProjectService struct {
	cells           *cellCatalog
	defaultTenantID string
}

func (s *tenantProjectService) GetProject(ctx context.Context, projectID string) (*storyapi.Project, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = s.defaultTenantID
	}
	project := &storyapi.Project{
		ID: projectID,
	}
	if s.cells != nil {
		cell, err := s.cells.SelfCell(ctx, projectID)
		if err == nil {
			project.GitRepoPath = cell.RepositorySource
			if strings.TrimSpace(cell.GitRef) != "" {
				ref := cell.GitRef
				project.GitRepoBranch = &ref
			}
		} else if !errors.Is(err, configpkg.ErrConfigNotFound) && !errors.Is(err, storyapi.ErrCellNotFound) {
			return nil, err
		}
	}
	return project, nil
}
