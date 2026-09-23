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
cortex --jobdb-url http://127.0.0.1:9047 --working-dir /path/to/cell
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
| `--jobdb-url` | `JOBDB_URL` | Required |
| `--working-dir` | `CORTEX_WORKING_DIR` | Current directory |
| `--tenant-id` | `CORTEX_TENANT_ID` | `1` |
| `--cors-origins` | `CORTEX_CORS_ORIGINS` | Empty; comma-separated origins |

Use `cortex --help` for command options and `cortex version` or `cortex --version`
to identify the release. Source builds report `dev`.

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
./build/cortex --jobdb-url http://127.0.0.1:9047 --working-dir /path/to/cell
```

`make build` builds the web packages, copies their output into the Go embed
directory, and produces `build/cortex`. A plain `go build` does not build the UI.
Go dependencies are pinned in [`server/go.mod`](server/go.mod); no sibling
checkout is required.

## Development

Start the API and web dev server in separate terminals:

```sh
JOBDB_URL=http://127.0.0.1:9047 CORTEX_WORKING_DIR=/path/to/cell pnpm dev:api
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
| `server/cmd/uitestserver` | In-memory test API and HTTP test JobDB |
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
node --test npm/cortex/test/*.test.cjs
```

Playwright installs Chromium if missing. On a fresh Linux host, first run
`pnpm --filter @colony2/app exec playwright install-deps chromium`.
The suite manages its own servers on ports 15173 and 18081–18083 and uses an
isolated fixture cell. It covers submission, stories, tenants, cells, and input
events without running recipe workers. Reports are in `web/app/playwright-report`.

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
