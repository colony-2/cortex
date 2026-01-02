package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipeops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/store"
	"github.com/colony-2/colony2/server/recipes/internal/testutil"
	"gorm.io/gorm"
)

func TestService_FullLifecycle(t *testing.T) {
	// Setup
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	svc := setupTestService(t, pg.DB)
	ctx := context.Background()
	projectID := project.ID("proj_test")

	// Test 1: Create recipe with AutoPublish
	t.Run("CreateWithAutoPublish", func(t *testing.T) {
		version, err := svc.CreateRecipe(ctx, model.CreateInput{
			ProjectID:   projectID,
			Name:        "test-recipe",
			Content:     testutil.CreateTestRecipeContent("test-recipe"),
			Description: "Test recipe",
			AutoPublish: true,
		})
		if err != nil {
			t.Fatalf("CreateRecipe failed: %v", err)
		}

		if version.Name != "test-recipe" {
			t.Errorf("Name = %q, want %q", version.Name, "test-recipe")
		}
		if version.CommitHash == "" {
			t.Error("CommitHash is empty")
		}
		if !version.IsPublished {
			t.Error("IsPublished = false, want true")
		}
	})

	// Test 2: Get published recipe
	t.Run("GetPublishedRecipe", func(t *testing.T) {
		recipe, err := svc.GetRecipe(ctx, projectID, "test-recipe", "")
		if err != nil {
			t.Fatalf("GetRecipe failed: %v", err)
		}

		if recipe.Name != "test-recipe" {
			t.Errorf("Name = %q, want %q", recipe.Name, "test-recipe")
		}
		if !recipe.IsPublished {
			t.Error("IsPublished = false, want true")
		}
		if len(recipe.Content) == 0 {
			t.Error("Content is empty")
		}
	})

	// Test 3: Update recipe without AutoPublish
	t.Run("UpdateWithoutAutoPublish", func(t *testing.T) {
		// Create different content by changing the message
		updatedContent := []byte(`version: "1.0"
id: "test-recipe"
op: echo
inputs:
  message: "Updated test recipe"
`)
		version, err := svc.UpdateRecipe(ctx, model.UpdateInput{
			ProjectID:   projectID,
			Name:        "test-recipe",
			Content:     updatedContent,
			Message:     "Update recipe",
			AutoPublish: false,
		})
		if err != nil {
			t.Fatalf("UpdateRecipe failed: %v", err)
		}

		if version.IsPublished {
			t.Error("IsPublished = true, want false (not auto-published)")
		}

		// Get recipe should still return old published version
		recipe, err := svc.GetRecipe(ctx, projectID, "test-recipe", "")
		if err != nil {
			t.Fatalf("GetRecipe failed: %v", err)
		}

		// Should not be the new version
		if recipe.CommitHash == version.CommitHash {
			t.Error("Got new version, but should still be old published version")
		}
	})

	// Test 4: Get recipe at specific commit
	t.Run("GetRecipeAtCommit", func(t *testing.T) {
		// Get latest version's commit
		history, err := svc.GetRecipeHistory(ctx, projectID, "test-recipe")
		if err != nil {
			t.Fatalf("GetRecipeHistory failed: %v", err)
		}
		defer history.Close(ctx)

		latest, err := history.Next(ctx)
		if err != nil {
			t.Fatalf("Next failed: %v", err)
		}

		// Get recipe at that commit
		recipe, err := svc.GetRecipe(ctx, projectID, "test-recipe", latest.CommitHash)
		if err != nil {
			t.Fatalf("GetRecipe at commit failed: %v", err)
		}

		if recipe.CommitHash != latest.CommitHash {
			t.Errorf("CommitHash = %q, want %q", recipe.CommitHash, latest.CommitHash)
		}
	})

	// Test 5: List recipes
	t.Run("ListRecipes", func(t *testing.T) {
		iter, err := svc.ListRecipes(ctx, model.RecipeFilter{
			ProjectIDs:    []project.ID{projectID},
			PublishStatus: model.PublishStatusAll,
		})
		if err != nil {
			t.Fatalf("ListRecipes failed: %v", err)
		}
		defer iter.Close(ctx)

		count := 0
		for {
			_, err := iter.Next(ctx)
			if errors.Is(err, store.ErrIteratorDone) {
				break
			}
			if err != nil {
				t.Fatalf("Iterator error: %v", err)
			}
			count++
		}

		if count == 0 {
			t.Error("No recipes found")
		}
	})

	// Test 6: Get recipe history
	t.Run("GetRecipeHistory", func(t *testing.T) {
		iter, err := svc.GetRecipeHistory(ctx, projectID, "test-recipe")
		if err != nil {
			t.Fatalf("GetRecipeHistory failed: %v", err)
		}
		defer iter.Close(ctx)

		count := 0
		var foundPublished bool
		for {
			version, err := iter.Next(ctx)
			if errors.Is(err, store.ErrIteratorDone) {
				break
			}
			if err != nil {
				t.Fatalf("Iterator error: %v", err)
			}
			count++
			if version.IsPublished {
				foundPublished = true
			}
		}

		if count < 2 {
			t.Errorf("Expected at least 2 versions, got %d", count)
		}
		if !foundPublished {
			t.Error("No published version found in history")
		}
	})

	// Test 7: Unpublish recipe
	t.Run("UnpublishRecipe", func(t *testing.T) {
		// Get current published commit
		recipe, err := svc.GetRecipe(ctx, projectID, "test-recipe", "")
		if err != nil {
			t.Fatalf("GetRecipe failed: %v", err)
		}

		// Unpublish
		err = svc.UnpublishRecipe(ctx, model.UnpublishInput{
			ProjectID:      projectID,
			Name:           "test-recipe",
			ExpectedCommit: recipe.CommitHash,
		})
		if err != nil {
			t.Fatalf("UnpublishRecipe failed: %v", err)
		}

		// Try to get published recipe - should fail
		_, err = svc.GetRecipe(ctx, projectID, "test-recipe", "")
		if !errors.Is(err, model.ErrNotPublished) {
			t.Errorf("Expected ErrNotPublished, got %v", err)
		}
	})

	// Test 8: Republish recipe
	t.Run("RepublishRecipe", func(t *testing.T) {
		// Get latest commit
		history, err := svc.GetRecipeHistory(ctx, projectID, "test-recipe")
		if err != nil {
			t.Fatalf("GetRecipeHistory failed: %v", err)
		}
		defer history.Close(ctx)

		latest, err := history.Next(ctx)
		if err != nil {
			t.Fatalf("Next failed: %v", err)
		}

		// Publish it
		_, err = svc.PublishRecipe(ctx, model.PublishInput{
			ProjectID:  projectID,
			Name:       "test-recipe",
			CommitHash: latest.CommitHash,
		})
		if err != nil {
			t.Fatalf("PublishRecipe failed: %v", err)
		}

		// Should be able to get it now
		recipe, err := svc.GetRecipe(ctx, projectID, "test-recipe", "")
		if err != nil {
			t.Fatalf("GetRecipe failed: %v", err)
		}
		if !recipe.IsPublished {
			t.Error("Recipe should be published")
		}
	})

	// Test 9: Delete recipe
	t.Run("DeleteRecipe", func(t *testing.T) {
		err := svc.DeleteRecipe(ctx, projectID, "test-recipe")
		if err != nil {
			t.Fatalf("DeleteRecipe failed: %v", err)
		}

		// Should not be published
		_, err = svc.GetRecipe(ctx, projectID, "test-recipe", "")
		if !errors.Is(err, model.ErrNotPublished) {
			t.Errorf("Expected ErrNotPublished after delete, got %v", err)
		}
	})
}

