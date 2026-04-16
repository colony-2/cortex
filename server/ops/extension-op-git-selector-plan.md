# Extension Op Selector-in-`op` Plan

## Goal

Make extension ops directly referenceable from recipes without preregistration, while keeping the existing `op:` field.

Under this model, `op` can be either:

1. a bare registered core op name
2. an extension selector

Examples:

```yaml
- id: ask_user
  op: input
  inputs:
    schema:
      type: object
```

```yaml
- id: implement
  op: git+https://github.com/acme/platform-ops.git//.colony2/ops/codex.exec@main
  inputs:
    prompt: "Implement the approved plan."
    worktree_path: "{{ context.environment.worktree_path }}"
    workdir_path: "{{ context.environment.workdir }}"
    artifact_inbox_path: "{{ context.environment.inbox }}"
    artifact_outbox_path: "{{ context.environment.outbox }}"
    cell_relative_path: "{{ context.workflow.cell_path }}"
```

This preserves the current recipe shape and reuses the selector conventions already appearing elsewhere in the system.

## Why This Is Better Than `uses:`

I think this is the cleaner direction given your constraint.

Reasons:

- it preserves the current recipe surface
- it avoids adding a second reusable-execution syntax
- it aligns with the fact that this system already uses git selectors in other places
- it lets us parse `op` in a deterministic order:
  1. exact registered op name
  2. valid selector
  3. otherwise error

The main tradeoff is that `op:` is no longer "registry-only." But that is a smaller conceptual shift than introducing a separate `uses:` node family.

## Recommended Selector Shape

Because you said "we use git selectors elsewhere," I recommend using the existing canonical git-selector form as the primary remote syntax:

`git+<scheme>://<repo-location>//<repo-relative-op-dir>@<git-ref>`

Example:

`git+https://github.com/acme/platform-ops.git//.colony2/ops/codex.exec@main`

This is better than inventing a GitHub-only shorthand here because:

1. the repo already has a real parser/resolver for canonical git selectors in recipe root loading
2. it avoids ambiguity with bare op names
3. it keeps the selector grammar uniform across features

## Optional Local Selector

If you want same-repo ergonomics later, it is safe to add:

`./.colony2/ops/codex.exec`

That can also live in `op:` because it does not overlap with bare registered names.

But I would not block the initial implementation on local shorthand. The first version can be:

- bare registered name
- canonical git selector

That gives a simpler parser and cleaner schema story.

## Why Canonical `git+...` Helps the Schema

This is an important advantage of your proposal.

If extension selectors are canonical git selectors, the static recipe schema can still distinguish the two `op` cases:

1. known registered op names
2. strings beginning with `git+` (and later maybe `./`)

That means the schema does not need to fall back to "any string."

If we instead used GitHub-style shorthand like:

`github.com/acme/platform-ops/.colony2/ops/codex.exec@v1`

then `op` becomes much harder to distinguish from ordinary dotted op names at schema time.

So if `op:` is going to multiplex bare names and selectors, canonical `git+...` is the safer design.

## Current Constraint

Today the recipe system assumes `op:` resolves through the global registry:

- parse-time unknown-op validation comes from the registry
- schema generation enumerates registered ops

Relevant code:

- [ops.go](/src/server/recipe-core/pkg/ops/ops.go)
- [schema.go](/src/server/recipe-core/pkg/recipe/schema.go)
- [node.go](/src/server/recipe-core/pkg/recipe/node.go:74)

To support selector-backed `op:` values, that assumption needs to be widened to:

1. `op` is a registered core op name, or
2. `op` is a valid extension selector

## Proposed Parse / Compile Semantics

For each node:

1. Read `op` as a string.
2. If it exactly matches a registered op, keep the existing path.
3. Else if it parses as a valid extension selector, resolve and load the extension bundle.
4. Else return the current unknown-op style error.

This keeps bare names as the fast path and makes selectors an extension of the existing field, not a replacement.

## Supported `op` Forms

## 1. Bare registered core op

Example:

```yaml
op: input
```

Semantics:

- exact match against the current global registry
- current behavior unchanged

## 2. Canonical remote git selector

Example:

```yaml
op: git+https://github.com/acme/platform-ops.git//.colony2/ops/codex.exec@main
```

