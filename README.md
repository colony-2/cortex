# Cortex

Cortex is a web interface for [c2j](https://github.com/colony-2/c2j) recipe jobs.
Submit and inspect jobs, read execution stories and artifacts, restart jobs, and
respond to human input requests. The `cortex` command serves the Go API and the
embedded React UI from one executable.

Cortex connects to a remote [JobDB](https://github.com/colony-2/jobdb) server.
Run c2j workers separately against the same JobDB. Projects in the UI are JobDB
tenants; cells are durable named records in JobDB, with generated IDs and repository locators.

## Recovering a job and navigating goals

You can submit **Continue this goal** while the source is running or its details
are unavailable. The restart service decides whether to accept the request. Restarting
does not cancel the source and may create another live attempt in the same goal.
**Start a new goal** also preserves the source and its siblings. Restart errors stay
beside the retained boundary and patch.

**Cancel source** opens the existing confirmation for that exact job; cancellation
never starts a restart or cancels a sibling. **Refresh source** updates its displayed
details. Cancellation remains pending until exact job detail reports archived storage
or completed scheduler state. You can submit a restart while cancellation remains
pending, once the cancellation request itself has finished. Cancellation success and
restart failure are reported separately; diagnostic text never establishes goal closure.

**Retry saved request** resends the saved ID, boundary, goal mode and patch unchanged,
including after reload or a lost response. It needs no new completion, open-goal or
sibling-absence check. Current authority and request validation still apply. Refresh
and cancellation preserve the saved identity; deliberately submitting a changed
boundary, patch or goal mode creates a new request. The saved request preview is
independent of the current editor.

Current-goal links and **Other attempts** follow successful exact detail's primary
goal. Remembered navigation is only a return destination. Old extra-goal fields can
remain in raw history but create no live membership. Old goal URLs stay literal:
**No retained jobs for this goal** means a successful exhausted read, without proving
closure or deletion. Empty pages with continuation and unavailable reads remain
distinct. Historical attempts, recorded parent/child links and settled result identities
remain separate; a missing exact job never selects a neighbor.

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
Archiving from a job story returns to its Goals list with an “Archived 1 job”
confirmation, preserving the saved list view and filters after success.
Restoring jobs to the inbox is not currently supported. The selected view stays
in the URL; switching views clears pagination and selected jobs.

Search available titles and full prompts at the right of the Cancel/Archive row,
including text beyond the shortened row label and text from other loaded attempts.
Both Goals and Reviews split
search on whitespace and require every term, in any order, within one field of
one loaded attempt. Matching ignores case and includes partial words: `foo bar`
matches `food that i had at the bar`. Click the Goal, Status, Cell, or
Submitted column header to sort ascending or descending; click again to restore
the original order. Each new sort keeps the preceding row order for equal values,
so Submitted descending then Cell ascending keeps newer goals first within each cell.
Only the latest column remains active. Search and sorting run in the browser on the current page.
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
JSON formatting preserves large numbers and duplicate keys. HTML/HTM pages and
standalone SVG diagrams open as static rendered previews, with Rendered / Source
controls and searchable original source. HTML uses an opaque-origin sandbox with a
restrictive content policy; SVG uses an isolated image with fit and zoom. Scripts,
navigation, forms, embedded documents and external/relative dependencies are
disabled. Only validated, bounded embedded raster data images are supported in HTML and SVG.
HTML styles use a conservative layout and typography subset; CSS resources, custom
properties, substitutions, quoted values and advanced selectors are omitted. SVG
filters and excessive references fall back to source with an explanation, as do
invalid SVG, incomplete inputs and unavailable rendering. Markdown links can open
HTML/SVG artifacts; SVG Markdown image embedding remains unsupported. Archives
remain downloads. Unknown binary files have a hex preview.

Previews are read-only and bounded. Text loads up to 2 MiB; structured previews
require complete inputs within 1 MiB and 20,000 lines. Raster previews allow up to
16 MiB and 32 megapixels. Banners identify limited previews, and Download always
returns the complete original bytes. Markdown images load only from unambiguous
attachments in the current review.

On the review page, opening another document adds a browser history entry.
Back/Forward revisits documents while keeping unfinished responses and chosen
uploads in the mounted review. Copying the URL includes `request_id` and
`document_id`, so another authorized reader can open the same current document.
Legacy links select the starting document. Missing attachments show a warning and
fall back to that document; links to a superseded review require **Open current
review** before showing its documents or response form. Reloading or leaving the
review keeps the existing draft behavior.

## Phones and new goals

Phone reviews show previews inline in the **Selected document** region. **Read**
scrolls to the selected preview; document selection and Markdown artifact links
switch its contents. Rendered/Source, source search, SVG fit/zoom and original
Download remain available there. Continue down the same page to the response and
review decisions with your draft preserved. Request details, goal context and
instructions remain available through disclosures. Story artifacts keep their reader.

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

The **Reviews** navigation item lists pending document reviews and ordinary input
requests together. Filter by cells from the loaded queue, search stored titles,
prompts or job IDs, and sort by request time, title or cell. Missing request times
stay last in either direction. Search and controls use loaded data without extra
requests; unavailable historical metadata and document contents are not searched.
The visible/total count reflects filters; the navigation badge counts the full queue.
Controls persist in the URL through reload, opening a request and returning or
responding. Reset restores all cells, blank search and oldest first. On phones,
expand **Filters and sorting** to use the controls.

A waiting job’s Conversation tab shows the review title, instructions, and document
count for both `kind: review` and `review/v2` requests. **Open review** goes directly
to its review page. Ordinary input requests use **Open form** and open inline in
the story.

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

### Reviewing commits

The review v2 reader opens on Documents, beside the document catalog and
discussion. Its Changes tab shows the job's latest recorded main
workspace, its repository, base/head and source, and the complete oldest-first
commit sequence since the base. Click a commit to inspect its changes; Shift-click
or Shift+Arrow selects an inclusive range. From / Through controls also support
touch. All changes compares base to head. Use the existing Unified, Side by side,
Source and file-navigation controls, or download the complete patch. Tabs preserve
the response draft, attachments and document display state.

Refresh changes loads the latest recorded workspace; advancement rejects old
selections. This is committed, persisted code, not a live checkout or a statement
about an earlier check or approval. An in-range merge explicitly disables commit
and range selection; an available All diff remains labeled as an endpoint net
comparison. Empty history, reverted net changes and unavailable objects have
separate states. No recipe opt-in or extra Git artifact is required.

The reader reuses retained c2j bundles in bounded temporary repositories. The
server needs Git on PATH and writable temporary storage. Repository access is
offline by default, including no local-checkout fallback. Missing prerequisites
produce `missing_closure` with object hashes. Embedded deployments may deliberately
configure `cortex.Config.Changes` (`jobinspect.ChangesConfig`) with a trusted
repository policy; browser parameters cannot enable repository access. The
standard command retains the offline default. Ambient Git credentials do not
authorize a fetch.

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
pnpm --filter @colony2/app test:e2e --project=previous-worker recipe-input.spec.ts
node --test npm/cortex/test/*.test.cjs
```

Compatibility tests build the previous c2j worker (`b9d8dae96cb3`) with its
previous JobDB pin in a temporary module, leaving the application module graph
unchanged. The Go suite checks retained success, retry, hook and restart history.
The `previous-worker` browser project produces pending inputs with that worker,
then answers through Cortex and continues with the current worker. Both workers
are built automatically; `make test-worker previous-worker` builds them directly.

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

The Changes acceptance suite uses the embedded production UI and real writer
bundles recorded in native recipe jobs over HTTP/SQLite. Producing checkouts are
deleted before reads. It compares All/single/range downloads with independent Git
oracles, exercises v2 and legacy reviews, desktop/phone selection and workspace
advancement, and keeps
normal review submissions and receipts working. Run it separately on port 18086:

```sh
pnpm --filter @colony2/app exec playwright test --config playwright.changes.config.ts
```

Set `CORTEX_REVIEW_CAPTURE_DIR` to retain individual UI screenshots from that run.

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
