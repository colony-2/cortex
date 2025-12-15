# Nx Migration Proposal for VibeThis

## Goal and scope
- Replace Moon with Nx in one migration changeset (no dual-running period or backward compatibility).
- Preserve existing build/test behaviors and task wiring while gaining Nx caching/graphing.
- Keep project boundaries identical to current Moon projects and align names with existing IDs where they exist.

## Workspace setup (single PR)
- Add a root `package.json` with `nx`, `@nx/js`, `@nx/workspace`, `@nx/vite`, `@nx/react`, and `@nx-go/nx-go` (Go plugin) as dev dependencies; reuse project-level dependencies already present.
- Create `nx.json` + `workspace.json` (or per-project `project.json`) at repo root. Set `defaultBase` to the main branch and enable caching.
- Define named inputs once to avoid repetition:
  - `go`: `["go.mod","go.sum","{projectRoot}/**/*.go"]`
  - `ts`: `["package.json","package-lock.json","{projectRoot}/tsconfig*.json","{projectRoot}/src/**/*"]`
- Configure `targetDefaults` once (shared by every project that declares the target):
  - `build`: `inputs: ["default","go?","ts?"], outputs: ["{projectRoot}/build","{projectRoot}/dist"], cache: true`
  - `test`: same inputs, `cache: true`
  - `lint`: `cache: true`
  - `integration`: `cache: false`
  - `serve`: `cache: false`
- Add `tsconfig.base.json` for shared TS path aliases (to match existing package import paths) and hook Vite targets into it.
- Add root `tools/` with helper scripts if needed (e.g., OpenAPI codegen wrappers) and wire them via `nx:run-commands`.
- Remove all `moon.yml` files in the same PR once Nx configs are in place.

## Shared target patterns (minimize duplication)
- Go libraries/apps: declare `build` (`@nx-go/nx-go:build`) and `test` (`@nx-go/nx-go:test`) with no per-project inputs/outputs because `targetDefaults` + `go` named input handle that.
- Vite/React packages: declare `build`/`serve`/`test` using `@nx/vite:*` with only `configFile` overrides; outputs come from `targetDefaults`.
- Run-command tasks: only specify the `command`/`cwd`/`dependsOn`; inputs/outputs rely on the defaults unless a task produces non-standard paths (call out where custom outputs are needed below).

## Project definitions and targets
Each entry lists how the Moon project maps to Nx (`projectType`, executor, and targets). All Go projects use the Go plugin executors (`@nx-go/nx-go:build` / `:test`) unless otherwise noted; Vite/React packages use the Nx Vite executors. Inputs/outputs/caching are inherited from `targetDefaults` unless called out as custom (e.g., generated code or copied bundles).

### API and OpenAPI
- `api-openapi` (api/openapi, tool): target `build` (`nx:run-commands` `echo "OpenAPI spec is ready"`), output `colony2-api.yaml`; tagged `api`.
- `be-openapi` (server/openapi, lib): target `build` (`nx:run-commands` running `go run github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@v2.1.0 -config codegen.yml -o pkg/openapi/generated.go ../../api/openapi/colony2-api.yaml`), `outputs: ["pkg/openapi/generated.go"]`, `dependsOn: ["api-openapi:build"]`.
- `fe-openapi` (web/openapi, lib): target `build` (`nx:run-commands` invoking `npm run generate && npm run build` or direct `openapi-typescript-codegen` + `tsup`), `outputs: ["src/generated/**/*","dist/**/*"]`, `dependsOn: ["api-openapi:build"]`; target `test` as a no-op (`true`) to mirror Moon.

### Backend services and libs
- `be-api` (server/api, lib): target `build-test-server` (`nx:run-commands` `go build -o build/testserver ./cmd/testserver`, output `build/testserver`). Standard `build`/`test` targets via Go plugin for the rest of the package.
- `be-core` (server/core, lib): Go lib with default `build`/`test`.
- `be-graph` (server/graph, lib): Go lib with default `build`/`test`.
- `be-git` (server/git, lib): Go lib with default `build`/`test`.
- `be-storage` (server/storage, lib): Go lib with default `build`/`test`.
- `server-ticket` (server/ticket, lib): Go lib with default `build`/`test`.
- `be-rucc` (server/rucc, lib): Go lib with default `build`/`test`.
- `server/ops` (server/ops, lib): Go lib; define standard `build`/`test` targets.
- `server/llm` (server/llm, lib): Go lib; add `integration` target (`nx:run-commands` `go test -v -tags=integration ./integration/...`, uncached, opt-in) plus default `build`/`test`.
- `be-graph-workflow` family:
  - `recipe-core` (server/recipe-core, lib), `recipe-history` (server/recipe-history, lib), `recipe-worker` (server/recipe-worker, lib): Go libs with default `build`/`test`.
