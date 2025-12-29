# Recipe UX Implementation Specification

This directory contains the specification for integrating the git-backed recipe service into the Colony2 testserver and web UI using an **OpenAPI-first approach**.

## Overview

The recipe service provides version-controlled recipe management with publish/unpublish lifecycle. This specification follows best practices by:

1. **Defining the API contract first** (OpenAPI specification)
2. **Generating code for both server and client** from the specification
3. **Implementing handlers and UI** using generated types for type safety

## Implementation Order

The specifications **must** be completed in order, as each depends on the previous:

| # | Document | Target Cell | Description |
|---|----------|-------------|-------------|
| 01 | [openapi_specification__api-openapi.md](./01_openapi_specification__api-openapi.md) | `api-openapi` | Define API contract (schemas + endpoints) |
| 02 | [server_code_generation__be-openapi.md](./02_server_code_generation__be-openapi.md) | `be-openapi` | Generate Go types/models from OpenAPI spec |
| 03 | [client_code_generation__fe-openapi.md](./03_client_code_generation__fe-openapi.md) | `fe-openapi` | Generate TypeScript client from OpenAPI spec |
| 04 | [recipe_service_integration__be-api.md](./04_recipe_service_integration__be-api.md) | `be-api` | Integrate recipe service and implement handlers using generated types |
| 05 | [web_recipe_navigation__web-app.md](./05_web_recipe_navigation__web-app.md) | `web-app` | Build recipe list UI using generated client |
| 06 | [web_recipe_editing__web-app.md](./06_web_recipe_editing__web-app.md) | `web-app` | Build recipe editor using generated client |

## OpenAPI-First Benefits

### Why OpenAPI First?

1. **Single Source of Truth:** API contract defined once, used everywhere
2. **Type Safety:** Generated types prevent runtime errors
3. **Automatic Documentation:** OpenAPI spec is self-documenting
4. **Client/Server Sync:** Both sides always match the contract
5. **Faster Development:** No manual type definitions needed
6. **Refactoring Safety:** Contract changes propagate automatically

### Code Generation Flow

```
┌─────────────────────────────────────┐
│  01: OpenAPI Specification          │
│      (api-openapi)                  │
│      colony2-api.yaml               │
└────────────────┬────────────────────┘
                 │
                 ├──────────────────────────────┐
                 │                              │
                 ▼                              ▼
  ┌──────────────────────────┐   ┌──────────────────────────┐
  │  02: Generate Go Types   │   │  03: Generate TS Client  │
  │      (be-openapi)        │   │      (fe-openapi)        │
  │      types.gen.go        │   │      @colony2/client     │
  └────────────┬─────────────┘   └─────────┬────────────────┘
               │                           │
               ▼                           ▼
  ┌──────────────────────────┐   ┌──────────────────────────┐
  │  04: Handler Impls       │   │  05-06: React UI         │
  │      (be-api)            │   │         (web-app)        │
  │      Uses generated      │   │         Uses generated   │
  │      Go types            │   │         TS client        │
  └──────────────────────────┘   └──────────────────────────┘
```

### Type Safety Example

**OpenAPI defines:**
```yaml
RecipeInfo:
  properties:
    name:
      type: string
    publishedCommit:
      type: string
      nullable: true
```

**Server gets:**
```go
type RecipeInfo struct {
    Name            string  `json:"name"`
    PublishedCommit *string `json:"publishedCommit,omitempty"`
}
```

**Client gets:**
```typescript
interface RecipeInfo {
    name: string;
    publishedCommit: string | null;
}
```

**Both sides are guaranteed to match!**

## Key Features

### Code Generation (02-03)
- Go types generated from OpenAPI spec (`be-openapi`)
- TypeScript client generated from OpenAPI spec (`fe-openapi`)
- Both use same source of truth (OpenAPI spec)
- Type safety guaranteed across stack

### Recipe Service Integration (04)
- Recipe service as first provider in chain (before registry and embedded)
- Backward compatibility with existing file-based recipes
- Support for versioned recipe references (`recipe@commit`, `recipe@tag`)
- Handlers use generated types for request/response

### API Endpoints (01, 04)
- List recipes (with filtering)
- Get recipe (published or specific ref)
- Create recipe
- Update recipe
- Delete recipe
- Publish recipe
- Unpublish recipe
- Get recipe history

### Web UI (05-06)
- Lefthand navigation item for "Recipes"
- Folder tree view for hierarchical recipe organization
- Recipe editor with YAML syntax highlighting (Monaco)
- Version history viewing and restoration
- Publish/unpublish controls
- Create/update/delete operations
- **All using type-safe generated client**

