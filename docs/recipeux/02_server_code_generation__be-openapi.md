# Server Code Generation from OpenAPI

**Target Cell:** `be-openapi` (`server/openapi`)
**Dependencies:** Completion of `01_openapi_specification__api-openapi.md`
**Status:** Not Started

## Overview

Generate Go types, models, and server stubs from the OpenAPI specification using `oapi-codegen`. This provides type-safe request/response structures that the API handlers will use.

## Background

The `be-openapi` cell uses `oapi-codegen` to generate Go code from the OpenAPI spec. The existing `codegen.yml` configuration generates:
- Models (request/response types)
- Embedded spec
- Client (for testing)

## Goals

1. Update `codegen.yml` to generate recipe-related types
2. Run code generation to produce Go types
3. Verify generated types compile
4. Make generated types available to `be-api` cell
5. Document generated types for handler implementation

## Current Configuration

**File:** `/src/server/openapi/codegen.yml`

```yaml
package: openapi
generate:
  models: true
  embedded-spec: true
  client: true
output-options:
  skip-prune: true
```

This configuration should already generate all necessary types from the OpenAPI spec.

## Implementation Steps

### 1. Verify OpenAPI Spec is Complete

Ensure the recipe endpoints and schemas from `01_openapi_specification__api-openapi.md` are in the spec:

```bash
cd /src/api/openapi

# Verify recipe schemas exist
grep -A 5 "RecipeInfo:" colony2-api.yaml
grep -A 5 "RecipeWithContent:" colony2-api.yaml
grep -A 5 "RecipeVersion:" colony2-api.yaml

# Verify recipe endpoints exist
grep "/projects/{projectId}/recipes" colony2-api.yaml
```

### 2. Generate Go Code

**Command:**

```bash
cd /src/server/openapi

# Generate code using oapi-codegen
# This reads codegen.yml and generates to pkg/openapi/
oapi-codegen -config codegen.yml ../../api/openapi/colony2-api.yaml > pkg/openapi/types.gen.go

# Or use moon task if configured
moon run be-openapi:build
```

### 3. Verify Generated Types

**File:** `/src/server/openapi/pkg/openapi/types.gen.go` (generated)

Check that recipe types were generated:

```bash
cd /src/server/openapi

# Check for recipe types
grep "type RecipeInfo" pkg/openapi/types.gen.go
grep "type RecipeWithContent" pkg/openapi/types.gen.go
grep "type RecipeVersion" pkg/openapi/types.gen.go
grep "type CreateRecipeRequest" pkg/openapi/types.gen.go
grep "type UpdateRecipeRequest" pkg/openapi/types.gen.go
```

**Expected Output Example:**

```go
// RecipeInfo - Summary information about a recipe
type RecipeInfo struct {
    LatestCommit    string     `json:"latestCommit"`
    LatestCommitAt  time.Time  `json:"latestCommitAt"`
    Name            string     `json:"name"`
    PublishedAt     *time.Time `json:"publishedAt,omitempty"`
    PublishedBy     *string    `json:"publishedBy,omitempty"`
    PublishedCommit *string    `json:"publishedCommit,omitempty"`
}

// RecipeWithContent - Recipe with full content and metadata
type RecipeWithContent struct {
    CommitHash  string                 `json:"commitHash"`
    Content     map[string]interface{} `json:"content"`
    IsPublished bool                   `json:"isPublished"`
    Name        string                 `json:"name"`
    PublishedAt *time.Time             `json:"publishedAt,omitempty"`
    PublishedBy *string                `json:"publishedBy,omitempty"`
    RawYaml     string                 `json:"rawYaml"`
}

// RecipeVersion - Version information from recipe history
type RecipeVersion struct {
    Author      string    `json:"author"`
    CommitHash  string    `json:"commitHash"`
    CreatedAt   time.Time `json:"createdAt"`
    IsPublished bool      `json:"isPublished"`
    Message     string    `json:"message"`
    ShortHash   string    `json:"shortHash"`
}

// CreateRecipeRequest - Request to create a new recipe
type CreateRecipeRequest struct {
    AutoPublish *bool   `json:"autoPublish,omitempty"`
    Content     string  `json:"content"`
    Description *string `json:"description,omitempty"`
    Name        string  `json:"name"`
}

// UpdateRecipeRequest - Request to update an existing recipe
type UpdateRecipeRequest struct {
    AutoPublish    *bool   `json:"autoPublish,omitempty"`
    Content        string  `json:"content"`
    ExpectedCommit *string `json:"expectedCommit,omitempty"`
    Message        *string `json:"message,omitempty"`
}

// PublishRecipeRequest - Request to publish a recipe version
type PublishRecipeRequest struct {
    CommitHash     *string `json:"commitHash,omitempty"`
    ExpectedCommit *string `json:"expectedCommit,omitempty"`
    PublishedBy    *string `json:"publishedBy,omitempty"`
}

// PublishedRecipe - Response after publishing a recipe
type PublishedRecipe struct {
    Name            string     `json:"name"`
    PublishedAt     time.Time  `json:"publishedAt"`
    PublishedBy     *string    `json:"publishedBy,omitempty"`
    PublishedCommit string     `json:"publishedCommit"`
}

// RecipeListResponse - Response containing list of recipes
type RecipeListResponse struct {
    Recipes []RecipeInfo `json:"recipes"`
}

// RecipeHistoryResponse - Response containing recipe version history
type RecipeHistoryResponse struct {
    Versions []RecipeVersion `json:"versions"`
}
```

### 4. Test Compilation

Ensure the generated code compiles:

```bash
cd /src/server/openapi

# Build the package
go build ./...

# Run tests (if any)
go test ./...

# Or use moon
moon run be-openapi:build
moon run be-openapi:test
```

### 5. Verify Dependency Chain

The `be-api` cell should depend on `be-openapi`:

**File:** `/src/server/api/go.mod`

```go
require (
    github.com/colony-2/colony2/server/openapi v0.0.0
    // ... other dependencies
)
```

Run from `be-api`:

```bash
cd /src/server/api
go mod tidy
```

### 6. Document Generated Types

Create or update documentation:

**File:** `/src/server/openapi/README.md` (create if doesn't exist)

```markdown
# OpenAPI Generated Code

This package contains Go types and models generated from the Colony2 OpenAPI specification.

## Generation

Code is generated using `oapi-codegen`:

\`\`\`bash
oapi-codegen -config codegen.yml ../../api/openapi/colony2-api.yaml > pkg/openapi/types.gen.go
\`\`\`

Or via moon:

\`\`\`bash
moon run be-openapi:build
\`\`\`

## Recipe Types

The following types are generated for recipe management:

### Request Types
- `CreateRecipeRequest` - Create a new recipe
- `UpdateRecipeRequest` - Update existing recipe
- `PublishRecipeRequest` - Publish a recipe version

### Response Types
- `RecipeInfo` - Recipe summary (for list responses)
- `RecipeWithContent` - Full recipe with content
- `RecipeVersion` - Version history item
- `PublishedRecipe` - Publish operation result
- `RecipeListResponse` - List response wrapper
- `RecipeHistoryResponse` - History response wrapper

### Usage in Handlers

Import in your handler code:

\`\`\`go
import (
    "github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func handleCreateRecipe(w http.ResponseWriter, r *http.Request) {
    var req openapi.CreateRecipeRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // handle error
    }

    // req.Name, req.Content, etc. are type-safe
}
\`\`\`

## Regeneration

Regenerate types after updating the OpenAPI spec:

1. Update `/src/api/openapi/colony2-api.yaml`
2. Run `moon run be-openapi:build`
3. Verify compilation: `go build ./...`
4. Commit generated files
```

## Automation

### Add to CI/CD

Ensure code generation is part of the build process:

**File:** `/src/server/openapi/moon.yml` (if exists)

Add build task that runs generation:

```yaml
tasks:
  build:
    command: |
      oapi-codegen -config codegen.yml ../../api/openapi/colony2-api.yaml > pkg/openapi/types.gen.go
      go build ./...
```

### Pre-commit Hook (Optional)

Ensure generated code is always up to date:

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Check if OpenAPI spec changed
if git diff --cached --name-only | grep -q "api/openapi/colony2-api.yaml"; then
  echo "OpenAPI spec changed, regenerating code..."
  cd server/openapi
  moon run be-openapi:build
  git add pkg/openapi/types.gen.go
fi
```

## Testing

### Verify Type Safety

Create a test to ensure types match expectations:

**File:** `/src/server/openapi/pkg/openapi/types_test.go` (new)

```go
package openapi

import (
    "encoding/json"
    "testing"
)

func TestRecipeInfoSerialization(t *testing.T) {
    info := RecipeInfo{
        Name:          "test/recipe",
        LatestCommit:  "abc123",
        LatestCommitAt: time.Now(),
    }

    // Should serialize to JSON
    data, err := json.Marshal(info)
    if err != nil {
        t.Fatalf("Failed to marshal: %v", err)
    }

    // Should deserialize from JSON
    var decoded RecipeInfo
    if err := json.Unmarshal(data, &decoded); err != nil {
        t.Fatalf("Failed to unmarshal: %v", err)
    }

    if decoded.Name != info.Name {
        t.Errorf("Expected name %s, got %s", info.Name, decoded.Name)
    }
}

func TestRecipeRequestTypes(t *testing.T) {
    // Verify required fields are present
    req := CreateRecipeRequest{
        Name:    "test",
        Content: "yaml content",
    }

    // Optional fields should be pointers
    var autoPublish bool = true
    req.AutoPublish = &autoPublish

    data, err := json.Marshal(req)
    if err != nil {
        t.Fatalf("Failed to marshal: %v", err)
    }

    var decoded CreateRecipeRequest
    if err := json.Unmarshal(data, &decoded); err != nil {
        t.Fatalf("Failed to unmarshal: %v", err)
    }
}
```

Run tests:

```bash
cd /src/server/openapi
go test ./...
```

## Troubleshooting

### Issue: Types not generated

**Solution:**
- Verify OpenAPI spec is valid: `swagger-cli validate ../../api/openapi/colony2-api.yaml`
- Check `codegen.yml` configuration
- Ensure `oapi-codegen` is installed: `go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest`

### Issue: Compilation errors

**Solution:**
- Check for syntax errors in OpenAPI spec
- Verify all references (`$ref`) resolve correctly
- Ensure enum values are valid

### Issue: Types don't match expectations

**Solution:**
- Review OpenAPI schema definitions
- Check nullable vs required fields
- Verify type mappings (string, integer, boolean, etc.)

## Success Criteria

- [ ] OpenAPI spec includes all recipe schemas
- [ ] Code generation runs without errors
- [ ] All recipe types generated correctly
- [ ] Generated code compiles without errors
- [ ] Types are importable from `be-api`
- [ ] Documentation updated
- [ ] Tests pass
- [ ] CI/CD includes generation step

## Next Steps

After completing this specification:
1. Proceed to `03_client_code_generation__fe-openapi.md` - Generate TypeScript client
2. Then `04_recipe_service_integration__be-api.md` - Use generated types in handlers
