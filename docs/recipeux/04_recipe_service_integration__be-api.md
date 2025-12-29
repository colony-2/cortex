# Recipe Service Integration and API Implementation

**Target Cell:** `be-api` (`server/api`)
**Dependencies:** Completion of `01_openapi_specification__api-openapi.md` (with generated Go types)
**Status:** Not Started

## Overview

Integrate the git-backed recipe service from `/src/server/recipes` into the testserver and implement the API handlers using the generated OpenAPI types. The recipe service will be the first provider in the chain, with fallback to existing registry and embedded providers.

## Background

Currently, the testserver uses a chained recipe provider:
1. **Registry Provider** - File-based recipes from `{nodesPath}/recipes`
2. **Embedded Provider** - Built-in `internal://new_ticket` recipe

The recipe service provides git-backed version control, publish/unpublish lifecycle, and version history. The OpenAPI specification (completed in step 01) defines the API contract and generates types that both server and client will use.

## Goals

1. Add recipe service as a dependency to `be-api`
2. Initialize recipe service in testserver
3. Create provider adapter for recipe service
4. Update provider chain (recipe service → registry → embedded)
5. Implement all 8 API handlers using generated OpenAPI types
6. Ensure backward compatibility

## Part 1: Recipe Service Integration

### 1. Add Dependency

**File:** `/src/server/api/go.mod`

```bash
cd /src/server/api
go get github.com/colony-2/colony2/server/recipes
go mod tidy
```

### 2. Initialize Recipe Service in Testserver

**File:** `/src/server/api/cmd/testserver/main.go`

**Location:** In `runServer()` function, after database setup (around line 180)

```go
// Initialize recipe service
recipeSvc, err := recipe.NewServiceFromDB(db, recipe.ServiceConfig{
    GitRepo:       gitRepo,  // Reuse from cell service
    Projects:      projectSvc,
    IDGen:         recipe.NewKSUIDGenerator(),
    Clock:         recipe.NewSystemClock(),
    WorkspaceRoot: filepath.Join(absNodesPath, ".recipe-workspaces"),
})
if err != nil {
    return fmt.Errorf("failed to create recipe service: %w", err)
}
```

### 3. Create Recipe Service Provider Adapter

**File:** `/src/server/api/internal/recipes/service_provider.go` (new file)

```go
package recipes

import (
    "context"
    "fmt"
    "strings"

    recipecore "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
    recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
    "github.com/colony-2/colony2/server/core/pkg/project"
)

// ServiceProvider wraps the recipe service to implement RecipeProvider interface
type ServiceProvider struct {
    svc       recipesvc.Service
    projectID project.ID
}

// NewServiceProvider creates a provider for a specific project
func NewServiceProvider(svc recipesvc.Service, projectID project.ID) *ServiceProvider {
    return &ServiceProvider{
        svc:       svc,
        projectID: projectID,
    }
}

// GetRecipe implements recipe.RecipeProvider
// Supports both simple names ("workflow/build") and versioned refs ("workflow/build@v1.0.0")
func (p *ServiceProvider) GetRecipe(name string) (*recipecore.Recipe, error) {
    ctx := context.Background()

    // Parse name@ref syntax
    recipeName := name
    ref := "" // empty = published version

    if idx := strings.Index(name, "@"); idx != -1 {
        recipeName = name[:idx]
        ref = name[idx+1:]
    }

    // Get recipe from service
    recipeWithContent, err := p.svc.GetRecipe(ctx, string(p.projectID), recipeName, ref)
    if err != nil {
        return nil, fmt.Errorf("recipe service get %s: %w", name, err)
    }

    return recipeWithContent.Content, nil
}
```

### 4. Update Provider Chain

**File:** `/src/server/api/cmd/testserver/main.go`

**Location:** Replace current provider chain setup (around lines 187-206)

```go
// Create recipe registry (for backward compatibility)
recipePath := filepath.Join(absNodesPath, "recipes")
reg, err := registry.NewRegistry(nil, recipePath)
if err != nil {
    return fmt.Errorf("failed to create worker registry: %w", err)
}

// Create embedded provider
embeddedProvider, err := recipes.NewEmbeddedProvider()
if err != nil {
    fmt.Printf("Warning: failed to create embedded recipe provider: %v\n", err)
    embeddedProvider = nil
}

// Create recipe service provider for default project
// TODO: Make this dynamic based on workflow context
defaultProjectID := project.ID("proj_default")
recipeServiceProvider := recipes.NewServiceProvider(recipeSvc, defaultProjectID)

// Create chained provider: recipe service -> registry -> embedded
providers := []recipe.RecipeProvider{recipeServiceProvider, reg}
if embeddedProvider != nil {
    providers = append(providers, embeddedProvider)
}
recipeProvider := recipes.NewChainedProvider(providers...)
```

