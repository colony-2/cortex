# Monolith Rehab Plan

This plan migrates `/src` from the old partially-decomponentized monolith into a
small application shell around `c2j` and JobDB:

- a Go REST API server for the React UI
- a Go UI-dev test server
- a React UI that develops and tests against the API server
- a production Go executable that embeds the built React assets and serves the
  same REST API

`/src/c2j_migration.md` is prior art for where the old monolith packages moved.
Use it as the import map and do not rebuild behavior that now lives in `/c2j`.

## Current State

The remaining `/src` code still contains old monolith assumptions:

- `server/api` depends on extracted old modules such as `server/cell`,
  `server/project`, `server/ticket`, `server/workflow`, `server/recipes`,
  `server/registry`, and `server/graph`.
- The API server still has swf-go, PGWF, and Strata setup code.
- Cell sync is still modeled as a Moon graph populator named `graph/moon`.
- Cells have old persisted IDs, working paths, dependency stores, and project
  ownership.
- Tickets are still a separate persisted service with events and workflow
  autostart.
- The UI is mostly still useful, but it expects project/cell/ticket/workflow
  endpoints shaped by the old services.
- Moon config remains in `moon.yml` files and generated OpenAPI instructions.

The important replacement model:

- `c2j` owns recipe parsing, recipe execution, standard ops, inputs, job
  starting/restarting, job story generation, current-cell config, and dependent
  cell discovery.
- JobDB owns durable job runtime APIs.
- Cells are identified by git repository URL strings. A short name is only a
  display or convenience alias resolved by c2j config.
- Dependent cells come from c2j config: an explicit list, a command, or
  auto-discovery such as Go module dependencies.

## Target Shape

Prefer a single Go application module for `/src`, plus a pnpm workspace for the
UI packages:

```text
/src
  go.mod
  go.work                 # optional local dev workspace with /c2j
  Makefile
  package.json
  pnpm-workspace.yaml
  cmd/
    api/                  # REST API only, no embedded UI
    uitestserver/          # UI-dev/test server
    monolith/              # production API + embedded React dist
  internal/
    api/                  # HTTP routes, DTOs, OpenAPI binding adapters
    app/                  # dependency wiring
    c2japp/               # thin c2j/jobdb composition layer
    webdist/              # copied React dist for go:embed
  api/
    openapi/
      colony2-api.yaml
  web/
    app/
    shared/
    flowchart/
    kanban/
    notebook/
    openapi/
```

If a root Go module is too disruptive for the first pass, keep `server/api` as
the only Go module temporarily. The final production executable still needs a
build step that copies `web/app/dist` under that module before `go:embed`.

## Non-Rebuild Rule

Before implementing or porting old code, check whether c2j already provides the
behavior. The migration should delete old monolith services when c2j or JobDB
already owns the concept.

Use these c2j packages directly:

- `github.com/colony-2/c2j/pkg/config` for current cell, root cell, dependent
  repos, short-name expansion, tenant ID derivation, and Go dependency
  auto-detection.
- `github.com/colony-2/c2j/pkg/starter` for recipe job submit/restart metadata
  and c2j recipe job shape.
- `github.com/colony-2/c2j/pkg/workflowctl` for runtime-agnostic job control
  interfaces.
- `github.com/colony-2/c2j/pkg/input` for user input HTTP routes, SSE, pending
  input details, submit, and cancel behavior.
- `github.com/colony-2/c2j/pkg/story` and `github.com/colony-2/c2j/pkg/story/live`
  for job detail/story/replay/restart/outcome behavior. Do not rebuild job
  stories in `/src`.
- `github.com/colony-2/c2j/pkg/recipetest` for recipe test validation and
  execution harnesses.
- `github.com/colony-2/c2j/pkg/recipe`, `pkg/template`, and
  `pkg/worker/compiler` for recipe loading, validation, source resolution, and
  worker construction.
- `github.com/colony-2/jobdb/pkg/jobdb` and `github.com/colony-2/jobdb/pkg/workflow`
  for JobDB runtime and engine APIs.
- `github.com/colony-2/jobdb/pkg/jobdb/runtime/remote`,
  `runtime/sqlite`, and `runtime/toy` for REST-backed, local durable, and test
  runtimes.

## API Model

Keep the UI API under `/api`. If this process hosts a JobDB REST runtime, mount
it separately, for example:

```text
/api/...          UI-specific API
/jobdb/v1/...     JobDB runtime API, via remote.NewServer with StripPrefix
/...              React SPA in the production executable
```

The UI API should be a c2j-oriented facade, not another workflow system.

### Projects

