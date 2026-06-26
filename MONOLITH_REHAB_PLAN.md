# Cortex Monolith Rehab Plan

This plan migrates `/src` from the partially-decomponentized monolith into a
small Cortex app shell around c2j and JobDB:

- a Go REST API server for the React UI
- a Go UI-dev/test server
- a React UI that develops and tests against that API server
- a production Go executable named `cortex` that embeds the built React assets
  and talks to a remote JobDB server

`/src/c2j_migration.md` is prior art for where old monolith behavior moved into
`/c2j`. Use it as the import map. Do not rebuild behavior that c2j already owns,
including job stories.

## Product Model

The old product concepts should be collapsed to the new model:

- JobDB tenants map to UI projects.
- The UI starts on tenant ID `1` by default.
- A different tenant can be selected by a URL parameter, for example
  `?tenantId=<id>`, and optionally by `VITE_DEFAULT_TENANT_ID` in dev.
- Cells are not managed records. The API can only see the current cell and its
  c2j-discovered dependent cells.
- Cells are identified by git repo URL strings. Short names are accepted only as
  c2j-resolved aliases.
- Tickets do not exist. There are only jobs.
- Recipe management does not exist in Cortex. Job submission may accept a recipe
  selector or default recipe name, but Cortex does not list, edit, publish,
  unpublish, version, or store recipes.

## Target Repo Shape

Keep `server` and `web` as separate top-level directories. Flatten Go to one
module under `server`:

```text
/src
  Makefile
  package.json
  pnpm-workspace.yaml
  go.work                 # optional local dev workspace with ./server and /c2j
  api/
    openapi/
      cortex-api.yaml
  server/
    go.mod
    cmd/
      api/                # REST API only, no embedded UI
      uitestserver/        # API server with embedded/toy JobDB for UI tests
      cortex/              # production API + embedded React dist
    internal/
      api/                # HTTP routes, DTOs, OpenAPI adapters
      app/                # dependency wiring
      c2japp/             # thin c2j/jobdb composition layer
      webdist/            # copied React dist for go:embed
  web/
    app/
    shared/
    flowchart/
    kanban/
    notebook/
    openapi/
```

Delete or archive old sibling Go modules once their behavior has been replaced.
Do not keep `server/project`, `server/cell`, `server/ticket`, `server/recipes`,
`server/workflow`, `server/registry`, or `server/graph` as active module
dependencies.

## Runtime Modes

Production `server/cmd/cortex`:

- requires a remote JobDB URL, for example `--jobdb-url http://127.0.0.1:9047`
- uses `github.com/colony-2/jobdb/pkg/jobdb/runtime/remote`
- does not start or embed JobDB
- does not support `embed:///`, toy, SQLite, PGWF, or Strata runtime modes
- serves the React SPA and the Cortex UI API

Development `server/cmd/api`:

- serves only the Cortex API
- should usually point at remote JobDB too
- may be useful for local UI dev against a separately running Vite server

Test server `server/cmd/uitestserver`:

- may use JobDB toy or SQLite runtime in-process
- does not run c2j workers
- should use seeded jobs or an external worker process for UI flows that need
  job progression
- is the only command that should support embedded/test runtime mode

## Non-Rebuild Rule

Before implementing or porting old code, check whether c2j already provides the
behavior. The migration should delete old monolith services when c2j or JobDB
already owns the concept.

Use these c2j and JobDB packages directly:

- `github.com/colony-2/c2j/pkg/config` for current cell, root cell, dependent
  repos, short-name expansion, and Go dependency auto-detection. Cortex tenant
  selection is UI/API state, defaulting to `1`, not implicit c2j config state.
- `github.com/colony-2/c2j/pkg/recipejob` for c2j-compatible target
  resolution, recipe job start request assembly, repo-url job list/read
  helpers, and c2j list defaults.
- `github.com/colony-2/c2j/pkg/starter` for c2j recipe job submit/restart shape
  and metadata.
- `github.com/colony-2/c2j/pkg/workflowctl` for runtime-agnostic job control.
- `github.com/colony-2/c2j/pkg/input` for user input HTTP behavior, SSE,
  pending input details, submit, and cancel behavior.
