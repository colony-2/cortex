package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/project/internal/model"
	"github.com/divisive-ai/vibethis/server/project/internal/service"
	"github.com/divisive-ai/vibethis/server/project/internal/store"
	"github.com/divisive-ai/vibethis/server/project/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestServiceCRUD(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	projectStore, err := store.New(pg.DB)
	require.NoError(t, err)

	now := time.Date(2024, 9, 2, 12, 0, 0, 0, time.UTC)
	svc, err := service.New(service.ServiceConfig{
		Store: projectStore,
		Clock: fixedClock{now: now},
		IDGen: fixedIDGen{id: "0ujtsYcgvSTl8PAuAdqWYSMnLOv"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	created, err := svc.CreateProject(ctx, service.CreateInput{
		Name:        "Alpha",
		GitRepoPath: "git@example.com/alpha.git",
	})
	require.NoError(t, err)
	require.Equal(t, model.ID("0ujtsYcgvSTl8PAuAdqWYSMnLOv"), created.ID)
	require.Equal(t, now, created.CreatedAt)
	require.Equal(t, now, created.UpdatedAt)

	fetched, err := svc.GetProject(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)

	iter, err := svc.ListProjects(ctx, model.SearchFilter{})
	require.NoError(t, err)
	defer testutil.MustCloseIterator(t, iter)
	first, err := iter.Next(ctx)
	require.NoError(t, err)
	require.Equal(t, "Alpha", first.Name)

	newName := "Alpha Prime"
	updated, err := svc.UpdateProject(ctx, created.ID, service.UpdateInput{Name: &newName})
	require.NoError(t, err)
	require.Equal(t, newName, updated.Name)
	require.True(t, updated.UpdatedAt.After(updated.CreatedAt))

	require.NoError(t, svc.DeleteProject(ctx, created.ID))
	_, err = svc.GetProject(ctx, created.ID)
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestServiceValidation(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	projectStore, err := store.New(pg.DB)
	require.NoError(t, err)

	svc, err := service.New(service.ServiceConfig{Store: projectStore})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = svc.CreateProject(ctx, service.CreateInput{Name: "  ", GitRepoPath: "git@example.com/alpha.git"})
	require.ErrorIs(t, err, service.ErrEmptyName)

	_, err = svc.CreateProject(ctx, service.CreateInput{Name: "Alpha", GitRepoPath: ""})
	require.ErrorIs(t, err, service.ErrEmptyGitRepo)

	_, err = svc.UpdateProject(ctx, model.ID("missing"), service.UpdateInput{Name: ptr("Beta")})
	require.ErrorIs(t, err, service.ErrNotFound)
}

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type fixedIDGen struct {
	id  string
	err error
}

func (f fixedIDGen) NewID() (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.id, nil
}

func ptr[T any](v T) *T { return &v }