The old project service should not be rebuilt. Replace it with a lightweight
workspace context:

- `projectId` is the c2j tenant ID.
- The default project comes from `c2j/pkg/config.ProjectConfig.SelfTenantID`.
- `/api/projects` can return a single current project for compatibility.
- Long term, remove project management screens or make them edit `.c2j/config.yaml`
  only if that is really desired.

### Cells

Cells are git repository URL strings.

- `id` in transport responses should be the canonical repo URL string.
- Path parameters must URL-encode the full repo URL, or the API should prefer a
  `?cell=<repo-url>` query parameter for endpoints where path encoding is
  awkward.
- `name` is a display alias from c2j pattern matching when available.
- `repo` is required.
- `workingPath`, `populator`, `populatorId`, and persisted dependency IDs should
  be removed or treated as compatibility-only fields during UI migration.
- `/api/projects/{projectId}/cells` should call
  `config.LoadProjectConfig(...).AllowedDependentRepos(ctx)`.
- `/api/projects/{projectId}/cells/sync` should be removed or changed to
  "re-read c2j config/discovery"; it must not invoke Moon.
- `/api/projects/{projectId}/graph` can return a simple graph derived from c2j
  dependents. Do not rebuild the old graph service unless a real graph is needed.

### Tickets

Do not rebuild the old ticket database/service unless product requirements still
need first-class ticket storage outside JobDB.

The first target model:

- Creating a ticket submits a c2j recipe job to a target cell repo URL.
- The target cell is sent through `workflowctl.StartJob.JobContext.Workflow`
  and `GitBase`/`RecipeSource` fields, then submitted with `starter.StartRecipeJob`.
- The created ticket ID is the JobDB job ID.
- Ticket title/body/creator/stage/state should be stored as job input and/or
  JobDB metadata if the UI needs to list/filter it.
- Ticket events are derived from JobDB job state, chapters, input activity, and
  c2j story data.

Only add a small local ticket metadata table if JobDB metadata cannot support
the UI's required list filters. If added, keep it as UI metadata keyed by
`jobdb.JobKey`, not as a workflow authority.

### Workflows And Jobs

The UI can keep its "workflow" language, but server code should translate to
c2j recipe jobs:

- list workflows: `jobdb.ListJobs` filtered by tenant ID, job type
  `starter.RecipeJobType`, and c2j metadata fields
- start workflow: `starter.StartRecipeJob`
- get workflow/job: JobDB job read APIs plus c2j metadata
- outcome: JobDB result plus c2j failure normalization where available
- restart: `starter.RestartRecipeJob`
- artifact fetch: JobDB artifact APIs
- story: `story/live.BuildJobRunStory` or `story.Service.GetJobRunStory`

Do not port the old `server/workflow` module into `/src`.

### Recipes

Prefer c2j recipe source resolution instead of the old recipe registry/service:

- local file and inline recipe handling should use `pkg/recipe` and
  `pkg/worker/compiler.ResolveInlineRecipes`.
- git selector resolution should use `compiler.NewRecipeSourceResolver`.
- recipe validation should use c2j parser/validator and worker compiler
  semantics.
- recipe tests should use `pkg/recipetest`.

If the UI still needs recipe CRUD, decide whether recipes are files in a repo or
records in a local database. Do not resurrect the old `server/recipes` service
unless it remains the intended persistence model.

### User Inputs

Reuse c2j input management:

- initialize `input.NewSimpleSSEManager`
- build `ops.ServiceDependencies2` with `WorkflowControl` and SSE manager
- use the routes exposed by `input.GetOp()` / input management setup
- keep the existing React input UI and point it at the c2j-backed endpoints

### Job Stories

c2j already has job stories. Use:

- `pkg/story/live.BuildJobRunStory` when the API only needs a replayed story
- `pkg/story.New` when the API wants the broader service facade

Current inspection shows story model types and service methods are already
exported through `pkg/story` and `pkg/story/live`. No story export is needed
unless a future endpoint requires a symbol that remains under `pkg/story/internal`.
If that happens, export it from c2j instead of rebuilding it in `/src`.

## C2J Export Requests

The API shell should avoid importing `cmd/c2j/internal/*`. If server work needs
one of those internals, add a small public c2j package first.

