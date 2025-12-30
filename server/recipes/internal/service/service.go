package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipes/internal/model"
	"github.com/colony-2/colony2/server/recipes/internal/store"
	"gorm.io/gorm"
)

// Service provides recipe lifecycle operations.
type Service interface {
	// Recipe Lifecycle Operations
	CreateRecipe(ctx context.Context, input model.CreateInput) (*model.RecipeVersion, error)
	UpdateRecipe(ctx context.Context, input model.UpdateInput) (*model.RecipeVersion, error)
	DeleteRecipe(ctx context.Context, projectID project.ID, name string) error

	// Publishing Operations
	PublishRecipe(ctx context.Context, input model.PublishInput) (*model.PublishedRecipe, error)
	UnpublishRecipe(ctx context.Context, input model.UnpublishInput) error

	// Retrieval Operations
	GetRecipe(ctx context.Context, projectID project.ID, name string, ref string) (*model.RecipeWithContent, error)
	ListRecipes(ctx context.Context, filter model.RecipeFilter) (store.Iterator[*model.RecipeInfo], error)
	GetRecipeHistory(ctx context.Context, projectID project.ID, name string) (store.Iterator[*model.RecipeVersion], error)

	// Validation
	ValidateRecipe(ctx context.Context, content []byte) error

	// Remote Sync
	SyncFromRemote(ctx context.Context, projectID project.ID) error
}

// ServiceConfig contains dependencies for the service.
type ServiceConfig struct {
	Store         store.Store
	GitRepo       git.Repository
	Projects      project.Service
	IDGen         model.ShortIDGenerator
	Clock         model.Clock
	WorkspaceRoot string // Root directory for git workspaces
}

type service struct {
	store         store.Store
	gitRepo       git.Repository
	projects      project.Service
	idGen         model.ShortIDGenerator
	clock         model.Clock
	workspaceRoot string
}

// New creates a new recipe service.
func New(cfg ServiceConfig) (Service, error) {
	if cfg.Store == nil {
		return nil, errors.New("recipe service: store is required")
	}
	if cfg.GitRepo == nil {
		return nil, errors.New("recipe service: git repository is required")
	}
	if cfg.Projects == nil {
		return nil, errors.New("recipe service: project service is required")
	}
	if cfg.IDGen == nil {
		return nil, errors.New("recipe service: ID generator is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = model.SystemClock{}
	}
	if cfg.WorkspaceRoot == "" {
		return nil, errors.New("recipe service: workspace root is required")
	}

	// Ensure workspace root exists
	if err := os.MkdirAll(cfg.WorkspaceRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace root: %w", err)
	}

	return &service{
		store:         cfg.Store,
		gitRepo:       cfg.GitRepo,
		projects:      cfg.Projects,
		idGen:         cfg.IDGen,
		clock:         cfg.Clock,
		workspaceRoot: cfg.WorkspaceRoot,
	}, nil
}

// CreateRecipe creates a new recipe in git and optionally publishes it.
func (s *service) CreateRecipe(ctx context.Context, input model.CreateInput) (*model.RecipeVersion, error) {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}

	// 2. Validate recipe name
	if err := validateRecipeName(input.Name); err != nil {
		return nil, err
	}

	// 3. Get git workspace and sync with primary repository
	gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, input.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get git workspace: %w", err)
	}

	if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
		return nil, fmt.Errorf("failed to sync workspace: %w", err)
	}

	// 4. Check recipe doesn't already exist
	gitPath := deriveGitPath(input.Name)
	exists, err := s.fileExistsInGit(ctx, gitWorkspace, gitPath)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, model.ErrAlreadyExists
	}

	// 5. If AutoPublish enabled: pre-validate content early
	if input.AutoPublish {
		if err := s.preValidateRecipe(ctx, input.Name, input.Content, false); err != nil {
			return nil, err
		}
	}

	// 6. Write file to git workspace
	filePath := filepath.Join(gitWorkspace, gitPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, input.Content, 0644); err != nil {
		return nil, err
	}

	// 7. Stage and commit
	if err := s.gitRepo.StageFiles(ctx, gitWorkspace, []string{gitPath}); err != nil {
		return nil, err
	}

	message := fmt.Sprintf("Create recipe: %s", input.Name)
	if input.Description != "" {
		message += "\n\n" + input.Description
	}

	if err := s.gitRepo.CreateCommit(ctx, gitWorkspace, message); err != nil {
		return nil, err
	}

	// 8. Get commit hash
	commitHash, err := s.gitRepo.GetCurrentCommit(ctx, gitWorkspace)
	if err != nil {
		return nil, err
	}

	// 9. Push to primary repository
	if err := s.pushToOrigin(ctx, gitWorkspace); err != nil {
		return nil, err
	}

	// 10. Auto-publish if requested
	isPublished := false
	if input.AutoPublish {
		_, err := s.PublishRecipe(ctx, model.PublishInput{
			ProjectID:  input.ProjectID,
			Name:       input.Name,
			CommitHash: commitHash,
		})
		if err != nil {
			return nil, err
		}
		isPublished = true
	}

	// 11. Build version info
	commits, err := s.gitRepo.GetHistory(ctx, gitWorkspace, 1)
	if err != nil {
		return nil, err
	}

	return &model.RecipeVersion{
		Name:        input.Name,
		CommitHash:  commitHash,
		ShortHash:   shortHash(commitHash),
		Author:      commits[0].Author,
		Message:     commits[0].Message,
		CreatedAt:   commits[0].Date,
		IsPublished: isPublished,
	}, nil
}

