# Recipe Service Specification

> **Note (2026-02-07):** This document describes the legacy git-backed implementation.
> The current direction is Postgres-backed, content-addressable storage (CAS) with an append-only `recipe_events` log.
> See `POSTGRES_CAS_RECIPE_STORAGE_SPEC.md` for the up-to-date design.

## Overview

The Recipe Service provides git-backed storage and versioning for recipe definitions. Users interact with the service API to create, update, delete, and publish recipes. The service handles all git operations internally, using git for versioning and history. The database maintains a lightweight index of published recipes, mapping recipe names to specific git commit hashes.

**Key Constraints**:
- **Service-Managed Git**: Users never interact with git directly - the service owns all recipe mutations and git operations
- **Naming**: Recipe names must use only alphanumeric characters, dashes, underscores, and forward slashes (`[a-zA-Z0-9_-/]`)
- **Recipe ID Matching**: The `id` field inside each recipe YAML must match the recipe's file path name
- **Repository Structure**: Recipes stored in project's base repo under `.c2/recipes/` directory
- **Main Branch Only**: All commits happen on the `main` branch
- **Local Workspace**: Service maintains its own private local clone for all operations, syncing with primary repository
- **Early Validation**: When `AutoPublish=true`, pre-validation occurs before git write to fail fast (full validation, including CEL expression checks, still happens during publish)

## Key Concepts

**Service-Managed Git**: The Recipe Service owns all recipe lifecycle operations. Users interact with the service API, not git directly. Git is an implementation detail providing versioning and history.

**Recipe Lifecycle**:
- **Create**: Service creates recipe file and commits to git
- **Update**: Service updates recipe file and commits to git (creates new version)
- **Delete**: Service removes recipe file and commits to git
- **Publish**: Service marks a specific version (commit) as published
- Each recipe name has exactly one published version (can be changed by re-publishing)
- Only published recipes are accessible to consumers (ticket service, etc.)
- All versions remain in git history and can be accessed via `name@ref` syntax

**Minimal Database**:
- Database stores only: `(ProjectID, RecipeName) → (CommitHash, PublishedAt, PublishedBy)`
- No recipe content in database
- No version history in database (use git)
- No validation state tracking (validate on-demand during publish)

**Recipe References**:
- **Simple name** (`"workflows/ci/build"`): Retrieves the currently published version
- **Name with ref** (`"workflows/ci/build@<ref>"`): Retrieves recipe at any git ref, regardless of publish status
  - Commit hash: `"workflows/ci/build@a1b2c3d4e5f6"` (full or short hash)
  - Branch: `"workflows/ci/build@main"` or `"workflows/ci/build@feature/new-api"`
  - Tag: `"workflows/ci/build@v1.0.0"` or `"workflows/ci/build@release-2024-01"`
  - Relative ref: `"workflows/ci/build@HEAD~1"` or `"workflows/ci/build@main~2"`
- The `name@ref` syntax enables:
  - Testing unpublished recipes
  - Version pinning in recipe dependencies
  - Referencing specific known-good versions
  - Using semantic version tags
  - Testing branches before merging
  - Rollback testing without re-publishing

## Design Goals

1. **Git as Source of Truth**: All recipe content and history stored in git
2. **Flexible Storage**: Support both local and remote git repositories (including GitHub)
3. **Hierarchical Organization**: Support directory-based recipe organization (e.g., `foo/bar/myrecipe.recipe.yaml`)
4. **Publish Workflow**: Users edit recipes in git, then publish specific versions after validation
5. **Publish Conflict Prevention**: Prevent concurrent publishes of the same recipe name using optimistic locking
6. **Project Isolation**: Recipes are scoped to projects for multi-tenancy

## Architecture

### Components

```
┌─────────────────────────────────────────────────┐
│          Recipe Service API                      │
│    (Publish, Unpublish, Get, List, Validate)    │
└─────────────────────────────────────────────────┘
                      │
        ┌─────────────┴─────────────┐
        │                           │
┌───────▼────────┐         ┌────────▼────────┐
│ Published Index│         │  Git Repository │
│  (Name→Hash)   │         │ (Source of Truth)│
└────────────────┘         └─────────────────┘
```

**Note**: Git operations (clone, fetch, push, GetFileAtCommit, etc.) are defined in `/src/server/recipes/GIT_COMMANDS_SPEC.md`.

### Store Layer

The service uses a simplified dual-storage model:

1. **Published Recipe Index (Database)**: Lightweight index mapping recipe names to published commit hashes
2. **Content Store (Git)**: All recipe content, versions, and history

This separation enables:
- Fast lookup of published recipes without git operations
- Complete version history through git
- Users edit recipes directly in git using normal git workflows
- Service only tracks which versions are published

## Data Model

### Published Recipe (Database)

```go
type PublishedRecipe struct {
    ID          string         // Unique ID (KSUID)
    ProjectID   project.ID     // Project this recipe belongs to
    Name        string         // Hierarchical name (e.g., "foo/bar/myrecipe")
    GitPath     string         // Path in git repo (e.g., ".c2/recipes/foo/bar/myrecipe.recipe.yaml")
    CommitHash  string         // Published commit hash

    // Publishing metadata
    PublishedAt time.Time      // When this version was published
    PublishedBy *string        // User who published (optional)

    // Optimistic locking
    Version     int            // For concurrent publish prevention
}
```

**Constraints**:
- Unique index on (ProjectID, Name)
- Each recipe name can have exactly one published version
- Users can change which commit is published by re-publishing

### Recipe Content (Git)

Each commit in git represents a version of the recipe:
- **File**: `<name>.recipe.yaml` (e.g., `foo/bar/myrecipe.recipe.yaml`)
- **Format**: YAML as defined in `server/recipe-core`
- **Commit Message**: Standard git commit messages (no special format required)

## API Design

### Service Interface

Following the pattern from `server/cell/internal/service/service.go`:

```go
type Service interface {
    // Recipe Lifecycle Operations
    // Create a new recipe (writes to git and commits)
    CreateRecipe(ctx context.Context, input CreateInput) (*RecipeVersion, error)

    // Update an existing recipe (writes to git and commits, creating new version)
    UpdateRecipe(ctx context.Context, input UpdateInput) (*RecipeVersion, error)

    // Delete a recipe (removes from git and commits)
    DeleteRecipe(ctx context.Context, projectID project.ID, name string) error

    // Publishing Operations
    // Publish a recipe version (validates then marks as published)
    PublishRecipe(ctx context.Context, input PublishInput) (*PublishedRecipe, error)

    // Unpublish a recipe (removes from published index)
    UnpublishRecipe(ctx context.Context, input UnpublishInput) error

    // Retrieval Operations
    // Get a recipe by name with optional ref
    // If ref is empty: returns published version if available, otherwise falls back to latest git commit
    // If ref provided: returns recipe at that git ref (commit, branch, tag, etc.)
    GetRecipe(ctx context.Context, projectID project.ID, name string, ref string) (*RecipeWithContent, error)

    // List recipes with filtering
    ListRecipes(ctx context.Context, filter RecipeFilter) (Iterator[*RecipeInfo], error)

    // Get version history for a recipe (paginated)
    GetRecipeHistory(ctx context.Context, projectID project.ID, name string) (Iterator[*RecipeVersion], error)

    // Validation
    // Validate recipe content without creating/publishing and return structured errors on failure
    ValidateRecipe(ctx context.Context, input ValidateInput) (*ValidationResult, error)

    // Remote Sync
    // Sync from remote repository
    SyncFromRemote(ctx context.Context, projectID project.ID) error
}

type CreateInput struct {
    ProjectID   project.ID
    Name        string              // Hierarchical name (e.g., "foo/bar/myrecipe")
    Content     []byte              // Recipe YAML content
    Description string              // Optional description (for commit message)
    AutoPublish bool                // Automatically publish after creation
}

type UpdateInput struct {
    ProjectID        project.ID
    Name             string              // Recipe name to update
    Content          []byte              // New recipe YAML content
    Message          string              // Commit message (optional, auto-generated if empty)
    AutoPublish      bool                // Automatically publish new version
    ExpectedCommit   string              // Expected current commit of this specific recipe file
}

type PublishInput struct {
    ProjectID      project.ID
    Name           string              // Recipe name to publish
    CommitHash     string              // Commit hash to publish (empty = latest)
    PublishedBy    *string             // User publishing (optional)
    ExpectedCommit *string             // Expected current published commit (for unpublish-then-publish atomicity)
}

type UnpublishInput struct {
    ProjectID      project.ID
    Name           string              // Recipe name to unpublish
    ExpectedCommit string              // Expected current published commit (optimistic concurrency control)
}

type ValidateInput struct {
    ProjectID project.ID
    Name      string              // Recipe name for ID matching
    Content   []byte              // Recipe YAML content
}

type ValidationResult struct {
    Valid  bool
    Errors []ValidationError
}

type ValidationError struct {
    Code       string             // e.g., "yaml_parse", "schema", "recipe_id_mismatch", "cel_invalid"
    Message    string
    Path       string             // YAML/JSON path to failing field (if known)
    Expression string             // CEL expression (if applicable)
}

type ValidationFailedError struct {
    Result *ValidationResult
}

func (e *ValidationFailedError) Error() string {
    return "recipe: validation failed"
}

type RecipeVersion struct {
    Name        string              // Recipe name
    CommitHash  string              // Commit hash for this version
    ShortHash   string              // Short commit hash (7 chars)
    Author      string              // Commit author
    Message     string              // Commit message
    CreatedAt   time.Time           // Commit timestamp
    IsPublished bool                // Whether this version is published
}

type RecipeInfo struct {
    Name            string          // Recipe name
    LatestCommit    string          // Latest commit hash from git
    LatestCommitAt  time.Time       // When latest commit was created
    PublishedCommit *string         // Published commit hash (nil if not published)
    PublishedAt     *time.Time      // When published (nil if not published)
    PublishedBy     *string         // Who published (nil if not published)
}

type RecipeWithContent struct {
    Name           string          // Recipe name
    CommitHash     string          // Commit hash for this content
    Content        *recipe.Recipe  // Parsed recipe content
    IsPublished    bool            // Whether this specific version is published
    PublishedAt    *time.Time      // When published (nil if not published version)
    PublishedBy    *string         // Who published (nil if not published version)
}

type PublishStatus string

const (
    PublishStatusAll         PublishStatus = "all"         // All recipes (published and unpublished)
    PublishStatusPublished   PublishStatus = "published"   // Only published recipes
    PublishStatusUnpublished PublishStatus = "unpublished" // Only unpublished recipes
)

type RecipeFilter struct {
    ProjectIDs    []project.ID
    Names         []string        // Exact name matches
    NamePrefix    string          // Hierarchical prefix (e.g., "foo/bar/")
    PublishStatus PublishStatus   // Filter by publish status (default: all)
}

// Iterator provides paginated access to results
// Following the pattern from server/cell/internal/store
type Iterator[T any] interface {
    Next(ctx context.Context) (T, error)
    Close(ctx context.Context) error
}

var ErrIteratorDone = errors.New("recipe: iterator done")
```

### Error Types

```go
var (
    ErrEmptyName        = errors.New("recipe: name is required")
    ErrInvalidName      = errors.New("recipe: invalid name format")
    ErrInvalidProject   = errors.New("recipe: project not found")
    ErrNotFound         = errors.New("recipe: not found")
    ErrAlreadyExists    = errors.New("recipe: already exists")
    ErrNotPublished     = errors.New("recipe: recipe is not published")
    ErrVersionConflict  = errors.New("recipe: version conflict")
    ErrInvalidContent   = errors.New("recipe: invalid recipe content")
    ErrValidationUnavailable = errors.New("recipe: validation service unavailable")
    ErrCommitNotFound   = errors.New("recipe: commit not found in git")
    ErrGitConflict      = errors.New("recipe: git merge conflict detected")
    ErrRemoteSync       = errors.New("recipe: failed to sync with remote")
)
```

**Validation Errors**:
- `ValidationFailedError` is returned by `ValidateRecipe` when validation fails and carries `ValidationResult` with structured details.

## Git Integration

**Note**: Detailed specifications for all git operations (Clone, Fetch, Push, GetFileAtCommit, etc.) are in `/src/server/recipes/GIT_COMMANDS_SPEC.md`.

### Repository Structure

Recipes are stored in the **project's base repository** under a top-level `.c2/recipes/` directory:

```
project-base-repo/
├── .git/
├── .c2/
│   └── recipes/
│       ├── simple-recipe.recipe.yaml
│       ├── foo/
│       │   └── bar/
│       │       └── nested-recipe.recipe.yaml
│       └── workflows/
│           ├── ci/
│           │   └── build-and-test.recipe.yaml
│           └── deploy/
│               └── production.recipe.yaml
├── src/
│   └── (project source code)
└── (other project files)
```

**Key Points**:
- **Shared Repository**: Recipes live in the same git repository as the project code
- **Reserved Directory**: `.c2/` is the Colony2 system directory (similar to `.github/`)
- **Main Branch**: All recipe commits occur on the `main` branch
- **Local Workspace**: Service maintains its own private local clone for all operations

### Local Workspace Management

The Recipe Service maintains its own **private local workspace** for each project:

**Workspace Isolation**:
- Service creates a dedicated local git clone per project
- Path: `{service-data-dir}/workspaces/{project-id}/`
- This workspace is separate from any user workspaces or primary repository location
- All recipe operations (create, update, delete) happen in this isolated workspace

**Synchronization Model**:
- **Before operations**: Service pulls latest changes from primary repository (local or remote)
- **After mutations**: Service commits locally, then pushes to primary repository
- **Conflict handling**: If push fails (non-fast-forward), service returns error to user
- **Primary repository**: Can be local filesystem or remote (GitHub, etc.)

**Benefits**:
- Clean separation between service operations and user operations
- Prevents conflicts with concurrent user git operations
- Enables atomic operations (pull → validate → commit → push)
- Service has full control over workspace state

### Git Workflow

**Service-Managed**: The Recipe Service manages all git operations through its API:
- **Create/Update/Delete**: Service handles file writes and git commits in its local workspace
- **Commit Messages**: Auto-generated or user-provided via API (optional `message` field on updates)
- **History**: Access via `GetRecipeHistory()` API
- **Versioning**: Every operation creates a git commit on the `main` branch
- **Syncing**: Service automatically syncs with primary repository (pull before operations, push after)

**Advanced Git Features** (Optional):
- **Tagging**: Advanced users can add git tags for semantic versioning (v1.0.0, v2.0.0) directly in primary repo
- **Direct Git Access**: While not required, advanced users can work in primary repository for inspection/tagging
- **`name@ref` Support**: Reference any git commit, tag, or branch via RecipeProvider

The Recipe Service owns all mutation operations. Git provides versioning and history.

### Git Ref Resolution

When using `name@ref` syntax, the service resolves refs using standard git mechanisms:

1. **Commit Hashes**: Direct reference to immutable commits
   - `name@a1b2c3d4e5f6...` (full hash)
   - `name@a1b2c3d` (short hash, if unambiguous)

2. **Branches**: Reference to current HEAD of branch (mutable)
   - `name@main` - current HEAD of main branch
   - `name@feature/xyz` - current HEAD of feature branch
   - **Note**: Branch refs are mutable and will change as branch evolves

3. **Tags**: Reference to tagged commits (immutable by convention)
   - `name@v1.0.0` - tag pointing to specific commit
   - `name@release-2024-01` - annotated tag
   - **Best Practice**: Use tags for stable version references

4. **Relative Refs**: Relative to other refs
   - `name@HEAD~1` - previous commit
   - `name@main~2` - two commits before main's HEAD
   - `name@v1.0.0^` - parent of tagged commit

**Important Considerations**:
- **Branches are mutable**: `name@develop` will always resolve to the current HEAD of the develop branch
- **Commits are immutable**: `name@a1b2c3d4` always references the same recipe content
- **Tags are immutable (by convention)**: `name@v1.0.0` should always point to the same commit
- **Published versions use commit hashes**: Database stores commit hash, not branch/tag names