| Need in `/src` | Current c2j location | Proposed c2j export | Priority |
| --- | --- | --- | --- |
| Open local/remote JobDB runtimes with the same c2j URL semantics and chapter visibility wrapper | `cmd/c2j/internal/swfruntime` | `pkg/runtimehost` or `pkg/jobruntime` with `Open`, `OpenWorker`, and `Handle` | High |
| Build and run a c2j worker loop against a JobDB runtime for local/test server use | `cmd/c2j/internal/workjob` | `pkg/workerhost` with worker build/run options | High |
| Register the exact standard c2j-safe op set without duplicating the list | `cmd/c2j/internal/c2jops` | `pkg/standardops` or `pkg/c2jops` with `Ops` and `Register` | Medium |
| Submit jobs with CLI-equivalent cell resolution, prompt/input/artifact loading, embedded recipe behavior, and result DTOs | `cmd/c2j/internal/submitjob` | `pkg/submit` if the API needs exact CLI semantics | Medium |
| List jobs with CLI-equivalent current-cell/explicit-cell filtering | `cmd/c2j/internal/listjobs` | `pkg/jobs` query helpers if needed | Low |
| Compile/run recipe test suites exactly like `c2j test` | `cmd/c2j/internal/testjob`; core is already `pkg/recipetest` | Prefer `pkg/recipetest`; export CLI suite orchestration only if the UI needs the same files/artifacts | Low |

The plan should not ask c2j to export job story internals at this point.

## Build System

Remove Moon entirely from active build/test flows.

### Go

Use ordinary Go commands:

```bash
go test ./...
go run ./cmd/api
go run ./cmd/uitestserver
go build -o bin/monolith ./cmd/monolith
```

Keep `go.work` only for local development, for example:

```text
use (
  .
  /c2j
)
```

Remove stale workspace entries for extracted modules that are no longer part of
this repo.

### pnpm

Create a root pnpm workspace:

```yaml
packages:
  - web/app
  - web/shared
  - web/flowchart
  - web/kanban
  - web/notebook
  - web/openapi
```

Root scripts should own UI workflows and API test-server startup:

```json
{
  "scripts": {
    "dev:api": "go run ./cmd/uitestserver --port 8080",
    "dev:web": "pnpm --filter @colony2/app dev",
    "build:web": "pnpm --filter @colony2/app build",
    "test:web": "pnpm -r --if-present test -- --run",
    "test:e2e": "pnpm --filter @colony2/app test:e2e",
    "openapi": "pnpm --filter @colony2/openapi-client build"
  }
}
```

Adjust as needed for pnpm filtering syntax and package scripts, but do not call
Moon.

### Make

Use Make only for the cross-language production executable:

```make
.PHONY: build
build:
	pnpm install --frozen-lockfile
	pnpm run build:web
	rm -rf internal/webdist/dist
	mkdir -p internal/webdist
	cp -R web/app/dist internal/webdist/dist
	go build -o bin/monolith ./cmd/monolith

.PHONY: test
test:
	go test ./...
	pnpm run test:web
	pnpm run test:e2e
```

Keep Make small. Go and pnpm remain the real build tools.

## React UI Migration

Keep the current UI components where they still match the new model:

- workflow/job list and detail
- job story page
- recipe editor/testing UI if recipe persistence is resolved
- input form rendering and SSE activity
- cell/dependent list views, after they are changed to repo-url cells

Required UI changes:

- Replace hard-coded `http://localhost:8080/api` with `VITE_API_BASE_URL`,
  defaulting to `/api` for production and `http://localhost:8080/api` in dev.
- Regenerate the OpenAPI client from the new c2j-oriented API spec.
- Treat cell IDs as repo URLs. Encode them for routes.
- Remove Moon sync controls or relabel them as c2j config/discovery refresh.
- Remove project admin flows that no longer have a backend authority.
- Change ticket creation forms to select/enter a target repo URL and submit a
  c2j recipe job.
- Update Playwright to start both the UI and API test server through pnpm
  scripts.

## OpenAPI

Keep a UI API OpenAPI spec, but update it to the new model:

- remove `graph/moon` descriptions and defaults
- document cells as git repo URLs
- document `projectId` as tenant ID
- align job/workflow schemas with JobDB/c2j job metadata
- make user-input schemas match c2j `pkg/input/openapi`
- stop describing old ticket/cell/project persistence as authoritative

Generate:

- Go server/client bindings with `oapi-codegen` or `go generate`
- TypeScript client under `web/openapi` with pnpm

Do not keep a separate `server/openapi` module unless it materially simplifies
generation.

## Migration Phases

### Phase 0: Inventory And Freeze Boundaries

- Mark `c2j_migration.md` as the migration map for moved package behavior.
- List every old module dependency still in `server/api/go.mod`.
- Decide whether `/src` becomes a root Go module immediately or uses
  `server/api` as the temporary only Go module.
- Decide which legacy UI screens stay in scope for the first rehab.

Exit criteria:

- A short inventory exists for old services to delete, replace, or temporarily
  shim.
