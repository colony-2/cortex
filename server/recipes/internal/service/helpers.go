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

// createEphemeralWorkspace creates a temporary workspace with sparse checkout
// for the given project. Returns the workspace path and a cleanup function.
// The caller MUST defer the cleanup function to ensure workspace removal.
func (s *service) createEphemeralWorkspace(
	ctx context.Context,
	projectID project.ID,
) (workspacePath string, cleanup func(), err error) {
	// Get project details
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get project: %w", err)
	}

	if proj.GitRepoPath == "" {
		return "", nil, fmt.Errorf("project has no git repository configured")
	}

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("recipe-workspace-%s-*", projectID))
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Cleanup function to remove temp directory
	cleanup = func() {
		if err := os.RemoveAll(tempDir); err != nil {
			// Log error but don't fail - best effort cleanup
			fmt.Fprintf(os.Stderr, "warning: failed to cleanup workspace %s: %v\n", tempDir, err)
		}
	}

	// If any error occurs after this point, cleanup temp dir
	defer func() {
		if err != nil && cleanup != nil {
			cleanup()
		}
	}()

	// Perform shallow clone with sparse checkout
	depth := 1
	cloneOpts := git.CloneOptions{
		Depth:        &depth,
		SingleBranch: true,
		SparseCheckout: &git.SparseCheckoutOptions{
			Cone:  true,
			Paths: []string{".c2/recipes"},
		},
	}

	if proj.GitRepoBranch != nil && *proj.GitRepoBranch != "" {
		cloneOpts.Branch = *proj.GitRepoBranch
	}

	if err := s.gitRepo.Clone(ctx, proj.GitRepoPath, tempDir, cloneOpts); err != nil {
		return "", nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Configure git user for commits
	if err := s.gitRepo.ConfigureUser(ctx, tempDir, "Recipe Service", "recipes@colony2.internal"); err != nil {
		return "", nil, fmt.Errorf("failed to configure git user: %w", err)
	}

	// Ensure .c2/recipes directory exists
	recipesDir := filepath.Join(tempDir, ".c2", "recipes")
	if _, err := os.Stat(recipesDir); os.IsNotExist(err) {
		if err := os.MkdirAll(recipesDir, 0755); err != nil {
			return "", nil, fmt.Errorf("failed to create recipes directory: %w", err)
		}

		// Create .gitkeep to ensure directory is tracked
		gitkeepPath := filepath.Join(recipesDir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte(""), 0644); err != nil {
			return "", nil, fmt.Errorf("failed to create .gitkeep: %w", err)
		}

		// Stage, commit, and push initial structure
		if err := s.gitRepo.StageFiles(ctx, tempDir, []string{".c2/recipes/.gitkeep"}); err != nil {
			return "", nil, fmt.Errorf("failed to stage .gitkeep: %w", err)
		}

		if err := s.gitRepo.CreateCommit(ctx, tempDir, "Initialize recipe repository"); err != nil {
			return "", nil, fmt.Errorf("failed to create initial commit: %w", err)
		}

		if err := s.pushToOrigin(ctx, tempDir); err != nil {
			return "", nil, fmt.Errorf("failed to push initial structure: %w", err)
		}
	}

	return tempDir, cleanup, nil
}

// pushToOrigin pushes changes to the primary repository.
// The git module handles bare vs non-bare repository logic automatically.
func (s *service) pushToOrigin(ctx context.Context, workspacePath string) error {
	// Push to origin (git module handles bare/non-bare logic)
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
		return fmt.Errorf("push was rejected by remote (possibly not a fast-forward)")
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
func (s *service) preValidateRecipe(ctx context.Context, input model.ValidateInput) error {
	_, err := s.validateRecipe(ctx, input)
	return err
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