**Best Practices**:
- Use **commit hashes** for maximum reproducibility
- Use **tags** for semantic versioning and stable releases
- Use **branches** for tracking development (e.g., `@develop`, `@staging`)
- **Lock dependencies** by converting refs to commit hashes in production

### Service Operations

#### Create Recipe
1. Validate project exists
2. Validate recipe name (hierarchical path format: alphanumeric, dash, underscore, slash only)
3. Sync local workspace: `git.Pull()` from primary repository
4. Check recipe doesn't already exist (query git)
5. **If `AutoPublish=true`**: Pre-validate recipe content to fail fast
   - Parse recipe and validate structure
   - Check recipe ID matches name
   - Validate CEL expressions via recipe-worker API
   - Check for database conflicts
   - **Note**: This is an optimization - full validation still occurs during publish step
6. Write recipe file to git workspace: `.c2/recipes/{name}.recipe.yaml`
7. Stage file: `git.StageFiles()`
8. Commit on `main` branch: `git.CreateCommit()` with message "Create recipe: {name}" (or user-provided message)
9. Push to primary repository: `git.Push()`
10. Get commit hash from git history
11. If `AutoPublish=true`: publish the new commit (includes full validation with version checks)
12. Return RecipeVersion with commit details

**Transaction**: File write + commit happen atomically within git

#### Update Recipe
1. Validate project exists
2. Sync local workspace: `git.Pull()` from primary repository
3. Verify recipe exists (check git for file)
4. **Optimistic Concurrency Check**:
   - Get current commit hash for this specific recipe file (not repo HEAD)
   - Use `git log -1 --format=%H -- .c2/recipes/{name}.recipe.yaml` to get last commit that touched this file
   - If `ExpectedCommit` doesn't match current recipe file commit: return `ErrVersionConflict`
   - This ensures user is updating based on current state, even if other recipes changed
5. **If `AutoPublish=true`**: Pre-validate recipe content to fail fast
   - Parse recipe and validate structure
   - Check recipe ID matches name
   - Validate CEL expressions via recipe-worker API
   - Verify existing published recipe exists if applicable
   - **Note**: This is an optimization - full validation still occurs during publish step
6. Write updated content to git workspace: `.c2/recipes/{name}.recipe.yaml`
7. Stage file: `git.StageFiles()`
8. Commit on `main` branch: `git.CreateCommit()` with user-provided `message` or auto-generated "Update recipe: {name}"
9. Push to primary repository: `git.Push()` (if fails due to non-fast-forward, return `ErrGitConflict`)
10. Get new commit hash from git history
11. If `AutoPublish=true`: publish the new commit (includes full validation with version checks)
12. Return RecipeVersion with new commit details

**Optimistic Concurrency Control**: Uses recipe file's last commit, not repository HEAD. This allows concurrent updates to different recipes while preventing conflicting updates to the same recipe.

#### Delete Recipe
1. Validate project exists
2. Sync local workspace: `git.Pull()` from primary repository
3. Verify recipe exists
4. **Optimistic Concurrency Check** (optional):
   - If `ExpectedCommit` provided: verify current recipe file commit matches
   - Prevents deleting recipe that was modified since user last read it
5. Unpublish if currently published (remove from database)
6. Remove file: `git rm .c2/recipes/{name}.recipe.yaml`
7. Commit on `main` branch: `git.CreateCommit()` with message "Delete recipe: {name}"
8. Push to primary repository: `git.Push()`
9. Return success

**Note**: Recipe history remains in git - can still access via `name@<old-commit>`

#### Publish Recipe

**Important**: This operation **always** performs full validation and version checking, regardless of whether pre-validation occurred during create/update with `AutoPublish=true`.

1. Validate project exists
2. Sync local workspace: `git.Pull()` from primary repository (ensure we have latest commits)
3. Verify recipe exists in git
4. If CommitHash empty: get latest commit for recipe file using `git log -1 --format=%H -- .c2/recipes/{name}.recipe.yaml`
5. Fetch content at specified commit from git using `git.GetFileAtCommit()`
6. Parse and validate using `recipe.LoadRecipeFromString()`
7. **Validate Recipe ID**: Verify the `id` field inside the recipe YAML matches the recipe name (excluding any `@ref`)
   - Example: For `.c2/recipes/foo/bar/test.recipe.yaml`, the recipe's `id` field must be `foo/bar/test`
   - If mismatch: return `ErrInvalidContent` with aggregated error details (`ValidationResult.Errors`)
8. **Validate CEL Expressions**: Call recipe-worker validation API to ensure CEL expressions reference potentially real fields/values
   - If validation errors: return `ErrInvalidContent` with aggregated error details (`ValidationResult.Errors`)
   - If validation service unavailable: return `ErrValidationUnavailable`
9. If validation succeeds:
   - **Database Transaction**:
     - Get existing published recipe (if any)
     - If `ExpectedCommit` provided: verify it matches current published commit
     - If mismatch: return `ErrVersionConflict` (someone else published different version)
     - Insert or update published recipe index (Name → CommitHash)
     - Store PublishedAt timestamp and PublishedBy user
     - Use optimistic locking (Version field) for concurrent publish prevention
10. Return published recipe metadata

**Dual Optimistic Locking**:
- **Content-based** (`ExpectedCommit`): Ensures user's expected state matches current published state
- **Database-based** (Version field): Prevents concurrent database modifications

This allows safe publish operations even when multiple users are working concurrently.

#### Unpublish Recipe
1. Validate project exists
2. **Database Transaction**:
   - Get current published recipe by (ProjectID, Name)
   - If not found: return `ErrNotPublished`
   - If `ExpectedCommit` doesn't match current published commit: return `ErrVersionConflict`
   - Delete from published recipe index
3. Return success

**Optimistic Concurrency Control**: The `ExpectedCommit` field ensures the user is unpublishing the version they expect, preventing race conditions.

**Note**: Unpublishing does NOT delete the file from git. It only removes the published reference.

#### Get Recipe
1. Validate project exists
2. **If ref is empty** (get published version):
   - Query published recipe index by (ProjectID, Name)
   - If not found: return `ErrNotPublished`
   - Use published CommitHash
   - Fetch file content from git at published commit
   - Parse recipe content
   - Return RecipeWithContent with IsPublished=true and publish metadata
3. **If ref is provided** (get specific version):
   - Sync local workspace: `git.Pull()` from primary repository
   - Derive git path: `.c2/recipes/{name}.recipe.yaml`
   - Fetch file content from git at specified ref using `git.GetFileAtCommit()`
   - If file not found at ref: return `ErrNotFound`
   - Parse recipe content
   - Check if this commit matches published commit (if recipe is published)
   - Return RecipeWithContent with IsPublished flag and publish metadata (if applicable)
4. Return combined content + metadata

**Note**: When using a git ref, the service resolves it to check if it matches the published version for metadata purposes.

#### List Recipes
1. Validate project exists (if filtering by single project)
2. Sync local workspace: `git.Pull()` from primary repository
3. Query git repository to get all recipe files under `.c2/recipes/`
4. For each recipe found in git:
   - Get latest commit for that recipe file
   - Query database to check if it's published
   - Build RecipeInfo with git and publish metadata
5. Apply filters:
   - Filter by ProjectIDs if specified
   - Filter by Names (exact match) if specified
   - Filter by NamePrefix if specified
   - Filter by PublishStatus:
     - `published`: Only include recipes with PublishedCommit != nil
     - `unpublished`: Only include recipes with PublishedCommit == nil
     - `all` (default): Include all recipes
6. Return Iterator[*RecipeInfo] with filtered results

**Note**: Following the pattern from `server/cell`, results are loaded into memory and wrapped in a slice-based iterator.

#### Get Recipe History
1. Validate project exists
2. Sync local workspace: `git.Pull()` from primary repository
3. Get git commit history for specific recipe file:
   - Use `git log --follow -- .c2/recipes/{name}.recipe.yaml`
   - Load all commits that touched this file
4. For each commit:
   - Extract commit hash, author, message, timestamp
   - Check if commit matches published commit
   - Build RecipeVersion with IsPublished flag
5. Return Iterator[*RecipeVersion] ordered by commit time (newest first)

**Note**: The `--follow` flag tracks renames, providing complete history even if file was moved.

