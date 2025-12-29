package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/testutil"
	"gorm.io/gorm"
)

func TestStore_CreateAndGet(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	store, err := New(pg.DB)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	recipe := &model.PublishedRecipe{
		ID:          "test_001",
		ProjectID:   "proj_123",
		Name:        "test-recipe",
		GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
		CommitHash:  "abc123",
		PublishedAt: time.Now().UTC(),
		PublishedBy: stringPtr("test@example.com"),
	}

	// Create
	err = store.Create(ctx, recipe)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get by ID
	got, err := store.GetByID(ctx, recipe.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.ID != recipe.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, recipe.ID)
	}
	if got.Name != recipe.Name {
		t.Errorf("Name mismatch: got %q, want %q", got.Name, recipe.Name)
	}
	if got.CommitHash != recipe.CommitHash {
		t.Errorf("CommitHash mismatch: got %q, want %q", got.CommitHash, recipe.CommitHash)
	}

	// Get by name
	got2, err := store.GetByName(ctx, recipe.ProjectID, recipe.Name)
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if got2.ID != recipe.ID {
		t.Errorf("ID mismatch from GetByName: got %q, want %q", got2.ID, recipe.ID)
	}
}

func TestStore_UniqueName(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	store, err := New(pg.DB)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	recipe1 := &model.PublishedRecipe{
		ID:          "test_001",
		ProjectID:   "proj_123",
		Name:        "test-recipe",
		GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
		CommitHash:  "abc123",
		PublishedAt: time.Now().UTC(),
	}

	// Create first recipe
	if err := store.Create(ctx, recipe1); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to create duplicate (same project + name)
	recipe2 := &model.PublishedRecipe{
		ID:          "test_002",
		ProjectID:   "proj_123",
		Name:        "test-recipe", // Same name!
		GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
		CommitHash:  "def456",
		PublishedAt: time.Now().UTC(),
	}

	err = store.Create(ctx, recipe2)
	if err == nil {
		t.Fatal("Expected error creating duplicate recipe, got nil")
	}

	// Different project should be OK
	recipe3 := &model.PublishedRecipe{
		ID:          "test_003",
		ProjectID:   "proj_456", // Different project!
		Name:        "test-recipe",
		GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
		CommitHash:  "ghi789",
		PublishedAt: time.Now().UTC(),
	}

	if err := store.Create(ctx, recipe3); err != nil {
		t.Fatalf("Create with different project failed: %v", err)
	}
}