## Part 2: API Handler Implementation

### 1. Update Handlers Struct

**File:** `/src/server/api/internal/handlers/handlers.go`

Add recipe service field:

```go
type Handlers struct {
    db        *gorm.DB
    projects  project.Service
    cells     cell.Service
    tickets   ticket.Service
    workflows workflow.Service
    recipeSvc recipe.Service  // Add this
    // ... existing fields
}
```

Update constructor in testserver:

**File:** `/src/server/api/cmd/testserver/main.go`

```go
handlers := handlers.NewHandlers(
    db,
    projectSvc,
    cellSvc,
    ticketSvc,
    workflowSvc,
    recipeSvc,  // Pass recipe service
    // ... other dependencies
)
```

### 2. Implement Recipe Handlers

**File:** `/src/server/api/internal/handlers/recipes.go` (new file)

```go
package handlers

import (
    "encoding/json"
    "errors"
    "fmt"
    "net/http"

    "github.com/gorilla/mux"
    "github.com/colony-2/colony2/server/core/pkg/project"
    recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
)

// handleListRecipes implements GET /projects/{projectId}/recipes
func (h *Handlers) handleListRecipes(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])

    // Parse status filter
    statusParam := r.URL.Query().Get("status")
    var publishStatus recipesvc.PublishStatus
    switch statusParam {
    case "published":
        publishStatus = recipesvc.PublishStatusPublished
    case "unpublished":
        publishStatus = recipesvc.PublishStatusUnpublished
    default:
        publishStatus = recipesvc.PublishStatusAll
    }

    // List recipes
    iter, err := h.recipeSvc.ListRecipes(r.Context(), recipesvc.RecipeFilter{
        ProjectIDs:    []project.ID{projectID},
        PublishStatus: publishStatus,
    })
    if err != nil {
        writeRecipeError(w, err)
        return
    }
    defer iter.Close(r.Context())

    // Collect results
    recipes := []map[string]interface{}{}
    for {
        info, err := iter.Next(r.Context())
        if errors.Is(err, recipesvc.ErrIteratorDone) {
            break
        }
        if err != nil {
            writeRecipeError(w, err)
            return
        }

        recipeInfo := map[string]interface{}{
            "name":          info.Name,
            "latestCommit":  info.LatestCommit,
            "latestCommitAt": info.LatestCommitAt,
        }
        if info.PublishedCommit != nil {
            recipeInfo["publishedCommit"] = *info.PublishedCommit
            recipeInfo["publishedAt"] = *info.PublishedAt
            if info.PublishedBy != nil {
                recipeInfo["publishedBy"] = *info.PublishedBy
            }
        }

        recipes = append(recipes, recipeInfo)
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "recipes": recipes,
    })
}

// handleCreateRecipe implements POST /projects/{projectId}/recipes
func (h *Handlers) handleCreateRecipe(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])

    var req struct {
        Name        string `json:"name"`
        Content     string `json:"content"`
        Description string `json:"description"`
        AutoPublish bool   `json:"autoPublish"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, err, http.StatusBadRequest)
        return
    }

    // Validate inputs
    if req.Name == "" || req.Content == "" {
        writeError(w, fmt.Errorf("name and content are required"), http.StatusBadRequest)
        return
    }

    // Create recipe
    version, err := h.recipeSvc.CreateRecipe(r.Context(), recipesvc.CreateInput{
        ProjectID:   projectID,
        Name:        req.Name,
        Content:     []byte(req.Content),
        Description: req.Description,
        AutoPublish: req.AutoPublish,
    })
    if err != nil {
        writeRecipeError(w, err)
        return
    }

    writeJSON(w, http.StatusCreated, map[string]interface{}{
        "commitHash": version.CommitHash,
        "shortHash":  version.ShortHash,
        "createdAt":  version.CreatedAt,
        "author":     version.Author,
        "message":    version.Message,
        "isPublished": version.IsPublished,
    })
}

// handleGetRecipe implements GET /projects/{projectId}/recipes/{recipeName}
func (h *Handlers) handleGetRecipe(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])
    recipeName := mux.Vars(r)["recipeName"]
    ref := r.URL.Query().Get("ref")

    recipe, err := h.recipeSvc.GetRecipe(r.Context(), string(projectID), recipeName, ref)
    if err != nil {
        writeRecipeError(w, err)
        return
    }

    // Convert to response format
    response := map[string]interface{}{
        "name":        recipe.Name,
        "commitHash":  recipe.CommitHash,
        "isPublished": recipe.IsPublished,
        "content":     recipe.Content,
        "rawYaml":     string(recipe.Content.Raw()), // Assuming Content has Raw() method
    }
    if recipe.PublishedAt != nil {
        response["publishedAt"] = *recipe.PublishedAt
    }
    if recipe.PublishedBy != nil {
        response["publishedBy"] = *recipe.PublishedBy
    }

    writeJSON(w, http.StatusOK, response)
}