- `be-nucleus` (server/nucleus, app): targets `build` (`go build -o build/nucleus ./cmd/nucleus`, outputs `build/nucleus`), `install` (`go install ./cmd/nucleus`), `integration` (`go test -v -tags=integration ./...`, uncached). Exclude from inherited `build` defaults as in Moon by setting `implicitDependencies: []` and explicit targets.
- `cortex` (server/cortex, app): 
  - `npm-install` (`nx:run-commands` `npm install`, output `node_modules`).
  - `build-frontend-copy` (`nx:run-commands` rsync from `web/app/dist` into `internal/static/assets`, outputs `internal/static/assets`, `dependsOn: ["ui-app:build"]`).
  - `build` (`nx:run-commands` `go build -o build/cortex ./cmd/cortex`, outputs `build/`, `dependsOn: ["build-frontend-copy"]`).
  - `serve` (`nx:run-commands` `go run ./cmd/cortex/*.go -n ../../`, `dependsOn: ["build-frontend-copy"]`, `cache: false`).
  - `install` (`nx:run-commands` `go install -tags=prod -ldflags="-X main.defaultPort=8081" ./cmd/cortex`, `dependsOn: ["build","build-frontend-copy"]`).
  - `playwright-install` (`nx:run-commands` `npx playwright install chromium`, `dependsOn: ["npm-install"]`).
  - `test` (`nx:run-commands` `./test.sh`, `dependsOn: ["build-frontend-copy","^build","npm-install","playwright-install"]`).

### Web frontends and shared packages
- `ui-app` (web/app, app): `build` via `@nx/vite:build` (or `nx:run-commands` `npm run build`), `serve` via `@nx/vite:dev` (`npm run dev`), `test` via `@nx/vite:test` (`vitest`), `e2e` via `nx:run-commands` calling Playwright (`npx playwright test --reporter=list`) with `dependsOn: ["be-api:build-test-server"]`; `build` depends on `ui-flowchart:build`, `ui-kanban:build`, `web-shared:build`.
- `ui-flowchart` (web/flowchart, lib): add `build` (`@nx/vite:build` pointing to `vite.config.ts`), `test` (`@nx/vite:test`), `lint` optional; depends on `web-shared`.
- `web-shared` (web/shared, lib): `compile` (`nx:run-commands` `tsc`), `bundle` (`nx:run-commands` `vite build --logLevel warn`), `build` depends on `compile` then `bundle`, `lint` via `eslint src`; outputs `dist/**`.
- `ui-kanban` (web/kanban, lib): `build` (`nx:run-commands` `npm run build`, outputs `dist`), `dev`/`test` optional using existing scripts; depends on `web-shared` and `fe-openapi` through package graph.
- `fe-openapi` dependency already covered above; ensure tags keep frontend boundary.

### Infrastructure, Docker, and shims
- `shai-docker` (docker, tool): `build` (`nx:run-commands` `docker build -t debian-dev:dev --target dev .`, cached), `integration` (`nx:run-commands` `bash tests/integration/test_image.sh`, depends on `build`, uncached).
- `rwshim-clib` (rwshim/clib, tool): `build` (`nx:run-commands` `docker build -t rwshim:latest .`), `test` (`nx:run-commands` `./run_tests_docker.sh`, uncached).
- `rwshim-go` (rwshim/rwshimgo, lib): Go lib with `integration` (`nx:run-commands` `./integration_test.sh`, uncached) plus default `build`/`test`.

## Dependency graph and tagging
- Apply tags to enforce layers consistent with AGENTS.md: `frontend`, `backend`, `workflow`, `infrastructure`, `api`, `docker`. Use `implicitDependencies` only where explicit commands need upstream artifacts (e.g., `cortex` depends on `ui-app` build artifacts; OpenAPI generators depend on `api-openapi`).
- Configure `nx.json` lint rules (enforce-no-circular-deps) to keep web packages from importing backend code and backend from infrastructure-only layers.

## Migration execution (single changeset)
- Add root Nx config, install Nx + plugins, generate `project.json` for every project above, and wire dependencies/outputs.
- Update CI to use `nx run-many --target=build` / `test` with affected scopes, and replace Moon commands in scripts/docs.
- Delete all `moon.yml` files and Moon tooling references.
- Validate by running `nx graph` (for sanity), `nx run-many --target=build --all`, `nx run-many --target=test --all`, plus targeted `integration` runs where available.