// UpdateRecipe updates an existing recipe in git and optionally publishes it.
func (s *service) UpdateRecipe(ctx context.Context, input model.UpdateInput) (*model.RecipeVersion, error) {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}

	// 2. Get git workspace and sync with primary repository
	gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, input.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get git workspace: %w", err)
	}

	if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
		return nil, fmt.Errorf("failed to sync workspace: %w", err)
	}

	gitPath := deriveGitPath(input.Name)

	// 3. Verify recipe exists
	exists, err := s.fileExistsInGit(ctx, gitWorkspace, gitPath)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, model.ErrNotFound
	}

	// 4. Optimistic Concurrency Check - get last commit for THIS FILE
	if input.ExpectedCommit != "" {
		currentCommit, err := s.getFileLastCommit(ctx, gitWorkspace, gitPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get recipe file commit: %w", err)
		}

		if input.ExpectedCommit != currentCommit {
			return nil, fmt.Errorf("%w: expected %s, got %s",
				model.ErrVersionConflict, input.ExpectedCommit, currentCommit)
		}
	}

	// 5. If AutoPublish enabled: pre-validate content early
	if input.AutoPublish {
		if err := s.preValidateRecipe(ctx, input.Name, input.Content, true); err != nil {
			return nil, err
		}
	}

	// 6. Write file
	filePath := filepath.Join(gitWorkspace, gitPath)
	if err := os.WriteFile(filePath, input.Content, 0644); err != nil {
		return nil, err
	}

	// 7. Stage and commit
	if err := s.gitRepo.StageFiles(ctx, gitWorkspace, []string{gitPath}); err != nil {
		return nil, err
	}

	message := input.Message
	if message == "" {
		message = fmt.Sprintf("Update recipe: %s", input.Name)
	}

	if err := s.gitRepo.CreateCommit(ctx, gitWorkspace, message); err != nil {
		return nil, err
	}

	// 8. Get new commit hash for this file
	newCommitHash, err := s.getFileLastCommit(ctx, gitWorkspace, gitPath)
	if err != nil {
		return nil, err
	}

	// 9. Push to primary repository
	if err := s.pushToOrigin(ctx, gitWorkspace); err != nil {
		return nil, err
	}

	// 10. Auto-publish if requested
	isPublished := false
	if input.AutoPublish {
		_, err := s.PublishRecipe(ctx, model.PublishInput{
			ProjectID:  input.ProjectID,
			Name:       input.Name,
			CommitHash: newCommitHash,
		})
		if err != nil {
			return nil, err
		}
		isPublished = true
	}

	// 11. Build version info
	commits, err := s.gitRepo.GetHistory(ctx, gitWorkspace, 1)
	if err != nil {
		return nil, err
	}

	return &model.RecipeVersion{
		Name:        input.Name,
		CommitHash:  newCommitHash,
		ShortHash:   shortHash(newCommitHash),
		Author:      commits[0].Author,
		Message:     commits[0].Message,
		CreatedAt:   commits[0].Date,
		IsPublished: isPublished,
	}, nil
}

