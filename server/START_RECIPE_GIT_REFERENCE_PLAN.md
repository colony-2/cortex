# Start Recipe Git Reference Plan

## Purpose

This plan covers the core recipe start path centered on `recipe-core/pkg/starter/start_recipe.go`, not the recipe testing API.

The goal is to stop embedding full recipe YAML in the job start payload/artifacts and instead carry a recipe reference using the same git-go-style shape already used by Codex skill refs:

`<host>/<org>/<repo>/<recipe-path>@<git-ref>`

Example:

`github.com/acme/platform-recipes/.colony2/recipes/new-ticket.recipe.yaml@main`

## Compatibility Stance

No backward-compatibility work is required for embedded root recipe YAML.

This plan assumes:

1. the new design applies to new jobs only,
2. the new code path does not need legacy fallback behavior,
3. supporting old embedded-YAML root jobs is out of scope.

## Current Flow

Today the top-level recipe content hop looks like this:

1. A caller decides which recipe to run.
2. The caller resolves that recipe to an in-memory `recipe.Recipe`.
3. `starter.StartRecipeJobWithOptions(...)` serializes the full recipe to YAML.
4. The starter attaches that YAML as a job-start artifact named `<recipe-id>.recipe.yaml`.
5. The recipe worker bootstraps the root recipe by reading that artifact back out of job start data.
6. Workflow replay and story surfaces assume the root recipe source is `jobStartArtifact`.

This means the effective recipe submission into the engine currently includes the full recipe content.

## Current Code Path

Primary path:

1. `workflow/internal/service/service.go`
2. `ticket/internal/service/service.go`
3. `recipe-child/pkg/recipe/launcher.go`
4. `recipe-worker/pkg/workflow/control.go`
5. `recipe-core/pkg/starter/start_recipe.go`
6. `recipe-worker/pkg/compiler/job_worker.go`
7. `recipe-worker/pkg/compiler/recipe_retriever.go`

Observability and replay surfaces that also depend on the current artifact model:

1. `workflow/internal/story/builder.go`
2. `workflow/internal/story/replay_recorder.go`
3. `workflow/internal/model/job_run_story.go`

## Main Architectural Constraint

The current starter is not the only component that assumes the recipe is embedded.

If `start_recipe.go` stops attaching YAML, the recipe worker loses its only source for the root recipe unless worker startup gains a resolver path for the root recipe reference.

This is the core downstream change that must be planned first.

## Recommended Direction

Introduce a first-class recipe reference through the start pipeline and migrate root recipe loading to resolve that reference at runtime.

Recommended steady-state model:

1. The start payload carries `recipe_ref`.
2. The start payload does not carry the root recipe YAML as a normal job-start artifact.
3. The worker resolves the root recipe from `recipe_ref`.
4. Replay/story surfaces report the recipe source as a reference, not as a start artifact.

## Important Design Decision

The system should not try to infer the git-go-style recipe reference only from the current `recipe_name` and `git_ref` fields.

Reason:

1. The current start surfaces do not consistently carry a canonical `<host>/<org>/<repo>` identity.
2. `project.GitRepoPath` and related fields are not guaranteed to already be in the target ref format.
3. Some stored repo values are path-like or SCP-like rather than the Codex-skill-style canonical shape.

Recommended outcome:

1. Add an explicit `recipe_ref` field to the internal start model.
2. Keep `recipe_name` separately for display, filtering, and compatibility where needed.

## Proposed Internal Contract Changes

### `workflowctl.StartJob`

Add:

1. `RecipeRef string`

Keep for now:

1. `RecipeName string`

Purpose split:

1. `RecipeRef` becomes the authoritative execution source.
2. `RecipeName` remains human-friendly metadata and a compatibility field for existing consumers.

### Starter metadata

Extend `starter.JobMetadata` with:

1. `recipe_ref`

Keep existing:

1. `recipe`
2. `git_ref`
3. actor/cell/ticket fields

## Starter Changes

