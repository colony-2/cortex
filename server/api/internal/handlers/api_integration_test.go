package handlers

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeGraphBuilder returns a static graph for testing.
type fakeGraphBuilder struct {
	graph *core.Graph
}

func (f *fakeGraphBuilder) BuildGraph(ctx context.Context) (*core.Graph, error) {
	return f.graph, nil
}

func (f *fakeGraphBuilder) GetCell(ctx context.Context, cellID string) (*core.Cell, error) {
	for _, c := range f.graph.Cells {
		if c.ID == cellID {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func TestOpenAPIIntegration_ProjectCellTicketFlow(t *testing.T) {
	// SQLite in-memory DB shared across services.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
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

	ticketStore, err := ticket.NewStore(db)
	if err != nil {
		t.Fatalf("ticket store: %v", err)
	}
	eventStore, err := ticket.NewEventStore(db)
	if err != nil {
		t.Fatalf("ticket event store: %v", err)
	}
	ticketSvc, err := ticket.NewService(ticket.ServiceConfig{
		Store:      ticketStore,
		EventStore: eventStore,
		Projects:   projectSvc,
		Cells:      cellSvc,
	})
	if err != nil {
		t.Fatalf("ticket service: %v", err)
	}

	// Fake graph with a single cell.
	graphBuilder := &fakeGraphBuilder{
		graph: &core.Graph{
			Cells: []core.Cell{
				{ID: "g1", Name: "graph-cell", Path: "/tmp", Type: "fake", Dependencies: []string{}},
			},
			Edges: nil,
		},
	}

	h := New(graphBuilder, nil, nil, projectSvc, cellSvc, ticketSvc, nil, nil, cellStore)
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	client, err := openapi.NewClientWithResponses(srv.URL, openapi.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx := context.Background()

	// Create project
	projResp, err := client.PostApiProjectsWithResponse(ctx, openapi.ProjectCreateRequest{
		Name:        "proj",
		GitRepoPath: "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projResp.JSON201 == nil {
		t.Fatalf("expected project JSON201, got %#v", projResp)
	}
	projID := projResp.JSON201.Id

	// List projects
	listProj, err := client.GetApiProjectsWithResponse(ctx, nil)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if listProj.JSON200 == nil || len(*listProj.JSON200) != 1 {
		t.Fatalf("expected 1 project, got %#v", listProj.JSON200)
	}

	// Create cell
	cellResp, err := client.PostApiProjectsProjectIdCellsWithResponse(ctx, projID, openapi.CellCreateRequest{
		Name:        "cell-a",
		WorkingPath: "/tmp/repo/cell-a",
	})
	if err != nil {
		t.Fatalf("create cell: %v", err)
	}
	if cellResp.JSON201 == nil {
		t.Fatalf("expected cell JSON201, got %#v", cellResp)
	}
	createdCellID := cellResp.JSON201.Id
	// List cells
	cellsResp, err := client.GetApiProjectsProjectIdCellsWithResponse(ctx, projID, nil)
	if err != nil {
		t.Fatalf("list cells: %v", err)
	}
	if cellsResp.JSON200 == nil || len(*cellsResp.JSON200) != 1 {
		t.Fatalf("expected 1 cell, got %#v", cellsResp.JSON200)
	}

	// Graph
	graphResp, err := client.GetApiProjectsProjectIdGraphWithResponse(ctx, projID)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if graphResp.JSON200 == nil || len(graphResp.JSON200.Cells) == 0 {
		t.Fatalf("expected graph cells, got %#v", graphResp.JSON200)
	}
	var foundCreated bool
	for _, gc := range graphResp.JSON200.Cells {
		if gc.Id == createdCellID {
			foundCreated = true
			if gc.Name != "cell-a" {
				t.Fatalf("expected graph cell name to match created cell, got %q", gc.Name)
			}
		}
	}
	if !foundCreated {
		t.Fatalf("graph response missing created cell %s in %#v", createdCellID, graphResp.JSON200.Cells)
	}

	// Create ticket
	ticketResp, err := client.PostApiProjectsProjectIdTicketsWithResponse(ctx, projID, openapi.TicketCreateRequest{
		Actor: openapi.Actor{
			Type: openapi.User,
			User: &openapi.ActorUser{Email: openapi_types.Email("user@example.com")},
		},
		Cell:        "cell-a",
		Stage:       "triage",
		State:       openapi.Working,
		Title:       "test ticket",
		Description: nil,
	})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if ticketResp.JSON201 == nil {
		t.Fatalf("expected ticket JSON201, got %#v", ticketResp)
	}
	ticketID := ticketResp.JSON201.Id
	version := ticketResp.JSON201.Version

	// Get ticket at now
	_, err = client.GetApiProjectsProjectIdTicketsTicketIdAtWithResponse(ctx, projID, ticketID, &openapi.GetApiProjectsProjectIdTicketsTicketIdAtParams{
		At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("get ticket at: %v", err)
	}

	// Update ticket
	stage := "review"
	expVersion := version
	patchResp, err := client.PatchApiProjectsProjectIdTicketsTicketIdWithResponse(ctx, projID, ticketID, openapi.TicketUpdateRequest{
		ExpectedVersion: &expVersion,
		Stage:           &stage,
	})
	if err != nil {
		t.Fatalf("patch ticket: %v", err)
	}
	if patchResp.JSON200 == nil || patchResp.JSON200.Stage != "review" {
		t.Fatalf("expected stage update, got %#v", patchResp.JSON200)
	}
}