Semantics:

- validate selector
- resolve mutable ref to concrete commit
- record resolved commit-pinned selector
- materialize the target extension bundle

## 3. Optional same-repo local selector

Possible later form:

```yaml
op: ./.colony2/ops/codex.exec
```

Semantics:

- resolve relative to the caller repo root
- bind to the same effective repo+commit as the caller recipe

Again, I would treat this as optional after the canonical remote path works.

## Bundle Layout

The selector points to an op directory.

Recommended layout:

```text
.colony2/ops/codex.exec/
  op.yaml
  run
  ...
```

`op.yaml` should define:

- `name`
- `description`
- `version`
- `run` or `command` / `args`
- `working_directory`
- `env`
- `timeout`
- `input_schema`
- `output_schema`
- `sandbox`

Suggested sandbox block:

```yaml
sandbox:
  type: host | shai
  shai:
    source: inline | file | cell
    config_path: .shai/config.yaml
    inline_config:
      type: shai-sandbox
      version: 1
      image: ghcr.io/colony-2/shai-mega
```

## Complete Examples

The examples below are intentionally complete enough to show:

- what lives in the extension bundle
- how `op.yaml` changes when sandboxing is absent or present
- how the recipe refers to the extension directly through `op:`

All examples assume the bundle lives in a repo that the recipe references with a canonical selector.

## Example 1: Host execution, no sandbox

This is the simplest case. Omitting `sandbox` means "run on the host" using the existing non-Shai execution path.

Bundle layout:

```text
.colony2/ops/text_stats/
  op.yaml
  main.py
```

`op.yaml`:

```yaml
name: text_stats
description: Count lines, words, and bytes in a text payload.
version: 1.0.0
command: ["python3", "main.py"]
working_directory: .
timeout: 30s
input_schema:
  type: object
  required: [text]
  properties:
    text:
      type: string
output_schema:
  type: object
  required: [lines, words, bytes]
  properties:
    lines:
      type: integer
    words:
      type: integer
    bytes:
      type: integer
```

`main.py`:

```python
import json
import sys

payload = json.load(sys.stdin)
text = payload["text"]

result = {
    "lines": len(text.splitlines()),
    "words": len(text.split()),
    "bytes": len(text.encode("utf-8")),
}

print(json.dumps(result))
```

Recipe usage:

```yaml
- id: summarize_text
  op: git+https://github.com/acme/platform-ops.git//.colony2/ops/text_stats@v1
  inputs:
    text: |
      alpha
      beta gamma
```

If you want to be explicit, this example could also include:

```yaml
sandbox:
  type: host
```

but omission is the cleaner default.

## Example 2: Shai sandbox with inline config

This example runs `go test` inside a Shai sandbox and writes a report artifact into the outbox.

Bundle layout:

```text
.colony2/ops/go_test/
  op.yaml
  runner.py
```

`op.yaml`:

```yaml
name: go_test
description: Run go test for a package inside a Shai sandbox.
version: 1.0.0
command: ["python3", "runner.py"]
working_directory: .
timeout: 10m
sandbox:
  type: shai
  shai:
    source: inline
    inline_config:
      type: shai-sandbox
      version: 1
      image: ghcr.io/colony-2/shai-mega
      resources:
        default:
          mounts:
            - source: ${{ env.HOME }}/.cache/go-build
              target: /home/shai/go/pkg
              mode: rw
            - source: ${{ env.HOME }}/.cache/go-mod
              target: /home/shai/go/mod
              mode: rw
          http:
            - proxy.golang.org
            - sum.golang.org
            - go.dev
      apply:
        - path: ./
          resources:
            - default
input_schema:
  type: object
  required: [package]
  properties:
    package:
      type: string
output_schema:
  type: object
  required: [ok, report_path]
  properties:
    ok:
      type: boolean
    report_path:
      type: string
```

`runner.py`:

```python
import json
import os
import subprocess
import sys

payload = json.load(sys.stdin)
pkg = payload["package"]

worktree = os.environ["VIBETHIS_WORKTREE_PATH"]
outbox = os.environ["VIBETHIS_ARTIFACT_OUTBOX"]
report_name = "go-test-report.txt"
report_path = os.path.join(outbox, report_name)

proc = subprocess.run(
    ["go", "test", pkg],
    cwd=worktree,
    capture_output=True,
    text=True,
)

with open(report_path, "w", encoding="utf-8") as f:
    f.write(proc.stdout)
    if proc.stderr:
        f.write("\n--- stderr ---\n")
        f.write(proc.stderr)

print(json.dumps({
    "ok": proc.returncode == 0,
    "report_path": report_name,
}))

if proc.returncode != 0:
    sys.exit(proc.returncode)
```

Recipe usage:

```yaml
- id: run_go_tests
  op: git+https://github.com/acme/platform-ops.git//.colony2/ops/go_test@v1
  inputs:
    package: ./server/ops/...
```

This example shows the main difference when sandboxing is included:

- `sandbox.type=shai`
- `sandbox.shai.source=inline`
- the bundle carries the full sandbox definition with it

## Example 3: Shai sandbox using the cell's config

This is the same idea, but the bundle does not ship its own full Shai config. Instead it says "use the current cell's `.shai/config.yaml`."

Bundle layout:

```text
.colony2/ops/cell_lint/
  op.yaml
  runner.py
```

`op.yaml`:

```yaml
name: cell_lint
description: Run the cell's lint command inside the cell's Shai sandbox.
version: 1.0.0
command: ["python3", "runner.py"]
working_directory: .
timeout: 10m
sandbox:
  type: shai
  shai:
    source: cell
input_schema:
  type: object
  properties:
    command:
      type: array
      items:
        type: string
output_schema:
  type: object
  required: [ok, report_path, cell_path]
  properties:
    ok:
      type: boolean
    report_path:
      type: string
    cell_path:
      type: string
```

`runner.py`:

```python
import json
import os
import subprocess
import sys

payload = json.load(sys.stdin)
command = payload.get("command", ["bash", "-lc", "make lint"])

worktree = os.environ["VIBETHIS_WORKTREE_PATH"]
cell_path = os.environ["VIBETHIS_CELL_PATH"]
outbox = os.environ["VIBETHIS_ARTIFACT_OUTBOX"]

cell_root = os.path.join(worktree, cell_path)
report_name = "lint-report.txt"
report_path = os.path.join(outbox, report_name)

proc = subprocess.run(
    command,
    cwd=cell_root,
    capture_output=True,
    text=True,
)

with open(report_path, "w", encoding="utf-8") as f:
    f.write(proc.stdout)
    if proc.stderr:
        f.write("\n--- stderr ---\n")
        f.write(proc.stderr)

print(json.dumps({
    "ok": proc.returncode == 0,
    "report_path": report_name,
    "cell_path": cell_path,
}))

if proc.returncode != 0:
    sys.exit(proc.returncode)
```

Recipe usage:

```yaml
- id: lint_current_cell
  op: git+https://github.com/acme/platform-ops.git//.colony2/ops/cell_lint@v1
  inputs:
    command: ["bash", "-lc", "npm test"]
```

This example shows the "use the cell's Shai config" path clearly:

- the bundle still opts into sandboxing with `sandbox.type=shai`
- but the actual sandbox definition comes from the active cell context, not from the bundle itself

## Example 4: Shai sandbox using a config file shipped in the bundle

If you want the bundle to carry a reusable sandbox definition without embedding it inline, the bundle can point at a config file.

Bundle layout:

```text
.colony2/ops/repo_scan/
  op.yaml
  .shai/config.yaml
  runner.py
```

`op.yaml`:

```yaml
name: repo_scan
description: Run a repo scan in a bundle-provided Shai sandbox.
version: 1.0.0
command: ["python3", "runner.py"]
sandbox:
  type: shai
  shai:
    source: file
    config_path: .shai/config.yaml
```

`runner.py`:

```python
import json
import os
import sys

print(json.dumps({
    "worktree": os.environ["VIBETHIS_WORKTREE_PATH"],
    "cell_path": os.environ.get("VIBETHIS_CELL_PATH", ""),
}))
```

`.shai/config.yaml`:

```yaml
type: shai-sandbox
version: 1
image: ghcr.io/colony-2/shai-mega
resources:
  default:
    http:
      - github.com
apply:
  - path: ./
    resources:
      - default
```

This is useful when:

- multiple ops in the same repo should share one sandbox definition
- the inline config would be too large or repetitive

## Resolution Model

## Remote canonical git selector

For:

```yaml
op: git+https://github.com/acme/platform-ops.git//.colony2/ops/codex.exec@main
```

Resolution should:

1. parse selector
2. clone/fetch repo into cache
3. resolve `@main` to a concrete commit
4. compute the resolved selector:
   - `git+https://github.com/acme/platform-ops.git//.colony2/ops/codex.exec@<commit>`
5. materialize the target directory for that exact commit
6. load `op.yaml`

## Optional local selector

For:

```yaml
op: ./.colony2/ops/codex.exec
```

Resolution should:

1. determine the effective repo+commit of the current recipe
2. resolve the relative path in that same repo
3. materialize from that same commit, not from an arbitrary live checkout

If the root recipe was itself loaded from a pinned remote selector, local extension selectors should inherit that same resolved source identity.

## Cache Design

Use a local cache under:

- `~/.c2/cache/ops`

Recommended layout:

```text
~/.c2/cache/ops/
  repos/
    <repo-key>/
  bundles/
    <repo-key>/<commit>/<bundle-key>/
  metadata/
    ...
```

### Repo cache

`repos/<repo-key>` stores the local clone or mirror.

Purpose:

- avoid recloning
- support fetch/refresh for mutable refs

### Bundle cache

`bundles/<repo-key>/<commit>/<bundle-key>` stores the exact materialized op directory.

Purpose:

- immutable by commit
- reusable across runs
- cheap cache-hit semantics

### Cache keys

- `repo-key`: stable hash of normalized repo URL
- `bundle-key`: stable hash of repo-relative op path

## Runtime Execution Model

There should still be one generic extension runtime in `server/ops`, but it is an internal implementation detail behind selector-backed `op:` values.

Runtime flow:

1. detect selector-backed `op`
2. resolve selector to concrete commit
3. materialize the bundle from cache or git
4. load `op.yaml`
5. validate `inputs` against `input_schema` if present
6. export runtime env values
7. execute in host mode or Shai mode
8. parse stdout JSON
9. validate against `output_schema` if present

## Schema and Validation Changes

## 1. Static recipe schema

The static schema should no longer insist that every `op` be one of the registered op consts only.

Instead, `op` should accept:

- a known built-in op name
- a canonical git selector string
- optionally later, a `./...` local selector string

The static schema can still be fairly strong because selector strings have recognizable prefixes.

## 2. Parse-time validation

Validation becomes:

1. if `op` matches a registered core op, validate as today
2. else if `op` parses as a selector, continue to dynamic extension validation
3. else error

## 3. Dynamic extension validation

After selector resolution:

1. load extension `input_schema`
2. validate node `inputs`
3. compile execution for that bundle

This is the main behavior change.

## Shared Resolver

Do not create a new git ref subsystem just for extension ops.

The repo already has a real canonical git-selector implementation for recipe roots in:

- [root_source.go](/src/server/recipe-worker/pkg/compiler/root_source.go)

Recommendation:

- extract the generic selector parse/resolve pieces into a shared package
- use that shared package for selector-backed `op:` values

That gives:

- one canonical parser
- one commit-resolution policy
- one place for fetch/clone semantics

## Shai Support

Selector-backed extension bundles should support:

- `sandbox.type=host`
- `sandbox.type=shai`

For Shai:

- inline config
- config file in the bundle
- "use the cell's shai config"

### Cell config resolution

Recommended lookup:

1. use `deps.WorktreePath()` and `deps.GitContext().CellPath`
2. check `<worktree>/<cell_path>/.shai/config.yaml`
3. optionally walk upward toward repo root for nearest `.shai/config.yaml`
4. fail clearly if none found

### Runtime env exported to extensions

The runtime should export at least:

- `VIBETHIS_WORKDIR`
- `VIBETHIS_WORKTREE_PATH`
- `VIBETHIS_ARTIFACT_INBOX`
- `VIBETHIS_ARTIFACT_OUTBOX`
- `VIBETHIS_CELL_PATH`
- `VIBETHIS_NODE_PATH`
- `VIBETHIS_PROJECT_ROOT`
- `VIBETHIS_OP_DIR`
- `VIBETHIS_OP_NAME`
- `VIBETHIS_INPUT_JSON`