func TestService_HierarchicalRecipes(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	svc := setupTestService(t, pg.DB)
	ctx := context.Background()
	projectID := project.ID("proj_test")

	// Create hierarchical recipes
	recipes := []string{
		"workflows/ci/build",
		"workflows/ci/test",
		"workflows/deploy/staging",
		"workflows/deploy/production",
		"utils/cleanup",
	}

	for _, name := range recipes {
		_, err := svc.CreateRecipe(ctx, model.CreateInput{
			ProjectID:   projectID,
			Name:        name,
			Content:     testutil.CreateTestRecipeContent(name),
			AutoPublish: true,
		})
		if err != nil {
			t.Fatalf("CreateRecipe %q failed: %v", name, err)
		}
	}

	// List with prefix filter
	iter, err := svc.ListRecipes(ctx, model.RecipeFilter{
		ProjectIDs:    []project.ID{projectID},
		NamePrefix:    "workflows/ci/",
		PublishStatus: model.PublishStatusPublished,
	})
	if err != nil {
		t.Fatalf("ListRecipes failed: %v", err)
	}
	defer iter.Close(ctx)

	count := 0
	for {
		info, err := iter.Next(ctx)
		if errors.Is(err, store.ErrIteratorDone) {
			break
		}
		if err != nil {
			t.Fatalf("Iterator error: %v", err)
		}
		count++

		// Verify it's in the workflows/ci/ namespace
		if !hasPrefix(info.Name, "workflows/ci/") {
			t.Errorf("Recipe %q doesn't match prefix filter", info.Name)
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 recipes in workflows/ci/, got %d", count)
	}
}

func TestService_ConcurrentPublish(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	svc := setupTestService(t, pg.DB)
	ctx := context.Background()
	projectID := project.ID("proj_test")

	// Create recipe
	version1, err := svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "concurrent-test",
		Content:     testutil.CreateTestRecipeContent("concurrent-test"),
		AutoPublish: true,
	})
	if err != nil {
		t.Fatalf("CreateRecipe failed: %v", err)
	}

	// Update to create version 2 (with different content)
	version2Content := []byte(`version: "1.0"
id: "concurrent-test"
op: echo
inputs:
  message: "Updated concurrent-test"
`)
	version2, err := svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:   projectID,
		Name:        "concurrent-test",
		Content:     version2Content,
		AutoPublish: false,
	})
	if err != nil {
		t.Fatalf("UpdateRecipe failed: %v", err)
	}

	// Try to publish version2 with wrong ExpectedCommit
	_, err = svc.PublishRecipe(ctx, model.PublishInput{
		ProjectID:      projectID,
		Name:           "concurrent-test",
		CommitHash:     version2.CommitHash,
		ExpectedCommit: stringPtr("wrong-commit"),
	})
	if !errors.Is(err, model.ErrVersionConflict) {
		t.Errorf("Expected ErrVersionConflict, got %v", err)
	}

	// Publish with correct ExpectedCommit should work
	_, err = svc.PublishRecipe(ctx, model.PublishInput{
		ProjectID:      projectID,
		Name:           "concurrent-test",
		CommitHash:     version2.CommitHash,
		ExpectedCommit: &version1.CommitHash,
	})
	if err != nil {
		t.Fatalf("PublishRecipe with correct ExpectedCommit failed: %v", err)
	}
}