// DeleteRecipe removes a recipe from git and unpublishes it.
func (s *service) DeleteRecipe(ctx context.Context, projectID project.ID, name string) error {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, projectID); err != nil {
		return err
	}

	// 2. Get git workspace and sync
	gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get git workspace: %w", err)
	}

	if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
		return fmt.Errorf("failed to sync workspace: %w", err)
	}

	// 3. Verify recipe exists
	gitPath := deriveGitPath(name)
	exists, err := s.fileExistsInGit(ctx, gitWorkspace, gitPath)
	if err != nil {
		return err
	}
	if !exists {
		return model.ErrNotFound
	}

	// 4. Unpublish if currently published
	publishedRecipe, err := s.store.GetByName(ctx, projectID, name)
	if err == nil {
		// Recipe is published, unpublish it first
		if err := s.store.Delete(ctx, publishedRecipe.ID); err != nil {
			return fmt.Errorf("failed to unpublish recipe: %w", err)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// 5. Remove file from git
	cmd := exec.CommandContext(ctx, "git", "rm", gitPath)
	cmd.Dir = gitWorkspace
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove file from git: %w", err)
	}

	// 6. Commit deletion
	message := fmt.Sprintf("Delete recipe: %s", name)
	if err := s.gitRepo.CreateCommit(ctx, gitWorkspace, message); err != nil {
		return err
	}

	// 7. Push to primary repository
	if err := s.pushToOrigin(ctx, gitWorkspace); err != nil {
		return err
	}

	return nil
}

// PublishRecipe validates and publishes a recipe version.
func (s *service) PublishRecipe(ctx context.Context, input model.PublishInput) (*model.PublishedRecipe, error) {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}

	// 2. Get git workspace and sync
	gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, input.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get git workspace: %w", err)
	}

	if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
		return nil, fmt.Errorf("failed to sync workspace: %w", err)
	}

	gitPath := deriveGitPath(input.Name)

	// 3. If no commit hash provided, get latest commit for this recipe file
	commitHash := input.CommitHash
	if commitHash == "" {
		var err error
		commitHash, err = s.getFileLastCommit(ctx, gitWorkspace, gitPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest commit for recipe: %w", err)
		}
	}

	// 4. Read file at specified commit from git
	content, err := s.gitRepo.GetFileAtCommit(ctx, gitWorkspace, commitHash, gitPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrCommitNotFound, err)
	}

	// 5. Parse and validate recipe structure
	rec, err := recipe.LoadRecipeFromString(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrInvalidContent, err)
	}

	// 6. Validate recipe ID matches name
	if rec.GetMetadata().ID != input.Name {
		return nil, fmt.Errorf("%w: recipe ID '%s' does not match name '%s'",
			model.ErrInvalidContent, rec.GetMetadata().ID, input.Name)
	}

	// 7. Insert or update published recipe in database
	var result *model.PublishedRecipe
	err = s.store.WithTx(ctx, func(ctx context.Context, txStore store.Store) error {
		existing, err := txStore.GetByName(ctx, input.ProjectID, input.Name)

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// New published recipe
			id, err := s.idGen.NewID()
			if err != nil {
				return err
			}

			result = &model.PublishedRecipe{
				ID:          model.ID(id),
				ProjectID:   input.ProjectID,
				Name:        input.Name,
				GitPath:     gitPath,
				CommitHash:  commitHash,
				PublishedAt: s.clock.Now(),
				PublishedBy: input.PublishedBy,
			}

			return txStore.Create(ctx, result)
		}

		if err != nil {
			return err
		}

		// Content-based optimistic locking
		if input.ExpectedCommit != nil && *input.ExpectedCommit != existing.CommitHash {
			return fmt.Errorf("%w: expected %s, current is %s",
				model.ErrVersionConflict, *input.ExpectedCommit, existing.CommitHash)
		}

		// Update existing published recipe
		existing.CommitHash = commitHash
		existing.PublishedAt = s.clock.Now()
		existing.PublishedBy = input.PublishedBy

		err = txStore.Update(ctx, existing)
		if errors.Is(err, store.ErrOptimisticLock) {
			return model.ErrVersionConflict
		}

		result = existing
		return err
	})

	return result, err
}

