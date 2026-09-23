# Cortex migration status

The monolith-to-Cortex migration is implemented. This file replaces the original
phase plan; [README.md](README.md) describes the supported setup and commands.

## Current architecture

- One Go module under `server`; `go.work` includes that module only by default.
- Remote JobDB in production and API development; toy JobDB in the UI test server.
- Public c2j APIs own target resolution, job submission/listing, schema
  registration, stories/restart/artifacts, and human input handling.
- Jobs are the work item; projects are tenant IDs; cells are current/dependent
  repository identities. Cortex does not run workers or manage recipes.
- React packages are `web/app` and `web/shared`. The shared client is handwritten;
  retired generated OpenAPI and auxiliary UI packages are not build dependencies.
- `make build` produces `build/cortex` containing the production web assets.
- `.go-version` and `scripts/go.sh` select Go 1.26.7 for the current dependencies.

## Compatibility update (2026-09-23)

Cortex pins c2j v0.0.53, matching upstream main at
`ef65f00019724f17069d318845b0bc75ddc0dc98`, and its JobDB dependency
`v0.0.19-0.20260919034646-71b6668a65db`.

New c2j submissions use schema hashes. Both Cortex runtime paths wrap their
engine with `jobdbschema.WorkflowEngine`, which registers missing schemas in the
selected tenant and retries submissions/restarts through c2j's public API.

Frontend API calls share one base URL and use the Vite proxy during development.
Tenant deep links select the same tenant for pages, navigation, and input SSE.
Embedded SPA deep links serve the index without redirect loops; unknown API
routes return 404. Job story/control routes verify tenant-scoped existence before
calling c2j replay APIs. Restart offsets must preserve the initial job chapter.
Obsolete graph API stubs/types and old test-runner package references are removed.

## Validation boundaries

`pnpm test` runs Go tests, TypeScript checks, Vitest, the production build, and
Playwright. Go integration tests cover both in-process toy and HTTP remote JobDB,
including initial schema registration, tenant separation, submission/list/read,
job stories, artifacts/restart, and pending inputs. Playwright starts a real Cortex
API and checks browser job submission/story retrieval, cells, tenant selection,
and input SSE. The same integration tests also run against the built executable's
embedded UI and a separate HTTP JobDB test server.
Mocked UI smoke tests remain as supplemental coverage.

Tests do not execute recipes or start c2j workers. Deployment-specific JobDB
storage and external workers must be configured independently.

## Historical references

- `c2j_migration.md`: map of old monolith packages moved into c2j.
- `C2J_CORTEX_EXPORT_REQUIREMENTS.md`: June 2026 export prerequisite review.
- `GUIDE-Cortex-RecipeJob-API.md`: original recipejob integration guide.

The June documents are historical context; current API contracts come from the
pinned c2j source and Cortex's handlers/tests.