## Codex / LLM Migration Under This Model

Under this design, `codex.exec`, `llm_inference`, and `llm_inference2` stop being statically registered names.

Instead, recipes reference them directly via selector-backed `op:` values.

Examples:

```yaml
op: git+https://github.com/acme/platform-ops.git//.colony2/ops/codex.exec@main
```

Potential later same-repo form:

```yaml
op: ./.colony2/ops/codex.exec
```

The bundles should preserve current contracts.

### Codex bundle

Must preserve:

- current input/output behavior
- session resume
- `stdout.jsonl` and `stderr.txt` in outbox
- skills materialization

### LLM bundles

Must preserve:

- existing request/response contracts
- response-schema validation
- file/tool behavior for `llm_inference2`

## Recommended First Slice

1. Make `op:` accept canonical git selectors in addition to bare names.
2. Reuse or extract the existing git-selector resolver.
3. Add local cache under `~/.c2/cache/ops`.
4. Add bundle materialization.
5. Add generic host/Shai execution.
6. Migrate `codex.exec` first.

Why `codex.exec` first:

- it already requires worktree/inbox/outbox/cell-path semantics
- it already has a Shai path to preserve
- it is the best stress test of the new runtime

## Phased Plan

## Phase 1: `op:` selector support

1. Widen recipe parsing so `op` can be a registered name or selector.
2. Add selector detection for canonical `git+...//path@ref`.
3. Optionally defer `./...` local shorthand to a later phase.
4. Add resolved-selector metadata where needed for traceability.

Acceptance criteria:

- `op: input` still works unchanged
- `op: git+...//path@ref` is accepted and resolved

## Phase 2: Cache and bundle resolution

1. Add repo cache under `~/.c2/cache/ops/repos`.
2. Add bundle cache under `~/.c2/cache/ops/bundles`.
3. Resolve refs to commits and materialize exact bundle directories.

Acceptance criteria:

- repeated use of the same resolved selector reuses cache

## Phase 3: Dynamic schema validation and execution

1. Read `op.yaml`.
2. Validate `inputs` against bundle schema.
3. Implement host execution.
4. Implement Shai execution.
5. Export runtime env values.

Acceptance criteria:

- selector-backed `op` values execute without prior registration

## Phase 4: Codex migration

1. Package codex as an extension bundle in a repo.
2. Invoke it through selector-backed `op:`.
3. Preserve artifacts and resume semantics.
4. Port integration coverage.

Acceptance criteria:

- codex behavior matches current behavior through selector-backed `op`

## Phase 5: LLM migration

1. Package `llm_inference` and `llm_inference2` as extension bundles.
2. Invoke them through selector-backed `op:`.
3. Preserve validation and execution behavior.

Acceptance criteria:

- llm bundles run correctly through selector-backed `op`

## Optional Later Work

- local `./...` selector support
- allowlists / policy controls for approved selectors
- cache pruning
- migration of skill refs to the same canonical selector grammar

## Testing Plan

### Unit tests

- selector detection vs bare-name detection
- canonical git-selector parsing
- commit-pinned resolution
- cache key generation
- cell-config Shai resolution

### Integration tests

- `op: input` still works
- `op: git+file://...//.colony2/ops/...@ref` works against local test repos
- host execution
- Shai execution
- cache hit vs miss
- codex bundle through selector-backed `op`

### Regression tests

- static core-op schema generation still works
- parse-time unknown-op errors still work for bad non-selector strings
- selector-backed `op` values get dynamic post-resolution validation

## Direct Answer

I think this is the right direction:

- keep `op:`
- make it either a bare registered name or a git selector
- prefer canonical `git+...//path@ref` syntax

That is more consistent with the existing system than adding `uses:`, and it avoids inventing another reusable-reference mechanism.

## Definition of Done

This is done when:

1. `op:` accepts either a registered core op name or a canonical git selector
2. selector-backed `op` values resolve to commit-pinned extension bundles
3. bundles are cached under `~/.c2/cache/ops`
4. bundles execute without prior registration
5. bundles can run in host mode or Shai mode
6. `codex.exec`, `llm_inference`, and `llm_inference2` can all be consumed this way