- `github.com/colony-2/c2j/pkg/story` and
  `github.com/colony-2/c2j/pkg/story/live` for job detail/story/replay/restart
  and outcome behavior. Do not rebuild job stories in `/src`.
- `github.com/colony-2/c2j/pkg/recipe`, `pkg/template`, and
  `pkg/worker/compiler` for job submission validation and recipe selector
  handling. Cortex does not construct or run workers.
- `github.com/colony-2/jobdb/pkg/jobdb` and
  `github.com/colony-2/jobdb/pkg/workflow` for runtime and engine APIs.
- `github.com/colony-2/jobdb/pkg/jobdb/runtime/remote` for production Cortex.
- `github.com/colony-2/jobdb/pkg/jobdb/runtime/toy` or `runtime/sqlite` only for
  `uitestserver` and Go tests.

## API Model

Keep the UI API under `/api`. Do not mount JobDB REST endpoints in Cortex unless
there is a later explicit requirement. Cortex is a UI-specific facade over a
remote JobDB runtime.

### Tenants As Projects

Remove the project service.

Compatibility endpoints may keep the word `projects`, but the value is a JobDB
tenant ID:

```text
GET /api/projects
GET /api/projects/{tenantId}
```

Implementation:

- `GET /api/projects` returns a synthetic list containing the selected tenant.
- The default selected tenant is `1`.
- `GET /api/projects/{tenantId}` returns synthetic metadata for that tenant.
- No create, patch, delete, settings, or admin project endpoints.
- No project persistence table.
- No project editing UI.

The React app should store the selected tenant in URL state. If no tenant is
provided, use `tenantId=1`.

### Cells

Remove the cell service, cell store, sync service, dependency store, graph
populator, and Moon graph assumptions.

Cortex can expose only:

- current cell from `c2j/pkg/config.ProjectConfig.SelfRepo`
- selected tenant from the API path or UI URL parameter, defaulting to `1`
- dependent cells from `ProjectConfig.AllowedDependentRepos`
- optional display short names from `ProjectConfig.CellNameFromRepo`

Keep the API intentionally small:

```text
GET /api/projects/{tenantId}/cells
GET /api/projects/{tenantId}/cells/self
```

Response shape should be repo-url first:

```json
{
  "repo": "github.com/acme/root",
  "name": "root",
  "role": "self"
}
```

Rules:

- `repo` is the stable identifier.
- `name` is display-only and may be empty.
- `role` is `self` or `dependent`.
- Remove `workingPath`, `populator`, `populatorId`, persisted IDs, and editable
  dependency fields from the UI and OpenAPI.
- Remove `/cells/sync`, `/cells/{id}`, `/cells/{id}/dependencies`, and `/graph`
  from the first target API.
- Do not implement cell traversal. The visible set is current cell plus
  dependents only.
- Job submission accepts short names, repo URLs, or local paths the same way c2j
  does. Use `recipejob.ResolveTarget`; do not reimplement target resolution
  from `pkg/config` primitives.

### Jobs

Remove tickets. Replace ticket and workflow UI language with jobs.

Core endpoints:

```text
GET  /api/projects/{tenantId}/jobs
POST /api/projects/{tenantId}/jobs
GET  /api/projects/{tenantId}/jobs/{jobId}
POST /api/projects/{tenantId}/jobs/{jobId}/restart
GET  /api/projects/{tenantId}/jobs/{jobId}/story
GET  /api/projects/{tenantId}/jobs/{jobId}/outcome
GET  /api/projects/{tenantId}/jobs/{jobId}/tasks/{taskOrdinal}/artifacts/{artifactName}
```

Job submission:

- mirrors c2j submit semantics where practical
- accepts a target cell value that may be a short name, repo URL, or path
- resolves target cells with `recipejob.ResolveTarget`
- accepts recipe selector/name and inputs
- defaults recipe name the same way c2j does when no recipe is specified
- assembles requests with `recipejob.BuildStartJob`
- submits via `starter.StartRecipeJob` or
  `starter.StartRecipeJobWithOptions`
