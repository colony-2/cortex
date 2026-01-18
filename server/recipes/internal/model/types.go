package model

import (
	"errors"
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
)

// CreateInput contains parameters for creating a new recipe.
type CreateInput struct {
	ProjectID   project.ID
	Name        string
	Content     []byte
	Description string
	AutoPublish bool
}

// UpdateInput contains parameters for updating an existing recipe.
type UpdateInput struct {
	ProjectID      project.ID
	Name           string
	Content        []byte
	Message        string
	AutoPublish    bool
	ExpectedCommit string // Expected current commit of this specific recipe file
}

// PublishInput contains parameters for publishing a recipe version.
type PublishInput struct {
	ProjectID      project.ID
	Name           string
	CommitHash     string // Empty = latest commit
	PublishedBy    *string
	ExpectedCommit *string // Expected current published commit
}

// UnpublishInput contains parameters for unpublishing a recipe.
type UnpublishInput struct {
	ProjectID      project.ID
	Name           string
	ExpectedCommit string // Expected current published commit
}

// ValidateInput contains parameters for validating a recipe.
type ValidateInput struct {
	ProjectID project.ID
	Name      string
	Content   []byte
}

// ValidationResult captures validation status and errors.
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// ValidationError provides structured validation details.
type ValidationError struct {
	Code       string
	Message    string
	Path       string
	Expression string
}

// ValidationFailedError is returned when validation fails.
type ValidationFailedError struct {
	Result *ValidationResult
}

func (e *ValidationFailedError) Error() string {
	return "recipe: validation failed"
}

func (e *ValidationFailedError) Unwrap() error {
	return ErrInvalidContent
}

// RecipeVersion represents a recipe at a specific git commit.
type RecipeVersion struct {
	Name        string
	CommitHash  string
	ShortHash   string
	Author      string
	Message     string
	CreatedAt   time.Time
	IsPublished bool
}

// RecipeInfo contains summary information about a recipe.
type RecipeInfo struct {
	Name            string
	LatestCommit    string
	LatestCommitAt  time.Time
	PublishedCommit *string
	PublishedAt     *time.Time
	PublishedBy     *string
}

// RecipeWithContent contains a recipe with its raw YAML content and metadata.
type RecipeWithContent struct {
	Name        string
	CommitHash  string
	Content     []byte // Raw YAML bytes - preserves original formatting
	IsPublished bool
	PublishedAt *time.Time
	PublishedBy *string
}

// PublishStatus represents the publish status filter for listing recipes.
type PublishStatus string

const (
	PublishStatusAll         PublishStatus = "all"
	PublishStatusPublished   PublishStatus = "published"
	PublishStatusUnpublished PublishStatus = "unpublished"
)

// RecipeFilter contains criteria for filtering recipes.
type RecipeFilter struct {
	ProjectIDs    []project.ID
	Names         []string
	NamePrefix    string
	PublishStatus PublishStatus
}

// Clock provides time for testability.
type Clock interface {
	Now() time.Time
}

// Iterator provides paginated access to results.
type Iterator[T any] interface {
	Next() (T, error)
	Close() error
}

// Error types
var (
	ErrEmptyName             = errors.New("recipe: name is required")
	ErrInvalidName           = errors.New("recipe: invalid name format")
	ErrInvalidProject        = errors.New("recipe: project not found")
	ErrNotFound              = errors.New("recipe: not found")
	ErrAlreadyExists         = errors.New("recipe: already exists")
	ErrNotPublished          = errors.New("recipe: recipe is not published")
	ErrVersionConflict       = errors.New("recipe: version conflict")
	ErrInvalidContent        = errors.New("recipe: invalid recipe content")
	ErrCommitNotFound        = errors.New("recipe: commit not found in git")
	ErrGitConflict           = errors.New("recipe: git merge conflict detected")
	ErrRemoteSync            = errors.New("recipe: failed to sync with remote")
	ErrValidationUnavailable = errors.New("recipe: validation service unavailable")
	ErrIteratorDone          = errors.New("recipe: iterator done")
	ErrOptimisticLock        = errors.New("recipe: optimistic lock failed")
)