func TestService_RecipeValidation(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	svc := setupTestService(t, pg.DB)
	ctx := context.Background()
	projectID := project.ID("proj_test")

	t.Run("InvalidRecipeID", func(t *testing.T) {
		// Create recipe with mismatched ID
		invalidContent := []byte(`version: "1.0"
id: "wrong-id"
op: echo
inputs:
  message: "test"
`)

		_, err := svc.CreateRecipe(ctx, model.CreateInput{
			ProjectID:   projectID,
			Name:        "test-recipe",
			Content:     invalidContent,
			AutoPublish: true,
		})
		if err == nil {
			t.Fatal("Expected error for mismatched recipe ID, got nil")
		}
		if !errors.Is(err, model.ErrInvalidContent) {
			t.Errorf("Expected ErrInvalidContent, got %v", err)
		}
	})

	t.Run("InvalidYAML", func(t *testing.T) {
		invalidContent := []byte(`this is not valid yaml: [[[`)

		_, err := svc.CreateRecipe(ctx, model.CreateInput{
			ProjectID:   projectID,
			Name:        "invalid-yaml",
			Content:     invalidContent,
			AutoPublish: true,
		})
		if err == nil {
			t.Fatal("Expected error for invalid YAML, got nil")
		}
	})

	t.Run("ValidateOnly", func(t *testing.T) {
		validContent := testutil.CreateTestRecipeContent("test-validate")

		err := svc.ValidateRecipe(ctx, validContent)
		if err != nil {
			t.Errorf("ValidateRecipe failed on valid content: %v", err)
		}

		err = svc.ValidateRecipe(ctx, []byte("invalid"))
		if err == nil {
			t.Error("ValidateRecipe should fail on invalid content")
		}
	})
}

