package service

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
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
		// Create ephemeral workspace for this project
		workspace, cleanup, err := s.createEphemeralWorkspace(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to create workspace: %w", err)
		}
		// Cleanup will be called at end of this iteration
		defer cleanup()

		// List all recipe files in .c2/recipes/
		recipesDir := filepath.Join(workspace, ".c2", "recipes")
		recipeFiles, err := s.findRecipeFiles(ctx, recipesDir)
		if err != nil {
			// If directory doesn't exist, no recipes for this project
			cleanup() // Cleanup before continuing
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
			latestCommit, latestDate, err := s.getFileLatestCommitWithDate(ctx, workspace, gitPath)
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

		// Cleanup workspace explicitly after processing this project
		cleanup()
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

	// 2. Create ephemeral workspace
	workspace, cleanup, err := s.createEphemeralWorkspace(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	defer cleanup()

	gitPath := deriveGitPath(name)

	// 3. Get git commit history for this file
	cmd := exec.CommandContext(ctx, "git", "log", "--follow", "--format=%H|%an|%at|%s", "--", gitPath)
	cmd.Dir = workspace
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

// SyncFromRemote syncs the local workspace from the remote repository.
// With ephemeral workspaces, this is a no-op since every operation creates
// a fresh workspace from the remote. Kept for API compatibility.
func (s *service) SyncFromRemote(ctx context.Context, projectID project.ID) error {
	// Validate project exists
	if err := s.ensureProject(ctx, projectID); err != nil {
		return err
	}

	// No action needed - ephemeral workspaces are always synced on creation
	return nil
}
