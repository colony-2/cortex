package service

import (
	"context"
	"fmt"
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

// ListRecipes lists recipes matching the filter criteria.
func (s *service) ListRecipes(ctx context.Context, filter model.RecipeFilter) (store.Iterator[*model.RecipeInfo], error) {
	// Validate project exists if filtering by single project
	if len(filter.ProjectIDs) == 1 {
		if err := s.ensureProject(ctx, filter.ProjectIDs[0]); err != nil {
			return nil, err
		}
	}

	var allRecipes []*model.RecipeInfo

	// For each project, list recipes from git
	projectIDs := filter.ProjectIDs
	if len(projectIDs) == 0 {
		// If no project filter, this would need to list all projects
		// For now, return empty - this is a safety measure
		return store.NewSliceIterator([]*model.RecipeInfo{}), nil
	}

	for _, projectID := range projectIDs {
		gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get git workspace: %w", err)
		}

		if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
			return nil, fmt.Errorf("failed to sync workspace: %w", err)
		}

		// List all recipe files in .c2/recipes/
		recipesDir := filepath.Join(gitWorkspace, ".c2", "recipes")
		recipeFiles, err := s.findRecipeFiles(ctx, recipesDir)
		if err != nil {
			// If directory doesn't exist, no recipes for this project
			continue
		}

		// Get published recipes for this project
		publishedRecipes, err := s.getPublishedRecipesMap(ctx, projectID)
		if err != nil {
			return nil, err
		}

		// Build RecipeInfo for each file
		for _, recipeFile := range recipeFiles {
			name := s.filePathToRecipeName(recipeFile, recipesDir)
			gitPath := deriveGitPath(name)

			// Get latest commit for this file
			latestCommit, latestDate, err := s.getFileLatestCommitWithDate(ctx, gitWorkspace, gitPath)
			if err != nil {
				continue
			}

			info := &model.RecipeInfo{
				Name:           name,
				LatestCommit:   latestCommit,
				LatestCommitAt: latestDate,
			}

			// Add published info if available
			if pub, ok := publishedRecipes[name]; ok {
				info.PublishedCommit = &pub.CommitHash
				info.PublishedAt = &pub.PublishedAt
				info.PublishedBy = pub.PublishedBy
			}

			allRecipes = append(allRecipes, info)
		}
	}

	// Apply filters
	filtered := s.applyRecipeFilters(allRecipes, filter)

	return store.NewSliceIterator(filtered), nil
}

// GetRecipeHistory returns the git commit history for a specific recipe.
func (s *service) GetRecipeHistory(ctx context.Context, projectID project.ID, name string) (store.Iterator[*model.RecipeVersion], error) {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, projectID); err != nil {
		return nil, err
	}

	// 2. Get git workspace and sync
	gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get git workspace: %w", err)
	}

	if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
		return nil, fmt.Errorf("failed to sync workspace: %w", err)
	}

	gitPath := deriveGitPath(name)

	// 3. Get git commit history for this file
	cmd := exec.CommandContext(ctx, "git", "log", "--follow", "--format=%H|%an|%at|%s", "--", gitPath)
	cmd.Dir = gitWorkspace
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git history: %w", err)
	}

	// 4. Get published commit (if any)
	var publishedCommit string
	publishedRecipe, err := s.store.GetByName(ctx, projectID, name)
	if err == nil {
		publishedCommit = publishedRecipe.CommitHash
	}

	// 5. Parse commits
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	versions := make([]*model.RecipeVersion, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}

		commitHash := parts[0]
		author := parts[1]
		timestampStr := parts[2]
		message := parts[3]

		// Parse timestamp
		var timestamp int64
		fmt.Sscanf(timestampStr, "%d", &timestamp)
		createdAt := time.Unix(timestamp, 0)

		version := &model.RecipeVersion{
			Name:        name,
			CommitHash:  commitHash,
			ShortHash:   shortHash(commitHash),
			Author:      author,
			Message:     message,
			CreatedAt:   createdAt,
			IsPublished: commitHash == publishedCommit,
		}

		versions = append(versions, version)
	}

	return store.NewSliceIterator(versions), nil
}

// ValidateRecipe validates recipe content without creating or publishing.
func (s *service) ValidateRecipe(ctx context.Context, content []byte) error {
	_, err := recipecore.LoadRecipeFromString(content)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

// SyncFromRemote syncs the local workspace from the remote repository.
func (s *service) SyncFromRemote(ctx context.Context, projectID project.ID) error {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, projectID); err != nil {
		return err
	}

	// 2. Get git workspace
	gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get git workspace: %w", err)
	}

	// 3. Fetch from remote
	if err := s.gitRepo.Fetch(ctx, gitWorkspace, git.FetchOptions{
		Remote: "origin",
	}); err != nil {
		return fmt.Errorf("%w: %v", model.ErrRemoteSync, err)
	}

	// 4. Pull changes
	if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
		return fmt.Errorf("%w: %v", model.ErrRemoteSync, err)
	}

	return nil
}
