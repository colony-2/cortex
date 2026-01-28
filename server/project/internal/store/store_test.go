package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/project/internal/model"
	"github.com/colony-2/colony2/server/project/internal/store"
	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStoreCRUD(t *testing.T) {
	pg := pgembed.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	s, err := store.New(pg.DB)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().UTC()

	recipe := "tickets.yaml"
	project := &model.Project{
		ID:                  model.ID("0ujtsYcgvSTl8PAuAdqWYSMnLOv"),
		Name:                "Alpha",
		GitRepoPath:         "git@example.com/alpha.git",
		DefaultTicketRecipe: &recipe,
		GitRepoBranch:       ptr("main"),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	require.NoError(t, s.Create(ctx, project))

	fetched, err := s.Get(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, project.Name, fetched.Name)
	require.Equal(t, project.GitRepoPath, fetched.GitRepoPath)
	require.NotNil(t, fetched.DefaultTicketRecipe)
	require.Equal(t, recipe, *fetched.DefaultTicketRecipe)
	require.NotNil(t, fetched.GitRepoBranch)
	require.Equal(t, "main", *fetched.GitRepoBranch)
	require.WithinDuration(t, project.CreatedAt, fetched.CreatedAt, time.Second)
	require.WithinDuration(t, project.UpdatedAt, fetched.UpdatedAt, time.Second)

	iter, err := s.Search(ctx, model.SearchFilter{Names: []string{"Alpha"}})
	require.NoError(t, err)
	defer pgembed.MustCloseIterator(t, iter)
	first, err := iter.Next(ctx)
	require.NoError(t, err)
	require.Equal(t, project.ID, first.ID)
	_, err = iter.Next(ctx)
	require.ErrorIs(t, err, store.ErrIteratorDone)

	// update the record
	updated := *fetched
	updated.Name = "Alpha Prime"
	updated.UpdatedAt = now.Add(time.Minute)
	require.NoError(t, s.Update(ctx, &updated))

	found, err := s.Get(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, "Alpha Prime", found.Name)

	// stale update should fail optimistic lock
	stale := *fetched
	stale.Name = "Alpha Stale"
	stale.UpdatedAt = now.Add(2 * time.Minute)
	err = s.Update(ctx, &stale)
	require.ErrorIs(t, err, store.ErrOptimisticLock)

	require.NoError(t, s.Delete(ctx, project.ID))
	_, err = s.Get(ctx, project.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestSearchFilters(t *testing.T) {
	pg := pgembed.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	s, err := store.New(pg.DB)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().UTC()

	projects := []*model.Project{
		{ID: model.ID("0ujtsYcgvSTl8PAuAdqWYSMnLOv"), Name: "Alpha", GitRepoPath: "git@example.com/a.git", CreatedAt: now, UpdatedAt: now},
		{ID: model.ID("0ujtsYcgvSTl8PAuAdqWYSMnLOw"), Name: "Beta", GitRepoPath: "git@example.com/b.git", CreatedAt: now, UpdatedAt: now},
		{ID: model.ID("0ujtsYcgvSTl8PAuAdqWYSMnLOx"), Name: "Gamma", GitRepoPath: "git@example.com/g.git", CreatedAt: now, UpdatedAt: now},
	}
	for _, p := range projects {
		require.NoError(t, s.Create(ctx, p))
	}

	iter, err := s.Search(ctx, model.SearchFilter{NameContains: "a"})
	require.NoError(t, err)
	defer pgembed.MustCloseIterator(t, iter)

	collected := collectAll(t, ctx, iter)
	require.Len(t, collected, 3)
	require.Equal(t, "Alpha", collected[0].Name)
	require.Equal(t, "Beta", collected[1].Name)
	require.Equal(t, "Gamma", collected[2].Name)
}

func collectAll(t *testing.T, ctx context.Context, iter store.Iterator[*model.Project]) []*model.Project {
	t.Helper()
	var projects []*model.Project
	for {
		next, err := iter.Next(ctx)
		if err != nil {
			require.ErrorIs(t, err, store.ErrIteratorDone)
			break
		}
		projects = append(projects, next)
	}
	return projects
}

func ptr[T any](v T) *T { return &v }



