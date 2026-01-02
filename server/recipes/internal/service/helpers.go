package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipecore "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/store"
)

// ensureProject validates that a project exists.
func (s *service) ensureProject(ctx context.Context, projectID project.ID) error {
	_, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return model.ErrInvalidProject
	}
	return nil
}

// getOrCreateGitWorkspace gets or creates the git workspace for a project.
func (s *service) getOrCreateGitWorkspace(ctx context.Context, projectID project.ID) (string, error) {
	workspacePath := filepath.Join(s.workspaceRoot, string(projectID))

	// Get project to access repository information
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project: %w", err)
	}

	if proj.GitRepoPath == "" {
		return "", fmt.Errorf("project has no git repository configured")
	}

	// Check if workspace already exists
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); err == nil {
		// Workspace exists, ensure origin remote is configured
		if err := s.ensureOriginRemote(ctx, workspacePath, proj.GitRepoPath); err != nil {
			return "", fmt.Errorf("failed to ensure origin remote: %w", err)
		}
		return workspacePath, nil
	}

	// Create workspace directory
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Clone the project's repository
	cloneOpts := git.CloneOptions{}
	if proj.GitRepoBranch != nil && *proj.GitRepoBranch != "" {
		cloneOpts.Branch = *proj.GitRepoBranch
	}

	if err := s.gitRepo.Clone(ctx, proj.GitRepoPath, workspacePath, cloneOpts); err != nil {
		return "", fmt.Errorf("failed to clone repository from %s: %w", proj.GitRepoPath, err)
	}

	// Ensure .c2/recipes directory structure exists
	recipesDir := filepath.Join(workspacePath, ".c2", "recipes")
	if _, err := os.Stat(recipesDir); os.IsNotExist(err) {
		if err := os.MkdirAll(recipesDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create recipes directory: %w", err)
		}

		// Create a .gitkeep file to ensure directory is tracked
		gitkeepPath := filepath.Join(recipesDir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte(""), 0644); err != nil {
			return "", fmt.Errorf("failed to create .gitkeep: %w", err)
		}

		// Stage and commit initial structure
		if err := s.gitRepo.StageFiles(ctx, workspacePath, []string{".c2/recipes/.gitkeep"}); err != nil {
			return "", fmt.Errorf("failed to stage .gitkeep: %w", err)
		}

		if err := s.gitRepo.CreateCommit(ctx, workspacePath, "Initialize recipe repository"); err != nil {
			return "", fmt.Errorf("failed to create initial commit: %w", err)
		}

		// Push the initial structure to origin
		if err := s.pushToOrigin(ctx, workspacePath); err != nil {
			return "", fmt.Errorf("failed to push initial structure: %w", err)
		}
	}

	return workspacePath, nil
}

// ensureOriginRemote ensures the origin remote is configured correctly.
func (s *service) ensureOriginRemote(ctx context.Context, workspacePath, repoURL string) error {
	// List existing remotes
	remotes, err := s.gitRepo.ListRemotes(ctx, workspacePath)
	if err != nil {
		return fmt.Errorf("failed to list remotes: %w", err)
	}

	// Check if origin exists
	var hasOrigin bool
	var originURL string
	for _, remote := range remotes {
		if remote.Name == "origin" {
			hasOrigin = true
			originURL = remote.FetchURL
			break
		}
	}

	if !hasOrigin {
		// Add origin remote
		if err := s.gitRepo.AddRemote(ctx, workspacePath, "origin", repoURL); err != nil {
			return fmt.Errorf("failed to add origin remote: %w", err)
		}
	} else if originURL != repoURL {
		// Origin exists but with different URL - update it
		cmd := exec.CommandContext(ctx, "git", "remote", "set-url", "origin", repoURL)
		cmd.Dir = workspacePath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to update origin remote URL: %w", err)
		}
	}

	return nil
}

// syncWorkspace syncs the workspace with the primary repository.
func (s *service) syncWorkspace(ctx context.Context, workspacePath string) error {
	// Pull from origin to get latest changes
	pullResult, err := s.gitRepo.Pull(ctx, workspacePath, git.PullOptions{
		Remote:      "origin",
		Branch:      "", // Use tracking branch
		FastForward: true,
		Rebase:      false,
	})
	if err != nil {
		return fmt.Errorf("failed to pull from origin: %w", err)
	}

	// Log if there were updates (for debugging)
	if pullResult.Updated {
		// Repository was updated
		_ = pullResult // Suppress unused warning
	}

	return nil
}

// pushToOrigin pushes changes to the primary repository.
func (s *service) pushToOrigin(ctx context.Context, workspacePath string) error {
	// Push to origin
	pushResult, err := s.gitRepo.Push(ctx, workspacePath, git.PushOptions{
		Remote:      "origin",
		Branch:      "", // Use current branch
		Force:       false,
		SetUpstream: true,
	})
	if err != nil {
		return fmt.Errorf("failed to push to origin: %w", err)
	}

	if pushResult.Rejected {
		return fmt.Errorf("push was rejected by remote")
	}

	return nil
}

