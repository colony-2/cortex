package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipeops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/store"
	"github.com/colony-2/colony2/server/recipes/internal/testutil"
)

func registerEchoOpForTests() {
	recipeops.Clear()
	recipeops.Register(recipeops.NewActivityMappedOpV2[map[string]interface{}, map[string]interface{}](
		recipeops.OpMetadata{Type: "echo"},
		func(_ recipeops.OpDependencies, _ context.Context, in map[string]interface{}) (map[string]interface{}, error) {
			return in, nil
		},
	))
}

func TestService_SavePublishAsOf(t *testing.T) {
	registerEchoOpForTests()

	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	ctx := context.Background()
	projectID := project.ID("proj_test")

	projects := testutil.NewMockProjectService()
	projects.AddProject(&project.Project{ID: projectID})

	clock := testutil.NewMockClock(time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC))
	idgen := testutil.NewMockIDGenerator("id_")
	cel := &testutil.MockCELValidator{}

	st, err := store.NewWithOptions(pg.DB, store.Options{Migrate: true})
	if err != nil {
		t.Fatalf("store.NewWithOptions failed: %v", err)
	}

	svc, err := New(ServiceConfig{
		Store:        st,
		Projects:     projects,
		IDGen:        idgen,
		Clock:        clock,
		CELValidator: cel,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	clock.Set(time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC))
	v1, err := svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "r1",
		Content:     testutil.CreateTestRecipeContent("r1"),
		Description: "v1",
		AutoPublish: true,
	})
	if err != nil {
		t.Fatalf("CreateRecipe failed: %v", err)
	}
	if v1.CommitHash != "v1" || !v1.IsPublished {
		t.Fatalf("CreateRecipe = (%s, published=%v), want (v1, true)", v1.CommitHash, v1.IsPublished)
	}

	// Save v2 without publishing.
	clock.Add(1 * time.Minute)
	v2Content := []byte(`version: "1.0"
id: "r1"
op: echo
inputs:
  message: "v2"
`)
	v2, err := svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:   projectID,
		Name:        "r1",
		Content:     v2Content,
		Message:     "save v2",
		AutoPublish: false,
	})
	if err != nil {
		t.Fatalf("UpdateRecipe failed: %v", err)
	}
	if v2.CommitHash != "v2" || v2.IsPublished {
		t.Fatalf("UpdateRecipe = (%s, published=%v), want (v2, false)", v2.CommitHash, v2.IsPublished)
	}

	// Default Get returns published v1, not latest saved v2.
	got, err := svc.GetRecipe(ctx, projectID, "r1", "")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}
	if got.CommitHash != "v1" || !got.IsPublished {
		t.Fatalf("GetRecipe() = (%s, published=%v), want (v1, true)", got.CommitHash, got.IsPublished)
	}

	// Publish v2.
	clock.Add(1 * time.Minute)
	_, err = svc.PublishRecipe(ctx, model.PublishInput{
		ProjectID:  projectID,
		Name:       "r1",
		CommitHash: "v2",
	})
	if err != nil {
		t.Fatalf("PublishRecipe failed: %v", err)
	}

	got2, err := svc.GetRecipe(ctx, projectID, "r1", "")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}
	if got2.CommitHash != "v2" || !got2.IsPublished {
		t.Fatalf("GetRecipe() = (%s, published=%v), want (v2, true)", got2.CommitHash, got2.IsPublished)
	}

	// As-of before publish should return v1.
	asofBefore := time.Date(2026, 2, 7, 12, 0, 30, 0, time.UTC)
	asofRecipe, err := svc.GetRecipe(ctx, projectID, "r1", "asof:"+asofBefore.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("GetRecipe asof failed: %v", err)
	}
	if asofRecipe.CommitHash != "v1" {
		t.Fatalf("asof commit = %s, want v1", asofRecipe.CommitHash)
	}

	// Unpublish and verify asof after unpublish is not published.
	clock.Add(1 * time.Minute)
	if err := svc.UnpublishRecipe(ctx, model.UnpublishInput{ProjectID: projectID, Name: "r1"}); err != nil {
		t.Fatalf("UnpublishRecipe failed: %v", err)
	}

	afterUnpub := time.Date(2026, 2, 7, 12, 5, 0, 0, time.UTC)
	_, err = svc.GetRecipe(ctx, projectID, "r1", "asof:"+afterUnpub.Format(time.RFC3339Nano))
	if !errors.Is(err, model.ErrNotPublished) {
		t.Fatalf("GetRecipe asof after unpublish error = %v, want ErrNotPublished", err)
	}
}

