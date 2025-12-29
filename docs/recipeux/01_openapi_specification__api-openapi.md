# OpenAPI Specification for Recipe Service

**Target Cell:** `api-openapi` (`api/openapi`)
**Dependencies:** None (this is the first step)
**Status:** Not Started

## Overview

Define the complete OpenAPI specification for the recipe service API. This specification will be used to generate both server-side types/stubs and client-side SDKs, ensuring type safety and contract consistency between backend and frontend.

## Background

The OpenAPI spec is located at `/src/api/openapi/colony2-api.yaml`. It currently defines limited recipe endpoints. The recipe service from `/src/server/recipes` provides comprehensive lifecycle management (CRUD, publish/unpublish, versioning) that needs to be fully documented in the API contract.

## Goals

1. Define all request/response schemas for recipe operations
2. Document all 8 recipe service endpoints with full specifications
3. Include validation rules, examples, and error responses
4. Enable code generation for both server (Go) and client (TypeScript)
5. Maintain consistency with existing API patterns

## Schema Definitions

**File:** `/src/api/openapi/colony2-api.yaml`

**Location:** Add to `components.schemas` section

### RecipeInfo

List response item with summary information:

```yaml
RecipeInfo:
  type: object
  description: Summary information about a recipe
  required:
    - name
    - latestCommit
    - latestCommitAt
  properties:
    name:
      type: string
      description: Hierarchical recipe name (e.g., "workflows/ci/build")
      example: "workflows/ci/build"
      pattern: ^[a-zA-Z0-9_\-/]+$
    latestCommit:
      type: string
      description: Git commit hash of latest version
      example: "a1b2c3d4e5f6789012345678"
    latestCommitAt:
      type: string
      format: date-time
      description: Timestamp of latest commit
      example: "2025-01-15T10:30:00Z"
    publishedCommit:
      type: string
      nullable: true
      description: Git commit hash of published version
      example: "a1b2c3d4e5f6789012345678"
    publishedAt:
      type: string
      format: date-time
      nullable: true
      description: Timestamp when published
      example: "2025-01-15T10:30:00Z"
    publishedBy:
      type: string
      nullable: true
      description: User who published the recipe
      example: "user@example.com"
```

### RecipeWithContent

Get response with full recipe content:

```yaml
RecipeWithContent:
  type: object
  description: Recipe with full content and metadata
  required:
    - name
    - commitHash
    - isPublished
    - content
    - rawYaml
  properties:
    name:
      type: string
      description: Recipe name
      example: "workflows/ci/build"
    commitHash:
      type: string
      description: Git commit hash of this version
      example: "a1b2c3d4e5f6789012345678"
    isPublished:
      type: boolean
      description: Whether this version is published
      example: true
    publishedAt:
      type: string
      format: date-time
      nullable: true
      description: When this was published
      example: "2025-01-15T10:30:00Z"
    publishedBy:
      type: string
      nullable: true
      description: Who published this version
      example: "user@example.com"
    content:
      type: object
      description: Parsed recipe object
      additionalProperties: true
      example:
        version: "1.0"
        id: "workflows/ci/build"
        op: "echo"
        inputs:
          message: "Building..."
    rawYaml:
      type: string
      description: Raw YAML content
      example: |
        version: "1.0"
        id: workflows/ci/build
        op: echo
        inputs:
          message: "Building..."
```

### RecipeVersion

Version history item:

```yaml
RecipeVersion:
  type: object
  description: Version information from recipe history
  required:
    - commitHash
    - shortHash
    - createdAt
    - author
    - message
    - isPublished
  properties:
    commitHash:
      type: string
      description: Full git commit hash
      example: "a1b2c3d4e5f6789012345678"
    shortHash:
      type: string
      description: Short git commit hash
      example: "a1b2c3d"
    createdAt:
      type: string
      format: date-time
      description: When this version was created
      example: "2025-01-15T10:30:00Z"
    author:
      type: string
      description: Commit author
      example: "system"
    message:
      type: string
      description: Commit message
      example: "Update build message"
    isPublished:
      type: boolean
      description: Whether this version is currently published
      example: false
```

### CreateRecipeRequest

```yaml
CreateRecipeRequest:
  type: object
  description: Request to create a new recipe
  required:
    - name
    - content
  properties:
    name:
      type: string
      description: Hierarchical recipe name
      example: "workflows/ci/build"
      pattern: ^[a-zA-Z0-9_\-/]+$
      minLength: 1
      maxLength: 200
    content:
      type: string
      description: Recipe YAML content
      example: |
        version: "1.0"
        id: workflows/ci/build
        op: echo
        inputs:
          message: "Building..."
    description:
      type: string
      description: Recipe description
      example: "CI build workflow"
      maxLength: 500
    autoPublish:
      type: boolean
      default: false
      description: Automatically publish after creation
```

### UpdateRecipeRequest