// fileExistsInGit checks if a file exists in the git working tree.
func (s *service) fileExistsInGit(ctx context.Context, workspacePath, gitPath string) (bool, error) {
	filePath := filepath.Join(workspacePath, gitPath)
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// getFileLastCommit gets the last commit hash that modified a file.
func (s *service) getFileLastCommit(ctx context.Context, workspacePath, gitPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%H", "--", gitPath)
	cmd.Dir = workspacePath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getFileLatestCommitWithDate gets the latest commit hash and date for a file.
func (s *service) getFileLatestCommitWithDate(ctx context.Context, workspacePath, gitPath string) (string, time.Time, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%H|%at", "--", gitPath)
	cmd.Dir = workspacePath
	output, err := cmd.Output()
	if err != nil {
		return "", time.Time{}, err
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) != 2 {
		return "", time.Time{}, fmt.Errorf("unexpected git log output format")
	}

	commitHash := parts[0]
	var timestamp int64
	fmt.Sscanf(parts[1], "%d", &timestamp)
	date := time.Unix(timestamp, 0)

	return commitHash, date, nil
}

// findRecipeFiles recursively finds all .recipe.yaml files in a directory.
func (s *service) findRecipeFiles(ctx context.Context, dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".recipe.yaml") {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// filePathToRecipeName converts a file path to a recipe name.
func (s *service) filePathToRecipeName(filePath, recipesDir string) string {
	// Remove the recipes directory prefix
	rel, _ := filepath.Rel(recipesDir, filePath)
	// Remove the .recipe.yaml suffix
	name := strings.TrimSuffix(rel, ".recipe.yaml")
	// Convert backslashes to forward slashes (for Windows)
	name = filepath.ToSlash(name)
	return name
}

// getPublishedRecipesMap gets all published recipes for a project as a map.
func (s *service) getPublishedRecipesMap(ctx context.Context, projectID project.ID) (map[string]*model.PublishedRecipe, error) {
	iter, err := s.store.Search(ctx, model.SearchFilter{
		ProjectIDs: []project.ID{projectID},
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close(ctx)

	result := make(map[string]*model.PublishedRecipe)
	for {
		recipe, err := iter.Next(ctx)
		if errors.Is(err, store.ErrIteratorDone) {
			break
		}
		if err != nil {
			return nil, err
		}
		result[recipe.Name] = recipe
	}

	return result, nil
}

// applyRecipeFilters applies filter criteria to a list of recipes.
func (s *service) applyRecipeFilters(recipes []*model.RecipeInfo, filter model.RecipeFilter) []*model.RecipeInfo {
	var result []*model.RecipeInfo

	for _, r := range recipes {
		// Filter by exact names
		if len(filter.Names) > 0 {
			found := false
			for _, name := range filter.Names {
				if r.Name == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Filter by name prefix
		if filter.NamePrefix != "" {
			if !strings.HasPrefix(r.Name, filter.NamePrefix) {
				continue
			}
		}

		// Filter by publish status
		switch filter.PublishStatus {
		case model.PublishStatusPublished:
			if r.PublishedCommit == nil {
				continue
			}
		case model.PublishStatusUnpublished:
			if r.PublishedCommit != nil {
				continue
			}
		case model.PublishStatusAll:
			// Include all
		}

		result = append(result, r)
	}

	return result
}

// preValidateRecipe performs early validation before git write (optimization for AutoPublish).
func (s *service) preValidateRecipe(ctx context.Context, name string, content []byte, isUpdate bool) error {
	// Parse recipe to validate structure
	rec, err := recipecore.LoadRecipeFromString(content)
	if err != nil {
		return fmt.Errorf("%w: %v", model.ErrInvalidContent, err)
	}

	// Validate recipe ID matches name
	if rec.GetMetadata().ID != name {
		return fmt.Errorf("%w: recipe ID '%s' does not match name '%s'",
			model.ErrInvalidContent, rec.GetMetadata().ID, name)
	}

	return nil
}

// deriveGitPath converts a recipe name to its git file path.
func deriveGitPath(name string) string {
	return filepath.Join(".c2", "recipes", name+".recipe.yaml")
}

// validateRecipeName validates a recipe name format.
func validateRecipeName(name string) error {
	if name == "" {
		return model.ErrEmptyName
	}

	// Split into path segments
	segments := strings.Split(name, "/")

	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("%w: empty path segment", model.ErrInvalidName)
		}

		// Each segment must contain only alphanumeric, dash, underscore
		for _, r := range segment {
			if !((r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') ||
				r == '_' || r == '-') {
				return fmt.Errorf("%w: invalid character '%c' in segment '%s'",
					model.ErrInvalidName, r, segment)
			}
		}
	}

	return nil
}

// shortHash returns a short (7-character) version of a commit hash.
func shortHash(hash string) string {
	if len(hash) >= 7 {
		return hash[:7]
	}
	return hash
}