#### Validate Recipe
1. Validate project exists
2. Parse and validate structure using `recipe.LoadRecipeFromString()`
3. Validate recipe ID matches `Name`
4. Call recipe-worker validation API to validate CEL expressions
5. On validation failure: return `ValidationFailedError` with `ValidationResult`
6. On success: return `ValidationResult` with `Valid=true`

**Note**: This API is intended for other services to validate a recipe and get structured error details without publishing.

#### Sync from Remote
1. Fetch from remote using `git.Fetch()` (see GIT_COMMANDS_SPEC.md)
2. Optionally pull changes using `git.Pull()` with fast-forward only
3. For each published recipe in database:
   - Verify commit still exists in git
   - If commit missing (e.g., force push): log warning or unpublish
4. Return sync status

**Note**: Uses Fetch, Pull, and IsAncestor operations defined in `/src/server/recipes/GIT_COMMANDS_SPEC.md`.

### Validation Integration

Validation is a two-phase check:

1. **Structural + Schema Validation** via `server/recipe-core/pkg/recipe`
2. **CEL Expression Validation** via the new recipe-worker validation API (ensures references in CEL expressions are potentially real)

Recipe Service aggregates all validation errors and returns them to callers.

```go
func (s *service) ValidateRecipe(ctx context.Context, input ValidateInput) (*ValidationResult, error) {
    result := &ValidationResult{Valid: true}

    rec, err := recipe.LoadRecipeFromString(input.Content)
    if err != nil {
        result.Valid = false
        result.Errors = append(result.Errors, ValidationError{
            Code:    "schema",
            Message: err.Error(),
        })
        return result, &ValidationFailedError{Result: result}
    }

    if rec.ID != input.Name {
        result.Valid = false
        result.Errors = append(result.Errors, ValidationError{
            Code:    "recipe_id_mismatch",
            Message: fmt.Sprintf("recipe id %q does not match name %q", rec.ID, input.Name),
            Path:    "id",
        })
    }

    // Call recipe-worker validation API for CEL expressions.
    celErrors, err := s.recipeWorkerValidator.ValidateCEL(ctx, input.ProjectID, rec)
    if err != nil {
        return nil, fmt.Errorf("cel validation unavailable: %w", err)
    }
    if len(celErrors) > 0 {
        result.Valid = false
        result.Errors = append(result.Errors, celErrors...)
    }

    if !result.Valid {
        return result, &ValidationFailedError{Result: result}
    }
    return result, nil
}
```

This validation is automatically performed during `PublishRecipe`. Validation failures hard-fail publish and return `ErrInvalidContent` with structured error details.

#### CEL Expression Validation (Recipe-Worker)

Recipe Service calls the recipe-worker validation API to validate CEL expressions embedded in recipe fields (conditions, filters, inputs, etc.). The validator ensures:

- Expressions parse successfully
- Referenced identifiers exist in the recipe's evaluation context (inputs, step outputs, workflow metadata)
- Obvious type mismatches are rejected when detectable

**Expected response** is a list of structured errors:

```go
[]ValidationError{
    {
        Code:       "cel_invalid",
        Message:    "unknown identifier: step.build.outputs.image",
        Path:       "steps[2].when",
        Expression: "step.build.outputs.image != \"\"",
    },
}
```

If the validation service is unavailable, the Recipe Service returns `ErrValidationUnavailable` and blocks publish.

### Conflict Resolution

**Publishing Conflicts**: Uses optimistic locking (Version field) to prevent concurrent publishes of the same recipe name:

1. User A fetches published recipe at version N
2. User A publishes commit X for recipe "foo"
3. User B attempts to publish commit Y for recipe "foo" at version N
4. Database update fails due to version mismatch
5. User B must retry: fetch current state, then publish again

**Git Conflicts**: Since users edit recipes directly in git, they handle conflicts using normal git workflows:

- Branch/merge strategies
- Rebase workflows
- Conflict resolution tools

The Recipe Service doesn't manage git conflicts - it only tracks published versions.

**Key Insight**: Simultaneous git edits to different recipe files will succeed. Simultaneous publishes of different recipe names will succeed. Only publishing the same recipe name concurrently causes a conflict (handled by optimistic locking).

### Remote Repository Support

#### Primary Repository Modes

**Local Primary Repository**:
- Primary repository on local filesystem
- Project repository path: `{project-workspace-dir}/`
- Service workspace: `{service-data-dir}/workspaces/{project-id}/`
- Service clones from local primary to its workspace
- Commits in service workspace are pushed back to local primary

**Remote Primary Repository (GitHub, etc.)**:
- Primary repository is remote (e.g., GitHub, GitLab)
- Configured per project via `GitRepoURL`
- Service workspace: `{service-data-dir}/workspaces/{project-id}/`
- Service clones from remote to its workspace
- Commits in service workspace are pushed back to remote
- Operations (see `/src/server/recipes/GIT_COMMANDS_SPEC.md` for details):
  - Clone on first access: `git.Clone()`
  - Pull before operations: `git.Pull()`
  - Push after commits: `git.Push()`

#### Authentication
Authentication is configured at the git Repository level. See `/src/server/recipes/GIT_COMMANDS_SPEC.md` for:
- AuthConfig structure
- SSH key authentication (preferred for automation)
- HTTPS with Personal Access Tokens
- Credential security and handling

## Recipe Provider Integration

Implement the `RecipeProvider` interface from `server/recipe-core`:

```go
type RecipeProvider interface {
    GetRecipe(name string) (*recipe.Recipe, error)
}
```

Implementation with support for `name@ref` syntax:

```go
type Provider struct {
    service   Service
    projectID project.ID
    gitPath   string  // Git workspace path for this project
}

func (p *Provider) GetRecipe(name string) (*recipe.Recipe, error) {
    ctx := context.Background()

    // Parse name@ref syntax
    recipeName, gitRef := p.parseRecipeRef(name)

    // Call unified GetRecipe API
    // If gitRef is empty: gets published version
    // If gitRef provided: gets recipe at that specific ref
    recipeWithContent, err := p.service.GetRecipe(ctx, p.projectID, recipeName, gitRef)
    if err != nil {
        if gitRef != "" {
            return nil, fmt.Errorf("recipe %q at ref %s not found: %w", recipeName, gitRef, err)
        }
        return nil, fmt.Errorf("recipe %q not found: %w", recipeName, err)
    }

    return recipeWithContent.Content, nil
}

// parseRecipeRef parses "name@ref" into (name, ref)
// Returns (name, "") if no @ref suffix
func (p *Provider) parseRecipeRef(nameWithRef string) (string, string) {
    if idx := strings.LastIndex(nameWithRef, "@"); idx > 0 {
        return nameWithRef[:idx], nameWithRef[idx+1:]
    }
    return nameWithRef, ""
}

func NewProvider(service Service, projectID project.ID, gitPath string) *Provider {
    return &Provider{
        service:   service,
        projectID: projectID,
        gitPath:   gitPath,
    }
}
```

This provider can be registered with systems like `server/ticket` that need recipe access.

**Usage Examples**:

```go
provider := recipe.NewProvider(svc, projectID, gitPath)

// Get published version
rec, err := provider.GetRecipe("workflows/ci/build")

// Get specific commit (full or short hash)
rec, err := provider.GetRecipe("workflows/ci/build@a1b2c3d4e5f6")
rec, err := provider.GetRecipe("workflows/ci/build@a1b2c3d")

// Get from branch
rec, err := provider.GetRecipe("workflows/ci/build@main")
rec, err := provider.GetRecipe("workflows/ci/build@feature/new-api")

// Get from tag
rec, err := provider.GetRecipe("workflows/ci/build@v1.0.0")
rec, err := provider.GetRecipe("workflows/ci/build@release-2024-01")

// Get relative refs
rec, err := provider.GetRecipe("workflows/ci/build@HEAD~1")
rec, err := provider.GetRecipe("workflows/ci/build@main~2")
```

**Benefits of `name@ref` syntax**:
1. **Testing**: Test recipes from any branch before publishing
2. **Version Pinning**: Reference specific commits, tags, or branches
3. **Semantic Versioning**: Use git tags (v1.0.0, v2.0.0) for version management
4. **Recipe Dependencies**: Recipes can depend on specific versions of other recipes
5. **Development Workflows**: Test recipes from feature branches
6. **Rollback Testing**: Test previous versions using tags or commit history

