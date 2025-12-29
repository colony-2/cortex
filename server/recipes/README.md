# Recipe Service

A git-backed recipe storage and versioning service for Colony2. The Recipe Service provides lifecycle management for recipe definitions with git-based versioning and a lightweight database index for published recipes.

## Overview

The Recipe Service manages recipes through a simple API while using git for version control behind the scenes. Users never interact with git directly - the service owns all recipe mutations and git operations.

### Key Features

- **Git-backed storage** - All recipe content and history stored in git (`.c2/recipes/` directory)
- **Service-managed lifecycle** - Create, update, delete, publish, and unpublish through API
- **Version references** - Access recipes by published version or any git ref (commit, branch, tag)
- **Optimistic concurrency** - Prevent conflicts during concurrent operations
- **Automatic validation** - Recipes validated before publishing
- **Hierarchical organization** - Support for nested recipe names (e.g., `workflows/ci/build`)

## Key Concepts

### Recipe Lifecycle

1. **Create** - Service creates recipe file and commits to git
2. **Update** - Service updates recipe file and commits (creates new version)
3. **Publish** - Service validates and marks a specific version as published
4. **Unpublish** - Service removes published reference
5. **Delete** - Service removes recipe file and commits

### Recipe References

Recipes can be referenced in two ways:

- **Simple name** - `"workflows/ci/build"` returns the currently published version
- **Name with ref** - `"workflows/ci/build@<ref>"` returns recipe at any git ref

Supported ref formats:
- Commit hash: `"workflows/ci/build@a1b2c3d4e5f6"` (full or short)
- Branch: `"workflows/ci/build@main"`
- Tag: `"workflows/ci/build@v1.0.0"`
- Relative: `"workflows/ci/build@HEAD~1"`

### Publishing Model

- Each recipe name has exactly one published version at a time
- Only published recipes are accessible to consumers by default
- Re-publishing updates which version is considered published
- All versions remain in git history and can be accessed via `name@ref`

## Installation

```go
import "github.com/colony-2/colony2/server/recipes/pkg/recipe"
```

## Quick Start

### Create a Service

```go
import (
    "github.com/colony-2/colony2/server/recipes/pkg/recipe"
    "gorm.io/gorm"
)

// Option 1: From existing DB connection
svc, err := recipe.NewServiceFromDB(db, recipe.ServiceConfig{
    GitRepo:       gitRepo,        // git.Repository implementation
    Projects:      projectService, // project.Service for validation
    IDGen:         recipe.NewKSUIDGenerator(),
    Clock:         recipe.NewSystemClock(),
    WorkspaceRoot: "/var/lib/colony2/recipe-workspaces",
})

// Option 2: With custom store
store, _ := recipe.NewStore(db)
svc, err := recipe.NewService(recipe.ServiceConfig{
    Store:         store,
    GitRepo:       gitRepo,
    Projects:      projectService,
    IDGen:         recipe.NewKSUIDGenerator(),
    Clock:         recipe.NewSystemClock(),
    WorkspaceRoot: "/var/lib/colony2/recipe-workspaces",
})
```

### Create and Publish a Recipe

```go
// Create and auto-publish in one step
version, err := svc.CreateRecipe(ctx, recipe.CreateInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    Content:     []byte(`
version: "1.0"
id: "workflows/ci/build"
op: echo
inputs:
  message: "Building project..."
`),
    Description: "CI build workflow",
    AutoPublish: true, // Automatically publish after creation
})

fmt.Printf("Created and published at commit: %s\n", version.CommitHash)
```

### Update a Recipe

```go
// Update without auto-publish (for review)
version, err := svc.UpdateRecipe(ctx, recipe.UpdateInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    Content:     updatedContent,
    Message:     "Update build message",
    AutoPublish: false, // Review before publishing
})

// Later, publish the new version
err = svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    CommitHash:  version.CommitHash,
    PublishedBy: stringPtr("user@example.com"),
})
```

### Get a Recipe

