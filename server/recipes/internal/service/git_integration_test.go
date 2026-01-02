package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipes/internal/testutil"
)

// TestGetOrCreateGitWorkspace_ClonesFromProjectRepo verifies that the workspace
// is cloned from the project's repository
func TestGetOrCreateGitWorkspace_ClonesFromProjectRepo(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Create a project repository
	projectRepoPath := t.TempDir()
	if err := testutil.CreateGitRepo(ctx, projectRepoPath); err != nil {
		t.Fatalf("Failed to create project repo: %v", err)
	}

	// Add a test file to project repo
	testFile := filepath.Join(projectRepoPath, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Project"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, projectRepoPath, []string{"README.md"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, projectRepoPath, "Add README"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Setup mock project service with git repo path
	mockProjects := testutil.NewMockProjectService()
	projectID := project.ID("proj_clone_test")
	mockProjects.AddProject(&project.Project{
		ID:          projectID,
		Name:        "Clone Test Project",
		GitRepoPath: projectRepoPath,
	})

	// Create service
	workspaceRoot := t.TempDir()
	svc := &service{
		gitRepo:       gitRepo,
		projects:      mockProjects,
		workspaceRoot: workspaceRoot,
	}

	// Test: getOrCreateGitWorkspace should clone from project repo
	workspacePath, err := svc.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		t.Fatalf("getOrCreateGitWorkspace failed: %v", err)
	}

	// Verify workspace was created
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); err != nil {
		t.Errorf("Workspace .git directory not found: %v", err)
	}

	// Verify README.md was cloned
	clonedReadme := filepath.Join(workspacePath, "README.md")
	content, err := os.ReadFile(clonedReadme)
	if err != nil {
		t.Errorf("Cloned README.md not found: %v", err)
	}
	if string(content) != "# Test Project" {
		t.Errorf("README.md content = %q, want %q", string(content), "# Test Project")
	}

	// Test: Calling again should reuse existing workspace
	workspacePath2, err := svc.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		t.Fatalf("Second getOrCreateGitWorkspace failed: %v", err)
	}
	if workspacePath2 != workspacePath {
		t.Errorf("Second call returned different path: %q != %q", workspacePath2, workspacePath)
	}
}

