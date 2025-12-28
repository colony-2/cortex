# Recipe Service Specification

## Overview

The Recipe Service provides git-backed storage and versioning for recipe definitions. Git is the primary storage and source of truth for all recipe content and history. The database maintains a lightweight index of published recipes, mapping recipe names to specific git commit hashes.

## Key Concepts

**Git-First Approach**: Users manage recipe files directly in git using normal workflows (create, edit, commit, push). The Recipe Service operates as a layer on top of git, not as a replacement.

**Publishing Model**:
- Recipes live in git (can be in any state: valid, invalid, draft, etc.)
- Only specific commits can be "published" via the service
- Publishing requires validation - invalid recipes cannot be published
- Each recipe name has exactly one published version (can be changed by re-publishing)
- Only published recipes are accessible to consumers (ticket service, etc.)

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
    GitPath     string         // Path in git repo (e.g., "recipes/foo/bar/myrecipe.recipe.yaml")
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
    // Publish a recipe version (validates then publishes)
    PublishRecipe(ctx context.Context, input PublishInput) (*PublishedRecipe, error)

    // Unpublish a recipe (removes from published index)
    UnpublishRecipe(ctx context.Context, projectID project.ID, name string) error

    // Get a published recipe with its content
    GetPublishedRecipe(ctx context.Context, projectID project.ID, name string) (*RecipeWithContent, error)

    // List published recipes
    ListPublishedRecipes(ctx context.Context, filter SearchFilter) (Iterator[*PublishedRecipe], error)

    // Get recipe content at any git ref (not just published)
    // Used by RecipeProvider to support name@ref syntax
    // ref can be: commit hash, branch name, tag, or any git ref (e.g., "HEAD~1")
    GetRecipeContent(ctx context.Context, projectID project.ID, gitPath string, ref string) (*recipe.Recipe, error)

    // Get git history for a recipe file
    GetHistory(ctx context.Context, projectID project.ID, gitPath string, limit int) ([]git.Commit, error)

    // Validate recipe content without publishing
    ValidateRecipe(ctx context.Context, content []byte) error

    // Sync from remote repository
    SyncFromRemote(ctx context.Context, projectID project.ID) error
}

type PublishInput struct {
    ProjectID   project.ID
    Name        string              // Hierarchical name (e.g., "foo/bar/myrecipe")
    GitPath     string              // Path in git repo (e.g., "recipes/foo/bar/myrecipe.recipe.yaml")
    CommitHash  string              // Commit hash to publish
    PublishedBy *string             // User publishing (optional)
}

type RecipeWithContent struct {
    PublishedRecipe
    Content     *recipe.Recipe      // Parsed recipe content
}

type SearchFilter struct {
    ProjectIDs []project.ID
    Names      []string            // Exact matches
    NamePrefix string              // Hierarchical prefix (e.g., "foo/bar/")
}
```

### Error Types

```go
var (
    ErrEmptyName        = errors.New("recipe: name is required")
    ErrInvalidName      = errors.New("recipe: invalid name format")
    ErrInvalidProject   = errors.New("recipe: project not found")
    ErrNotFound         = errors.New("recipe: not found")
    ErrNotPublished     = errors.New("recipe: recipe is not published")
    ErrVersionConflict  = errors.New("recipe: version conflict")
    ErrInvalidContent   = errors.New("recipe: invalid recipe content")
    ErrCommitNotFound   = errors.New("recipe: commit not found in git")
    ErrGitConflict      = errors.New("recipe: git merge conflict detected")
    ErrRemoteSync       = errors.New("recipe: failed to sync with remote")
)
```

## Git Integration

### Repository Structure

```
project-{project-id}/
├── .git/
└── recipes/
    ├── simple-recipe.recipe.yaml
    ├── foo/
    │   └── bar/
    │       └── nested-recipe.recipe.yaml
    └── tests/
        └── integration/
            └── test-recipe.recipe.yaml
