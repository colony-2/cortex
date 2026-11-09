# UI and linked library build information

The account menu shows the email used by Cortex's existing cookie-based login. About reports the loaded frontend bundle independently of the API process. c2j and JobDB identify libraries linked into that API process, not remote workers or services.

For release or source archive builds, supply the Cortex release version and full source revision explicitly:

```sh
CORTEX_UI_VERSION=1.2.3 CORTEX_UI_COMMIT=<source-revision> make web-embed
bash scripts/go.sh build -buildvcs=false -o build/cortex ./server/cmd/cortex
```

The release workflow supplies its resolved VERSION and COMMIT to this same embedding step. The static web package version is never used as a release version. Without explicit inputs, Vite records the checkout revision and appends `-dirty` when Git detects source changes; without Git provenance, the commit is unavailable. Ignored generated bundles do not affect dirty detection.

`GET /api/build-info` returns nullable version and commit fields for effective linked c2j and JobDB modules. Go build metadata provides module versions; pseudo-versions provide recorded revision prefixes. Tagged versions alone cannot provide commits. Local replacements display `local/development` and no revision or filesystem path. Missing metadata remains unavailable. No network resolution is performed.

Focused browser verification uses a local toy JobDB, API test server, embedded binary and Vite. Build the embedded bundle with the explicit review inputs used in `playwright.account.config.ts`, then build `build/cortex` with `-buildvcs=false`. Run:

```sh
CORTEX_ACCOUNT_BINARY=./build/cortex pnpm --filter @colony2/app exec playwright test --config playwright.account.config.ts
```

Set CORTEX_SCREENSHOT_DIR to an absolute evidence directory for screenshots. Chromium must be on PATH. This check does not need Docker or a deployed service.

The standard `pnpm test:e2e` suite supplies the same explicit UI build inputs from `web/app/tests/helpers/uiBuild.ts` to Vite and the production embedding step. It runs the account checks in both Chromium development and production projects. The test asserts these exact compiled values even when library metadata differs.