func TestService_PublishOldDoesNotMoveLatestSaved(t *testing.T) {
	registerEchoOpForTests()

	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	ctx := context.Background()
	projectID := project.ID("proj_test")

	projects := testutil.NewMockProjectService()
	projects.AddProject(&project.Project{ID: projectID})

	clock := testutil.NewMockClock(time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC))
	idgen := testutil.NewMockIDGenerator("id_")
	cel := &testutil.MockCELValidator{}

	st, err := store.NewWithOptions(pg.DB, store.Options{Migrate: true})
	if err != nil {
		t.Fatalf("store.NewWithOptions failed: %v", err)
	}

	svc, err := New(ServiceConfig{
		Store:        st,
		Projects:     projects,
		IDGen:        idgen,
		Clock:        clock,
		CELValidator: cel,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if _, err := svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "r2",
		Content:     testutil.CreateTestRecipeContent("r2"),
		Description: "v1",
		AutoPublish: true,
	}); err != nil {
		t.Fatalf("CreateRecipe failed: %v", err)
	}

	clock.Add(1 * time.Minute)
	if _, err := svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:   projectID,
		Name:        "r2",
		Content:     []byte(`version: "1.0"\nid: "r2"\nop: echo\ninputs:\n  message: "v2"\n`),
		Message:     "save v2",
		AutoPublish: false,
	}); err != nil {
		t.Fatalf("UpdateRecipe failed: %v", err)
	}

	// Publish v1 again.
	clock.Add(1 * time.Minute)
	if _, err := svc.PublishRecipe(ctx, model.PublishInput{
		ProjectID:  projectID,
		Name:       "r2",
		CommitHash: "v1",
	}); err != nil {
		t.Fatalf("PublishRecipe failed: %v", err)
	}

	iter, err := svc.ListRecipes(ctx, model.RecipeFilter{ProjectIDs: []project.ID{projectID}, PublishStatus: model.PublishStatusAll})
	if err != nil {
		t.Fatalf("ListRecipes failed: %v", err)
	}
	defer iter.Close(ctx)

	info, err := iter.Next(ctx)
	if err != nil {
		t.Fatalf("Next failed: %v", err)
	}

	if info.LatestCommit != "v2" {
		t.Fatalf("LatestCommit = %s, want v2", info.LatestCommit)
	}
	if info.PublishedCommit == nil || *info.PublishedCommit != "v1" {
		t.Fatalf("PublishedCommit = %v, want v1", info.PublishedCommit)
	}
}