```yaml
UpdateRecipeRequest:
  type: object
  description: Request to update an existing recipe
  required:
    - content
  properties:
    content:
      type: string
      description: Updated recipe YAML content
      example: |
        version: "1.1"
        id: workflows/ci/build
        op: echo
        inputs:
          message: "Building v2..."
    message:
      type: string
      description: Commit message
      example: "Update build message"
      maxLength: 500
    autoPublish:
      type: boolean
      default: false
      description: Automatically publish after update
    expectedCommit:
      type: string
      description: Expected current commit hash (optimistic locking)
      example: "a1b2c3d4e5f6789012345678"
```

### PublishRecipeRequest

```yaml
PublishRecipeRequest:
  type: object
  description: Request to publish a recipe version
  properties:
    commitHash:
      type: string
      description: Commit to publish (empty = latest)
      example: "a1b2c3d4e5f6789012345678"
    publishedBy:
      type: string
      description: User who is publishing
      example: "user@example.com"
      maxLength: 200
    expectedCommit:
      type: string
      description: Expected current published commit (optimistic locking)
      example: "old-commit-hash"
```

### PublishedRecipe

```yaml
PublishedRecipe:
  type: object
  description: Response after publishing a recipe
  required:
    - name
    - publishedCommit
    - publishedAt
  properties:
    name:
      type: string
      description: Recipe name
      example: "workflows/ci/build"
    publishedCommit:
      type: string
      description: Commit that was published
      example: "a1b2c3d4e5f6789012345678"
    publishedAt:
      type: string
      format: date-time
      description: When it was published
      example: "2025-01-15T10:30:00Z"
    publishedBy:
      type: string
      nullable: true
      description: Who published it
      example: "user@example.com"
```

### RecipeListResponse

```yaml
RecipeListResponse:
  type: object
  description: Response containing list of recipes
  required:
    - recipes
  properties:
    recipes:
      type: array
      items:
        $ref: '#/components/schemas/RecipeInfo'
```

### RecipeHistoryResponse

```yaml
RecipeHistoryResponse:
  type: object
  description: Response containing recipe version history
  required:
    - versions
  properties:
    versions:
      type: array
      items:
        $ref: '#/components/schemas/RecipeVersion'
```

## Endpoint Definitions

**File:** `/src/api/openapi/colony2-api.yaml`

**Location:** Update `paths` section

### 1. List Recipes

```yaml
/projects/{projectId}/recipes:
  get:
    summary: List recipes
    description: List all recipes in a project with optional status filtering
    operationId: listRecipes
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
        example: "proj_123"
      - name: status
        in: query
        required: false
        schema:
          type: string
          enum: [all, published, unpublished]
          default: all
        description: Filter by publish status
    responses:
      '200':
        description: List of recipes
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RecipeListResponse'
      '400':
        description: Bad request
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

### 2. Create Recipe

```yaml
  post:
    summary: Create recipe
    description: Create a new recipe (optionally auto-publish)
    operationId: createRecipe
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/CreateRecipeRequest'
    responses:
      '201':
        description: Recipe created
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RecipeVersion'
      '400':
        description: Invalid request (validation failed)
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '409':
        description: Recipe already exists
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

### 3. Get Recipe

```yaml
/projects/{projectId}/recipes/{recipeName}:
  get:
    summary: Get recipe
    description: Get a recipe with content (published version or specific ref)
    operationId: getRecipe
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
      - name: recipeName
        in: path
        required: true
        schema:
          type: string
        description: Recipe name (can include slashes, URL-encoded)
        example: "workflows%2Fci%2Fbuild"
      - name: ref
        in: query
        required: false
        schema:
          type: string
        description: Git reference (commit, tag, branch). Empty = published version
        example: "v1.0.0"
    responses:
      '200':
        description: Recipe details
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RecipeWithContent'
      '404':
        description: Recipe not found
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '409':
        description: Recipe exists but not published (when ref is empty)
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

### 4. Update Recipe

```yaml
  put:
    summary: Update recipe
    description: Update an existing recipe (create new version)
    operationId: updateRecipe
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
      - name: recipeName
        in: path
        required: true
        schema:
          type: string
        description: Recipe name (URL-encoded if contains slashes)
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/UpdateRecipeRequest'
    responses:
      '200':
        description: Recipe updated
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RecipeVersion'
      '400':
        description: Invalid request
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '404':
        description: Recipe not found
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '409':
        description: Version conflict
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

### 5. Delete Recipe

```yaml
  delete:
    summary: Delete recipe
    description: Delete a recipe and all its history
    operationId: deleteRecipe
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
      - name: recipeName
        in: path
        required: true
        schema:
          type: string
        description: Recipe name (URL-encoded if contains slashes)
    responses:
      '204':
        description: Recipe deleted
      '404':
        description: Recipe not found
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

### 6. Publish Recipe

```yaml
/projects/{projectId}/recipes/{recipeName}/publish:
  post:
    summary: Publish recipe
    description: Mark a specific version as published
    operationId: publishRecipe
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
      - name: recipeName
        in: path
        required: true
        schema:
          type: string
        description: Recipe name (URL-encoded if contains slashes)
    requestBody:
      required: false
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/PublishRecipeRequest'
    responses:
      '200':
        description: Recipe published
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PublishedRecipe'
      '404':
        description: Recipe or commit not found
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '409':
        description: Version conflict
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