// handleUpdateRecipe implements PUT /projects/{projectId}/recipes/{recipeName}
func (h *Handlers) handleUpdateRecipe(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])
    recipeName := mux.Vars(r)["recipeName"]

    var req struct {
        Content        string  `json:"content"`
        Message        string  `json:"message"`
        AutoPublish    bool    `json:"autoPublish"`
        ExpectedCommit *string `json:"expectedCommit"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, err, http.StatusBadRequest)
        return
    }

    if req.Content == "" {
        writeError(w, fmt.Errorf("content is required"), http.StatusBadRequest)
        return
    }

    expectedCommit := ""
    if req.ExpectedCommit != nil {
        expectedCommit = *req.ExpectedCommit
    }

    version, err := h.recipeSvc.UpdateRecipe(r.Context(), recipesvc.UpdateInput{
        ProjectID:      projectID,
        Name:           recipeName,
        Content:        []byte(req.Content),
        Message:        req.Message,
        AutoPublish:    req.AutoPublish,
        ExpectedCommit: expectedCommit,
    })
    if err != nil {
        writeRecipeError(w, err)
        return
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "commitHash":  version.CommitHash,
        "shortHash":   version.ShortHash,
        "createdAt":   version.CreatedAt,
        "author":      version.Author,
        "message":     version.Message,
        "isPublished": version.IsPublished,
    })
}

// handleDeleteRecipe implements DELETE /projects/{projectId}/recipes/{recipeName}
func (h *Handlers) handleDeleteRecipe(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])
    recipeName := mux.Vars(r)["recipeName"]

    err := h.recipeSvc.DeleteRecipe(r.Context(), string(projectID), recipeName)
    if err != nil {
        writeRecipeError(w, err)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

// handlePublishRecipe implements POST /projects/{projectId}/recipes/{recipeName}/publish
func (h *Handlers) handlePublishRecipe(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])
    recipeName := mux.Vars(r)["recipeName"]

    var req struct {
        CommitHash     string  `json:"commitHash"`
        PublishedBy    *string `json:"publishedBy"`
        ExpectedCommit *string `json:"expectedCommit"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // Empty body is OK
        req = struct {
            CommitHash     string
            PublishedBy    *string
            ExpectedCommit *string
        }{}
    }

    published, err := h.recipeSvc.PublishRecipe(r.Context(), recipesvc.PublishInput{
        ProjectID:      projectID,
        Name:           recipeName,
        CommitHash:     req.CommitHash,
        PublishedBy:    req.PublishedBy,
        ExpectedCommit: req.ExpectedCommit,
    })
    if err != nil {
        writeRecipeError(w, err)
        return
    }

    response := map[string]interface{}{
        "name":            published.Name,
        "publishedCommit": published.CommitHash,
        "publishedAt":     published.PublishedAt,
    }
    if published.PublishedBy != nil {
        response["publishedBy"] = *published.PublishedBy
    }

    writeJSON(w, http.StatusOK, response)
}

// handleUnpublishRecipe implements POST /projects/{projectId}/recipes/{recipeName}/unpublish
func (h *Handlers) handleUnpublishRecipe(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])
    recipeName := mux.Vars(r)["recipeName"]

    err := h.recipeSvc.UnpublishRecipe(r.Context(), recipesvc.UnpublishInput{
        ProjectID: projectID,
        Name:      recipeName,
    })
    if err != nil {
        writeRecipeError(w, err)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

// handleGetRecipeHistory implements GET /projects/{projectId}/recipes/{recipeName}/history
func (h *Handlers) handleGetRecipeHistory(w http.ResponseWriter, r *http.Request) {
    projectID := project.ID(mux.Vars(r)["projectId"])
    recipeName := mux.Vars(r)["recipeName"]

    iter, err := h.recipeSvc.GetRecipeHistory(r.Context(), string(projectID), recipeName)
    if err != nil {
        writeRecipeError(w, err)
        return
    }
    defer iter.Close(r.Context())

    versions := []map[string]interface{}{}
    for {
        version, err := iter.Next(r.Context())
        if errors.Is(err, recipesvc.ErrIteratorDone) {
            break
        }
        if err != nil {
            writeRecipeError(w, err)
            return
        }

        versions = append(versions, map[string]interface{}{
            "commitHash":  version.CommitHash,
            "shortHash":   version.ShortHash,
            "createdAt":   version.CreatedAt,
            "author":      version.Author,
            "message":     version.Message,
            "isPublished": version.IsPublished,
        })
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "versions": versions,
    })
}

