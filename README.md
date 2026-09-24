# Cortex

Cortex is a web interface for [c2j](https://github.com/colony-2/c2j) recipe jobs.
Submit and inspect jobs, read execution stories and artifacts, restart jobs, and
respond to human input requests. The `cortex` command serves the Go API and the
embedded React UI from one executable.

Cortex connects to a remote [JobDB](https://github.com/colony-2/jobdb) server.
Run c2j workers separately against the same JobDB. Projects in the UI are JobDB
tenants; cells are the current Git repository and its c2j-configured dependents.

## Install and run

Releases provide Linux and macOS binaries for x86_64 and ARM64. Install through
npm (Node.js 14 or newer and `tar` required):

```sh
npm install -g @colony2/cortex
cortex version
cortex --jobdb http://127.0.0.1:9047/c2 --working-dir /path/to/cell
```

The npm installer downloads the matching binary from
[GitHub Releases](https://github.com/colony-2/cortex/releases) and verifies its
SHA-256 checksum. You can also download and extract an archive directly; those
binaries do not require Node.js. Each archive contains the built web UI inside
the executable.

Open `http://localhost:8080`. The working directory should be a Git checkout
with a c2j cell configuration; see the
[minimal fixture](web/app/tests/fixtures/cell/.c2j/config.yaml).

| Option | Environment variable | Default |
| --- | --- | --- |
| `--addr` | `CORTEX_ADDR` | `:8080` |
| `--jobdb` | `C2J_JOBDB` | `jobdb` from `.c2j/config.yaml` |
| `--working-dir` | `CORTEX_WORKING_DIR` | Current directory |
| `--cors-origins` | `CORTEX_CORS_ORIGINS` | Empty; comma-separated origins |

Cortex uses Cobra, like c2j: long flags use `--` (`--jobdb`, `--addr`) and
short flags use `-` (`-h` for help). `cortex serve` explicitly starts the server;
`cortex` with the same flags is equivalent. Use `cortex version` or
`cortex --version` to identify the release. Source builds report `dev`.

The JobDB URI includes the tenant, just like c2j:

```sh
cortex serve --jobdb http://localhost:9047/c2
# Or share the same environment setting with c2j:
C2J_JOBDB=http://localhost:9047/c2 cortex
```

Resolution follows `--jobdb`, then `C2J_JOBDB`, then the nearest ancestor
`.c2j/config.yaml`. Cortex calls c2j's `pkg/config.LoadProjectConfig` and
`ProjectConfig.JobDBURI` directly, including support for command-valued settings:

```yaml
jobdb: http://localhost:9047/c2
# Alternatively:
# jobdb:
#   command: your-command-that-prints-a-jobdb-uri
```

Run `cortex` from that cell or a subdirectory, or select the discovery directory
with `--working-dir`. Cortex requires a remote HTTP(S) JobDB server; `embed:///`
is not supported.

For migration, `--jobdb-url` remains a deprecated alias accepting either the new
URI or a server-only URL with `--tenant-id`. Legacy `JOBDB_URL` is used only when
neither a connection flag nor `C2J_JOBDB` is set, before consulting the config.
`CORTEX_TENANT_ID` applies only to legacy server-only URLs (default `1`).

Select a tenant in the header, use `/?tenantId=42`, or open a tenant route such as
`/project/42/jobs`. The email screen records an identity cookie; it does not
provide server-side authentication.

## Build from source

Install Git, Make, Node.js 22 or newer, pnpm 10, and Go with automatic toolchain
downloads enabled. The Go wrapper selects the version in [`.go-version`](.go-version).

```sh
pnpm install --frozen-lockfile
bash scripts/go.sh -C server mod download
make build
./build/cortex --jobdb http://127.0.0.1:9047/c2 --working-dir /path/to/cell
```

`make build` builds the web packages, copies their output into the Go embed
directory, and produces `build/cortex`. A plain `go build` does not build the UI.
Go dependencies are pinned in [`server/go.mod`](server/go.mod); no sibling
checkout is required.

## Development

Start the API and web dev server in separate terminals:

```sh
C2J_JOBDB=http://127.0.0.1:9047/c2 CORTEX_WORKING_DIR=/path/to/cell pnpm dev:api
pnpm dev:web
```

Open `http://localhost:3000`. Vite proxies `/api` to `http://127.0.0.1:8080`;
set `CORTEX_API_URL` to use another API address. Browser requests use the same
origin unless `VITE_CORTEX_API_BASE` is set at build time.

For UI development with in-memory jobs, replace the API command with:

```sh
pnpm dev:test-api --addr 127.0.0.1:8080 --working-dir web/app/tests/fixtures/cell
```

The test API does not execute recipes. Use remote JobDB and separate c2j workers
when you need job execution.

| Directory | Purpose |
| --- | --- |
| `server/cmd/cortex` | Production API and embedded UI |
| `server/cmd/api` | Development API using remote JobDB |
| `server/cmd/uitestserver` | In-memory test API and HTTP test JobDB; `--jobdb-only --sqlite` uses temporary SQLite storage |
| `server/internal/cortex` | Routes, tenant/cell mapping, and c2j integration |
| `server/internal/webdist` | Embedded production web assets |
| `web/app` | React application |
| `web/shared` | API client, types, identity, and input event state |
| `npm/cortex` | npm launcher and release binary installer |

## Test

```sh
pnpm test             # Go tests, type checks, web tests, build, browser tests
pnpm test:api         # Go tests, including remote JobDB integration
pnpm typecheck:web
pnpm test:web
pnpm test:e2e         # Browser tests against development and embedded production UI
pnpm --filter @colony2/app test:e2e --project=production recipe-input.spec.ts
node --test npm/cortex/test/*.test.cjs
```

Playwright installs Chromium if missing. On a fresh Linux host, first run
`pnpm --filter @colony2/app exec playwright install-deps chromium`.
The suite manages its own servers on ports 15173 and 18081–18083. It covers
submission, stories, tenants, cells, and input events. The production tests use
HTTP JobDB backed by temporary SQLite storage.

The recipe-input tests build the pinned c2j command and execute real Git-backed
recipes with single-question and multi-field input requests. They submit jobs
through the browser, observe pending requests in a second tab over SSE, check
required fields and page reloads, submit answers while the worker is stopped,
and restart it to verify the exact recipe output and completed story. They also
cover cancellation from another client, rejected late answers, and consecutive
prompts in one job without reloading either tab. Each test has a separate tenant
and temporary Git repository; the fixture cell's Git URL
is mapped to that repository in the worker's environment. No API or EventSource
mocks are used in these tests. Worker logs and recipe outcomes are attached to
the Playwright report in `web/app/playwright-report`.

The timeout regression also runs against the real worker. It currently records
an **expected failure**: with c2j v0.0.53 / JobDB v0.0.19, an unanswered external
input task remains pending past its recipe deadline. Step deadlines have the
same limitation. The test first verifies that the prompt appears, then marks
only a confirmed still-pending outcome after the deadline as expected; setup
and other failures still fail CI. Timeout handling needs an upstream JobDB fix.

## Releases

The [release workflow](.github/workflows/release.yaml) follows
[c2j's release pipeline](https://github.com/colony-2/c2j/blob/main/.github/workflows/release.yaml):

1. A push to `main` runs the test suite on Linux x86_64, Linux ARM64, and macOS.
2. Passing tests allow automatic version tagging (`vMAJOR.MINOR.PATCH`, patch by
   default, using `anothrNick/github-tag-action`).
3. The workflow builds the UI, embeds it into four Go binaries, and injects the
   release version, commit, and build date.
4. Optional Apple signing and notarization run before packaging. GitHub Releases
   receive `cortex_<version>_<Linux|Darwin>_<x86_64|arm64>.tar.gz`, checksums, and
   generated release notes.
5. The workflow packs and installs `@colony2/cortex` to smoke-test the published
   download, then publishes it to npm.

GitHub Actions needs permission to write repository contents and npm publishing
access: configure `NPM_TOKEN` or an npm trusted publisher for this repository's
`release.yaml` workflow. `READ_ALL_C2_REPOS` supplies private Go module access
when needed. For macOS signing, configure all five secrets: `MACOS_SIGN_P12`,
`MACOS_SIGN_PASSWORD`, `APPLE_API_ISSUER`, `APPLE_API_KEY_ID`, and `APPLE_API_KEY`.
With none configured, macOS binaries are unsigned; a partial configuration fails
the release. These secret names match c2j.
