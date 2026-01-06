package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/testutil"
)

// TestPushToOrigin verifies that changes are pushed to origin
func TestPushToOrigin(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Create bare origin repo (can receive pushes)
	originPath := t.TempDir()
	cmd := testutil.GitCommand(ctx, filepath.Dir(originPath), "init", "--bare", filepath.Base(originPath))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create bare repo: %v", err)
	}

	// Clone to workspace
	workspacePath := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, workspacePath, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone: %v", err)
	}

	// Configure git user for commits
	if err := testutil.ConfigureGitUser(ctx, workspacePath); err != nil {
		t.Fatalf("Failed to configure git user: %v", err)
	}

	// Make a change in workspace
	testFile := filepath.Join(workspacePath, "new.txt")
	if err := os.WriteFile(testFile, []byte("New content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, workspacePath, []string{"new.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, workspacePath, "Add new file"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	svc := &service{gitRepo: gitRepo}

	// Push to origin
	err := svc.pushToOrigin(ctx, workspacePath)
	if err != nil {
		t.Fatalf("pushToOrigin failed: %v", err)
	}

	// Verify by cloning origin to a new location
	verifyPath := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, verifyPath, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone for verification: %v", err)
	}

	// Check that new.txt exists
	verifyFile := filepath.Join(verifyPath, "new.txt")
	content, err := os.ReadFile(verifyFile)
	if err != nil {
		t.Errorf("new.txt not found in origin after push: %v", err)
	}
	if string(content) != "New content" {
		t.Errorf("Content = %q, want %q", string(content), "New content")
	}
}