- The team agrees that cells are repo URLs and project ID is tenant ID.

### Phase 1: Build System Cleanup

- Add root `package.json` and `pnpm-workspace.yaml`.
- Normalize package scripts in `web/*`.
- Remove active Moon task references from docs/scripts/configs.
- Replace `scripts/run-all-tests.sh` with Go plus pnpm commands.
- Reduce `go.work` to active modules only.
- Add the small production `Makefile`.

Exit criteria:

- `go test ./...` works for the active Go module(s).
- `pnpm -r --if-present test -- --run` works.
- No active build/test command invokes Moon.

### Phase 2: C2J/JobDB Application Wiring

- Build `internal/c2japp` around public c2j and JobDB packages.
- Create runtime config for:
  - remote JobDB URL
  - local SQLite runtime
  - in-memory toy runtime for UI tests
- Use c2j `pkg/config` for tenant/current/dependent cell resolution.
- Use `pkg/starter` for start/restart.
- Use c2j input routes and SSE manager.
- Use c2j story APIs.
- If needed, first export c2j runtime/worker helpers listed above.

Exit criteria:

- A Go test can start a toy runtime, submit a recipe job through the app layer,
  list it through JobDB, and build a c2j job story.

### Phase 3: API Rehab

- Replace old handlers that call project/cell/ticket/workflow services with
  c2j/jobdb adapters.
- Keep compatibility endpoint paths only where it helps the current UI migrate.
- Remove `serverdeps/engine.go` swf-go/PGWF/Strata wiring.
- Delete old service dependencies from `go.mod`.
- Update OpenAPI and regenerate Go/TS clients.

Exit criteria:

- The API server can list cells from c2j config, submit a ticket/job to a repo
  URL, list jobs, show pending inputs, and return job story data.
- `server/cell`, `server/project`, `server/ticket`, `server/workflow`,
  `server/recipes`, `server/registry`, `server/graph`, `swf-go`, `pgwf`, and
  Strata are gone from the API server dependency graph unless there is a
  documented temporary reason.

### Phase 4: UI Rehab

- Update API base URL handling.
- Update generated client imports.
- Update cell views for repo-url identity.
- Update ticket creation and detail pages to JobDB/c2j-backed data.
- Update workflow/job story pages to consume c2j story DTOs.
- Remove or hide screens whose old backend authority no longer exists.

Exit criteria:

- Vite dev UI works against `cmd/uitestserver`.
- Playwright smoke tests create/list/open a c2j-backed job.
- No UI flow depends on Moon cell sync.

### Phase 5: Production Executable

- Add `cmd/monolith` with embedded React dist.
- Serve `/api` from the same handlers as `cmd/api`.
- Optionally mount `/jobdb` when configured to expose the runtime REST API.
- Add config/env flags for runtime mode, JobDB URL, c2j working directory,
  listen address, CORS, and worker enablement.
- Keep worker execution optional in production; enable it by default only for
  `cmd/uitestserver`.

Exit criteria:

- `make build` produces one executable.
- The executable serves the SPA, API health, cell list, job list, job story, and
  user input endpoints.

### Phase 6: Deletion And Documentation

- Delete obsolete Moon files.
- Delete obsolete old-service adapters and stale generated files.
- Update README with:
  - local dev commands
  - test commands
  - production build command
  - c2j config expectations
  - JobDB runtime options
- Keep `c2j_migration.md` as the moved-package reference.

Exit criteria:

- Fresh checkout setup does not mention Moon.
- Dependency graph clearly shows `/src` as an app shell around c2j and JobDB.

## Validation Matrix

Minimum validation before calling the rehab complete:

- Go unit tests for app wiring and route handlers.
- Go integration test with toy or SQLite JobDB runtime.
- Vitest for retained UI components.
- Playwright smoke test against `cmd/uitestserver`.
- Production binary smoke test:
  - starts on a random port
  - serves `/`
  - serves `/api/health`
  - lists cells from a fixture `.c2j/config.yaml`
  - submits and reads a c2j recipe job
  - returns a c2j job story

## Risks And Decisions

- Ticket persistence: decide whether JobDB metadata is sufficient for list and
  filter needs. Add a small metadata table only if necessary.
- Recipe CRUD: decide whether recipes are files/git selectors or local records.
- Worker placement: decide whether production server is API-only by default or
  also runs c2j workers.
- Public c2j exports: runtime and worker composition helpers should move out of
  `cmd/c2j/internal` before `/src` duplicates them.
- API compatibility: keeping old endpoint paths speeds UI migration, but schemas
  should move toward c2j/JobDB concepts immediately.