func TestService_UpdateOptimisticLocking(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	svc := setupTestService(t, pg.DB)
	ctx := context.Background()
	projectID := project.ID("proj_test")

	// Create recipe
	version1, err := svc.CreateRecipe(ctx, model.CreateInput{
		ProjectID:   projectID,
		Name:        "lock-test",
		Content:     testutil.CreateTestRecipeContent("lock-test"),
		AutoPublish: false,
	})
	if err != nil {
		t.Fatalf("CreateRecipe failed: %v", err)
	}

	// Update recipe (version 2) - with different content
	version2Content := []byte(`version: "1.0"
id: "lock-test"
op: echo
inputs:
  message: "Lock test v2"
`)
	version2, err := svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:      projectID,
		Name:           "lock-test",
		Content:        version2Content,
		ExpectedCommit: version1.CommitHash,
		AutoPublish:    false,
	})
	if err != nil {
		t.Fatalf("UpdateRecipe failed: %v", err)
	}

	// Try to update with version1 commit (should fail) - with different content
	version3Content := []byte(`version: "1.0"
id: "lock-test"
op: echo
inputs:
  message: "Lock test v3 - stale"
`)
	_, err = svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:      projectID,
		Name:           "lock-test",
		Content:        version3Content,
		ExpectedCommit: version1.CommitHash, // Stale!
		AutoPublish:    false,
	})
	if !errors.Is(err, model.ErrVersionConflict) {
		t.Errorf("Expected ErrVersionConflict, got %v", err)
	}

	// Update with version2 commit (should succeed) - with different content
	version4Content := []byte(`version: "1.0"
id: "lock-test"
op: echo
inputs:
  message: "Lock test v4"
`)
	_, err = svc.UpdateRecipe(ctx, model.UpdateInput{
		ProjectID:      projectID,
		Name:           "lock-test",
		Content:        version4Content,
		ExpectedCommit: version2.CommitHash,
		AutoPublish:    false,
	})
	if err != nil {
		t.Fatalf("UpdateRecipe with correct commit failed: %v", err)
	}
}

func setupTestService(t *testing.T, db *gorm.DB) Service {
	t.Helper()

	// Register test ops
	registerTestOps()

	store, err := store.New(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	gitRepo := testutil.NewRealGitRepository()
	mockProjects := testutil.NewMockProjectService()

	// Create a bare git repository for the test project
	projectRepoPath := t.TempDir()
	ctx := context.Background()
	cmd := testutil.GitCommand(ctx, filepath.Dir(projectRepoPath), "init", "--bare", filepath.Base(projectRepoPath))
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}

	// Create initial commit in the bare repo via a temp clone
	tempClone := t.TempDir()
	if err := gitRepo.Clone(ctx, projectRepoPath, tempClone, git.CloneOptions{}); err != nil {
		t.Fatalf("failed to clone: %v", err)
	}
	initFile := filepath.Join(tempClone, ".gitkeep")
	if err := os.WriteFile(initFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write .gitkeep: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, tempClone, []string{".gitkeep"}); err != nil {
		t.Fatalf("failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, tempClone, "Initial commit"); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	if _, err := gitRepo.Push(ctx, tempClone, git.PushOptions{
		Remote:      "origin",
		SetUpstream: true,
	}); err != nil {
		t.Fatalf("failed to push: %v", err)
	}

	// Add test project with git repo path
	mockProjects.AddProject(&project.Project{
		ID:          "proj_test",
		Name:        "Test Project",
		GitRepoPath: projectRepoPath,
	})

	workspaceRoot := t.TempDir()

	svc, err := New(ServiceConfig{
		Store:         store,
		GitRepo:       gitRepo,
		Projects:      mockProjects,
		IDGen:         testutil.NewMockIDGenerator("recipe_"),
		Clock:         testutil.NewMockClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	return svc
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func stringPtr(s string) *string {
	return &s
}

// Test op input/output types
type EchoInput struct {
	Message string `yaml:"message"`
}

type EchoOutput struct {
	Output string `yaml:"output"`
}

// registerTestOps registers operations needed for integration tests
func registerTestOps() {
	// Register echo op used by test recipes
	echoOp := recipeops.NewActivityMappedOpV2[EchoInput, EchoOutput](
		recipeops.OpMetadata{Type: "echo"},
		func(_ recipeops.OpDependencies, _ context.Context, input EchoInput) (EchoOutput, error) {
			return EchoOutput{
				Output: input.Message,
			}, nil
		},
	)
	recipeops.Register(echoOp)
}
