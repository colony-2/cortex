# Astore artifact capture for recipe-worker

## Goal
Add an `astore` lifecycle around every activity invocation so that artifacts produced by an op are captured automatically and merged with any artifacts the op already emits. This mirrors the existing git restore/persist wrapping, but for ephemeral artifact storage.

## Behavior in activity registry
- Wrap each task worker with an `astore` middleware: **create** before invoking the op, **persist** after a successful op. No restore step.
- Creation: allocate a unique temp directory for the op (0700) under a configurable root.
- Invocation: the op can write files into the provided artifact dir. Existing behavior for `outputArtifact` on `OpDependencies` stays unchanged.
- Persist: after the op returns successfully, walk the artifact dir, build `swf.Artifact` objects, and append them to the task’s output artifact list along with the op-supplied `outputArtifact` entries. Skip on op error to avoid masking failures.
- Cleanup: remove the temp directory in a `defer`, even when persist fails.
- Ordering with git: git restore happens first; `astore` create happens after restore and before invoking the op; git persist runs after op output is produced but before artifact persistence so we can include any artifacts that depend on git state.

## Package design (`server/astore` module)
- New standalone module at `./server/astore` so other services can reuse it; recipe-worker consumes it as a dependency.
- API:
  - `Create(ctx, opID string, in []swf.Artifact) (root string, inbound string, outbound string, cleanup func(), err error)` – uses `os.MkdirTemp` under a root (`ASTORE_ROOT`, default `os.TempDir()`), creates sibling subdirs `inbound/` and `outbound/`, expands inbound artifacts into `inbound/`, and provides a cleanup function.
  - `Persist(ctx, outbound string, limits Limits) ([]swf.Artifact, error)` – walks the `outbound/` directory tree, skips symlinks, applies limits, and streams artifacts, treating the tree as a directory-based artifact set (relative paths preserved under outbound).
- Limits (configurable via env/worker config with sane defaults):
  - Max total bytes, max file count; exceeding limits fails persist with a clear error.
  - Exclude patterns such as `.git`, `.DS_Store`, editor swap files.
- Artifact creation:
  - Only regular files are supported; symlinks are rejected with an error.
  - Capture relative path (slash-separated), size, and digest (e.g., sha256). No content type/media type is set.
  - Treat outbound artifacts as a directory snapshot rooted at `outbound/`: each file becomes a streamed `swf.Artifact` entry with its relative path in outbound.
  - Two-pass stream per file: first pass computes digest without loading whole file; second pass streams the file into the artifact uploader/constructor (digest must be known before upload). Avoid buffering entire files in memory in both passes.
- Error handling: an empty outbound dir yields an empty slice (not an error); persist errors bubble to the activity invocation; cleanup still runs.

## Integration points
- Activity registry wrapper composes:
  1. git restore
  2. astore create (hydrate inbound, set up outbound)
  3. op invocation (op visibility into dirs TBD; separate ticket)
  4. git persist
  5. astore persist (scan outbound, append to artifacts)
- Task data assembly uses `swf.NewTaskData(opOutput, append(opDeps.OutputArtifacts, astoreArtifacts...)...)`.

## Testing
- **Unit tests (`server/astore`)**
  - Create returns unique, 0700 dirs under configured root.
  - Inbound artifacts expand into `inbound/` with correct contents and relative paths.
  - Persist on empty `outbound/` returns no artifacts.
  - Persist captures file path, size, digest; uses two-pass streaming (digest then upload) and does not spike memory on large files.
  - Excludes hidden/VCS patterns.
  - Symlinks are rejected.
  - Limit enforcement for size and count.
- **Integration tests (activity registry wrapper)**
  - Outbound empty path yields empty artifacts and succeeds.
  - Inbound artifacts are hydrated into `inbound` (even if ops do not yet read them).
  - Op emitting only `outputArtifact` still works.
  - Failure path: if op returns error, persist is skipped and no artifacts added.
  - Parallel ops get isolated temp dirs (no crosstalk).

## Configuration and docs
- New env/config knobs: `ASTORE_ROOT`, `ASTORE_MAX_BYTES`, `ASTORE_MAX_FILES`, `ASTORE_EXCLUDES`.
- Document astore module usage in `VIBETHIS.md`/developer docs (symlinks unsupported; two-pass streaming; no media type). Op directory exposure is tracked in a separate ticket.

## Work plan
1) Introduce `server/astore` module with create/persist APIs, limits, two-pass streaming, inbound expansion, and unit tests.
2) Wire recipe-worker activity registry to consume `server/astore`: call `Create`/`Persist`, hydrate inbound, append outbound artifacts into task output, and add integration tests (op dir exposure handled separately).