```go
// Get published version
recipeWithContent, err := svc.GetRecipe(ctx, "proj_123", "workflows/ci/build", "")

fmt.Printf("Recipe: %s\n", recipeWithContent.Name)
fmt.Printf("Commit: %s\n", recipeWithContent.CommitHash)
fmt.Printf("Is Published: %v\n", recipeWithContent.IsPublished)

// Access the parsed recipe
recipeContent := recipeWithContent.Content // *recipe.Recipe

// Get specific version by commit hash
recAtCommit, err := svc.GetRecipe(ctx, "proj_123", "workflows/ci/build", "a1b2c3d")

// Get from a tag
recAtTag, err := svc.GetRecipe(ctx, "proj_123", "workflows/ci/build", "v1.0.0")
```

### List Recipes

```go
// List all published recipes in a project
iter, err := svc.ListRecipes(ctx, recipe.RecipeFilter{
    ProjectIDs:    []project.ID{"proj_123"},
    PublishStatus: recipe.PublishStatusPublished,
})
defer iter.Close(ctx)

for {
    info, err := iter.Next(ctx)
    if errors.Is(err, recipe.ErrIteratorDone) {
        break
    }
    if err != nil {
        return err
    }

    fmt.Printf("Recipe: %s\n", info.Name)
    fmt.Printf("Latest: %s at %v\n", info.LatestCommit, info.LatestCommitAt)
    if info.PublishedCommit != nil {
        fmt.Printf("Published: %s at %v\n", *info.PublishedCommit, *info.PublishedAt)
    }
}
```

### View Recipe History

```go
// Get version history for a recipe
iter, err := svc.GetRecipeHistory(ctx, "proj_123", "workflows/ci/build")
defer iter.Close(ctx)

for {
    version, err := iter.Next(ctx)
    if errors.Is(err, recipe.ErrIteratorDone) {
        break
    }
    if err != nil {
        return err
    }

    fmt.Printf("Commit: %s (%s)\n", version.CommitHash, version.ShortHash)
    fmt.Printf("Author: %s\n", version.Author)
    fmt.Printf("Date: %v\n", version.CreatedAt)
    fmt.Printf("Message: %s\n", version.Message)
    fmt.Printf("Published: %v\n", version.IsPublished)
}
```

## Using the RecipeProvider

The `RecipeProvider` implements the `recipe.RecipeProvider` interface from `server/recipe-core` and supports the `name@ref` syntax:

```go
import recipecore "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"

// Create provider for a project
provider := recipe.NewProvider(svc, "proj_123")

// Get published version
rec, err := provider.GetRecipe("workflows/ci/build")

// Get specific commit
rec, err := provider.GetRecipe("workflows/ci/build@a1b2c3d4e5f6")

// Get from branch
rec, err := provider.GetRecipe("workflows/ci/build@main")

// Get from tag
rec, err := provider.GetRecipe("workflows/ci/build@v1.0.0")

// Get previous version
rec, err := provider.GetRecipe("workflows/ci/build@HEAD~1")
```

## Common Workflows

### Test Before Publishing

```go
// 1. Create new version without publishing
version, err := svc.UpdateRecipe(ctx, recipe.UpdateInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    Content:     newContent,
    Message:     "Add new build features",
    AutoPublish: false, // Don't publish yet
})

// 2. Test the unpublished version
provider := recipe.NewProvider(svc, "proj_123")
testRec, err := provider.GetRecipe("workflows/ci/build@" + version.CommitHash)

// Run tests with testRec...

// 3. If tests pass, publish
err = svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:  "proj_123",
    Name:       "workflows/ci/build",
    CommitHash: version.CommitHash,
})
```

### Version Pinning in Dependencies

```yaml
# Recipe can reference specific versions of other recipes
version: "1.0"
id: "workflows/deploy"
op: sequence
inputs:
  steps:
    # Pin to specific commit (maximum stability)
    - recipe: "workflows/ci/build@a1b2c3d4e5f6"

    # Pin to tag (semantic versioning)
    - recipe: "workflows/test@v1.2.0"

    # Use published version (most common)
    - recipe: "workflows/cleanup"
```

### Rollback to Previous Version