## Architecture

### Provider Chain

After implementation, recipe lookups will follow this order:

1. **Recipe Service** - Git-backed recipes with version control (NEW)
2. **Registry** - File-based recipes from `{nodesPath}/recipes` (backward compatibility)
3. **Embedded** - Built-in `internal://new_ticket` recipe

### Recipe Storage

- **Database:** Lightweight index mapping `(ProjectID, Name) → CommitHash`
- **Git:** Source of truth at `.c2/recipes/` with full history
- **Workspaces:** Isolated per-project git workspaces

### Recipe References

Recipes can be referenced as:
- `workflows/ci/build` - Published version
- `workflows/ci/build@v1.0.0` - Specific tag
- `workflows/ci/build@a1b2c3d` - Specific commit
- `workflows/ci/build@main` - Branch

## Testing Strategy

Each specification includes:
- Unit tests for new components
- Integration tests for end-to-end flows
- Manual testing checklists
- Error handling scenarios

## Success Metrics

The implementation is complete when:
- [ ] OpenAPI spec validates and generates code
- [ ] Recipe service integrated as primary provider
- [ ] All 8 API endpoints implemented using generated types
- [ ] Web UI uses generated client exclusively
- [ ] Type checking passes in both Go and TypeScript
- [ ] Recipe list shows folder tree structure
- [ ] Recipe editor supports create/edit/delete/publish
- [ ] Version history viewable and restorable
- [ ] Existing workflows continue to work (backward compatibility)
- [ ] New recipes can be created and used in workflows

## Dependencies

### Backend
- `github.com/colony-2/colony2/server/recipes` - Recipe service
- `gorm.io/gorm` - Database access
- Existing git repository integration
- Generated Go types from OpenAPI

### Frontend
- `@colony2/openapi-client` - **Generated API client**
- `@monaco-editor/react` - YAML editor
- `js-yaml` - YAML parsing and validation
- `antd` - UI components
- Generated TypeScript types from OpenAPI

## Related Documentation

- [Recipe Service README](../../server/recipes/README.md) - Service usage and API
- [Recipe Service Spec](../../server/recipes/RECIPE_SERVICE_SPEC.md) - Detailed design
- [Git Commands Spec](../../server/recipes/GIT_COMMANDS_SPEC.md) - Git operations

## Questions & Support

For questions or issues during implementation:
1. Review the specific spec document for the task
2. Check the recipe service documentation
3. Review existing similar implementations (e.g., workflow endpoints)
4. Verify OpenAPI spec matches implementation
5. Consult with team leads

## Timeline Estimate

Based on complexity and dependencies:

- 01_openapi_specification: 1-2 days (define contract, validation)
- 02_server_code_generation: 0.5 day (run generation, verify output)
- 03_client_code_generation: 0.5 day (run generation, build package)
- 04_recipe_service_integration: 2-3 days (service integration + handlers)
- 05_web_recipe_navigation: 1-2 days (list UI with generated client)
- 06_web_recipe_editing: 2-3 days (editor UI + version management)

**Total: ~8-12 days** (including testing and iteration)

Note: Timeline is shorter than a non-OpenAPI approach because generated code eliminates manual type definitions and reduces integration errors.

## Development Workflow

### Step 1: Define API Contract
```bash
# Edit OpenAPI spec
vim /src/api/openapi/colony2-api.yaml

# Validate
swagger-cli validate colony2-api.yaml
```

### Step 2A: Generate Server Code
```bash
cd /src/server/openapi
moon run be-openapi:build
# Generates types.gen.go with Go types
```

### Step 2B: Generate Client Code
```bash
cd /src/web/openapi
moon run fe-openapi:generate
moon run fe-openapi:build
# Generates TypeScript client package
```

### Step 3: Implement Server
```bash
# Implement handlers using generated types
cd /src/server/api
# Handlers use generated request/response types
```

### Step 4: Implement UI
```bash
# Build UI using generated client
cd /src/web/app
# Components use generated service and types
```

### Step 5: Test End-to-End
- API contract ensures compatibility
- Type errors caught at compile time
- Runtime errors minimized

## Notes

- Each cell is updated independently (moon projects)
- Maintain backward compatibility throughout
- Test after each specification completion
- UI should match existing Colony2 design patterns
- Security: Recipe content is validated before publish
- Performance: Recipe lookups are cached in provider chain
- **Type safety is enforced throughout the stack**
