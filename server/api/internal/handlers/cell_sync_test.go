package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/registry/pkg/registry"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type staticGraphBuilder struct {
	graph *core.Graph
}

func (s *staticGraphBuilder) BuildGraph(ctx context.Context) (*core.Graph, error) {
	return s.graph, nil
}

func (s *staticGraphBuilder) GetCell(ctx context.Context, cellID string) (*core.Cell, error) {
	for _, c := range s.graph.Cells {
		if c.ID == cellID {
			return &c, nil
		}
	}
	return nil, nil
}

func TestHandleSyncCells_Success(t *testing.T) {
	dsn := fmt.Sprintf("file:%s-success?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	projectStore, err := project.NewStore(db)
	if err != nil {
		t.Fatalf("project store: %v", err)
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		t.Fatalf("project service: %v", err)
	}

	cellStore, err := cell.NewStore(db)
	if err != nil {
		t.Fatalf("cell store: %v", err)
	}
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	if err != nil {
		t.Fatalf("cell service: %v", err)
	}

	builder := &staticGraphBuilder{
		graph: &core.Graph{
			Cells: []core.Cell{
				{ID: "cells/a", Name: "cells/a", Path: "/repo/cells/a", Dependencies: []string{"cells/b"}},
				{ID: "cells/b", Name: "cells/b", Path: "/repo/cells/b", Dependencies: []string{}},
			},
		},
	}
	graphFactory := func(ctx context.Context, projectID string) (core.GraphBuilder, error) {
		return builder, nil
	}

	noopRecipes := func(ctx context.Context, projectID project.ID, repoPath string) (*registry.Registry, string, func(), error) {
		return nil, repoPath, func() {}, nil
	}

	h := New(graphFactory, noopRecipes, projectSvc, cellSvc, nil, nil, nil, cellStore)
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	client, err := openapi.NewClientWithResponses(srv.URL, openapi.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx := context.Background()
	projResp, err := client.PostApiProjectsWithResponse(ctx, openapi.ProjectCreateRequest{
		Name:        "proj",
		GitRepoPath: "/repo",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projResp.JSON201 == nil {
		t.Fatalf("expected project, got %#v", projResp)
	}
	projID := projResp.JSON201.Id

	syncResp, err := client.PostApiProjectsProjectIdCellsSyncWithResponse(ctx, projID, openapi.CellSyncRequest{})
	if err != nil {
		t.Fatalf("sync cells: %v", err)
	}
	if syncResp.JSON200 == nil {
		t.Fatalf("expected sync JSON200, got %#v", syncResp)
	}
	if syncResp.JSON200.Created != 2 || syncResp.JSON200.DependenciesUpdated != 2 {
		t.Fatalf("unexpected sync counts: %#v", syncResp.JSON200)
	}

	it, err := cellSvc.ListCells(ctx, cell.SearchFilter{ProjectIDs: []project.ID{project.ID(projID)}})
	if err != nil {
		t.Fatalf("list cells: %v", err)
	}
	defer it.Close(ctx)

	nameToID := map[string]cell.ID{}
	for {
		c, err := it.Next(ctx)
		if errors.Is(err, cell.ErrIteratorDone) {
			break
		}
		if err != nil {
			t.Fatalf("iterator: %v", err)
		}
		nameToID[c.Name] = c.ID
	}
	if len(nameToID) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(nameToID))
	}

	deps, err := cellStore.ListDependencies(ctx, project.ID(projID), nameToID["cells/a"])
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(deps) != 1 || deps[0] != nameToID["cells/b"] {
		t.Fatalf("unexpected dependencies: %#v", deps)
	}
}

func TestHandleSyncCells_UnsupportedPopulator(t *testing.T) {
	dsn := fmt.Sprintf("file:%s-unsupported?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	projectStore, _ := project.NewStore(db)
	projectSvc, _ := project.NewService(project.ServiceConfig{Store: projectStore})
	cellStore, _ := cell.NewStore(db)
	cellSvc, _ := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})

	noopRecipes := func(ctx context.Context, projectID project.ID, repoPath string) (*registry.Registry, string, func(), error) {
		return nil, repoPath, func() {}, nil
	}
	noopGraphFactory := func(ctx context.Context, projectID string) (core.GraphBuilder, error) {
		return nil, fmt.Errorf("graph factory not implemented in test")
	}

	h := New(noopGraphFactory, noopRecipes, projectSvc, cellSvc, nil, nil, nil, cellStore)
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	client, err := openapi.NewClientWithResponses(srv.URL, openapi.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx := context.Background()
	projResp, err := client.PostApiProjectsWithResponse(ctx, openapi.ProjectCreateRequest{
		Name:        "proj",
		GitRepoPath: "/repo",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projResp.JSON201 == nil {
		t.Fatalf("expected project, got %#v", projResp)
	}
	projID := projResp.JSON201.Id

	pop := "unknown"
	syncResp, err := client.PostApiProjectsProjectIdCellsSyncWithResponse(ctx, projID, openapi.CellSyncRequest{
		Populator: &pop,
	})
	if err != nil {
		t.Fatalf("sync cells: %v", err)
	}
	if syncResp.StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", syncResp.StatusCode())
	}
}