- returns JobDB tenant ID and job ID

Job listing:

- uses `recipejob.ListRecipeJobs` and `recipejob.GetRecipeJob`
- filters by tenant ID
- filters to c2j recipe jobs with `starter.RecipeJobType`
- filters by repo URL through `starter.MetaFieldRepo`
- resolves short names with `recipejob.ResolveTarget` before repo filtering
- does not depend on ticket stages or ticket states

Job detail/story/outcome:

- use JobDB job read APIs plus c2j metadata
- use `pkg/story/live.BuildJobRunStory` or `pkg/story.New`
- use `starter.RestartRecipeJob` for restart
- use JobDB artifact APIs for artifact fetch

### User Inputs

Keep the existing input UI, but back it with c2j input management:

```text
GET  /api/projects/{tenantId}/user-inputs/pending
GET  /api/projects/{tenantId}/user-inputs/stream
GET  /api/projects/{tenantId}/user-inputs/{jobId}
POST /api/projects/{tenantId}/user-inputs/{jobId}/respond
POST /api/projects/{tenantId}/user-inputs/{jobId}/cancel
```

Implementation:

- initialize `input.NewSimpleSSEManager`
- build `ops.ServiceDependencies2` with `WorkflowControl` and SSE manager
- use c2j input route behavior rather than reimplementing it

### Recipe Management

Remove recipe management entirely:

- remove recipe list page
- remove recipe detail page
- remove recipe editor page
- remove recipe history tab
- remove recipe publish/unpublish flows
- remove recipe CRUD endpoints
- remove recipe management OpenAPI schemas
- remove old `server/recipes` and `server/registry` dependencies

Job submission may still pass a recipe selector/name to c2j. Cortex does not
own or edit that recipe.

### Project And Admin UI

Remove:

- project settings modal/page
- project admin page
- project create/edit/delete flows
- project git repo mapping UI
- any UI copy implying project persistence

Replace with tenant selection:

- default tenant ID `1`
- optional URL parameter `?tenantId=<id>`
- compact tenant switcher if useful

## C2J API Status

The c2j API prerequisites for Cortex are complete. The Cortex server must still
not import `cmd/c2j/internal/*`.

Implemented c2j surfaces are documented in `GUIDE-Cortex-RecipeJob-API.md` and
summarized in `C2J_CORTEX_EXPORT_REQUIREMENTS.md`.

Use `github.com/colony-2/c2j/pkg/recipejob` for:

- `ResolveTarget`
- `BuildStartJob`
- `SubmitRecipeJob`
- `ListRecipeJobs`
- `GetRecipeJob`
- `DefaultVisibleStatuses`
- `StoresForStatuses`

c2j now persists target repo metadata with `starter.MetaFieldRepo` and
`starter.JobMetadata.RepositorySource`. Jobs submitted before that metadata
field existed may not be filterable by repo URL at the JobDB metadata layer,
but new Cortex submissions will be.

Cortex still owns the UI/API facade and production runtime wiring, and
`cortex` still talks only to remote JobDB.

## Build System

Remove Moon entirely from active build/test flows.

### Go

Use one Go module under `server`:

```bash
cd server
go test ./...
go run ./cmd/api --jobdb-url http://127.0.0.1:9047
go run ./cmd/uitestserver --port 8080
go run ./cmd/cortex --jobdb-url http://127.0.0.1:9047
go build -o ../bin/cortex ./cmd/cortex
```

Root `go.work` is optional for local development:

```text
use (
  ./server
  /c2j
)
```

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
    "dev:api": "cd server && go run ./cmd/uitestserver --port 8080",
    "dev:web": "pnpm --filter @colony2/app dev",
    "build:web": "pnpm --filter @colony2/app build",
    "test:web": "pnpm -r --if-present test -- --run",
    "test:e2e": "pnpm --filter @colony2/app test:e2e",
    "openapi": "pnpm --filter @colony2/openapi-client build"
  }
}
```

### Make

Use Make only for cross-language production packaging:

```make
.PHONY: build
build:
	pnpm install --frozen-lockfile
	pnpm run build:web
	rm -rf server/internal/webdist/dist
	mkdir -p server/internal/webdist
	cp -R web/app/dist server/internal/webdist/dist
	cd server && go build -o ../bin/cortex ./cmd/cortex