## File Extension Convention

All recipes must use the `.recipe.yaml` extension:
- Enables future expansion (e.g., `.recipe.test.yaml` for tests)
- Clear identification in git repositories
- Supports tooling and IDE integration
- Example: `myrecipe.recipe.yaml`, `foo/bar/nested.recipe.yaml`

## Naming Conventions

### Recipe Names
- Must be unique within a project
- Support hierarchical structure: `category/subcategory/name`
- **Valid characters**: Alphanumeric, underscore, dash, and forward slash only: `[a-zA-Z0-9_-/]`
- **Directory and file name restrictions**: Each path segment (directory or filename) must contain only `[a-zA-Z0-9_-]`
- **Path separator**: Only `/` is allowed for hierarchy
- Examples:
  - ✅ Valid: `deployment-recipe`, `workflows/ci/build-and-test`, `data_processing/etl/load_users`
  - ❌ Invalid: `my.recipe` (dots), `my recipe` (spaces), `my@recipe` (special chars)

### Recipe ID Matching
- **Critical Requirement**: The `id` field inside the recipe YAML **must match** the recipe's full path name
- The ID excludes any `@ref` suffix but includes the full hierarchical path
- Examples:
  - File: `.c2/recipes/simple.recipe.yaml` → ID must be: `simple`
  - File: `.c2/recipes/foo/bar/nested.recipe.yaml` → ID must be: `foo/bar/nested`
  - Reference: `foo/bar/nested@a1b2c3d` → ID must still be: `foo/bar/nested`
- Validation enforces this constraint during publish operations

### Recipe Name Validation

The service validates recipe names using strict rules:

```go
func validateRecipeName(name string) error {
    if name == "" {
        return ErrEmptyName
    }

    // Split into path segments
    segments := strings.Split(name, "/")

    for _, segment := range segments {
        if segment == "" {
            return fmt.Errorf("%w: empty path segment", ErrInvalidName)
        }

        // Each segment must contain only alphanumeric, dash, underscore
        // Regex: ^[a-zA-Z0-9_-]+$
        for _, r := range segment {
            if !((r >= 'a' && r <= 'z') ||
                 (r >= 'A' && r <= 'Z') ||
                 (r >= '0' && r <= '9') ||
                 r == '_' || r == '-') {
                return fmt.Errorf("%w: invalid character '%c' in segment '%s'",
                    ErrInvalidName, r, segment)
            }
        }
    }

    return nil
}
```

This ensures clean git paths and prevents path traversal attacks.

### Git Paths
- All recipes stored in project repo under `.c2/recipes/` directory
- Path format: `.c2/recipes/{name}.recipe.yaml`
- Directories auto-created as needed
- Examples:
  - Name: `simple` → Path: `.c2/recipes/simple.recipe.yaml`
  - Name: `foo/bar/nested` → Path: `.c2/recipes/foo/bar/nested.recipe.yaml`

## Dependencies on Existing Modules

### Required
- `server/git/pkg/git`: Git operations
  - Base operations: GetStatus, CreateCommit, GetHistory, StageFiles
  - Extended operations: See `/src/server/recipes/GIT_COMMANDS_SPEC.md` for additional required operations
- `server/recipe-core/pkg/recipe`: Recipe parsing and validation (LoadRecipeFromString)
- `server/project/pkg/project`: Project validation

### Git Operations Implementation

The recipe service requires additional git operations beyond the base set in `server/git/pkg/git`. These operations are **fully specified** in `/src/server/recipes/GIT_COMMANDS_SPEC.md`, which defines:
- Complete method signatures with options/parameters
- Detailed behavior specifications
- Error handling and error types
- Authentication configuration (SSH, HTTPS, tokens)
- Concurrency guarantees and thread safety
- Usage examples and workflows
- Testing requirements

**Key Operations Used by Recipe Service**:

| Operation | Purpose | Used For |
|-----------|---------|----------|
| **GetFileAtCommit** | Retrieve file content at any git ref | Fetching recipe YAML at specific commit/branch/tag |
| **Clone** | Create local copy of remote repository | Initial setup for remote recipe repos |
| **Fetch** | Update remote-tracking branches | Checking for remote changes before publish |
| **Pull** | Fetch and integrate remote changes | Syncing local repo with remote |
| **Push** | Upload local commits to remote | Publishing recipe commits to remote |
| **IsAncestor** | Check commit ancestry | Verifying fast-forward is possible |
| **InitRepository** | Create new git repository | Initializing local recipe repos for new projects |
| **GetCurrentCommit** | Get current HEAD commit hash | Tracking recipe versions |
| **AddRemote** | Add remote repository reference | Configuring remote for new repos |
| **ListRemotes** | Get configured remotes | Checking if remote is configured |

**Important Notes**:
- All operations support context cancellation
- Operations are thread-safe and can run concurrently on different repos
- Authentication is configured once at Repository level, not per-operation
- All remote operations handle network failures and authentication errors gracefully

**Refer to `/src/server/recipes/GIT_COMMANDS_SPEC.md` for complete implementation details.**

## Service Configuration

```go
type ServiceConfig struct {
    Store          Store                 // Database store
    GitRepo        git.Repository        // Git operations interface
    Projects       project.Service       // Project service
    IDGen          ShortIDGenerator      // ID generator
    Clock          Clock                 // Time provider
    GitConfig      GitConfig             // Git configuration
    WorkspaceRoot  string                // Root directory for git workspaces
}

type GitConfig struct {
    DefaultAuthor string                 // Default git author name
    DefaultEmail  string                 // Default git author email
    DefaultBranch string                 // Branch to use (always "main")
    RemoteAuth    RemoteAuthConfig       // Authentication for remote repos
}

type RemoteAuthConfig struct {
    SSHKeyPath    string                 // Path to SSH private key
    AccessToken   string                 // Personal access token
}
```

**Important Configuration Notes**:
- **DefaultBranch**: Always set to `"main"`. All recipe commits occur on this branch.
- **WorkspaceRoot**: Directory where service creates isolated workspaces (e.g., `/var/lib/colony2/recipe-workspaces/`)
- Each project gets its own workspace at `{WorkspaceRoot}/{project-id}/`

## Transaction Handling

Following the pattern from `cell` service:

