# Cortex

Cortex is a web interface for [c2j](https://github.com/colony-2/c2j) recipe jobs.
Submit and inspect jobs, read execution stories and artifacts, restart jobs, and
respond to human input requests. The `cortex` command serves the Go API and the
embedded React UI from one executable.

Cortex connects to a remote [JobDB](https://github.com/colony-2/jobdb) server.
Run c2j workers separately against the same JobDB. Projects in the UI are JobDB
tenants; cells are durable named records in JobDB, with generated IDs and repository locators.

## Goal list filters and search

The Status column's filter matches the badges shown for jobs, including Succeeded,
Failed (application/system), Timed out, and Cancellation requested. Multiple
selections match any selected status. All statuses start checked; uncheck individual
items or use Select all / Select none, then Apply. Selecting none shows no jobs.
Finished (outcome unavailable) matches
completed jobs with no stored outcome; retrying jobs retain their scheduling
status even if a previous attempt failed. The Cell column uses the same checkbox
controls, with all cells selected initially. Choices come from the JobDB registry,
including cells with no jobs and retired cells for historical inspection. Cell and
status filters run on the backend before pagination. URLs store stable cell IDs;
renames preserve selections and refresh displayed names. Missing required cell
fields produce a protocol error rather than an inferred identity.

The **Inbox / All jobs** selector controls visibility independently of execution
status. Inbox is the default and includes finished jobs until they are archived.
**Archive selected** removes jobs from the inbox and cancels unfinished work;
history and artifacts remain available through All jobs and goal overview.
Restoring jobs to the inbox is not currently supported. The selected view stays
in the URL; switching views clears pagination and selected jobs.

Search full prompts at the right of the Cancel/Archive row, including text beyond the shortened row
label and prompts from other loaded attempts. Click the Goal, Status, Cell, or
Submitted column header to sort ascending or descending; click again to restore
the original order. Search and sorting run in the browser on the current page.
Search, sorting, and filters persist in the URL when opening a goal and returning.

The jobs API accepts repeated `displayStatus` parameters for these states, for
example `?displayStatus=READY&displayStatus=failed_app`. The existing `status`
parameter still filters raw scheduling states; the two parameters cannot be
combined. Display filtering scans compact JobDB pages without loading job
details. Each request scans at most 20 backend pages, so a sparse or empty result
can still have a `next_page_token`; use **Next jobs** to continue.
Repeated `cellNames` parameters (for example `?cellNames=alpha&cellNames=beta`)
match jobs owned by either named cell before pagination. Repository selectors and
the old `cell` query parameter are no longer accepted.
`includeHidden=true` includes jobs archived from the inbox; omitted or `false`
keeps the inbox view. It composes with status, cell and goal filters before
pagination. Goal overview includes archived attempts.

## Review document previews

Reviews support Markdown, unified and side-by-side patches, raster images, JSON,
JSONL, logs and source files. Source views provide line numbers, search and wrapping.
JSON formatting preserves large numbers and duplicate keys. HTML and SVG appear
as source; archives remain downloads. Unknown binary files have a hex preview.

Previews are read-only and bounded. Text loads up to 2 MiB; structured previews
require complete inputs within 1 MiB and 20,000 lines. Raster previews allow up to
16 MiB and 32 megapixels. Banners identify limited previews, and Download always
returns the complete original bytes. Markdown images load only from unambiguous
attachments in the current review.

## Phones and new goals

Phone reviews list documents without embedding their full contents. **Read** opens
a full-screen reader with one vertical scrolling area; **Back to review** returns
to the same response draft and attachments. **Document options** provides source,
document selection, and download. Request details, goal context, and instructions
remain available through disclosures.

Below 768px, navigation opens from **Open navigation**, and goals, attempts, and
cells use cards. Goal cards start with a two-line prompt, status, cell, and attempt
count; expand a card for submission time, related links, and inspection.
**Filters and sorting** reveals search and the existing controls. The phone
filter dialogs retain Select all / Select none / Apply;
search and sorting keep the same current-page and URL behavior as the desktop list.
Story and details stack vertically, with a **View selected details** jump link.

**New goal** selects an active registered cell. A single cell filter seeds the
picker, and the selected ID is resolved to its current name before submission.
Existing-cell submission uses the registry repository. **New cell** mode requires
an explicit name and repository. It registers the cell first, then submits the goal.
If submission fails, the cell remains registered; retrying reuses the completed
registration and the same job ID for an unchanged request.

## Cells and creation

The Cells page lists the registry with active/retired/all filters,
exact-name/alias lookup, and pagination. **Create cell** requires a name and
repository, with optional description and JSON metadata. It registers a cell
without submitting work; creating or publishing its Git repository is separate.
Hosted repository locators, HTTPS URLs, and supported SSH forms are normalized by
JobDB. Local filesystem paths cannot serve as cell identity.

A cell's detail page shows its current name, repository, state, aliases and metadata.
**Goals for this cell** opens the goals list filtered by its stable ID;
**New goal in this cell** preselects it for submission. Retired cells remain
inspectable but cannot receive new jobs. `root` is an ordinary cell name with no
special designation or creation restrictions.
Cell editing and retirement UI are deferred.

Job rows display current cell names while retaining each job's submitted repository
snapshot. Changing a registry repo does not change historical source links.

This release requires compatible JobDB/c2j workers and fresh execution stores.
Old jobs, pagination cursors and saved execution selections are unsupported; deploy
the server and bundled UI together. See the
[cell upgrade plan](docs/Cortex-Cell-Registry-Upgrade-Plan.md) for cutover details.
Authentication and user-specific permissions are outside this migration.

## Chapter downloads

On a job's story page, **Download chapters** saves a JSON inspection export of
all persisted chapters for that job, including retained attempts and artifact
descriptors. Running jobs export the records currently available. Child jobs
and artifact file contents are excluded; individual artifacts can still be
downloaded from the story. This export is not an import or restore format.

## Document reviews

The **Reviews** navigation item lists pending input forms marked `kind: review`.
Reviews are excluded from **Pending Inputs**, and each section has its own count.
Open a review to switch between its documents, view Markdown rendered or as its
exact source, and answer the recipe's questions. Original documents are read only;
file-upload questions attach separate response documents (one file per question,
32 MiB total HTTP request limit). Other document formats can be downloaded.

Responses use c2j's request identity, validation, and receipts. A job's story links
to its review. Completed reviews leave the pending list; historical review lookup
is not available yet. See the [recipe author guide](GUIDE-Review-Recipe-Authors.md)
and [c2j API follow-up requirements](docs/C2J-Review-API-Requirements.md).

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
[minimal fixture](web/app/tests/fixtures/cell/.c2/config.yaml).

| Option | Environment variable | Default |
| --- | --- | --- |
| `--addr` | `CORTEX_ADDR` | `:8080` |
| `--jobdb` | `C2J_JOBDB` | `jobdb` from `.c2/config.yaml` |
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
`.c2/config.yaml`. Cortex calls c2j's `pkg/config.LoadProjectConfig` and
`ProjectConfig.JobDBURI` directly, including support for command-valued settings:

```yaml
jobdb: http://localhost:9047/c2
# Alternatively:
# jobdb:
#   command: your-command-that-prints-a-jobdb-uri
```

If your project still uses `.c2j/config.yaml`, move it to `.c2/config.yaml`;
c2j no longer discovers configuration in the old directory.

Run `cortex` from that cell or a subdirectory, or select the configuration directory
with `--working-dir`. Cortex requires a remote HTTP(S) JobDB server; `embed:///`
is not supported.

For migration, `--jobdb-url` remains a deprecated alias accepting either the new
URI or a server-only URL with `--tenant-id`. Legacy `JOBDB_URL` is used only when
neither a connection flag nor `C2J_JOBDB` is set, before consulting the config.
`CORTEX_TENANT_ID` applies only to legacy server-only URLs (default `1`).

Opening the UI uses the tenant in your configured JobDB URI. To override it,
select a tenant in the header, use `/?tenantId=42`, or open a tenant route such as
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

The c2 execution tools in [`.c2/config.yaml`](.c2/config.yaml) declare Go, Bash,
Make, Node.js, pnpm, Playwright, Chromium, and FFmpeg. Nix supplies Chromium's
runtime libraries. Linux workers must also expose fonts through a usable
Fontconfig configuration; on minimal workers, set `FONTCONFIG_FILE` to a
configuration referencing the provisioned fonts and a writable cache directory.
Having font files in the Nix store alone does not configure font discovery.
The browser tests use Chromium and FFmpeg from
`PATH`, without downloading browsers or installing system packages.

Outside c2, either provide Chromium and FFmpeg on `PATH`, or install Playwright's
browser before running the tests:

```sh
PLAYWRIGHT_BROWSERS_PATH="${CORTEX_PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}" \
  pnpm --filter @colony2/app exec playwright install --with-deps chromium
```

Install full Chromium as well as its headless shell: the native 200% zoom test
loads a temporary extension in a fresh browser profile and verifies
`chrome.tabs.setZoom`/`getZoom`, layout dimensions, and device-pixel ratio. The
extension exists only during testing and is never bundled into the app.

The Nix Chromium package targets Linux workers; macOS developers can use the
Playwright installation command above.

The image-review recipes also require a full `ffmpeg` executable on `PATH`,
with PNG, JPEG, GIF, WebP, AVIF and BMP encoding support. Playwright's bundled
video recorder does not satisfy this requirement. CI installs the pinned
`imageio-ffmpeg==0.8.1` binary distribution for Linux x64/ARM64 and macOS ARM64
and checks all fixture encoders before running the suite. Local Nix workers
already declare full FFmpeg in `.c2/config.yaml`.

Setting `CORTEX_PLAYWRIGHT_BROWSERS_PATH` selects that Playwright installation
even when system browsers are on `PATH`, as in CI. Otherwise, system executables
are used only when both Chromium and FFmpeg are available; an incomplete pair
falls back to Playwright's browser cache.

The suite manages its own servers on ports 15173 and 18081–18083. It covers
submission, stories, tenants, cells, and input events. The production tests use
HTTP JobDB backed by temporary SQLite storage.

The recipe-input tests build the pinned c2j command and execute real Git-backed
recipes with single-question and multi-field input requests, plus document
reviews. The review test covers rendered/source Markdown, multiple documents,
stale and invalid responses, separate response attachments, and receipts. They submit jobs
through the browser, observe pending requests in a second tab over SSE, check
required fields and page reloads, submit answers while the worker is stopped,
and restart it to verify the exact recipe output and completed story. They also
cover cancellation from another client, rejected late answers, and consecutive
prompts in one job without reloading either tab. Each test has a separate tenant
and temporary Git repository; the fixture cell's Git URL
is mapped to that repository by a test-only Git transport shim in the worker's PATH. No API or EventSource
mocks are used in these tests. Worker logs and recipe outcomes are attached to
the Playwright report in `web/app/playwright-report`.

The timeout regression also runs against the real worker. It verifies that an
unanswered input expires, clears pending prompts in both browser tabs, and rejects
late submissions.

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