.PHONY: test
test:
	cd server && go test ./...
	pnpm run test:web
	pnpm run test:e2e
```

Keep Make small. Go and pnpm remain the real build tools.

## React UI Migration

Keep only UI surfaces that match the new model:

- tenant/project selector using tenant ID
- current/dependent cell list
- job list
- job submission form
- job detail
- job story
- pending user input views and form rendering

Remove:

- project editing/admin/settings
- cell editing/sync/dependency graph/detail pages
- ticket list/detail/create/stage/state/event pages
- recipe list/detail/editor/history/publish/unpublish pages
- old workflow language where it means tickets or recipe management

Required UI changes:

- Replace hard-coded `http://localhost:8080/api` with `VITE_API_BASE_URL`,
  defaulting to `/api` for production and `http://localhost:8080/api` in dev.
- Add tenant selection from `?tenantId=<id>`, defaulting to `1`.
- Regenerate the OpenAPI client from the new Cortex API spec.
- Treat cells as repo URLs and short-name display aliases.
- Replace ticket creation with job submission.
- Update Playwright to start both Vite and `server/cmd/uitestserver` through
  pnpm scripts.

## OpenAPI

Update the UI API spec to the new model:

- rename or regenerate as `api/openapi/cortex-api.yaml`
- document `projectId` as JobDB tenant ID
- keep `/projects/{tenantId}` compatibility only as tenant mapping
- document cells as repo URL records from c2j config
- remove cell sync, cell dependency mutation, cell detail editing, and graph
- remove all ticket schemas and paths
- remove all recipe management schemas and paths
- align job schemas with JobDB and c2j starter metadata
- make user-input schemas match c2j `pkg/input/openapi`

Generate:

- Go bindings inside the `server` module
- TypeScript client under `web/openapi` with pnpm

Do not keep `server/openapi` as a separate Go module unless generation truly
requires it.

## Migration Phases

### Phase 0: Boundary Decisions

- Mark `c2j_migration.md` as the moved-package map.
- Confirm tenant ID `1` as the UI default.
- Confirm URL parameter shape for alternate tenants.
- Confirm `cortex` requires `--jobdb-url` and never starts JobDB.
- Inventory every old module dependency in `server/api/go.mod`.

Exit criteria:

- The team agrees that projects are tenants, cells are repo URLs, tickets are
  removed, and recipe management is removed.

### Phase 1: Build Layout

- Create one Go module under `server`.
- Move/rename commands to `server/cmd/api`, `server/cmd/uitestserver`, and
  `server/cmd/cortex`.
- Add root `package.json`, `pnpm-workspace.yaml`, and small `Makefile`.
- Reduce `go.work` to `./server` and optional `/c2j`.
- Remove active Moon commands from scripts and docs.

Exit criteria:

- `cd server && go test ./...` works for active Go code.
- `pnpm -r --if-present test -- --run` works.
- No active build/test command invokes Moon.

### Phase 2: C2J/JobDB App Layer

- Build `server/internal/c2japp` around public c2j and JobDB packages.
- Production runtime path uses only `remote.New(jobdbURL, client)`.
- Test runtime path can use JobDB toy or SQLite.
- Use c2j config for current/dependent cells and short-name expansion.
- Use `recipejob.ResolveTarget` for submitted/listed cell values.
- Use `recipejob.BuildStartJob` plus `starter.StartRecipeJob` for job
  submission.
- Use `recipejob.ListRecipeJobs` and `recipejob.GetRecipeJob` for job list/read.
- Use `starter.RestartRecipeJob`.
- Use c2j input routes and SSE manager.
- Use c2j story APIs.
- Do not run c2j workers from Cortex. If UI tests need job progression, run a
  separate c2j worker process or explicitly revisit this requirement.

Exit criteria:

- Go tests can use a test JobDB runtime or remote test server, submit a c2j
  recipe job, list it, and build a c2j job story without importing
  `cmd/c2j/internal/*`.

