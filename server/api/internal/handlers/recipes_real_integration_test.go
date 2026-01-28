package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipeops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
	"github.com/stretchr/testify/require"
)

// TestRecipeHandlers_RealIntegration tests the recipe handlers with real services
// This test exposes the iterator done error bug
func TestRecipeHandlers_RealIntegration(t *testing.T) {
	ctx := context.Background()

	// Register test ops
	registerTestOpsForHandlers()

	// Start embedded postgres
	pg := pgembed.StartEmbeddedPostgres(t)
	defer pg.Close(t)

	// Setup git repository
	repoRoot := t.TempDir()
	runGit(t, "init", "-q", repoRoot)
	runGit(t, "-C", repoRoot, "config", "user.email", "test@example.com")
	runGit(t, "-C", repoRoot, "config", "user.name", "Test User")
	runGit(t, "-C", repoRoot, "checkout", "-q", "-b", "main")
	runGit(t, "-C", repoRoot, "commit", "--allow-empty", "-m", "init")

	// Setup project service
	projectStore, err := project.NewStore(pg.DB)
	require.NoError(t, err)
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	require.NoError(t, err)

	// Create test project
	proj, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "test-project",
		GitRepoPath: repoRoot,
	})
	require.NoError(t, err)
	projID := string(proj.ID)

	// Setup recipe service with real implementation
	recipeStore, err := recipesvc.NewStore(pg.DB)
	require.NoError(t, err)

	gitRepo := git.NewRepository(git.Config{
		DefaultAuthor: "Test User",
		DefaultEmail:  "test@example.com",
	})

	recipeSvc, err := recipesvc.NewService(recipesvc.ServiceConfig{
		Store:        recipeStore,
		GitRepo:      gitRepo,
		Projects:     projectSvc,
		IDGen:        recipesvc.NewKSUIDGenerator(),
		Clock:        recipesvc.NewSystemClock(),
		CELValidator: recipesvc.NewRecipeWorkerCELValidator(recipeops.NewServiceDepsBuilder().Build()),
	})
	require.NoError(t, err)

	// Setup handlers
	h := &Handlers{recipeSvc: recipeSvc}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Test 1: List recipes (should be empty initially)
	t.Run("ListRecipesEmpty", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/projects/"+projID+"/recipes", nil)
		require.NoError(t, err)

		resp, err := srv.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// This should NOT return 500 with "iterator: done" error
		if resp.StatusCode != http.StatusOK {
			var errResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("Expected 200, got %d. Error: %v", resp.StatusCode, errResp)
		}

		var listResp openapi.RecipeListResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		require.Equal(t, 0, len(listResp.Recipes), "should have no recipes initially")
	})

	// Test 2: Create a recipe
	t.Run("CreateRecipe", func(t *testing.T) {
		// Can't easily create via API without full request handling, so we'll create via service
		version, err := recipeSvc.CreateRecipe(ctx, recipesvc.CreateInput{
			ProjectID:   project.ID(projID),
			Name:        "test-recipe",
			Content:     []byte("version: \"1.0\"\nid: test-recipe\nop: echo\ninputs:\n  message: \"Hello\""),
			Description: "Test recipe",
			AutoPublish: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, version.CommitHash)
	})

	// Test 3: List recipes (should have 1 recipe now)
	t.Run("ListRecipesWithData", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/projects/"+projID+"/recipes", nil)
		require.NoError(t, err)

		resp, err := srv.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// THIS IS THE KEY TEST - this will fail with "iterator: done" error before the fix
		if resp.StatusCode != http.StatusOK {
			var errResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("Expected 200, got %d. Error: %v", resp.StatusCode, errResp)
		}

		var listResp openapi.RecipeListResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		require.Equal(t, 1, len(listResp.Recipes), "should have 1 recipe")
		require.Equal(t, "test-recipe", listResp.Recipes[0].Name)
	})

	// Test 4: Get recipe history
	t.Run("GetRecipeHistory", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/projects/"+projID+"/recipes/test-recipe/history", nil)
		require.NoError(t, err)

		resp, err := srv.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// THIS IS THE KEY TEST - this will fail with "iterator: done" error before the fix
		if resp.StatusCode != http.StatusOK {
			var errResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("Expected 200, got %d. Error: %v", resp.StatusCode, errResp)
		}

		var historyResp openapi.RecipeHistoryResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&historyResp))
		require.GreaterOrEqual(t, len(historyResp.Versions), 1, "should have at least 1 version")
	})
}

// Test op input/output types
type EchoInput struct {
	Message string `yaml:"message"`
}

type EchoOutput struct {
	Output string `yaml:"output"`
}

// registerTestOpsForHandlers registers operations needed for handler tests
func registerTestOpsForHandlers() {
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
