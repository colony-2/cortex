# Cortex

Cortex is a Go API and React web application for c2j recipe jobs. It submits and
lists jobs, displays execution stories and artifacts, restarts jobs, and collects
human input. JobDB tenants appear as projects (default `1`); cells are the current
Git repository and its c2j-configured dependents. Workers run separately.

## Setup

Install Node.js 22 or newer, pnpm 10, Git, Make, and Go with toolchain download
support. Then run:

```sh
pnpm install --frozen-lockfile
bash scripts/go.sh -C server mod download
```

The Go commands in Make and package scripts use Go **1.26.7**, recorded in
`.go-version`. The wrapper downloads it automatically if needed. Current c2j
transitive dependencies do not support Go 1.27; `GOTOOLCHAIN` can explicitly
override the wrapper when testing another toolchain.

Cortex pins **c2j v0.0.53** (`ef65f00019724f17069d318845b0bc75ddc0dc98`,
verified against upstream main on 2026-09-23), and the JobDB version required by
that release. No sibling checkout is required. To develop against a local c2j
checkout, run `bash scripts/go.sh work use ../c2j`; remove that workspace entry when finished.

## Development

Start an API connected to a running remote JobDB server:

```sh
JOBDB_URL=http://127.0.0.1:9047 CORTEX_WORKING_DIR=/path/to/cell pnpm dev:api
```

In another terminal:

```sh
pnpm dev:web
```

Open `http://localhost:3000`. Vite proxies `/api` to `http://127.0.0.1:8080`.
Set `CORTEX_API_URL` when starting Vite to use another API address. Browser API
calls use the same origin by default; `VITE_CORTEX_API_BASE` overrides their base.

For UI development without a JobDB process:

```sh
pnpm dev:test-api --addr 127.0.0.1:8080 --working-dir web/app/tests/fixtures/cell
```

The test API stores jobs in memory and does not run workers. Submitted jobs can
be listed and inspected, but will not execute until a separate worker is used
with a shared remote JobDB runtime.

Choose a tenant in the header or open `/?tenantId=42`. Deep links such as
`/project/42/jobs` select the route's tenant for navigation and input activity.
The last selected tenant is retained in browser storage. The email screen stores
an identity cookie; it is not server-side authentication.

## Production

```sh
make build
./build/cortex --jobdb-url http://127.0.0.1:9047 --working-dir /path/to/cell
```

The executable embeds the built UI and serves it and `/api` on port 8080.
It requires remote JobDB and never starts JobDB or c2j workers. c2j's schema-aware
engine registers the recipe schema on first submission in each tenant.

Options are `--addr`, `--jobdb-url`, `--working-dir`, `--tenant-id`, and
`--cors-origins`. Their environment equivalents are `CORTEX_ADDR`, `JOBDB_URL`,
`CORTEX_WORKING_DIR`, `CORTEX_TENANT_ID`, and `CORTEX_CORS_ORIGINS`.

The cell directory supplies `.c2j/config.yaml`; see
[`web/app/tests/fixtures/cell/.c2j/config.yaml`](web/app/tests/fixtures/cell/.c2j/config.yaml)
for a minimal example. Recipe resolution and execution belong to c2j.

## Components

| Path | Responsibility |
| --- | --- |
| `server/cmd/api` | API connected to remote JobDB |
| `server/cmd/uitestserver` | In-memory test API; `--jobdb-only` exposes the test JobDB over HTTP |
| `server/cmd/cortex` | Production API and embedded UI |
| `server/internal/cortex` | HTTP routes, tenant/cell mapping, c2j composition |
| `server/internal/webdist` | Embedded Vite build |
| `web/app` | React, TypeScript, Ant Design, and Vite UI |
| `web/shared` | API client, types, identity helpers, and input SSE state |

The API includes health, tenant metadata, current/dependent cells, job
submit/list/detail/story/restart/outcome/artifacts, and c2j pending-input,
respond/cancel, and SSE routes. `projectId` in URLs is a JobDB tenant ID.
The web client is maintained directly alongside these handlers. Legacy project
administration, tickets, recipe management, graph UI, generated OpenAPI clients,
and Moon packages have been retired.

## Validation

```sh
pnpm test             # Go tests, type checks, web tests, production build, browser tests
pnpm test:api         # Go integration tests with toy and HTTP remote JobDB
pnpm typecheck:web
pnpm test:web
pnpm test:e2e         # builds Cortex and runs browser tests with managed servers
make build
```

Playwright installs its headless Chromium browser if missing. On a fresh Linux
host, install its OS dependencies with
`pnpm --filter @colony2/app exec playwright install-deps chromium`.
`CORTEX_PLAYWRIGHT_BROWSERS_PATH` selects the browser cache directory (default
`$HOME/.cache/ms-playwright`).

The browser suite uses dedicated ports **15173** (Vite), **18081** (test API),
**18082** (test JobDB), and **18083** (production Cortex). It refuses to reuse
existing servers and uses an isolated fixture cell. Real API tests exercise login,
job submission, repository filtering, story retrieval, tenant switching/isolation,
cells, pending inputs, and a real SSE connection. These run both against the dev
UI/test API and against the embedded production UI with HTTP remote JobDB.
Mocked browser tests separately check rendering and console cleanliness.
The suite runs six browser tests and no recipe workers. Reports are in
`web/app/playwright-report`.

See [the migration status](MONOLITH_REHAB_PLAN.md) and
[the historical package migration map](c2j_migration.md) for background.
