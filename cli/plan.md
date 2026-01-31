# Colony2 CLI Pattern Proposal

## Goals (phase 1 scope)
- Provide a single CLI that targets the Colony2 REST OpenAPI (server/openapi bindings).
- Cover first-wave operations: ticket list/create, recipe create/update, workflow list, input-request responses, cell list.
- Deliver consistent UX: shared flags, predictable subcommand shapes, tabular **and** JSON I/O.
- Keep implementation small: thin wrappers over generated client; reusable helpers for output, auth, and error handling.

## Architectural shape
- **CLI framework**: `cobra` for command tree & help; optional `viper` only if config file live-reload is needed (otherwise manual config load).
- **Package layout**
  - `cmd/colony2/main.go` – bootstrap, bind root flags, wire dependencies.
  - `internal/config` – load/merge config file (`~/.config/colony2/config.yaml`), env vars (`COLONY2_*`), flags. Fields: `api_url`, `token`, `output` (table|json), `pager` (bool), `timeout`.
- `internal/client` – wraps generated OpenAPI client creation; injects auth header via request editor; adds retry/backoff and per-command timeout context.
  - `internal/output`
    - `printer` interface with implementations: `tablePrinter`, `jsonPrinter`.
    - Column definitions per resource (`ticket`, `recipe`, `workflow`, `inputRequest`, `cell`) with sensible defaults and overridable via `--columns`.
    - Helper to stream JSON (for piping) or render `go-pretty/table` (or std `tabwriter` if we avoid deps).
  - `internal/prompt` – only for commands that need interactive payload editing or confirmation.
  - `internal/cmd/<resource>.go` – each resource registers subcommands and uses shared helpers.
- **Error handling**: central `fail(err)` that unwraps HTTP status + body excerpt; consistent exit codes (0 success, 1 user/validation, 2 transport).

## Command design (shared patterns)
- Common flags on root: `--api-url`, `--token`, `--output (table|json)`, `--timeout`, `--trace` (emit raw request/response to stderr for debugging).
- Pagination flags (where supported by API): `--page`, `--page-size`.
- Input flags for create/update: `--file` (JSON/YAML), `--stdin` (default if `-`), or `--field name=value` repeatable to override/patch parsed payload.
- Output selection: default `table`; `--output json` returns raw OpenAPI models (no post-processing). `--columns` to choose subset for tables; `--quiet` to suppress headers.
- Authentication: bearer token header `Authorization: Bearer <token>` from flag/env/config. Validate token presence before network call.

## Resource-specific shape
- **Tickets**
  - `colony2 ticket list [--status ... --assignee ... --workflow-id ...]`
  - `colony2 ticket create (--file f|--stdin|-|--field key=val)`; echo created ticket id; optional `--json` output.
- **Recipes**
  - `colony2 recipe create --file recipe.yaml|json` (uses OpenAPI `Recipe` model); validate locally against schema if present.
  - `colony2 recipe update <recipe-id> --file ...` (PATCH/PUT per API semantics); support `--set key=val`.
- **Workflows**
  - `colony2 workflow list [--recipe-id ... --state ... --cell ... --owner ...]`; table columns: id, recipe, state, created, updated.
- **Input requests**
  - `colony2 input-request list [--workflow-id ...]`
  - `colony2 input-request respond <request-id> (--file ... | --stdin | --field key=val)`; confirm before send unless `--yes`.
- **Cells**
  - `colony2 cell list [--type ... --owner ...]`; include columns id, name, path, type, dependencies count.

## Output strategy
- Table: use `go-pretty/table` (add dep) for alignment + color toggle; fallback to plain ASCII for scripting when `--no-color` or `TERM=dumb`.
- JSON: marshal the OpenAPI model or a view struct; support `--pretty` for human-reading.
- Error bodies: if API returns JSON error, pretty-print to stderr; otherwise include status + first 256 bytes.

## Testing approach
- Unit: command handlers accept injected `ClientInterface` (from generated client) allowing mocked responses. Table rendering golden tests under `testdata/`.
- Integration (optional later): env-driven tests hitting a dev endpoint; gated by `COLONY2_E2E_URL`.

## Delivery steps
1) Add deps (`cobra`, `go-pretty/table`, maybe `viper`) and scaffold `cmd/colony2/main.go` + root wiring.
2) Implement `internal/config`, `internal/client`, `internal/output` with shared flag binding.
3) Build resource commands above, starting with read-only (`list`) then create/update/respond.
4) Add tests for command handlers and output formats; wire CI job if repo uses CI.
