# Recipes Endpoint Plan (server/api)

Purpose: add HTTP endpoints that expose available recipes for a specific project so clients can list project-scoped recipes and fetch a specific recipe’s content. Recipes live under each project’s git repo at `.vibethis/recipes`.

## Step 1 — Extend OpenAPI in `/src/api/openapi`
- Add a new tag `Recipes`.
- Paths to introduce (project-scoped):
  - `GET /api/projects/{projectId}/recipes` → returns an array of recipe summaries for that project.
    - Query params: `ids` (string, repeatable), `nameContains` (string substring match on recipe ID), `types` (enum filter).
  - `GET /api/projects/{projectId}/recipes/{recipeId}` → returns details for a single recipe in that project; `404` if missing.
- Schemas / objects:
  - `RecipeType` enum: `state`, `sequence`, `op`.
  - `RecipeSummary`: `{ id: string, version: string, description: string, type: RecipeType, hash: string, relativePath: string, lastModified: date-time }`.
  - `RecipeDetail`: `{ meta: RecipeSummary, rawYaml: string, recipe: object }` where `recipe` is the parsed recipe structure rendered as JSON (`additionalProperties: true`).
  - Errors: reuse existing `ErrorResponse` schema; if absent, inline `{ message: string }`.
- Responses:
  - List → `200` `[RecipeSummary]`.
  - Get → `200` `RecipeDetail`; `404` on unknown recipe.
- Rationale: keep responses minimal but sufficient for UIs to render catalog + fetch full content; avoid leaking absolute paths by using a relative path from the registry root. Recipes are resolved under `<project.gitRepoPath>/.vibethis/recipes`.

## Step 2 — Regenerate Go bindings in `/src/server/openapi`
- Run the existing generator (e.g., `moon run server-openapi:generate` or the go:generate wrapper) to refresh `server/openapi/pkg/openapi/generated.go` with the new `Recipes` tag, routes, and types.
- Validate the generated types include request/response structs for list/get recipes and the new schemas.

## Step 3 — Add handlers in `/src/server/api`
- Add the new routes to `registerAPIRoutes` under `/api/projects/{projectId}/recipes` and `/api/projects/{projectId}/recipes/{recipeId}`.
- Extend handler dependencies to accept a project-scoped recipe registry provider; likely a factory that builds/returns a registry pointing at `<project.gitRepoPath>/.vibethis/recipes` (fall back to default path when git repo path is empty). Interface should expose at least:
  - `ListRecipes() []*recipe.RecipeFile`
  - `GetRecipeFile(name string) (*recipe.RecipeFile, error)`
- Ensure the registry is initialized per project (cached) and points to the project’s repo path.
- List handler behavior:
  - Resolve project → git repo path → registry rooted at `.vibethis/recipes`.
  - Fetch from `registry.ListRecipes()`, apply filters (`ids`, `nameContains`, `types`), sort deterministically (e.g., by `id`), and map to `RecipeSummary` (fill `relativePath`, `lastModified`).
- Get handler behavior:
  - Use `registry.GetRecipeFile(id)` in the project’s registry; return `404` when missing.
  - Build `RecipeDetail` with metadata, `rawYaml` read from the recipe file, and a JSON rendering of the parsed recipe struct.
- Tests: create temporary project repo directory with `.vibethis/recipes` sample YAMLs, assert list and get return expected shapes/status codes using the generated OpenAPI types.