Current behavior in `start_recipe.go`:

1. accepts `recipes ...recipe.Recipe`
2. marshals them to YAML
3. attaches `*.recipe.yaml` artifacts

Recommended migration behavior:

1. Add reference-first start support.
2. Stop requiring a fully materialized root recipe for production starts.
3. Remove root recipe YAML packaging from the start path.

## Worker Bootstrap Changes

Current root recipe bootstrap:

1. `job_worker.go` reads job artifacts.
2. `recipe_retriever.go` finds `<recipe-name>.recipe.yaml`.
3. the worker unmarshals YAML and uses that as the root recipe.

Recommended replacement:

1. Add a root recipe resolver to `RecipeJobWorkerOptions`.
2. Resolve the root recipe from `workflowctl.StartJob.RecipeRef` when present.
3. Fail immediately when `RecipeRef` is missing or cannot be resolved.

More explicit recipe-worker changes:

1. `recipe-worker/pkg/compiler/job_worker.go`
   - stop treating the start artifact list as the authoritative root recipe source
   - resolve the root recipe from `StartJob.RecipeRef`
2. `recipe-worker/pkg/compiler/recipe_retriever.go`
   - remove or repurpose the root-artifact lookup logic
   - it should no longer be required for root recipe bootstrap
3. `recipe-worker/pkg/compiler/job_worker.go` and `compiler.NewRecipeWorker(...)`
   - plumb a root recipe resolver into `RecipeJobWorkerOptions`
4. `workflow/internal/story/builder.go`
   - pass the same root recipe resolver when constructing `NewRecipeJobWorker(...)` for replay/story building

## Resolver Shape

The current registry shape is already close:

`func(projectId string, recipeRef string) (*recipe.Recipe, error)`

But current implementations mostly treat `recipeRef` as simple `name` or `name@ref`.

Recommended change:

1. Widen resolver semantics so `recipeRef` may be either:
   - existing server recipe references, or
   - git-go-style recipe references
2. Move git-go-style parsing into a shared recipe-ref helper instead of duplicating Codex skill parsing logic.

A shared helper package is preferable so the recipe ref format and the Codex skill ref format do not drift.

## Start Callers That Must Change

### Workflow service

`workflow/internal/service/service.go`

Current behavior:

1. resolves recipe immediately via `s.recipes(projectID, recipeName)`
2. passes the full recipe to `starter.StartRecipeJobWithOptions(...)`

Planned behavior:

1. accept or construct `recipe_ref`
2. submit `recipe_ref` into the start path
3. stop requiring root recipe YAML packaging at this layer

### Ticket autostart

`ticket/internal/service/service.go`

Current behavior:

1. resolves the default ticket recipe to a `recipe.Recipe`
2. passes that to the starter

Planned behavior:

1. produce a canonical `recipe_ref` for the selected default recipe
2. submit that reference instead of embedded YAML

### Child recipe launches

`recipe-child/pkg/recipe/launcher.go`

Current behavior:

1. child starts carry `recipe.Name`
2. workflow control resolves the recipe and starter packages it as YAML

Planned behavior:

1. child starts should carry a first-class recipe reference field
2. nested recipe starts should use the same root-reference mechanism as top-level starts

This is important so the system does not fix only top-level starts while child starts continue embedding recipe content.

## Replay And Story Changes

Current story model assumes:

1. recipe source kind is `jobStartArtifact`
2. source artifact name is `<recipe-name>.recipe.yaml`

This is no longer correct for new jobs once root recipe YAML is removed.

Recommended story model update:

1. add a new source kind such as `jobStartRef`
2. include the resolved or submitted `recipe_ref`

Files likely affected:

1. `workflow/internal/story/builder.go`
2. `workflow/internal/story/replay_recorder.go`
3. `workflow/internal/model/job_run_story.go`

`BuildJobRunStory(...)` will also need access to the same root recipe resolver used by normal job execution, otherwise replay for new jobs cannot reconstruct the root recipe tree.