// writeRecipeError handles recipe service errors with appropriate HTTP status codes
func writeRecipeError(w http.ResponseWriter, err error) {
    switch {
    case errors.Is(err, recipesvc.ErrNotFound):
        writeError(w, err, http.StatusNotFound)
    case errors.Is(err, recipesvc.ErrNotPublished):
        writeError(w, fmt.Errorf("recipe not published"), http.StatusConflict)
    case errors.Is(err, recipesvc.ErrVersionConflict):
        writeError(w, fmt.Errorf("version conflict"), http.StatusConflict)
    case errors.Is(err, recipesvc.ErrInvalidContent):
        writeError(w, err, http.StatusBadRequest)
    default:
        writeError(w, err, http.StatusInternalServerError)
    }
}
```

### 3. Register Routes

**File:** `/src/server/api/internal/handlers/api.go`

Update recipe routes (around line 89):

```go
// Recipe management endpoints
recipeRouter := api.PathPrefix("/projects/{projectId}/recipes").Subrouter()

// List and create
recipeRouter.HandleFunc("", withHandlerLog("recipes:list", h.handleListRecipes)).Methods(http.MethodGet)
recipeRouter.HandleFunc("", withHandlerLog("recipes:create", h.handleCreateRecipe)).Methods(http.MethodPost)

// Individual recipe operations (use {recipeName:.*} to allow slashes)
recipeRouter.HandleFunc("/{recipeName:.*}", withHandlerLog("recipes:get", h.handleGetRecipe)).Methods(http.MethodGet)
recipeRouter.HandleFunc("/{recipeName:.*}", withHandlerLog("recipes:update", h.handleUpdateRecipe)).Methods(http.MethodPut)
recipeRouter.HandleFunc("/{recipeName:.*}", withHandlerLog("recipes:delete", h.handleDeleteRecipe)).Methods(http.MethodDelete)

// Publish/unpublish operations
recipeRouter.HandleFunc("/{recipeName:.*}/publish", withHandlerLog("recipes:publish", h.handlePublishRecipe)).Methods(http.MethodPost)
recipeRouter.HandleFunc("/{recipeName:.*}/unpublish", withHandlerLog("recipes:unpublish", h.handleUnpublishRecipe)).Methods(http.MethodPost)

// History
recipeRouter.HandleFunc("/{recipeName:.*}/history", withHandlerLog("recipes:history", h.handleGetRecipeHistory)).Methods(http.MethodGet)
```

## Testing

### Unit Tests

**File:** `/src/server/api/internal/recipes/service_provider_test.go`

Test the provider adapter.

### Integration Tests

**File:** `/src/server/api/internal/handlers/recipes_test.go`

Test all handler endpoints.

### Manual Testing

```bash
# Start testserver
cd /src/server/api
go run ./cmd/testserver

# Create recipe
curl -X POST http://localhost:8080/api/v1/projects/proj_123/recipes \
  -H "Content-Type: application/json" \
  -d '{
    "name": "workflows/test",
    "content": "version: \"1.0\"\nid: workflows/test\nop: echo\ninputs:\n  message: \"test\"",
    "autoPublish": true
  }'

# List recipes
curl http://localhost:8080/api/v1/projects/proj_123/recipes

# Get recipe
curl http://localhost:8080/api/v1/projects/proj_123/recipes/workflows%2Ftest

# Update recipe
curl -X PUT http://localhost:8080/api/v1/projects/proj_123/recipes/workflows%2Ftest \
  -H "Content-Type: application/json" \
  -d '{"content": "version: \"1.1\"\nid: workflows/test\nop: echo\ninputs:\n  message: \"updated\""}'
```

## Success Criteria

- [ ] Recipe service dependency added
- [ ] Recipe service initialized in testserver
- [ ] ServiceProvider adapter created
- [ ] Provider chain updated (service first)
- [ ] All 8 handlers implemented
- [ ] Routes registered correctly
- [ ] Error handling standardized
- [ ] Integration tests pass
- [ ] Manual testing confirms all operations work
- [ ] Backward compatibility maintained

## Next Steps

After completing this implementation:
1. Proceed to `03_web_recipe_navigation__web-app.md` - Build recipe list UI
2. Then `04_web_recipe_editing__web-app.md` - Build recipe editor UI