func TestService_GetByRefsAndHistory(t *testing.T) {
	registerEchoOpForTests()

	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	ctx := context.Background()
	projectID := project.ID("proj_test")

	projects := testutil.NewMockProjectService()
	projects.AddProject(&project.Project{ID: projectID})

	clock := testutil.NewMockClock(time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC))
	idgen := testutil.NewMockIDGenerator("id_")
	cel := &testutil.MockCELValidator{}

	st, err := store.NewWithOptions(pg.DB, store.Options{Migrate: true})
	if err != nil {
		t.Fatalf("store.NewWithOptions failed: %v", err)
	}

	svc, err := New(ServiceConfig{
		Store:        st,
		Projects:     projects,
		IDGen:        idgen,
		Clock:        clock,
		CELValidator: cel,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	v1Content := testutil.CreateTestRecipeContent("r3")
	_, err = svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "r3",
		Content:     v1Content,
		Description: "v1",
		AutoPublish: true,
	})
	if err != nil {
		t.Fatalf("CreateRecipe failed: %v", err)
	}

	clock.Add(1 * time.Minute)
	v2Content := []byte(`version: "1.0"
id: "r3"
op: echo
inputs:
  message: "v2"
`)
	_, err = svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:   projectID,
		Name:        "r3",
		Content:     v2Content,
		Message:     "save v2",
		AutoPublish: false,
	})
	if err != nil {
		t.Fatalf("UpdateRecipe failed: %v", err)
	}

	// v1 is still published.
	gotV1, err := svc.GetRecipe(ctx, projectID, "r3", "v1")
	if err != nil {
		t.Fatalf("GetRecipe v1 failed: %v", err)
	}
	if gotV1.CommitHash != "v1" || !gotV1.IsPublished {
		t.Fatalf("GetRecipe v1 = (%s, published=%v), want (v1, true)", gotV1.CommitHash, gotV1.IsPublished)
	}

	gotV2, err := svc.GetRecipe(ctx, projectID, "r3", "v2")
	if err != nil {
		t.Fatalf("GetRecipe v2 failed: %v", err)
	}
	if gotV2.CommitHash != "v2" || gotV2.IsPublished {
		t.Fatalf("GetRecipe v2 = (%s, published=%v), want (v2, false)", gotV2.CommitHash, gotV2.IsPublished)
	}

	// sha256 ref resolves to the most recent saved version with that digest.
	v2DigestRef := digestToRef(sha256DigestBytes(v2Content))
	gotByDigest, err := svc.GetRecipe(ctx, projectID, "r3", v2DigestRef)
	if err != nil {
		t.Fatalf("GetRecipe sha256 failed: %v", err)
	}
	if gotByDigest.CommitHash != "v2" {
		t.Fatalf("GetRecipe sha256 commit = %s, want v2", gotByDigest.CommitHash)
	}

	// ver:<id> ref resolves a saved event id.
	impl := svc.(*service)
	var row model.RecipeRow
	if err := impl.store.DB().WithContext(ctx).First(&row, "project_id = ? AND name = ?", projectID, "r3").Error; err != nil {
		t.Fatalf("load recipe row failed: %v", err)
	}
	var ev model.RecipeEvent
	if err := impl.store.DB().WithContext(ctx).First(&ev, "recipe_id = ? AND saved_ordinal = ?", row.ID, int64(2)).Error; err != nil {
		t.Fatalf("load saved event failed: %v", err)
	}

	gotByVer, err := svc.GetRecipe(ctx, projectID, "r3", "ver:"+ev.ID)
	if err != nil {
		t.Fatalf("GetRecipe ver:id failed: %v", err)
	}
	if gotByVer.CommitHash != "v2" {
		t.Fatalf("GetRecipe ver:id commit = %s, want v2", gotByVer.CommitHash)
	}

	// History is ordered by saved ordinal desc and marks published.
	it, err := svc.GetRecipeHistory(ctx, projectID, "r3")
	if err != nil {
		t.Fatalf("GetRecipeHistory failed: %v", err)
	}
	defer it.Close(ctx)

	h1, err := it.Next(ctx)
	if err != nil {
		t.Fatalf("history next failed: %v", err)
	}
	if h1.CommitHash != "v2" {
		t.Fatalf("history[0] = %s, want v2", h1.CommitHash)
	}
	if h1.IsPublished {
		t.Fatalf("history[0] IsPublished = true, want false")
	}

	h2, err := it.Next(ctx)
	if err != nil {
		t.Fatalf("history next failed: %v", err)
	}
	if h2.CommitHash != "v1" || !h2.IsPublished {
		t.Fatalf("history[1] = (%s, published=%v), want (v1, true)", h2.CommitHash, h2.IsPublished)
	}
}

func TestService_UpdateIdenticalBytesIsNoOp(t *testing.T) {
	registerEchoOpForTests()

	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	ctx := context.Background()
	projectID := project.ID("proj_test")

	projects := testutil.NewMockProjectService()
	projects.AddProject(&project.Project{ID: projectID})

	clock := testutil.NewMockClock(time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC))
	idgen := testutil.NewMockIDGenerator("id_")
	cel := &testutil.MockCELValidator{}

	st, err := store.NewWithOptions(pg.DB, store.Options{Migrate: true})
	if err != nil {
		t.Fatalf("store.NewWithOptions failed: %v", err)
	}

	svc, err := New(ServiceConfig{
		Store:        st,
		Projects:     projects,
		IDGen:        idgen,
		Clock:        clock,
		CELValidator: cel,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	content := testutil.CreateTestRecipeContent("r4")
	v1, err := svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "r4",
		Content:     content,
		Description: "v1",
		AutoPublish: false,
	})
	if err != nil {
		t.Fatalf("CreateRecipe failed: %v", err)
	}
	if v1.CommitHash != "v1" {
		t.Fatalf("v1 = %s, want v1", v1.CommitHash)
	}

	clock.Add(1 * time.Minute)
	v2, err := svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:   projectID,
		Name:        "r4",
		Content:     content,
		Message:     "noop",
		AutoPublish: false,
	})
	if err != nil {
		t.Fatalf("UpdateRecipe failed: %v", err)
	}
	if v2.CommitHash != "v1" {
		t.Fatalf("noop update created new version %s, want v1", v2.CommitHash)
	}
}