## API Surface Impact

The public API impact depends on where the canonical `recipe_ref` is introduced.

### Recommended approach

Make `recipe_ref` explicit at workflow-start boundaries where a new top-level job is created.

Likely affected:

1. `workflow/internal/model/StartWorkflowRequest`
2. `api/internal/handlers/workflows.go`
3. OpenAPI `StartWorkflowRequest`

Why this is recommended:

1. It avoids guessing a canonical git-go-style ref from incomplete repo metadata.
2. It keeps the execution source explicit and auditable.
3. It makes the starter contract match the user requirement directly.

## Shared Ref Format Rules

The new recipe reference should follow the same structural rules as the Codex skill ref format:

1. split on the last `@`
2. require `<host>/<org>/<repo>/<path>@<git-ref>`
3. reject empty path segments
4. reject absolute paths
5. reject path traversal
6. resolve symbolic refs to a concrete commit for traceability

Recipe-specific addition:

1. the path must resolve to a file, not a directory

## Migration Strategy

1. Add `recipe_ref` to the internal start contract.
2. Update top-level and child start callers to populate it.
3. Remove root recipe YAML emission from `start_recipe.go`.
4. Update recipe-worker bootstrap to resolve root recipes from `recipe_ref`.
5. Update replay/story to use the same resolver and report reference-based source metadata.

## Testing Plan

Implementation should add or update tests in these areas:

1. `recipe-core/pkg/starter`
   - metadata includes `recipe_ref`
   - new start path does not require embedded YAML for reference-based starts
2. `recipe-worker/pkg/compiler`
   - root recipe resolves from `recipe_ref`
   - missing `recipe_ref` fails clearly
3. `workflow/internal/service`
   - workflow start submits reference-based start payload
4. `ticket/internal/service`
   - ticket autostart submits reference-based start payload
5. `recipe-child/pkg/recipe`
   - child starts use the new reference field
6. `workflow/internal/story`
   - replay/story works for reference-based jobs
7. API/OpenAPI tests
   - only if `recipe_ref` is added to external workflow start requests

## Likely Files To Touch During Implementation

Core start path:

1. `recipe-core/pkg/starter/start_recipe.go`
2. `recipe-core/pkg/starter/start_recipe_test.go`

Worker bootstrap:

1. `recipe-worker/pkg/compiler/job_worker.go`
2. `recipe-worker/pkg/compiler/recipe_retriever.go`
3. `recipe-worker/pkg/compiler/*tests`

Start callers:

1. `workflow/internal/service/service.go`
2. `workflow/internal/model/types.go`
3. `ticket/internal/service/service.go`
4. `recipe-child/pkg/recipe/op.go`
5. `recipe-child/pkg/recipe/launcher.go`
6. `recipe-worker/pkg/workflow/control.go`

Replay and story:

1. `workflow/internal/story/builder.go`
2. `workflow/internal/story/replay_recorder.go`
3. `workflow/internal/model/job_run_story.go`

API and schema, if externalized:

1. `api/internal/handlers/workflows.go`
2. `api/internal/handlers/workflows_test.go`
3. OpenAPI schema and generated bindings

## Risks

1. Existing embedded-YAML jobs will not be supported by this new path.
2. Story/replay can become nondeterministic if replay-time recipe resolution is not pinned to a concrete commit.
3. External API changes may be required if the system cannot reliably derive canonical git-go-style refs from current project metadata.
4. Child recipe starts can become inconsistent if only the top-level path is migrated.

## Non-Goals

This plan does not cover:

1. recipe testing API changes
2. recipe CRUD storage changes in the recipe service
3. generic artifact system redesign
4. backward compatibility for pre-change embedded root recipe jobs

## Recommended Implementation Bias

1. add first-class `recipe_ref`
2. make all new starts reference-based
3. remove embedded root recipe content from the start path
4. require recipe-worker and replay code to resolve the root recipe from `recipe_ref`