### 7. Unpublish Recipe

```yaml
/projects/{projectId}/recipes/{recipeName}/unpublish:
  post:
    summary: Unpublish recipe
    description: Remove published status from a recipe
    operationId: unpublishRecipe
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
      - name: recipeName
        in: path
        required: true
        schema:
          type: string
        description: Recipe name (URL-encoded if contains slashes)
    responses:
      '204':
        description: Recipe unpublished
      '404':
        description: Recipe not found or not published
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

### 8. Get Recipe History

```yaml
/projects/{projectId}/recipes/{recipeName}/history:
  get:
    summary: Get recipe history
    description: Get version history for a recipe
    operationId: getRecipeHistory
    tags:
      - Recipes
    parameters:
      - name: projectId
        in: path
        required: true
        schema:
          type: string
        description: Project ID
      - name: recipeName
        in: path
        required: true
        schema:
          type: string
        description: Recipe name (URL-encoded if contains slashes)
    responses:
      '200':
        description: Recipe version history
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RecipeHistoryResponse'
      '404':
        description: Recipe not found
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
      '500':
        description: Internal server error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
```

## Code Generation

### Validate Specification

```bash
cd /src/api/openapi

# Install validator if needed
npm install -g @apidevtools/swagger-cli

# Validate spec
swagger-cli validate colony2-api.yaml
```

### Generate Go Server Types

If using OpenAPI code generation for Go:

```bash
# Install oapi-codegen
go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest

# Generate Go types and server stubs
oapi-codegen -package openapi -generate types,server \
  -o /src/server/api/internal/openapi/generated.go \
  /src/api/openapi/colony2-api.yaml
```

### Generate TypeScript Client

```bash
cd /src/web/openapi

# Generate TypeScript client (update package.json script if needed)
npm run generate

# Or manually:
npx @openapitools/openapi-generator-cli generate \
  -i ../../api/openapi/colony2-api.yaml \
  -g typescript-fetch \
  -o src/generated \
  --additional-properties=supportsES6=true,npmName=@colony2/openapi-client
```

## Testing

### Validation Tests

1. **Spec validates:** `swagger-cli validate colony2-api.yaml` passes
2. **No linting errors:** OpenAPI linter passes
3. **Examples valid:** All example payloads validate against schemas

### Generation Tests

1. **Go types generate:** No compilation errors
2. **TypeScript client generates:** No TypeScript errors
3. **Type safety:** Request/response types match between client and server

## Documentation

Update `/src/api/openapi/README.md` with recipe endpoints section:

```markdown
### Recipe Service Endpoints

The recipe service provides git-backed recipe management:

#### Endpoints

- `GET /projects/{projectId}/recipes?status=all|published|unpublished` - List recipes
- `POST /projects/{projectId}/recipes` - Create recipe
- `GET /projects/{projectId}/recipes/{recipeName}?ref={ref}` - Get recipe
- `PUT /projects/{projectId}/recipes/{recipeName}` - Update recipe
- `DELETE /projects/{projectId}/recipes/{recipeName}` - Delete recipe
- `POST /projects/{projectId}/recipes/{recipeName}/publish` - Publish recipe
- `POST /projects/{projectId}/recipes/{recipeName}/unpublish` - Unpublish recipe
- `GET /projects/{projectId}/recipes/{recipeName}/history` - Get history

#### Recipe Naming

- Pattern: `[a-zA-Z0-9_\-/]+`
- Examples: `build`, `workflows/ci/build`, `data/etl/load`
- URL encode slashes in path parameters: `workflows%2Fci%2Fbuild`

#### Version References

Access specific versions via `ref` query parameter:
- Published: (no ref) - `GET /recipes/build`
- Commit: `GET /recipes/build?ref=a1b2c3d4`
- Tag: `GET /recipes/build?ref=v1.0.0`
- Branch: `GET /recipes/build?ref=main`
```

## Success Criteria

- [ ] All 8 schemas defined in `components.schemas`
- [ ] All 8 endpoints documented in `paths`
- [ ] Request/response examples provided
- [ ] Error responses documented (400, 404, 409, 500)
- [ ] Validation rules specified (pattern, minLength, maxLength)
- [ ] OpenAPI spec validates without errors
- [ ] Go types generation works
- [ ] TypeScript client generation works
- [ ] README documentation updated

## Next Steps

After completing this specification:

1. **Generate code for both sides:**
   - Run Go type generation for server
   - Run TypeScript client generation for web

2. **Proceed to implementation:**
   - `02_recipe_service_integration__be-api.md` - Implement handlers using generated types
   - `03_web_recipe_navigation__web-app.md` - Build UI using generated client
   - `04_web_recipe_editing__web-app.md` - Build editor using generated client

The generated types ensure type safety and contract adherence across the stack.