// TestGetOrCreateGitWorkspace_WithBranch verifies that cloning respects the project's branch
func TestGetOrCreateGitWorkspace_WithBranch(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Create a project repository with multiple branches
	projectRepoPath := t.TempDir()
	if err := testutil.CreateGitRepo(ctx, projectRepoPath); err != nil {
		t.Fatalf("Failed to create project repo: %v", err)
	}

	// Create a feature branch with different content
	cmd := testutil.GitCommand(ctx, projectRepoPath, "checkout", "-b", "feature")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	featureFile := filepath.Join(projectRepoPath, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("Feature content"), 0644); err != nil {
		t.Fatalf("Failed to write feature file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, projectRepoPath, []string{"feature.txt"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, projectRepoPath, "Add feature"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Setup project with specific branch
	mockProjects := testutil.NewMockProjectService()
	projectID := project.ID("proj_branch_test")
	featureBranch := "feature"
	mockProjects.AddProject(&project.Project{
		ID:            projectID,
		Name:          "Branch Test Project",
		GitRepoPath:   projectRepoPath,
		GitRepoBranch: &featureBranch,
	})

	workspaceRoot := t.TempDir()
	svc := &service{
		gitRepo:       gitRepo,
		projects:      mockProjects,
		workspaceRoot: workspaceRoot,
	}

	// Test: workspace should be cloned from feature branch
	workspacePath, err := svc.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		t.Fatalf("getOrCreateGitWorkspace failed: %v", err)
	}

	// Verify feature.txt exists (only in feature branch)
	clonedFeature := filepath.Join(workspacePath, "feature.txt")
	content, err := os.ReadFile(clonedFeature)
	if err != nil {
		t.Errorf("feature.txt not found in cloned workspace: %v", err)
	}
	if string(content) != "Feature content" {
		t.Errorf("feature.txt content = %q, want %q", string(content), "Feature content")
	}
}

// TestGetOrCreateGitWorkspace_NoGitRepo verifies proper error when project has no git repo
func TestGetOrCreateGitWorkspace_NoGitRepo(t *testing.T) {
	ctx := context.Background()

	mockProjects := testutil.NewMockProjectService()
	projectID := project.ID("proj_no_git")
	mockProjects.AddProject(&project.Project{
		ID:          projectID,
		Name:        "No Git Project",
		GitRepoPath: "", // No git repo configured
	})

	workspaceRoot := t.TempDir()
	svc := &service{
		gitRepo:       testutil.NewRealGitRepository(),
		projects:      mockProjects,
		workspaceRoot: workspaceRoot,
	}

	_, err := svc.getOrCreateGitWorkspace(ctx, projectID)
	if err == nil {
		t.Fatal("Expected error for project with no git repo, got nil")
	}
	if err.Error() != "project has no git repository configured" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestEnsureOriginRemote verifies that the origin remote is properly managed
func TestEnsureOriginRemote(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	t.Run("AddOriginWhenMissing", func(t *testing.T) {
		workspacePath := t.TempDir()
		if err := gitRepo.InitRepository(ctx, workspacePath, git.InitOptions{}); err != nil {
			t.Fatalf("Failed to init repo: %v", err)
		}

		svc := &service{gitRepo: gitRepo}
		repoURL := "/path/to/origin"

		err := svc.ensureOriginRemote(ctx, workspacePath, repoURL)
		if err != nil {
			t.Fatalf("ensureOriginRemote failed: %v", err)
		}

		// Verify origin was added
		remotes, err := gitRepo.ListRemotes(ctx, workspacePath)
		if err != nil {
			t.Fatalf("ListRemotes failed: %v", err)
		}

		found := false
		for _, remote := range remotes {
			if remote.Name == "origin" && remote.FetchURL == repoURL {
				found = true
				break
			}
		}
		if !found {
			t.Error("Origin remote not added correctly")
		}
	})

	t.Run("UpdateOriginWhenDifferent", func(t *testing.T) {
		workspacePath := t.TempDir()
		if err := gitRepo.InitRepository(ctx, workspacePath, git.InitOptions{}); err != nil {
			t.Fatalf("Failed to init repo: %v", err)
		}

		// Add origin with old URL
		oldURL := "/old/path"
		if err := gitRepo.AddRemote(ctx, workspacePath, "origin", oldURL); err != nil {
			t.Fatalf("Failed to add remote: %v", err)
		}

		svc := &service{gitRepo: gitRepo}
		newURL := "/new/path"

		err := svc.ensureOriginRemote(ctx, workspacePath, newURL)
		if err != nil {
			t.Fatalf("ensureOriginRemote failed: %v", err)
		}

		// Verify origin was updated
		remotes, err := gitRepo.ListRemotes(ctx, workspacePath)
		if err != nil {
			t.Fatalf("ListRemotes failed: %v", err)
		}

		found := false
		for _, remote := range remotes {
			if remote.Name == "origin" && remote.FetchURL == newURL {
				found = true
				break
			}
		}
		if !found {
			t.Error("Origin remote not updated correctly")
		}
	})

	t.Run("KeepOriginWhenSame", func(t *testing.T) {
		workspacePath := t.TempDir()
		if err := gitRepo.InitRepository(ctx, workspacePath, git.InitOptions{}); err != nil {
			t.Fatalf("Failed to init repo: %v", err)
		}

		repoURL := "/path/to/origin"
		if err := gitRepo.AddRemote(ctx, workspacePath, "origin", repoURL); err != nil {
			t.Fatalf("Failed to add remote: %v", err)
		}

		svc := &service{gitRepo: gitRepo}

		// Call with same URL - should not error
		err := svc.ensureOriginRemote(ctx, workspacePath, repoURL)
		if err != nil {
			t.Fatalf("ensureOriginRemote failed: %v", err)
		}
	})
}

// TestSyncWorkspace verifies that workspace sync pulls from origin
func TestSyncWorkspace(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Create origin repo
	originPath := t.TempDir()
	if err := testutil.CreateGitRepo(ctx, originPath); err != nil {
		t.Fatalf("Failed to create origin repo: %v", err)
	}

	// Clone to workspace
	workspacePath := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, workspacePath, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone: %v", err)
	}

	// Make a change in origin
	testFile := filepath.Join(originPath, "update.txt")
	if err := os.WriteFile(testFile, []byte("Updated content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, originPath, []string{"update.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, originPath, "Update"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	svc := &service{gitRepo: gitRepo}

	// Sync workspace - should pull the new commit
	err := svc.syncWorkspace(ctx, workspacePath)
	if err != nil {
		t.Fatalf("syncWorkspace failed: %v", err)
	}

	// Verify update.txt now exists in workspace
	workspaceFile := filepath.Join(workspacePath, "update.txt")
	content, err := os.ReadFile(workspaceFile)
	if err != nil {
		t.Errorf("update.txt not found after sync: %v", err)
	}
	if string(content) != "Updated content" {
		t.Errorf("Content = %q, want %q", string(content), "Updated content")
	}
}

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

// TestGitIntegration_FullWorkflow tests the complete workflow with git operations
func TestGitIntegration_FullWorkflow(t *testing.T) {
	ctx := context.Background()
	gitRepo := testutil.NewRealGitRepository()

	// Setup bare origin repo
	originPath := t.TempDir()
	cmd := testutil.GitCommand(ctx, filepath.Dir(originPath), "init", "--bare", filepath.Base(originPath))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create bare repo: %v", err)
	}

	// Create initial commit in a temp clone
	tempClone := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, tempClone, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone: %v", err)
	}
	initFile := filepath.Join(tempClone, ".gitkeep")
	if err := os.WriteFile(initFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write .gitkeep: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, tempClone, []string{".gitkeep"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, tempClone, "Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if _, err := gitRepo.Push(ctx, tempClone, git.PushOptions{
		Remote:      "origin",
		SetUpstream: true,
	}); err != nil {
		t.Fatalf("Failed to push: %v", err)
	}

	// Setup project
	mockProjects := testutil.NewMockProjectService()
	projectID := project.ID("proj_full_test")
	mockProjects.AddProject(&project.Project{
		ID:          projectID,
		Name:        "Full Test Project",
		GitRepoPath: originPath,
	})

	workspaceRoot := t.TempDir()
	svc := &service{
		gitRepo:       gitRepo,
		projects:      mockProjects,
		workspaceRoot: workspaceRoot,
	}

	// Step 1: Get workspace (should clone)
	workspace1, err := svc.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		t.Fatalf("First getOrCreateGitWorkspace failed: %v", err)
	}

	// Verify .gitkeep was cloned
	if _, err := os.Stat(filepath.Join(workspace1, ".gitkeep")); err != nil {
		t.Errorf(".gitkeep not found: %v", err)
	}

	// Step 2: Make a change in another clone (simulating another user)
	otherClone := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, otherClone, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone for other user: %v", err)
	}
	otherFile := filepath.Join(otherClone, "other.txt")
	if err := os.WriteFile(otherFile, []byte("Other change"), 0644); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, otherClone, []string{"other.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, otherClone, "Other change"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if _, err := gitRepo.Push(ctx, otherClone, git.PushOptions{
		Remote: "origin",
	}); err != nil {
		t.Fatalf("Failed to push: %v", err)
	}

	// Step 3: Sync workspace (should pull other change)
	if err := svc.syncWorkspace(ctx, workspace1); err != nil {
		t.Fatalf("syncWorkspace failed: %v", err)
	}

	// Verify other.txt now exists
	if _, err := os.Stat(filepath.Join(workspace1, "other.txt")); err != nil {
		t.Errorf("other.txt not found after sync: %v", err)
	}

	// Step 4: Make our own change and push
	ourFile := filepath.Join(workspace1, "our.txt")
	if err := os.WriteFile(ourFile, []byte("Our change"), 0644); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if err := gitRepo.StageFiles(ctx, workspace1, []string{"our.txt"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	if err := gitRepo.CreateCommit(ctx, workspace1, "Our change"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if err := svc.pushToOrigin(ctx, workspace1); err != nil {
		t.Fatalf("pushToOrigin failed: %v", err)
	}

	// Step 5: Verify our change made it to origin by cloning fresh
	verifyClone := t.TempDir()
	if err := gitRepo.Clone(ctx, originPath, verifyClone, git.CloneOptions{}); err != nil {
		t.Fatalf("Failed to clone for verification: %v", err)
	}
	if _, err := os.Stat(filepath.Join(verifyClone, "our.txt")); err != nil {
		t.Errorf("our.txt not found in fresh clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(verifyClone, "other.txt")); err != nil {
		t.Errorf("other.txt not found in fresh clone: %v", err)
	}
}