```

### Git Workflow

Users manage recipe files directly through git:
- **Create/Edit**: Use normal git workflow (create file, edit, commit, push)
- **Branch/Merge**: Use git branches for development
- **History**: Use git log to view version history
- **Commit Messages**: Standard git messages (no special format required)

The Recipe Service operates on top of git, tracking which commits are published.

### Service Operations

#### Publish Recipe
1. Validate project exists
2. Verify commit exists in git repository
3. Fetch content at specified commit from git
4. Parse and validate using `recipe.LoadRecipeFromString()`
5. If validation fails: return `ErrInvalidContent` with details
6. If validation succeeds:
   - Insert or update published recipe index (Name → CommitHash)
   - Store PublishedAt timestamp and PublishedBy user
7. Return published recipe metadata

**Transaction**: Database insert/update uses optimistic locking (Version field) to prevent concurrent publish conflicts.

#### Unpublish Recipe
1. Validate project exists
2. Delete from published recipe index
3. Return success

**Note**: Unpublishing does NOT delete the file from git. It only removes the published reference.

#### Get Published Recipe
1. Query published recipe index by (ProjectID, Name)
2. If not found: return `ErrNotPublished`
3. Fetch file content from git at stored CommitHash
4. Parse and return recipe content
5. Return combined metadata + content

#### Sync from Remote
1. Fetch from remote: `git fetch origin {branch}`
2. Merge or rebase (strategy TBD)
3. For each published recipe in database:
   - Verify commit still exists in git
   - If commit missing (e.g., force push): log warning or unpublish
4. Return sync status

### Validation Integration

Uses `server/recipe-core/pkg/recipe` for validation:

```go
func (s *service) ValidateRecipe(ctx context.Context, content []byte) error {
    _, err := recipe.LoadRecipeFromString(content)
    if err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    return nil
}
```

This validation is automatically performed during `PublishRecipe`. Only valid recipes can be published.

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

#### Local Repository
- Default mode
- Repository path: `{data_dir}/recipes/{project_id}`
- Auto-initialized on first recipe creation

#### Remote Repository (GitHub, etc.)
- Configured per project via `GitRepoURL`
- Operations:
  - Clone on first access: `git clone {url} {local_path}`
  - Fetch before operations: `git fetch origin`
  - Push after commits: `git push origin {branch}`
  - Periodic sync to pull remote changes

#### Authentication
- SSH keys (preferred for automation)
- Personal Access Tokens (for HTTPS)
- Configured at service level, not per-recipe

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

    // Check if name contains @ref suffix
    recipeName, gitRef := p.parseRecipeRef(name)

    if gitRef != "" {
        // Direct reference to specific git ref (commit, branch, tag, etc.)
        // Examples: "foo/bar@a1b2c3d4", "foo/bar@main", "foo/bar@v1.0.0", "foo/bar@HEAD~1"
        // This allows testing unpublished versions or pinning to specific refs
        gitPath := p.deriveGitPath(recipeName)
        content, err := p.service.GetRecipeContent(ctx, p.projectID, gitPath, gitRef)
        if err != nil {
            return nil, fmt.Errorf("recipe %q at ref %s not found: %w", recipeName, gitRef, err)
        }
        return content, nil
    }

    // No @ref suffix - get published version
    recipeWithContent, err := p.service.GetPublishedRecipe(ctx, p.projectID, recipeName)
    if err != nil {
        return nil, err
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

// deriveGitPath converts recipe name to git path
// e.g., "foo/bar/myrecipe" → "recipes/foo/bar/myrecipe.recipe.yaml"
func (p *Provider) deriveGitPath(name string) string {
    return filepath.Join("recipes", name + ".recipe.yaml")
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
- Valid characters: `a-z`, `0-9`, `-`, `_`, `/`
- Examples:
  - `deployment-recipe`
  - `workflows/ci/build-and-test`
  - `data-processing/etl/load-users`

### Git Paths
- Derived from name: `recipes/{name}.recipe.yaml`
- Directories auto-created as needed
- Examples:
  - Name: `simple` → Path: `recipes/simple.recipe.yaml`
  - Name: `foo/bar/nested` → Path: `recipes/foo/bar/nested.recipe.yaml`

## Dependencies on Existing Modules

### Required
- `server/git/pkg/git`: Git operations (GetStatus, CreateCommit, GetHistory, StageFiles)
- `server/recipe-core/pkg/recipe`: Recipe parsing and validation (LoadRecipeFromString)
- `server/project/pkg/project`: Project validation

### Missing Git Capabilities
The following capabilities are not currently in `server/git` and need to be added:
1. **Clone**: `Clone(ctx context.Context, url string, localPath string, branch string) error`
2. **Fetch**: `Fetch(ctx context.Context, nodePath string, remote string) error`
3. **Pull**: `Pull(ctx context.Context, nodePath string) error`
4. **Push**: `Push(ctx context.Context, nodePath string, remote string, branch string) error`
5. **CheckAncestor**: `IsAncestor(ctx context.Context, nodePath string, commit1 string, commit2 string) (bool, error)`
6. **GetRemoteHead**: `GetRemoteHead(ctx context.Context, nodePath string, remote string, branch string) (string, error)`

These can be added following the existing pattern in `server/git/internal/commands/commands.go`.

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
    RemoteAuth    RemoteAuthConfig       // Authentication for remote repos
}

type RemoteAuthConfig struct {
    SSHKeyPath    string                 // Path to SSH private key
    AccessToken   string                 // Personal access token
}
```