```go
func (s *service) CreateRecipe(ctx context.Context, input CreateInput) (*RecipeVersion, error) {
    // 1. Validate project exists
    if err := s.ensureProject(ctx, input.ProjectID); err != nil {
        return nil, err
    }

    // 2. Validate recipe name (alphanumeric, dash, underscore, slash only)
    if err := s.validateRecipeName(input.Name); err != nil {
        return nil, err
    }

    // 3. Get git workspace and sync with primary repository
    gitWorkspace := s.getGitWorkspace(input.ProjectID)
    if err := s.gitRepo.Pull(ctx, gitWorkspace, git.PullOptions{
        Remote: "origin",
        FastForward: true,
    }); err != nil {
        return nil, fmt.Errorf("failed to sync workspace: %w", err)
    }

    // 4. Check recipe doesn't already exist
    gitPath := s.deriveGitPath(input.Name) // Returns ".c2/recipes/{name}.recipe.yaml"
    exists, err := s.fileExistsInGit(ctx, gitWorkspace, gitPath)
    if err != nil {
        return nil, err
    }
    if exists {
        return nil, ErrAlreadyExists
    }

    // 5. If AutoPublish enabled: pre-validate content early to fail fast (before git write)
    // Note: This doesn't replace full validation during publish - it's an optimization
    if input.AutoPublish {
        // Parse recipe to validate structure
        rec, err := recipe.LoadRecipeFromString(input.Content)
        if err != nil {
            return nil, fmt.Errorf("%w: %v", ErrInvalidContent, err)
        }

        // Validate recipe ID matches name
        if rec.GetMetadata().ID != input.Name {
            return nil, fmt.Errorf("%w: recipe ID '%s' does not match name '%s'",
                ErrInvalidContent, rec.GetMetadata().ID, input.Name)
        }

        // Check for existing published recipe (should not exist for create)
        _, err = s.store.GetByName(ctx, input.ProjectID, input.Name)
        if err == nil {
            return nil, fmt.Errorf("%w: recipe already published", ErrAlreadyExists)
        }
        if !errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, err
        }
        // Full validation with version checks still happens in PublishRecipe below
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
    if err := s.gitRepo.Push(ctx, gitWorkspace, git.PushOptions{
        Remote: "origin",
    }); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrRemoteSync, err)
    }

    // 10. Auto-publish if requested
    // PublishRecipe performs full validation including version checks
    isPublished := false
    if input.AutoPublish {
        _, err := s.PublishRecipe(ctx, PublishInput{
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

    return &RecipeVersion{
        Name:        input.Name,
        CommitHash:  commitHash,
        ShortHash:   commitHash[:7],
        Author:      commits[0].Author,
        Message:     commits[0].Message,
        CreatedAt:   commits[0].Date,
        IsPublished: isPublished,
    }, nil
}

func (s *service) UpdateRecipe(ctx context.Context, input UpdateInput) (*RecipeVersion, error) {
    // 1. Validate project exists
    if err := s.ensureProject(ctx, input.ProjectID); err != nil {
        return nil, err
    }

    // 2. Get git workspace and sync with primary repository
    gitWorkspace := s.getGitWorkspace(input.ProjectID)
    if err := s.gitRepo.Pull(ctx, gitWorkspace, git.PullOptions{
        Remote: "origin",
        FastForward: true,
    }); err != nil {
        return nil, fmt.Errorf("failed to sync workspace: %w", err)
    }

    gitPath := s.deriveGitPath(input.Name) // Returns ".c2/recipes/{name}.recipe.yaml"

    // 3. Verify recipe exists
    exists, err := s.fileExistsInGit(ctx, gitWorkspace, gitPath)
    if err != nil {
        return nil, err
    }
    if !exists {
        return nil, ErrNotFound
    }

    // 4. Optimistic Concurrency Check - get last commit for THIS FILE
    cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%H", "--", gitPath)
    cmd.Dir = gitWorkspace
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("failed to get recipe file commit: %w", err)
    }
    currentCommit := strings.TrimSpace(string(output))

    if input.ExpectedCommit != "" && input.ExpectedCommit != currentCommit {
        return nil, fmt.Errorf("%w: expected %s, got %s",
            ErrVersionConflict, input.ExpectedCommit, currentCommit)
    }

    // 5. If AutoPublish enabled: pre-validate content early to fail fast (before git write)
    // Note: This doesn't replace full validation during publish - it's an optimization
    if input.AutoPublish {
        // Parse recipe to validate structure
        rec, err := recipe.LoadRecipeFromString(input.Content)
        if err != nil {
            return nil, fmt.Errorf("%w: %v", ErrInvalidContent, err)
        }

        // Validate recipe ID matches name
        if rec.GetMetadata().ID != input.Name {
            return nil, fmt.Errorf("%w: recipe ID '%s' does not match name '%s'",
                ErrInvalidContent, rec.GetMetadata().ID, input.Name)
        }

        // If publishing, check existing published version exists
        existing, err := s.store.GetByName(ctx, input.ProjectID, input.Name)
        if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, err
        }
        // Full validation with version checks (ExpectedCommit, etc.) happens in PublishRecipe below
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
    cmd = exec.CommandContext(ctx, "git", "log", "-1", "--format=%H", "--", gitPath)
    cmd.Dir = gitWorkspace
    output, err = cmd.Output()
    if err != nil {
        return nil, err
    }
    newCommitHash := strings.TrimSpace(string(output))

    // 9. Push to primary repository
    if err := s.gitRepo.Push(ctx, gitWorkspace, git.PushOptions{
        Remote: "origin",
    }); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrGitConflict, err)
    }

    // 10. Auto-publish if requested
    // PublishRecipe performs full validation including version checks
    isPublished := false
    if input.AutoPublish {
        _, err := s.PublishRecipe(ctx, PublishInput{
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

    return &RecipeVersion{
        Name:        input.Name,
        CommitHash:  newCommitHash,
        ShortHash:   newCommitHash[:7],
        Author:      commits[0].Author,
        Message:     commits[0].Message,
        CreatedAt:   commits[0].Date,
        IsPublished: isPublished,
    }, nil
}

func (s *service) PublishRecipe(ctx context.Context, input PublishInput) (*PublishedRecipe, error) {
    // 1. Validate project exists
    if err := s.ensureProject(ctx, input.ProjectID); err != nil {
        return nil, err
    }

    // 2. Get git workspace and sync with primary repository
    gitWorkspace := s.getGitWorkspace(input.ProjectID)
    if err := s.gitRepo.Pull(ctx, gitWorkspace, git.PullOptions{
        Remote: "origin",
        FastForward: true,
    }); err != nil {
        return nil, fmt.Errorf("failed to sync workspace: %w", err)
    }

    gitPath := s.deriveGitPath(input.Name) // Returns ".c2/recipes/{name}.recipe.yaml"

    // 3. If no commit hash provided, get latest commit for this recipe file
    commitHash := input.CommitHash
    if commitHash == "" {
        cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%H", "--", gitPath)
        cmd.Dir = gitWorkspace
        output, err := cmd.Output()
        if err != nil {
            return nil, fmt.Errorf("failed to get latest commit for recipe: %w", err)
        }
        commitHash = strings.TrimSpace(string(output))
    }

    // 4. Read file at specified commit from git
    content, err := s.gitRepo.GetFileAtCommit(ctx, gitWorkspace, commitHash, gitPath)
    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrCommitNotFound, err)
    }

    // 5. Parse and validate recipe structure
    rec, err := recipe.LoadRecipeFromString(content)
    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrInvalidContent, err)
    }

    // 6. Validate recipe ID matches name (excluding any @ref)
    if rec.GetMetadata().ID != input.Name {
        return nil, fmt.Errorf("%w: recipe ID '%s' does not match name '%s'",
            ErrInvalidContent, rec.GetMetadata().ID, input.Name)
    }

    // 7. Insert or update published recipe in database (with dual optimistic locking)
    var result *PublishedRecipe
    err = s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
        existing, err := txStore.GetByName(ctx, input.ProjectID, input.Name)

        if errors.Is(err, gorm.ErrRecordNotFound) {
            // New published recipe - no ExpectedCommit check needed
            id, err := s.idGen.NewID()
            if err != nil {
                return err
            }

            result = &PublishedRecipe{
                ID:          string(id),
                ProjectID:   input.ProjectID,
                Name:        input.Name,
                GitPath:     gitPath,
                CommitHash:  commitHash,
                PublishedAt: s.clock.Now(),
                PublishedBy: input.PublishedBy,
                Version:     1,
            }

            return txStore.Create(ctx, result)
        }

        if err != nil {
            return err
        }

        // Content-based optimistic locking: verify expected matches current
        if input.ExpectedCommit != nil && *input.ExpectedCommit != existing.CommitHash {
            return fmt.Errorf("%w: expected %s, current is %s",
                ErrVersionConflict, *input.ExpectedCommit, existing.CommitHash)
        }

        // Update existing published recipe
        existing.CommitHash = commitHash
        existing.PublishedAt = s.clock.Now()
        existing.PublishedBy = input.PublishedBy

        // Database-based optimistic locking: Version field prevents concurrent modifications
        err = txStore.Update(ctx, existing)
        if errors.Is(err, store.ErrOptimisticLock) {
            return ErrVersionConflict
        }

        result = existing
        return err
    })

    return result, err
}

func (s *service) GetRecipe(ctx context.Context, projectID project.ID, name string, ref string) (*RecipeWithContent, error) {
    // 1. Validate project exists
    if err := s.ensureProject(ctx, projectID); err != nil {
        return nil, err
    }

    gitPath := s.deriveGitPath(name) // Returns ".c2/recipes/{name}.recipe.yaml"

    // 2. Determine which version to fetch
    var commitHash string
    var publishedRecipe *PublishedRecipe

    if ref == "" {
        // Get published version
        var err error
        publishedRecipe, err = s.store.GetByName(ctx, projectID, name)
        if err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) {
                return nil, ErrNotPublished
            }
            return nil, err
        }
        commitHash = publishedRecipe.CommitHash
    } else {
        // Get specific version at ref
        gitWorkspace := s.getGitWorkspace(projectID)

        // Sync workspace to ensure we have latest refs
        if err := s.gitRepo.Pull(ctx, gitWorkspace, git.PullOptions{
            Remote: "origin",
            FastForward: true,
        }); err != nil {
            return nil, fmt.Errorf("failed to sync workspace: %w", err)
        }

        // Resolve ref to commit hash (for checking against published version)
        cmd := exec.CommandContext(ctx, "git", "rev-parse", ref)
        cmd.Dir = gitWorkspace
        output, err := cmd.Output()
        if err != nil {
            return nil, fmt.Errorf("failed to resolve ref %s: %w", ref, err)
        }
        commitHash = strings.TrimSpace(string(output))

        // Check if recipe is published (to populate metadata)
        publishedRecipe, _ = s.store.GetByName(ctx, projectID, name)
    }

    // 3. Fetch content from git
    gitWorkspace := s.getGitWorkspace(projectID)
    content, err := s.gitRepo.GetFileAtCommit(ctx, gitWorkspace, commitHash, gitPath)
    if err != nil {
        return nil, fmt.Errorf("%w: failed to get recipe at commit %s", ErrNotFound, commitHash)
    }

    // 4. Parse recipe content
    rec, err := recipe.LoadRecipeFromString(content)
    if err != nil {
        return nil, fmt.Errorf("failed to parse recipe: %w", err)
    }

    // 5. Build result with metadata
    result := &RecipeWithContent{
        Name:       name,
        CommitHash: commitHash,
        Content:    rec,
    }

    // If published and this is the published version, include publish metadata
    if publishedRecipe != nil && publishedRecipe.CommitHash == commitHash {
        result.IsPublished = true
        result.PublishedAt = &publishedRecipe.PublishedAt
        result.PublishedBy = publishedRecipe.PublishedBy
    }

    return result, nil
}
```

