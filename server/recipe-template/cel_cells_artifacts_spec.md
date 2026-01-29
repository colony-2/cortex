# CEL Custom Function Spec: cells & artifact sets

## Goals
- Expose a CEL function that returns the complete list of cells for the current project so recipes can reason about topology, defaults, and paths.
- Add a small, composable set of CEL helpers for building and organizing artifact collections (normalize, concat, filter, extract fields) without forcing every template to reimplement the same glue.

## Non-Goals
- Implement new persistence or APIs in `server/cell`; reuse the existing service/store.
- Download or materialize artifact contents inside CEL; helpers work only with metadata already available in memory.
- Change how artifacts are stored on `StepOutput`; the helpers are read-only utilities on top of existing data.

## Function Signatures
### `cells`
- `cells()` → `list<cell>`
- `cells(filter)` → `list<cell>` where `filter` is an optional map with fields:
  - `names` (`list<string>`): restrict to named cells.
  - `name_prefix` (`string`): include cells whose name starts with the prefix.
  - `include_deleted` (`bool`, default `false`): include soft-deleted rows.
- Output element shape (`cell`):
  ```json
  {
    "id": "string",
    "name": "string",
    "description": "string",
    "working_path": "string",
    "default_recipe": "string|null",
    "git_repo_name": "string|null",
    "git_branch": "string|null",
    "dependencies": ["cell_name", ...], // outbound deps by name, empty list when none
    "project_id": "string"
  }
  ```

### Artifact helpers (all work on `list<swf.Artifact>` after normalization)
- `artifact_set(...inputs)` → `list<artifact>`
  - Accepts any mix of: single `artifact`, `list<artifact>`, `map<string,artifact>`, `StepOutput` (uses `.Artifacts`), `RunOutput` (uses `.Artifacts`).
  - Flattens/normalizes to a list, preserving order of arguments and deterministic map iteration (keys sorted).
- `artifact_concat(a, b, ...)` → `list<artifact>` (alias for `artifact_set`).
- `artifact_filter(arts, opts)` → `list<artifact>`
  - `opts` map keys (all optional):
    - `name_prefix` (`string`)
    - `name_suffix` (`string`)
    - `name_contains` (`string`) — substring match (case-sensitive).
    - `name_regex` (`string`, re2) — “grep-like” pattern; compile failure is a CEL error.
    - `min_size` (`int`, bytes) / `max_size` (`int`) — size range; both inclusive when both provided.
    - `has_key` (`bool`): keep only artifacts with a persisted `ArtifactKey`.
  - Multiple criteria AND together; missing/unknown size is treated as `-1` and only fails `min_size` when `min_size > -1`.
- `artifact_names(arts)` → `list<string>` (calls `Name()`).
- `artifact_keys(arts, on_missing="error|skip|null")` → `list<ArtifactKey|null>`
  - Default `on_missing="error"` raises a CEL error if any artifact key is unavailable; `skip` drops those entries; `null` returns `null` in place.
- `artifact_unique(arts, by="key|name")` → `list<artifact>`
  - `by="key"` dedupes using `ArtifactKey` when available, falling back to name; `by="name"` dedupes by `Name()` only.

## Behavior
### `cells`
- Source of truth is `cell.Service.ListCells` scoped to the project ID in `context.workflow.project_id`.
- When `project_id` is missing or empty, return a CEL error: `cells: project_id is required in context.workflow.project_id`.
- Fetch dependencies via `ReplaceDependencies/ListDependencies` equivalent and materialize them as **names** (matching the service’s name uniqueness per project). If a dependency name cannot be resolved, omit it and log at `warn` in Go (not a CEL error).
- Results are sorted by `name` ASC for determinism.
- Values are cached per `ResolutionContext` (subsequent `cells()` calls return the same slice without re-querying).

### Artifact helpers
- Normalization (`artifact_set`) treats `null` inputs as no-op and ignores non-artifact primitives with a CEL error: `artifact_set: unsupported input type <type>`.
- Filtering uses lightweight metadata only: `Name()`, `Size()`, and `ArtifactKey()` (when needed). Helpers must **not** open streams or read bytes.
- Size filters treat unknown size (`-1`) as failing `min_size` when `min_size > -1`, and failing `max_size` only when `max_size < -1` (i.e., unknown size passes `max_size` checks).
- All helpers return empty lists, not `null`, when no matches remain after filtering.
- Functions operate on live `swf.Artifact` interfaces to remain compatible with downstream Go consumers (no conversion to DTOs). They never mutate the artifacts.

## Integration Points
- Wire `cells` and artifact helpers during CEL env creation in `template_resolver.go`, alongside `jq/json_stringify/string` options.
- Pass a `cell.Service` (or read-only interface with `ListCells(ctx, filter)`) into `ResolutionContext` via `ResolutionOptions`; fail fast during resolver construction when missing.
- Add an LRU or per-context memoization to avoid multiple DB hits for `cells()` within one template evaluation.
- Keep bindings pure: no I/O beyond the `cell` read and artifact metadata method calls.

## Error Messages (exact)
- `cells: project_id is required in context.workflow.project_id`
- `cells: failed to list cells: <err>`
- `artifact_set: unsupported input type <type>`
- `artifact_filter: expected list<artifact>`
- `artifact_keys: artifact key unavailable` (when `on_missing=error` and `ArtifactKey()` returns `ErrArtifactKeyUnavailable`)

## Example Usage
```yaml
inputs:
  project_id: "{{ context.workflow.project_id }}"
steps:
  - id: list
    op: debug
    inputs:
      cell_paths: "{{ cells().map(c, c.working_path) }}"
      active_cells: "{{ cells({name_prefix: 'web/'}) }}"
  - id: aggregate
    op: pack
    inputs:
      artifacts: "{{ artifact_filter(artifact_concat(sequence.build.artifacts, states.test.artifacts), {name_suffix: '.tar.gz', has_key: true}) }}"
      artifact_names: "{{ artifact_names(sequence.build.artifacts) }}"
```

## Testing
- Unit tests in `pkg/template/template_resolver_test.go` (or new file) covering:
  - `cells()` happy path returns sorted list with dependencies populated.
  - Missing `context.workflow.project_id` yields CEL error.
  - Filters: `names`, `name_prefix`, `include_deleted`.
  - Artifact normalization from single, list, map, `StepOutput`, `RunOutput`.
  - `artifact_filter` combinations (prefix + size) and regex failure cases.
  - `artifact_keys` behavior for persisted vs non-persisted artifacts across `on_missing` modes.
  - `artifact_unique` by key vs by name.
- Integration test (similar to `template_interpolation_integration_test.go`) that wires a fake `cell.Service` and stub artifacts to verify CEL registration and end-to-end template resolution.

## Rollout Notes
- Requires injecting `cell.Service` into template resolution; add a zero-value option that fails loudly when not provided to avoid silent partial behavior.
- No schema or migration work needed; reads existing cell data.
- Artifact helpers are backward compatible: they only add new functions and do not change current interpolation semantics.
