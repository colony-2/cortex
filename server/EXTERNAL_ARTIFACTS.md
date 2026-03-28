# External Artifact Pointers

## Problem

Colony2's artifact system assumes artifacts are local — an op writes data to its `outbox/`, and downstream ops import it from their `inbox/`. The data lives in the c2 artifact store at all times.

But many integrations produce artifacts in external systems: CI runners store test results, cloud storage holds build outputs, remote APIs return downloadable content. Today, consuming these requires the op itself to download everything into the outbox — even if no downstream op ever uses it. This is wasteful, slow, and couples the producing op to knowledge of what consumers need.

## External Artifact Pointers

An external artifact pointer is a c2 artifact record that contains a **URL** pointing to data in an external system rather than data in the c2 outbox. From the recipe author's perspective, external artifacts are identical to regular artifacts — same template syntax, same `inbox`/`outbox` flow, same scoping rules. The difference is internal: the data is fetched on demand rather than eagerly.

**Key principle: nothing is downloaded unless another op imports the artifact.** If no downstream op references an external artifact, it's never pulled. If an op declares it as an input artifact, c2 resolves the URL and materializes the data into the op's inbox at that point.

## Pointer Record

When an op registers an external artifact, the pointer contains:

| Field | Description |
|-------|-------------|
| `key` | The artifact name (becomes the c2 artifact key) |
| `url` | URL to the data. Supports any scheme the engine can fetch: `https://`, `http://`, `file://` |
| `expand` | `true` to extract archives (zip, tar) into a directory, `false` to write content as-is |

The URL is the locator. The scheme determines how to fetch. The `expand` flag determines whether the downloaded content is extracted.

**Examples:**

```
{ "key": "coverage-report", "url": "https://api.github.com/repos/acme/app/actions/artifacts/12345/zip", "expand": true }
{ "key": "test-results",    "url": "file:///tmp/act-artifacts/test-results/",                           "expand": false }
{ "key": "gha-logs",        "url": "https://api.github.com/repos/acme/app/actions/runs/67890/logs",    "expand": false }
```

For GitHub, both artifacts and logs are downloadable via authenticated API URLs. For act (local), artifacts are local paths expressed as `file://` URLs. The same pointer model covers both.

## Lifecycle

### 1. Registration

An op registers external artifact pointers as part of its output. This is lightweight — no data is fetched. The pointer is stored in the artifact namespace alongside regular artifacts.

```
op completes → registers pointer { key: "coverage-report", url: "https://..." }
                                  ↓
                     artifact namespace now contains "coverage-report" as an external pointer
```

### 2. Reference

Recipe authors reference external artifacts the same way as regular artifacts. At this stage, c2 only passes the pointer — no data transfer.

```yaml
artifacts:
  coverage: '${{ sequence.ci.artifacts["coverage-report"] }}'
```

### 3. Materialization

When a downstream op starts and has an external artifact in its `artifacts:` imports, the engine resolves the pointer:

1. Check if the artifact is an external pointer (has a `url` field, no outbox data)
2. Fetch from the URL (`https://` → HTTP GET with auth, `file://` → local copy)
3. Write data to the op's `inbox/<artifact-key>/` directory
4. The op sees a regular directory in its inbox — it doesn't know or care that the data was external

```
downstream op starts → artifact resolver finds external pointer
                     → fetches data from URL
                     → data written to inbox/coverage/
                     → op runs, reads inbox/coverage/ as normal
```

## Authentication

URLs that require authentication (e.g., GitHub API) are fetched using credentials from the op's execution context. The pointer itself does not contain credentials — auth is resolved at materialization time using the same credential chain the engine already uses for git operations and API calls.

## Recipe Author Experience

External artifacts are transparent to recipe authors. They reference them identically to regular artifacts:

```yaml
sequence:
  - id: ci
    op: gha.run
    const: true
    inputs:
      workflow: 'repo://.github/workflows/ci.yaml'
      continue_on_error: true

  # This op imports coverage — c2 materializes the external artifact into inbox
  - id: coverage_check
    op: command_execution
    const: true
    artifacts:
      coverage: '${{ sequence.ci.artifacts["coverage-report"] }}'
    inputs:
      run: |
        # coverage-report is materialized in inbox/coverage/
        cat $INBOX/coverage/lcov.info | npx lcov-summary

  # This op doesn't reference any artifacts — nothing is downloaded
  - id: log_status
    op: command_execution
    const: true
    inputs:
      run: echo "CI status was ${{ sequence.ci.outputs.status }}"
```

In this example, `coverage_check` triggers materialization of the `coverage-report` artifact. `log_status` only uses outputs — no artifacts are fetched.

## Engine Changes

1. **Artifact record.** Add an optional `url` field to the artifact record. When present, the artifact data is not in the outbox — it's at the URL. Existing artifacts (no `url`) continue to work unchanged.

2. **Materialization hook.** Before an op starts, the artifact resolver checks if any imported artifacts are external (have a `url`). If so, it fetches the data and writes it to the op's inbox. This happens after the inbox directory is created but before the op's process is launched.

3. **Fetch by scheme.** `https://` and `http://` → HTTP GET (with auth from execution context). `file://` → local file copy. Additional schemes can be added later if needed.

4. **Failure handling.** If materialization fails (network error, 404, auth failure), the op fails with a clear error indicating which artifact could not be resolved and why. The external pointer itself is preserved for debugging.

5. **Failure preservation.** External artifact pointers are preserved on op failure (same as regular artifacts). The pointers remain valid for debugging even if the producing op fails — the data may still be retrievable from the external system.

## Scope and Non-Goals

External artifact pointers are specifically about **lazy data retrieval via URL**. They do not change:

- **Artifact scoping rules.** External artifacts follow the same scope crossing rules as regular artifacts — they must be explicitly surfaced through `outputs:` blocks to cross node boundaries.
- **Artifact naming.** The `key` is the artifact name in c2's namespace, same as regular artifacts.
- **Template syntax.** `${{ node.artifacts["key"] }}` works the same way.
- **Outbox artifacts.** Ops can still write to their outbox normally. An op can produce both outbox artifacts and register external pointers.

## Design Decisions

1. **No caching.** Each materialization fetches from the URL independently. No deduplication across ops importing the same external artifact. Caching can be added later if it becomes a performance issue.

2. **Registration API.** The existing artifact registration API is extended to accept two artifact types as a closed interface: **outbox artifacts** (data in the outbox, existing behavior) and **external artifacts** (URL pointer, new). No open-ended type system — just these two concrete types. Ops register external artifacts through the same API they use for outbox artifacts, specifying the URL instead of outbox data.

3. **Lazy passthrough.** When an external artifact is surfaced through `outputs:`, the output carries the pointer — not materialized data. Materialization only happens when an op actually imports the artifact into its inbox. There is no reason to download something that's only being returned as an output reference.

4. **Multi-file artifacts and expansion.** External URLs may point to archives (zip from GitHub, tarballs). The pointer record includes an `expand` field:
   - `expand: true` — the engine extracts the archive into `inbox/<key>/` as a directory
   - `expand: false` — the engine writes the downloaded content as-is to `inbox/<key>`

   Ops registering external artifacts choose the appropriate mode. For example, GHA artifacts from GitHub are zip archives and should be expanded; a single log file should not.