## Usage Examples

### Creating a Recipe

```go
svc, _ := recipe.NewService(config)

// Create a new recipe (service handles git commit)
version, err := svc.CreateRecipe(ctx, recipe.CreateInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    Content:     []byte(`
version: "1.0"
op: echo
inputs:
  message: "Building project..."
`),
    Description: "CI build workflow",
    AutoPublish: true,  // Automatically publish after creation
})

if err != nil {
    return err
}

fmt.Printf("Created recipe at commit: %s\n", version.CommitHash)
fmt.Printf("Published: %v\n", version.IsPublished)
```

### Updating a Recipe

```go
// Update existing recipe (service handles git commit)
version, err := svc.UpdateRecipe(ctx, recipe.UpdateInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    Content:     []byte(`
version: "1.0"
op: echo
inputs:
  message: "Updated build message"
`),
    Message:     "Update build message",
    AutoPublish: false,  // Don't auto-publish, review first
})

if err != nil {
    return err
}

fmt.Printf("Updated to commit: %s\n", version.CommitHash)

// Review and publish later
err = svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    CommitHash:  version.CommitHash,
    PublishedBy: strPtr("user@example.com"),
})
```

### Publishing a Recipe

```go
// Publish the latest version
published, err := svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    CommitHash:  "",  // Empty = latest commit
    PublishedBy: strPtr("user@example.com"),
})

// Or publish a specific version
published, err := svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    CommitHash:  "a1b2c3d4e5f6",  // Specific commit hash
    PublishedBy: strPtr("user@example.com"),
})
```

### Deleting a Recipe

```go
// Delete recipe (removes file and commits to git)
err := svc.DeleteRecipe(ctx, "proj_123", "workflows/ci/build")

// Recipe is deleted from git working tree
// History still exists - can access via version history
```

### Getting a Recipe

```go
// Get published recipe with content (ref = "")
recipeWithContent, err := svc.GetRecipe(ctx, "proj_123", "workflows/ci/build", "")
if err != nil {
    return err
}

fmt.Printf("Recipe: %s\n", recipeWithContent.Name)
fmt.Printf("Commit: %s\n", recipeWithContent.CommitHash)
fmt.Printf("Is Published: %v\n", recipeWithContent.IsPublished)
if recipeWithContent.PublishedAt != nil {
    fmt.Printf("Published at: %v\n", *recipeWithContent.PublishedAt)
}
// Use recipeWithContent.Content for parsed recipe

// Get recipe at specific ref (commit, branch, tag, etc.)
recipeAtRef, err := svc.GetRecipe(ctx, "proj_123", "workflows/ci/build", "a1b2c3d4")
if err != nil {
    return err
}
fmt.Printf("Recipe at commit %s\n", recipeAtRef.CommitHash)
fmt.Printf("Is this the published version? %v\n", recipeAtRef.IsPublished)
```

### Viewing Recipe History

```go
// Get version history for a recipe (returns iterator)
iter, err := svc.GetRecipeHistory(ctx, "proj_123", "workflows/ci/build")
if err != nil {
    return err
}
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
    fmt.Println("---")
}
```

### Unpublishing a Recipe

```go
// Unpublish with optimistic concurrency control
err := svc.UnpublishRecipe(ctx, recipe.UnpublishInput{
    ProjectID:      "proj_123",
    Name:           "workflows/ci/build",
    ExpectedCommit: "a1b2c3d4e5f6",  // Must match current published commit
})
```

### Listing Recipes

```go
// List all published recipes
iter, err := svc.ListRecipes(ctx, recipe.RecipeFilter{
    ProjectIDs:    []project.ID{"proj_123"},
    PublishStatus: recipe.PublishStatusPublished,
})
if err != nil {
    return err
}
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
    fmt.Printf("Latest: %s\n", info.LatestCommit)
    if info.PublishedCommit != nil {
        fmt.Printf("Published: %s at %v\n", *info.PublishedCommit, *info.PublishedAt)
    }
}

// List all recipes (both published and unpublished)
iter, err = svc.ListRecipes(ctx, recipe.RecipeFilter{
    ProjectIDs:    []project.ID{"proj_123"},
    PublishStatus: recipe.PublishStatusAll,
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
        fmt.Printf("Published: %s\n", *info.PublishedCommit)
    } else {
        fmt.Printf("Published: (none)\n")
    }
}

// List only unpublished recipes
iter, err = svc.ListRecipes(ctx, recipe.RecipeFilter{
    ProjectIDs:    []project.ID{"proj_123"},
    PublishStatus: recipe.PublishStatusUnpublished,
})
defer iter.Close(ctx)

// ... iterate as above ...
```

### Using the Recipe Provider

```go
provider := recipe.NewProvider(svc, projectID, gitPath)

// Retrieve a published recipe (used by ticket service, etc.)
rec, err := provider.GetRecipe("workflows/ci/build")
if err != nil {
    return err
}
// Use the recipe
```

### Using `name@ref` for Version Pinning and Testing

```go
provider := recipe.NewProvider(svc, projectID, gitPath)

// Get currently published version
publishedRec, err := provider.GetRecipe("workflows/ci/build")

// Get specific version by commit hash (even if not published)
specificRec, err := provider.GetRecipe("workflows/ci/build@a1b2c3d4e5f6")

// Get from a branch (e.g., testing feature branch)
featureRec, err := provider.GetRecipe("workflows/ci/build@feature/new-api")

// Get from a tag (semantic versioning)
stableRec, err := provider.GetRecipe("workflows/ci/build@v1.0.0")

// Test a new version before publishing
testRec, err := provider.GetRecipe("workflows/ci/build@f7e8d9c0b1a2")
if err != nil {
    fmt.Printf("Test recipe not valid: %v\n", err)
}
```

**Use Cases**:

1. **Testing Before Publishing**:
```go
// Create new version (don't auto-publish)
version, err := svc.UpdateRecipe(ctx, recipe.UpdateInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    Content:     newContent,
    Message:     "Add new build features",
    AutoPublish: false,  // Test first
})

// Test the unpublished version using provider
provider := recipe.NewProvider(svc, projectID, gitPath)
testRec, err := provider.GetRecipe("workflows/ci/build@" + version.CommitHash)

// Run tests with testRec...

// If tests pass, publish
svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:  "proj_123",
    Name:       "workflows/ci/build",
    CommitHash: version.CommitHash,
})
```