func TestService_AsOfAcrossRecipes(t *testing.T) {
	registerEchoOpForTests()

	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	ctx := context.Background()
	projectID := project.ID("proj_test")

	projects := testutil.NewMockProjectService()
	projects.AddProject(&project.Project{ID: projectID})

	clock := testutil.NewMockClock(time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC))
	idgen := testutil.NewMockIDGenerator("id_")
	cel := &testutil.MockCELValidator{}

	st, err := store.NewWithOptions(pg.DB, store.Options{Migrate: true})
	if err != nil {
		t.Fatalf("store.NewWithOptions failed: %v", err)
	}

	svc, err := New(ServiceConfig{
		Store:        st,
		Projects:     projects,
		IDGen:        idgen,
		Clock:        clock,
		CELValidator: cel,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// rA published at 12:00.
	clock.Set(time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC))
	if _, err := svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "rA",
		Content:     testutil.CreateTestRecipeContent("rA"),
		Description: "v1",
		AutoPublish: true,
	}); err != nil {
		t.Fatalf("CreateRecipe rA failed: %v", err)
	}

	// rB published at 12:01.
	clock.Set(time.Date(2026, 2, 7, 12, 1, 0, 0, time.UTC))
	if _, err := svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "rB",
		Content:     testutil.CreateTestRecipeContent("rB"),
		Description: "v1",
		AutoPublish: true,
	}); err != nil {
		t.Fatalf("CreateRecipe rB failed: %v", err)
	}

	// rA v2 saved+published at 12:02.
	clock.Set(time.Date(2026, 2, 7, 12, 2, 0, 0, time.UTC))
	if _, err := svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID: projectID,
		Name:      "rA",
		Content: []byte(`version: "1.0"
id: "rA"
op: echo
inputs:
  message: "v2"
`),
		Message:     "save+publish v2",
		AutoPublish: true,
	}); err != nil {
		t.Fatalf("UpdateRecipe rA failed: %v", err)
	}

	// rB unpublished at 12:03.
	clock.Set(time.Date(2026, 2, 7, 12, 3, 0, 0, time.UTC))
	if err := svc.UnpublishRecipe(ctx, model.UnpublishInput{ProjectID: projectID, Name: "rB"}); err != nil {
		t.Fatalf("UnpublishRecipe rB failed: %v", err)
	}

	// As-of 12:02:30 => rA is v2, rB is still v1.
	asof := time.Date(2026, 2, 7, 12, 2, 30, 0, time.UTC).Format(time.RFC3339Nano)
	rA, err := svc.GetRecipe(ctx, projectID, "rA", "asof:"+asof)
	if err != nil {
		t.Fatalf("GetRecipe rA asof failed: %v", err)
	}
	if rA.CommitHash != "v2" {
		t.Fatalf("rA asof commit = %s, want v2", rA.CommitHash)
	}

	rB, err := svc.GetRecipe(ctx, projectID, "rB", "asof:"+asof)
	if err != nil {
		t.Fatalf("GetRecipe rB asof failed: %v", err)
	}
	if rB.CommitHash != "v1" {
		t.Fatalf("rB asof commit = %s, want v1", rB.CommitHash)
	}

	// As-of 12:03:30 => rB not published.
	asof2 := time.Date(2026, 2, 7, 12, 3, 30, 0, time.UTC).Format(time.RFC3339Nano)
	_, err = svc.GetRecipe(ctx, projectID, "rB", "asof:"+asof2)
	if !errors.Is(err, model.ErrNotPublished) {
		t.Fatalf("rB asof after unpublish err = %v, want ErrNotPublished", err)
	}
}
