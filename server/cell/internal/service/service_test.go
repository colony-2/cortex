package service

import (
	"context"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/cell/internal/model"
	"github.com/colony-2/colony2/server/cell/internal/store"
	"github.com/colony-2/colony2/server/cell/internal/testutil"
	"github.com/colony-2/colony2/server/project/pkg/project"
)

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type fakeIDGen struct {
	ids []string
	i   int
}

func (g *fakeIDGen) NewID() (string, error) {
	if g.i >= len(g.ids) {
		return "", ErrIDGeneration
	}
	id := g.ids[g.i]
	g.i++
	return id, nil
}

type stubPopulator struct {
	name  string
	cells []PopulatorCell
	err   error
}

func (s stubPopulator) Name() string { return s.name }
func (s stubPopulator) Populate(ctx context.Context, projectID project.ID) ([]PopulatorCell, error) {
	return s.cells, s.err
}

func setupService(t *testing.T) (Service, *project.Project, func()) {
	t.Helper()
	pg := testutil.StartEmbeddedPostgres(t)
	closeFn := func() { pg.Close(t) }

	projectStore, err := project.NewStore(pg.DB)
	if err != nil {
		t.Fatalf("project store init: %v", err)
	}
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	if err != nil {
		t.Fatalf("project service init: %v", err)
	}
	cellStore, err := store.New(pg.DB)
	if err != nil {
		t.Fatalf("cell store init: %v", err)
	}

	proj, err := projectSvc.CreateProject(context.Background(), project.CreateInput{
		Name:        "proj-1",
		GitRepoPath: "/repo",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	svc, err := New(ServiceConfig{
		Store:    cellStore,
		Clock:    fixedClock{now: time.Unix(1000, 0).UTC()},
		IDGen:    &fakeIDGen{ids: []string{"000000000000000000000000001", "000000000000000000000000002", "000000000000000000000000003", "000000000000000000000000004"}},
		Projects: projectSvc,
	})
	if err != nil {
		t.Fatalf("service init: %v", err)
	}
	return svc, proj, closeFn
}

func TestCreateGetUpdate(t *testing.T) {
	svc, proj, closeFn := setupService(t)
	defer closeFn()

	ctx := context.Background()
	cell, err := svc.CreateCell(ctx, CreateInput{
		ProjectID:     proj.ID,
		Name:          "cell-a",
		Description:   "desc",
		WorkingPath:   "/repo/cell-a",
		GitRepoName:   "repo-1",
		GitBranch:     "main",
		DefaultRecipe: "build",
	})
	if err != nil {
		t.Fatalf("create cell: %v", err)
	}

	fetched, err := svc.GetCell(ctx, cell.ID)
	if err != nil {
		t.Fatalf("get cell: %v", err)
	}
	if fetched.Name != "cell-a" || fetched.WorkingPath != "/repo/cell-a" {
		t.Fatalf("unexpected fetched cell: %+v", fetched)
	}
	if fetched.GitRepoName == nil || *fetched.GitRepoName != "repo-1" {
		t.Fatalf("git repo not set: %+v", fetched.GitRepoName)
	}
	if fetched.GitBranch == nil || *fetched.GitBranch != "main" {
		t.Fatalf("git branch not set: %+v", fetched.GitBranch)
	}
	if fetched.DefaultRecipe == nil || *fetched.DefaultRecipe != "build" {
		t.Fatalf("default recipe not set: %+v", fetched.DefaultRecipe)
	}

	newName := "cell-a-renamed"
	newPath := "/repo/cell-a2"
	newRepo := "repo-2"
	newBranch := "feature/x"
	clearRecipe := ""
	updated, err := svc.UpdateCell(ctx, cell.ID, UpdateInput{
		Name:          &newName,
		WorkingPath:   &newPath,
		GitRepoName:   &newRepo,
		GitBranch:     &newBranch,
		DefaultRecipe: &clearRecipe,
	})
	if err != nil {
		t.Fatalf("update cell: %v", err)
	}
	if updated.Name != newName || updated.WorkingPath != newPath {
		t.Fatalf("update did not apply: %+v", updated)
	}
	if updated.GitRepoName == nil || *updated.GitRepoName != newRepo {
		t.Fatalf("git repo update failed: %+v", updated.GitRepoName)
	}
	if updated.GitBranch == nil || *updated.GitBranch != newBranch {
		t.Fatalf("git branch update failed: %+v", updated.GitBranch)
	}
	if updated.DefaultRecipe != nil {
		t.Fatalf("expected default recipe cleared: %+v", updated.DefaultRecipe)
	}
}

func TestReplaceDependencies(t *testing.T) {
	svc, proj, closeFn := setupService(t)
	defer closeFn()
	ctx := context.Background()

	c1, err := svc.CreateCell(ctx, CreateInput{ProjectID: proj.ID, Name: "cell-a", WorkingPath: "/a"})
	if err != nil {
		t.Fatalf("create c1: %v", err)
	}
	c2, err := svc.CreateCell(ctx, CreateInput{ProjectID: proj.ID, Name: "cell-b", WorkingPath: "/b"})
	if err != nil {
		t.Fatalf("create c2: %v", err)
	}

	if err := svc.ReplaceDependencies(ctx, c1.ID, []model.ID{c2.ID}); err != nil {
		t.Fatalf("replace deps: %v", err)
	}
	svcImpl := svc.(*service)
	deps, err := svcImpl.store.ListDependencies(ctx, proj.ID, c1.ID)
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(deps) != 1 || deps[0] != c2.ID {
		t.Fatalf("unexpected deps: %+v", deps)
	}
}

func TestSyncFromPopulator(t *testing.T) {
	svc, proj, closeFn := setupService(t)
	defer closeFn()
	ctx := context.Background()

	pop := stubPopulator{
		name: "graph/moon",
		cells: []PopulatorCell{
			{Name: "cell-a", WorkingPath: "/a", ExternalID: "node-a", Dependencies: []string{"cell-b"}, GitRepoName: "repo-1", GitBranch: "main", DefaultRecipe: "build"},
			{Name: "cell-b", WorkingPath: "/b", ExternalID: "node-b", GitRepoName: "repo-1", GitBranch: "main"},
		},
	}

	result, err := svc.SyncFromPopulator(ctx, proj.ID, pop, SyncOptions{PruneMissing: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Created != 2 || result.DependenciesUpdated != 2 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	it, err := svc.ListCells(ctx, model.SearchFilter{Names: []string{"cell-a"}})
	if err != nil {
		t.Fatalf("list cells: %v", err)
	}
	defer it.Close(ctx)
	cellA, err := it.Next(ctx)
	if err != nil {
		t.Fatalf("next cell: %v", err)
	}
	if cellA.GitRepoName == nil || *cellA.GitRepoName != "repo-1" {
		t.Fatalf("git repo not set from populator: %+v", cellA.GitRepoName)
	}
	if cellA.GitBranch == nil || *cellA.GitBranch != "main" {
		t.Fatalf("git branch not set from populator: %+v", cellA.GitBranch)
	}
	if cellA.DefaultRecipe == nil || *cellA.DefaultRecipe != "build" {
		t.Fatalf("default recipe not set from populator: %+v", cellA.DefaultRecipe)
	}

	// Run again with one cell removed to exercise prune.
	pop.cells = []PopulatorCell{
		{Name: "cell-a", WorkingPath: "/a2", ExternalID: "node-a"},
	}
	result, err = svc.SyncFromPopulator(ctx, proj.ID, pop, SyncOptions{PruneMissing: true})
	if err != nil {
		t.Fatalf("resync: %v", err)
	}
	if result.Deleted != 1 || result.Updated != 1 {
		t.Fatalf("unexpected resync result: %+v", result)
	}
}
