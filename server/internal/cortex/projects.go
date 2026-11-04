package cortex

import (
	"context"
	"strings"

	storyapi "github.com/colony-2/c2j/pkg/story/api"
)

type Project struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
}

type tenantProjectService struct {
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
	return project, nil
}