2. **Semantic Versioning with Git Tags**:
```bash
# Advanced users can tag versions in git for semantic versioning
cd /path/to/project-repo
git tag -a v1.0.0 -m "Release 1.0.0"
git push origin v1.0.0

# Consumers can reference tagged versions
# provider.GetRecipe("workflows/ci/build@v1.0.0")
```

3. **Version Pinning in Dependencies**:
```yaml
# Recipe A depends on specific versions of Recipe B
version: "1.0"
op: sequence
inputs:
  steps:
    # Pin to specific commit (maximum stability)
    - recipe: "workflows/ci/build@a1b2c3d4e5f6"

    # Pin to tag (semantic versioning)
    - recipe: "workflows/deploy@v1.2.0"

    # Track branch (e.g., for development)
    - recipe: "workflows/test@develop"

    # Use published version (most common)
    - recipe: "workflows/cleanup"
```

4. **Rollback Without Re-publishing**:
```go
// Test previous version using tag
oldVersion, err := provider.GetRecipe("workflows/ci/build@v0.9.0")

// Or test previous commit
olderVersion, err := provider.GetRecipe("workflows/ci/build@a1b2c3d4")

// If it works better, can re-publish the old version
svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    CommitHash:  "a1b2c3d4e5f6",  // Old commit hash (or resolve from tag)
    PublishedBy: strPtr("user@example.com"),
})
```

5. **Multi-Environment Testing**:
```yaml
# Development environment uses feature branches
dev:
  recipe: "workflows/ci/build@develop"

# Staging uses release candidates
staging:
  recipe: "workflows/ci/build@rc/v2.0.0"

# Production uses tagged stable versions
production:
  recipe: "workflows/ci/build@v1.5.0"
```

### Validating Before Publishing

```go
// Read file content from git
content := []byte(`
version: "1.0"
op: echo
inputs:
  message: "Test"
`)

// Validate without publishing
result, err := svc.ValidateRecipe(ctx, recipe.ValidateInput{
    ProjectID: projectID,
    Name:      "workflows/ci/build",
    Content:   content,
})
if err != nil {
    if vErr, ok := err.(*recipe.ValidationFailedError); ok {
        fmt.Printf("Validation failed: %+v\n", vErr.Result.Errors)
        return
    }
    fmt.Printf("Validation error: %v\n", err)
    return
}
```

## Migration Path

For existing recipe systems (e.g., `server/registry`):

1. **Phase 1: Parallel Operation**
   - Deploy new git-backed recipe service alongside existing filesystem registry
   - Migrate recipes to git repository (bulk import tool)
   - Publish recipes to establish baseline
   - Keep registry running for existing consumers

2. **Phase 2: Gradual Migration**
   - New recipes use git-backed service only
   - Update consumers to use RecipeProvider from new service
   - Validate published recipes match registry recipes
   - Monitor for discrepancies

3. **Phase 3: Complete Migration**
   - All consumers use new recipe service
   - Decommission filesystem registry
   - Archive old data

## Testing Strategy

### Unit Tests
- CreateRecipe, UpdateRecipe, DeleteRecipe with mock git repository
- PublishRecipe with validation and recipe ID matching
- ValidateRecipe returns structured errors (schema, ID mismatch, CEL)
- PublishRecipe fails on CEL validation errors
- PublishRecipe fails when CEL validation service unavailable
- UnpublishRecipe with optimistic concurrency control (ExpectedCommit)
- Optimistic locking conflicts on concurrent publishes and unpublishes
- Transaction rollback scenarios
- RecipeProvider implementation
- RecipeProvider.parseRecipeRef for name@ref parsing
- Auto-publish flag handling with early validation
- Commit message generation

### Integration Tests
- Real git operations with temporary repositories
- Full lifecycle: Create → Update → Publish → Get
- Remote repository sync and push/pull operations
- Concurrent publish detection (same recipe name)
- GetRecipe with ref="" (published version)
- GetRecipe with various git refs:
  - Commit hashes (full and short)
  - Branch names (if using git tags/branches)
  - Tag names (if using git tags)
  - Relative refs (HEAD~1, main~2, etc.)
- ValidateRecipe API with CEL expression validation
- GetRecipeHistory returns iterator with correct version list
- ListRecipes with PublishStatus filters:
  - PublishStatusAll returns all recipes
  - PublishStatusPublished returns only published
  - PublishStatusUnpublished returns only unpublished
- Iterator pagination for ListRecipes and GetRecipeHistory

### E2E Tests
- Full workflow: CreateRecipe (auto-publish) → retrieve via provider
- Update workflow: Create → Update → Publish new version → Get
- Multi-project isolation
- Remote GitHub integration
- Version rollback (publish older commit)
- Recipe dependencies using name@ref references:
  - Commit hash pinning
  - Tag version pinning (if using git tags)
- Testing unpublished recipes via name@ref:
  - Recent commits not yet published
  - Git tags (if using semantic versioning)

## Security Considerations

1. **Access Control**: Project-level authorization required for all operations
2. **Git Credentials**: Securely stored, never logged
3. **Input Validation**:
   - Recipe names sanitized to prevent path traversal
   - Git refs validated to prevent command injection (e.g., `name@; rm -rf /`)
   - Only alphanumeric, `-`, `_`, `/`, `~`, `^`, `@`, `.` allowed in refs
4. **Content Validation**: YAML parsing protects against malicious content
5. **Audit Trail**: All operations logged with user context
6. **Ref Resolution**: Git ref resolution happens in isolated git workspace per project
7. **Branch Security**: Users with repo access can reference any branch/commit; access control at project level

## Performance Considerations

1. **Local Cache**: Git repositories cached locally, periodic sync with remote
2. **Lazy Clone**: Remote repos cloned on-demand, not at service startup
3. **Parallel Operations**: Different projects can operate concurrently
4. **Lightweight Index**: Database only stores published recipes (Name→Hash mapping)
5. **Pagination**: Iterator-based listing for large recipe sets
6. **Content Caching**: Consider caching parsed recipe content at published commits

## Open Questions

1. **Git LFS**: Do we need support for large recipe files (e.g., with embedded data)?
2. **Publish UI**: How do users discover commits/tags/branches to publish? (Git log, web UI, CLI tool?)
3. **Orphan Cleanup**: Should we detect/warn about published recipes pointing to missing commits?
4. **Recipe Dependencies**: How do we handle recipes that reference other recipes?
5. **Schema Evolution**: How do we handle recipe schema version migrations?
6. **Bulk Publishing**: Tool to bulk-publish multiple recipes from git?
7. **Ref to Hash Resolution**: Should we provide API to resolve refs (tags, branches) to commit hashes?
8. **Branch Tracking**: Should we warn users when using mutable refs (branches) vs immutable refs (commits, tags)?
9. **Validation Caching**: Should we cache validation results for specific refs to avoid re-validating?

## Future Enhancements

1. **Recipe Diffing**: Visual diff between recipe versions (git commits)
2. **Recipe Templates**: Template recipes with variable substitution
3. **Bulk Publishing**: Publish multiple recipes at once
4. **Import/Export**: Import recipes from URLs or export as bundles
5. **Recipe Testing**: Run recipe tests before publishing (`.recipe.test.yaml` files)
6. **Webhooks**: Notify external systems on publish/unpublish
7. **Recipe Marketplace**: Share recipes across projects/organizations
8. **Auto-Publish**: CI/CD integration to auto-publish on git push
9. **Commit Discovery**: API to list commits for a recipe file (for publish UI)
10. **Dependency Resolution**: Automatic resolution of recipe dependencies with version constraints
11. **Lock Files**: Generate lock files with resolved name@ref for all dependencies (convert refs to commit hashes)
12. **Ref Validation**: Validate that branch/tag refs exist before allowing publish
13. **Caching**: Cache recipe content at popular refs (branches, tags, published commits)
14. **Ref Aliases**: Support aliases like `name@latest` → published version, `name@stable` → specific tag