## Transaction Handling

Following the pattern from `cell` service:

```go
func (s *service) PublishRecipe(ctx context.Context, input PublishInput) (*PublishedRecipe, error) {
    // 1. Validate project exists
    if err := s.ensureProject(ctx, input.ProjectID); err != nil {
        return nil, err
    }

    // 2. Get git workspace
    gitPath := s.getGitWorkspace(input.ProjectID)

    // 3. Read file at specified commit from git
    content, err := s.readFileAtCommit(ctx, gitPath, input.GitPath, input.CommitHash)
    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrCommitNotFound, err)
    }

    // 4. Validate recipe content
    if err := s.ValidateRecipe(ctx, content); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrInvalidContent, err)
    }

    // 5. Insert or update published recipe in database (with optimistic locking)
    var result *PublishedRecipe
    err = s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
        existing, err := txStore.GetByName(ctx, input.ProjectID, input.Name)

        if errors.Is(err, gorm.ErrRecordNotFound) {
            // New published recipe
            id, err := s.idGen.NewID()
            if err != nil {
                return err
            }

            result = &PublishedRecipe{
                ID:          string(id),
                ProjectID:   input.ProjectID,
                Name:        input.Name,
                GitPath:     input.GitPath,
                CommitHash:  input.CommitHash,
                PublishedAt: s.clock.Now(),
                PublishedBy: input.PublishedBy,
                Version:     1,
            }

            return txStore.Create(ctx, result)
        }

        if err != nil {
            return err
        }

        // Update existing published recipe
        existing.CommitHash = input.CommitHash
        existing.PublishedAt = s.clock.Now()
        existing.PublishedBy = input.PublishedBy

        err = txStore.Update(ctx, existing)
        if errors.Is(err, store.ErrOptimisticLock) {
            return ErrVersionConflict
        }

        result = existing
        return err
    })

    return result, err
}
```

## Usage Examples

### Creating and Editing a Recipe (Git Workflow)

```bash
# Users manage recipes directly in git
cd /path/to/project-{project-id}/recipes

# Create a new recipe file
cat > workflows/ci/build.recipe.yaml <<EOF
version: "1.0"
op: echo
inputs:
  message: "Building project..."
EOF

# Commit to git
git add workflows/ci/build.recipe.yaml
git commit -m "Add CI build recipe"
git push origin main

# Note the commit hash for publishing
git log -1 --format="%H"
```

### Publishing a Recipe

```go
svc, _ := recipe.NewService(config)

// Publish the recipe at a specific commit
published, err := svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    GitPath:     "recipes/workflows/ci/build.recipe.yaml",
    CommitHash:  "a1b2c3d4e5f6...",  // Commit hash from git log
    PublishedBy: strPtr("user@example.com"),
})
```

### Updating a Published Recipe

```bash
# Edit the recipe in git
vim workflows/ci/build.recipe.yaml

# Commit changes
git add workflows/ci/build.recipe.yaml
git commit -m "Update build message"
git push origin main

# Get new commit hash
git log -1 --format="%H"
```

```go
// Re-publish with the new commit
published, err := svc.PublishRecipe(ctx, recipe.PublishInput{
    ProjectID:   "proj_123",
    Name:        "workflows/ci/build",
    GitPath:     "recipes/workflows/ci/build.recipe.yaml",
    CommitHash:  "f7e8d9c0b1a2...",  // New commit hash
    PublishedBy: strPtr("user@example.com"),
})
```