```go
// Get history to find the commit you want
iter, _ := svc.GetRecipeHistory(ctx, "proj_123", "workflows/ci/build")

// Re-publish an older commit
err := svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    CommitHash:  "previous-commit-hash",
    PublishedBy: stringPtr("user@example.com"),
})
```

## Recipe Naming Rules

Recipe names must follow these rules:

- **Valid characters**: Alphanumeric, dash (`-`), underscore (`_`), and forward slash (`/`)
- **Path segments**: Each directory/file name can only contain `[a-zA-Z0-9_-]`
- **Hierarchical**: Use `/` for organization (e.g., `workflows/ci/build`)

✅ Valid names:
- `deployment-recipe`
- `workflows/ci/build-and-test`
- `data_processing/etl/load_users`

❌ Invalid names:
- `my.recipe` (dots not allowed)
- `my recipe` (spaces not allowed)
- `my@recipe` (@ is reserved for ref syntax)

## Error Handling

```go
import "errors"

_, err := svc.GetRecipe(ctx, projectID, name, "")
if err != nil {
    switch {
    case errors.Is(err, recipe.ErrNotPublished):
        // Recipe exists but isn't published
    case errors.Is(err, recipe.ErrNotFound):
        // Recipe doesn't exist
    case errors.Is(err, recipe.ErrVersionConflict):
        // Concurrent modification detected
    case errors.Is(err, recipe.ErrInvalidContent):
        // Recipe validation failed
    default:
        // Other error
    }
}
```

## Architecture

### Storage Model

- **Database**: Lightweight index storing `(ProjectID, Name) → CommitHash` mappings
- **Git**: Source of truth for all recipe content and history at `.c2/recipes/`

### Workspace Management

- Each project gets an isolated git workspace at `{WorkspaceRoot}/{ProjectID}/`
- Service automatically syncs before mutations
- All commits happen on the `main` branch

### Concurrency Control

- **Per-file optimistic locking**: Tracks last commit for each recipe file
- **Database optimistic locking**: Version field prevents concurrent DB updates
- **ExpectedCommit checks**: User-provided version expectations for atomic operations

## API Reference

### Service Interface

```go
type Service interface {
    CreateRecipe(ctx, CreateInput) (*RecipeVersion, error)
    UpdateRecipe(ctx, UpdateInput) (*RecipeVersion, error)
    DeleteRecipe(ctx, projectID, name) error
    PublishRecipe(ctx, PublishInput) (*PublishedRecipe, error)
    UnpublishRecipe(ctx, UnpublishInput) error
    GetRecipe(ctx, projectID, name, ref) (*RecipeWithContent, error)
    ListRecipes(ctx, RecipeFilter) (Iterator[*RecipeInfo], error)
    GetRecipeHistory(ctx, projectID, name) (Iterator[*RecipeVersion], error)
    ValidateRecipe(ctx, content) error
    SyncFromRemote(ctx, projectID) error
}
```

### Key Types

```go
type CreateInput struct {
    ProjectID   project.ID
    Name        string
    Content     []byte
    Description string
    AutoPublish bool
}

type UpdateInput struct {
    ProjectID      project.ID
    Name           string
    Content        []byte
    Message        string
    AutoPublish    bool
    ExpectedCommit string // For optimistic concurrency
}

type PublishInput struct {
    ProjectID      project.ID
    Name           string
    CommitHash     string  // Empty = latest
    PublishedBy    *string
    ExpectedCommit *string // For optimistic concurrency
}

type RecipeWithContent struct {
    Name        string
    CommitHash  string
    Content     *recipe.Recipe
    IsPublished bool
    PublishedAt *time.Time
    PublishedBy *string
}
```

## Testing

Run tests:

```bash
go test ./...
```

Build:

```bash
go build ./...
```

## See Also

- [Recipe Service Specification](./RECIPE_SERVICE_SPEC.md) - Detailed design documentation
- [Git Commands Specification](./GIT_COMMANDS_SPEC.md) - Git operations reference
- `server/recipe-core` - Recipe parsing and validation
- `server/git` - Git operations implementation