// UnpublishRecipe removes a recipe from the published index.
func (s *service) UnpublishRecipe(ctx context.Context, input model.UnpublishInput) error {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, input.ProjectID); err != nil {
		return err
	}

	// 2. Get and verify published recipe
	publishedRecipe, err := s.store.GetByName(ctx, input.ProjectID, input.Name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ErrNotPublished
		}
		return err
	}

	// 3. Optimistic concurrency control
	if input.ExpectedCommit != publishedRecipe.CommitHash {
		return fmt.Errorf("%w: expected %s, current is %s",
			model.ErrVersionConflict, input.ExpectedCommit, publishedRecipe.CommitHash)
	}

	// 4. Delete from published index
	return s.store.Delete(ctx, publishedRecipe.ID)
}

// GetRecipe retrieves a recipe by name with optional ref.
func (s *service) GetRecipe(ctx context.Context, projectID project.ID, name string, ref string) (*model.RecipeWithContent, error) {
	// 1. Validate project exists
	if err := s.ensureProject(ctx, projectID); err != nil {
		return nil, err
	}

	gitPath := deriveGitPath(name)

	// 2. Determine which version to fetch
	var commitHash string
	var publishedRecipe *model.PublishedRecipe

	if ref == "" {
		// Get published version
		var err error
		publishedRecipe, err = s.store.GetByName(ctx, projectID, name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, model.ErrNotPublished
			}
			return nil, err
		}
		commitHash = publishedRecipe.CommitHash
	} else {
		// Get specific version at ref
		gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get git workspace: %w", err)
		}

		// Sync workspace to ensure we have latest refs
		if err := s.syncWorkspace(ctx, gitWorkspace); err != nil {
			return nil, fmt.Errorf("failed to sync workspace: %w", err)
		}

		// Resolve ref to commit hash
		cmd := exec.CommandContext(ctx, "git", "rev-parse", ref)
		cmd.Dir = gitWorkspace
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve ref %s: %w", ref, err)
		}
		commitHash = strings.TrimSpace(string(output))

		// Check if recipe is published
		publishedRecipe, _ = s.store.GetByName(ctx, projectID, name)
	}

	// 3. Fetch content from git
	gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get git workspace: %w", err)
	}

	content, err := s.gitRepo.GetFileAtCommit(ctx, gitWorkspace, commitHash, gitPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get recipe at commit %s", model.ErrNotFound, commitHash)
	}

	// 4. Build result with raw content (no parsing - invalid recipes can be saved, just not published)
	result := &model.RecipeWithContent{
		Name:       name,
		CommitHash: commitHash,
		Content:    content,
	}

	// If published and this is the published version, include publish metadata
	if publishedRecipe != nil && publishedRecipe.CommitHash == commitHash {
		result.IsPublished = true
		result.PublishedAt = &publishedRecipe.PublishedAt
		result.PublishedBy = publishedRecipe.PublishedBy
	}

	return result, nil
}

// Continued in next file...