// TestPushToOrigin_NonBareRepo_SameBranch verifies pushing to non-bare repo on same branch
func TestPushToOrigin_NonBareRepo_SameBranch(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Create a non-bare origin repo (regular working directory)
	originPath := t.TempDir()
	if err := testutil.CreateGitRepo(ctx, originPath); err != nil {
		t.Fatalf("Failed to create origin repo: %v", err)
	}

	// Add initial content to origin
	testFile := filepath.Join(originPath, "initial.txt")
	if err := os.WriteFile(testFile, []byte("Initial"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, originPath, []string{"initial.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, originPath, "Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Clone to workspace
	workspacePath := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, workspacePath, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone: %v", err)
	}

	// Configure git user for commits
	if err := testutil.ConfigureGitUser(ctx, workspacePath); err != nil {
		t.Fatalf("Failed to configure git user: %v", err)
	}

	// Make a change in workspace (on same branch as origin)
	newFile := filepath.Join(workspacePath, "new.txt")
	if err := os.WriteFile(newFile, []byte("New content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, workspacePath, []string{"new.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, workspacePath, "Add new file"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	svc := &service{gitRepo: gitRepo}

	// Push to non-bare origin on same branch
	// This should configure receive.denyCurrentBranch = updateInstead
	err := svc.pushToOrigin(ctx, workspacePath)
	if err != nil {
		t.Fatalf("pushToOrigin failed: %v", err)
	}

	// Verify origin's working directory was updated
	originFile := filepath.Join(originPath, "new.txt")
	content, err := os.ReadFile(originFile)
	if err != nil {
		t.Errorf("new.txt not found in origin working directory: %v", err)
	}
	if string(content) != "New content" {
		t.Errorf("Content = %q, want %q", string(content), "New content")
	}

	// Verify receive.denyCurrentBranch is set to updateInstead
	cmd := testutil.GitCommand(ctx, originPath, "config", "receive.denyCurrentBranch")
	output, err := cmd.Output()
	if err != nil {
		t.Errorf("Failed to read receive.denyCurrentBranch config: %v", err)
	}
	if strings.TrimSpace(string(output)) != "updateInstead" {
		t.Errorf("receive.denyCurrentBranch = %q, want %q", strings.TrimSpace(string(output)), "updateInstead")
	}
}

// TestPushToOrigin_NonBareRepo_DifferentBranch verifies pushing to non-bare repo on different branch
func TestPushToOrigin_NonBareRepo_DifferentBranch(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Create a non-bare origin repo (will be on default branch)
	originPath := t.TempDir()
	if err := testutil.CreateGitRepo(ctx, originPath); err != nil {
		t.Fatalf("Failed to create origin repo: %v", err)
	}

	// Add initial content to origin
	testFile := filepath.Join(originPath, "initial.txt")
	if err := os.WriteFile(testFile, []byte("Initial"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, originPath, []string{"initial.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, originPath, "Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Clone to workspace
	workspacePath := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, workspacePath, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone: %v", err)
	}

	// Configure git user for commits
	if err := testutil.ConfigureGitUser(ctx, workspacePath); err != nil {
		t.Fatalf("Failed to configure git user: %v", err)
	}

	// Create and checkout a feature branch in workspace
	cmd := testutil.GitCommand(ctx, workspacePath, "checkout", "-b", "feature")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a change in workspace on feature branch
	featureFile := filepath.Join(workspacePath, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("Feature content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, workspacePath, []string{"feature.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, workspacePath, "Add feature"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	svc := &service{gitRepo: gitRepo}

	// Push to non-bare origin on different branch (origin on default branch, workspace on feature)
	// This should work without updateInstead since we're not pushing to checked-out branch
	err := svc.pushToOrigin(ctx, workspacePath)
	if err != nil {
		t.Fatalf("pushToOrigin failed: %v", err)
	}

	// Verify feature branch exists in origin
	cmd = testutil.GitCommand(ctx, originPath, "rev-parse", "feature")
	if err := cmd.Run(); err != nil {
		t.Errorf("feature branch not found in origin: %v", err)
	}

	// Verify origin is still on its original branch (not feature)
	cmd = testutil.GitCommand(ctx, originPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get origin branch: %v", err)
	}
	originBranch := strings.TrimSpace(string(output))
	if originBranch == "feature" {
		t.Errorf("Origin should not be on feature branch, got: %q", originBranch)
	}

	// Verify feature.txt does NOT exist in origin working directory (since it's on feature branch)
	originFeatureFile := filepath.Join(originPath, "feature.txt")
	if _, err := os.Stat(originFeatureFile); err == nil {
		t.Errorf("feature.txt should not exist in origin working directory (origin is on %s, not feature)", originBranch)
	}

	// Checkout feature branch in origin to verify the content was pushed
	cmd = testutil.GitCommand(ctx, originPath, "checkout", "feature")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to checkout feature branch in origin: %v", err)
	}

	// Now verify feature.txt exists
	content, err := os.ReadFile(originFeatureFile)
	if err != nil {
		t.Errorf("feature.txt not found in origin after checkout: %v", err)
	}
	if string(content) != "Feature content" {
		t.Errorf("Content = %q, want %q", string(content), "Feature content")
	}
}

// TestCreateEphemeralWorkspace_SparseCheckout verifies ephemeral workspaces use sparse checkout
func TestCreateEphemeralWorkspace_SparseCheckout(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Create a non-bare origin repo with initial content
	projectRepoPath := t.TempDir()
	if err := testutil.CreateGitRepo(ctx, projectRepoPath); err != nil {
		t.Fatalf("Failed to create project repo: %v", err)
	}

	// Add files both inside and outside .c2/recipes
	recipesDir := filepath.Join(projectRepoPath, ".c2", "recipes")
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatalf("Failed to create recipes dir: %v", err)
	}

	// File inside .c2/recipes (should be in sparse checkout)
	recipeFile := filepath.Join(recipesDir, "test.recipe.yaml")
	if err := os.WriteFile(recipeFile, []byte("recipe: test"), 0644); err != nil {
		t.Fatalf("Failed to write recipe file: %v", err)
	}

	// File outside .c2/recipes (should NOT be in sparse checkout)
	rootFile := filepath.Join(projectRepoPath, "README.md")
	if err := os.WriteFile(rootFile, []byte("# Project"), 0644); err != nil {
		t.Fatalf("Failed to write README: %v", err)
	}

	if err := gitRepo.StageFiles(ctx, projectRepoPath, []string{".c2/recipes/test.recipe.yaml", "README.md"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, projectRepoPath, "Add files"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Setup service with project
	mockProjects := testutil.NewMockProjectService()
	projectID := "proj_sparse_test"
	mockProjects.AddProject(&project.Project{
		ID:          project.ID(projectID),
		Name:        "Sparse Test",
		GitRepoPath: projectRepoPath,
	})

	svc := &service{
		gitRepo:  gitRepo,
		projects: mockProjects,
	}

	// Create ephemeral workspace
	workspace, cleanup, err := svc.createEphemeralWorkspace(ctx, project.ID(projectID))
	if err != nil {
		t.Fatalf("createEphemeralWorkspace failed: %v", err)
	}
	defer cleanup()

	// Verify .c2/recipes exists and contains the recipe
	recipePath := filepath.Join(workspace, ".c2", "recipes", "test.recipe.yaml")
	if _, err := os.Stat(recipePath); err != nil {
		t.Errorf("Recipe file should exist in sparse checkout: %v", err)
	}

	// In cone mode, root-level files are included, so README.md should exist
	readmePath := filepath.Join(workspace, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Errorf("README.md should exist in cone mode sparse checkout (root files are included): %v", err)
	}

	// Verify workspace is in a temp directory
	if !strings.Contains(workspace, "recipe-workspace-") {
		t.Errorf("Workspace path should be in temp directory, got: %s", workspace)
	}

	// Call cleanup and verify workspace is removed
	cleanup()
	if _, err := os.Stat(workspace); err == nil {
		t.Errorf("Workspace should be removed after cleanup")
	}
}