### Getting a Published Recipe

```go
// Get published recipe with content
recipeWithContent, err := svc.GetPublishedRecipe(ctx, "proj_123", "workflows/ci/build")
if err != nil {
    return err
}

fmt.Printf("Recipe: %s\n", recipeWithContent.Name)
fmt.Printf("Published at: %v\n", recipeWithContent.PublishedAt)
fmt.Printf("Commit: %s\n", recipeWithContent.CommitHash)
// Use recipeWithContent.Content for parsed recipe
```

### Unpublishing a Recipe

```go
// Remove from published index (doesn't delete from git)
err := svc.UnpublishRecipe(ctx, "proj_123", "workflows/ci/build")
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

1. **Testing Branch Before Publishing**:
```bash
# Create feature branch
git checkout -b feature/improved-build
vim workflows/ci/build.recipe.yaml
git add workflows/ci/build.recipe.yaml
git commit -m "Add new build features"
git push origin feature/improved-build

# Test the feature branch version (not published)
# Reference it as "workflows/ci/build@feature/improved-build"

# Once tested, merge to main and publish
git checkout main
git merge feature/improved-build
git push origin main
```

2. **Semantic Versioning with Tags**:
```bash
# Tag stable versions
git tag -a v1.0.0 -m "Release 1.0.0"
git push origin v1.0.0

# Consumers can reference tagged versions
# "workflows/ci/build@v1.0.0"
# "workflows/ci/build@v2.0.0"
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
    GitPath:     "recipes/workflows/ci/build.recipe.yaml",
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
err := svc.ValidateRecipe(ctx, content)
if err != nil {
    fmt.Printf("Validation failed: %v\n", err)
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
- PublishRecipe with mock git repository
- Validation integration
- Optimistic locking conflicts
- Transaction rollback scenarios
- RecipeProvider implementation
- RecipeProvider.parseRecipeRef for name@hash parsing

### Integration Tests
- Real git operations with temporary repositories
- Publish workflow with actual commits
- Remote repository sync
- Concurrent publish detection (same recipe name)
- GetPublishedRecipe with git content retrieval
- GetRecipeContent with various git refs:
  - Commit hashes (full and short)
  - Branch names
  - Tag names
  - Relative refs (HEAD~1, main~2, etc.)

### E2E Tests
- Full workflow: git commit → publish → retrieve via provider
- Multi-project isolation
- Remote GitHub integration
- Version rollback (publish older commit)
- Recipe dependencies using name@ref references:
  - Commit hash pinning
  - Tag version pinning
  - Branch tracking
- Testing unpublished recipes via name@ref:
  - Feature branches
  - Tagged releases
  - Specific commits

## Security Considerations

1. **Access Control**: Project-level authorization required for all operations
2. **Git Credentials**: Securely stored, never logged
3. **Input Validation**: Recipe names sanitized to prevent path traversal
4. **Content Validation**: YAML parsing protects against malicious content
5. **Audit Trail**: All operations logged with user context

## Performance Considerations

1. **Local Cache**: Git repositories cached locally, periodic sync with remote
2. **Lazy Clone**: Remote repos cloned on-demand, not at service startup
3. **Parallel Operations**: Different projects can operate concurrently
4. **Lightweight Index**: Database only stores published recipes (Name→Hash mapping)
5. **Pagination**: Iterator-based listing for large recipe sets
6. **Content Caching**: Consider caching parsed recipe content at published commits

## Open Questions

1. **Git LFS**: Do we need support for large recipe files (e.g., with embedded data)?
2. **Branch Strategy**: Users can use branches in git - should we track published recipes per branch?
3. **Publish UI**: How do users discover commit hashes to publish? (Git log, web UI, CLI tool?)
4. **Orphan Cleanup**: Should we detect/warn about published recipes pointing to missing commits?
5. **Recipe Dependencies**: How do we handle recipes that reference other recipes?
6. **Schema Evolution**: How do we handle recipe schema version migrations?
7. **Bulk Publishing**: Tool to bulk-publish multiple recipes from git?

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