func TestStore_Update(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	store, err := New(pg.DB)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	recipe := &model.PublishedRecipe{
		ID:          "test_001",
		ProjectID:   "proj_123",
		Name:        "test-recipe",
		GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
		CommitHash:  "abc123",
		PublishedAt: time.Now().UTC(),
	}

	if err := store.Create(ctx, recipe); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get fresh copy
	fresh, err := store.GetByID(ctx, recipe.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// Update
	fresh.CommitHash = "def456"
	fresh.PublishedAt = time.Now().UTC().Add(1 * time.Hour)
	fresh.PublishedBy = stringPtr("other@example.com")

	if err := store.Update(ctx, fresh); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	updated, err := store.GetByID(ctx, recipe.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}

	if updated.CommitHash != "def456" {
		t.Errorf("CommitHash not updated: got %q, want %q", updated.CommitHash, "def456")
	}
	if updated.PublishedBy == nil || *updated.PublishedBy != "other@example.com" {
		t.Errorf("PublishedBy not updated")
	}
}

func TestStore_OptimisticLocking(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	store, err := New(pg.DB)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	recipe := &model.PublishedRecipe{
		ID:          "test_001",
		ProjectID:   "proj_123",
		Name:        "test-recipe",
		GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
		CommitHash:  "abc123",
		PublishedAt: time.Now().UTC(),
	}

	if err := store.Create(ctx, recipe); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get two copies
	copy1, _ := store.GetByID(ctx, recipe.ID)
	copy2, _ := store.GetByID(ctx, recipe.ID)

	// Update copy1
	copy1.CommitHash = "version1"
	if err := store.Update(ctx, copy1); err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	// Try to update copy2 (should fail due to version mismatch)
	copy2.CommitHash = "version2"
	err = store.Update(ctx, copy2)
	if err == nil {
		t.Fatal("Expected optimistic lock error, got nil")
	}
	if !errors.Is(err, ErrOptimisticLock) {
		t.Errorf("Expected ErrOptimisticLock, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	store, err := New(pg.DB)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	recipe := &model.PublishedRecipe{
		ID:          "test_001",
		ProjectID:   "proj_123",
		Name:        "test-recipe",
		GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
		CommitHash:  "abc123",
		PublishedAt: time.Now().UTC(),
	}

	if err := store.Create(ctx, recipe); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete
	if err := store.Delete(ctx, recipe.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = store.GetByID(ctx, recipe.ID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("Expected ErrRecordNotFound after delete, got %v", err)
	}

	// Delete again should fail
	err = store.Delete(ctx, recipe.ID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("Expected ErrRecordNotFound deleting non-existent, got %v", err)
	}
}

func TestStore_Search(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	store, err := New(pg.DB)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create test data
	recipes := []*model.PublishedRecipe{
		{
			ID:          "test_001",
			ProjectID:   "proj_123",
			Name:        "workflows/ci/build",
			GitPath:     ".c2/recipes/workflows/ci/build.recipe.yaml",
			CommitHash:  "abc123",
			PublishedAt: time.Now().UTC(),
		},
		{
			ID:          "test_002",
			ProjectID:   "proj_123",
			Name:        "workflows/ci/test",
			GitPath:     ".c2/recipes/workflows/ci/test.recipe.yaml",
			CommitHash:  "def456",
			PublishedAt: time.Now().UTC(),
		},
		{
			ID:          "test_003",
			ProjectID:   "proj_123",
			Name:        "deployment",
			GitPath:     ".c2/recipes/deployment.recipe.yaml",
			CommitHash:  "ghi789",
			PublishedAt: time.Now().UTC(),
		},
		{
			ID:          "test_004",
			ProjectID:   "proj_456",
			Name:        "workflows/ci/build",
			GitPath:     ".c2/recipes/workflows/ci/build.recipe.yaml",
			CommitHash:  "jkl012",
			PublishedAt: time.Now().UTC(),
		},
	}

	for _, r := range recipes {
		if err := store.Create(ctx, r); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	tests := []struct {
		name   string
		filter model.SearchFilter
		want   int
	}{
		{
			name: "all recipes for project",
			filter: model.SearchFilter{
				ProjectIDs: []project.ID{"proj_123"},
			},
			want: 3,
		},
		{
			name: "name prefix filter",
			filter: model.SearchFilter{
				ProjectIDs: []project.ID{"proj_123"},
				NamePrefix: "workflows/ci/",
			},
			want: 2,
		},
		{
			name: "exact name filter",
			filter: model.SearchFilter{
				ProjectIDs: []project.ID{"proj_123"},
				Names:      []string{"deployment"},
			},
			want: 1,
		},
		{
			name: "multiple projects",
			filter: model.SearchFilter{
				ProjectIDs: []project.ID{"proj_123", "proj_456"},
			},
			want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter, err := store.Search(ctx, tt.filter)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			defer testutil.MustCloseIterator(t, iter)

			count := 0
			for {
				_, err := iter.Next(ctx)
				if errors.Is(err, ErrIteratorDone) {
					break
				}
				if err != nil {
					t.Fatalf("Iterator error: %v", err)
				}
				count++
			}

			if count != tt.want {
				t.Errorf("Search returned %d results, want %d", count, tt.want)
			}
		})
	}
}

func TestStore_Transaction(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	store, err := New(pg.DB)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Successful transaction
	err = store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		recipe := &model.PublishedRecipe{
			ID:          "test_001",
			ProjectID:   "proj_123",
			Name:        "test-recipe",
			GitPath:     ".c2/recipes/test-recipe.recipe.yaml",
			CommitHash:  "abc123",
			PublishedAt: time.Now().UTC(),
		}
		return txStore.Create(ctx, recipe)
	})
	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	// Verify created
	_, err = store.GetByID(ctx, "test_001")
	if err != nil {
		t.Errorf("Recipe not found after successful transaction: %v", err)
	}

	// Failed transaction (should rollback)
	testErr := errors.New("test error")
	err = store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		recipe := &model.PublishedRecipe{
			ID:          "test_002",
			ProjectID:   "proj_123",
			Name:        "test-recipe-2",
			GitPath:     ".c2/recipes/test-recipe-2.recipe.yaml",
			CommitHash:  "def456",
			PublishedAt: time.Now().UTC(),
		}
		if err := txStore.Create(ctx, recipe); err != nil {
			return err
		}
		return testErr // Force rollback
	})
	if !errors.Is(err, testErr) {
		t.Errorf("Expected test error, got %v", err)
	}

	// Verify not created
	_, err = store.GetByID(ctx, "test_002")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("Expected recipe to be rolled back, but it exists")
	}
}

func stringPtr(s string) *string {
	return &s
}