### Phase 3: API Replacement

- Replace old project handlers with tenant compatibility handlers.
- Replace old cell handlers with current/dependent c2j config handlers.
- Replace ticket and workflow handlers with job handlers.
- Remove recipe management handlers entirely.
- Remove `serverdeps/engine.go` swf-go/PGWF/Strata wiring.
- Delete old service dependencies from `server/go.mod`.
- Update OpenAPI and regenerate Go/TS clients.

Exit criteria:

- API supports tenant `1`, cells self/dependents, job submit/list/detail/story,
  restart, outcome, artifact fetch, and user inputs.
- API dependency graph no longer includes old project/cell/ticket/recipe/
  workflow/registry/graph services or swf-go/PGWF/Strata.

### Phase 4: UI Replacement

- Add tenant URL state, defaulting to `1`.
- Remove project admin/settings/editing routes.
- Remove ticket routes and components.
- Remove recipe management routes and components.
- Replace workflow/ticket navigation with jobs.
- Replace cell graph/editing UI with current/dependents list.
- Keep input activity UI and point it at c2j-backed endpoints.

Exit criteria:

- Vite UI works against `server/cmd/uitestserver`.
- Playwright smoke tests submit/list/open a c2j-backed job.
- No UI path depends on Moon, tickets, project editing, recipe management, or
  old cell traversal.

### Phase 5: Cortex Executable

- Add `server/cmd/cortex`.
- Embed `web/app/dist` via `server/internal/webdist`.
- Require `--jobdb-url` or `CORTEX_JOBDB_URL`.
- Fail startup if remote JobDB URL is missing.
- Serve `/api` and SPA assets.
- Do not expose embedded runtime flags.
- Do not start JobDB.

Exit criteria:

- `make build` produces `bin/cortex`.
- `bin/cortex --jobdb-url http://127.0.0.1:9047` serves SPA, health, cells,
  jobs, job story, and user input endpoints.

### Phase 6: Deletion And Documentation

- Delete obsolete Moon files.
- Delete obsolete old-service adapters and stale generated files.
- Update README with:
  - local dev commands
  - UI tenant selection
  - test commands
  - production `cortex` build/run command
  - remote JobDB requirement
  - c2j config expectations
- Keep `c2j_migration.md` as the moved-package reference.

Exit criteria:

- Fresh checkout setup does not mention Moon.
- Dependency graph clearly shows Cortex as an app shell around c2j and remote
  JobDB.

## Validation Matrix

Minimum validation before calling the rehab complete:

- Go unit tests for tenant mapping, c2j cell resolution, job submission, job
  list filters, job stories, and input routes.
- Go integration tests with JobDB toy or SQLite runtime for `uitestserver`.
- Go startup test proving `cortex` fails without a remote JobDB URL.
- Vitest for retained UI components.
- Playwright smoke test against `server/cmd/uitestserver`.
- Production binary smoke test against a real remote JobDB process:
  - starts with `--jobdb-url`
  - serves `/`
  - serves `/api/health`
  - defaults UI/API tenant to `1`
  - accepts alternate tenant parameter
  - lists self/dependent cells from fixture `.c2j/config.yaml`
  - submits and reads a c2j job
  - returns a c2j job story

## Risks And Decisions

- Job metadata: decide which submitted input fields become JobDB metadata for UI
  filtering. Do not add a local job table unless JobDB metadata is insufficient.
- Tenant selection: choose exact URL parameter and whether to persist the last
  tenant in local storage.
- Worker placement: production `cortex` should not embed JobDB or run c2j
  workers. Workers stay outside Cortex unless this is explicitly revisited.
- c2j API prerequisite status: `pkg/recipejob` and `starter.MetaFieldRepo`
  satisfy the repo-url list/read, target resolution, and submit assembly
  requirements. The remaining caveat is historical jobs submitted before
  `repo` metadata existed; handle any legacy backfill as a separate decision.
- API compatibility: old `/projects/{id}` paths can stay as tenant aliases, but
  old project editing, tickets, recipe management, and cell traversal should be
  removed rather than shimmed.
