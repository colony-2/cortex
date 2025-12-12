# Artifact Support in Template Resolution (user experience first)

This doc focuses on the authoring and template experience for artifacts as first-class, opaque values that flow between ops. Implementation/storage details are intentionally omitted.

## Goals (UX)
- Authors can declare inputs that expect a single artifact or an artifact tree.
- Templates can reference artifacts explicitly (e.g., `sequence.myop1.outputs.artifacts['foo.txt']`) or artifact trees via globs (e.g., `sequence.myop1.outputs.artifacts['foo/*']`, `sequence.myop1.outputs.artifacts['*']`).
- Artifact-typed inputs only accept artifact references; artifact-tree inputs accept glob selections; non-artifact inputs cannot point to artifacts.
- Artifacts remain opaque in CEL/interpolation; authors can see metadata (name/size/type) but not bytes.

## UX Concepts
- **Artifact-typed input fields**: Inputs on ops can be typed as `artifact`. These fields only resolve from single-artifact expressions; non-artifact values are validation errors.
- **Artifact-tree input fields**: Inputs on ops can be typed as `artifact_tree` (glob-based selection). Expressions must be a glob index into an artifacts map (e.g., `artifacts['foo/*']` or `artifacts['*']`), returning a collection with relative paths preserved.
- **Scoped access**: Visibility mirrors outputs today—sequences see sibling artifacts; states see prior state artifacts; ops see their container’s artifacts.
- **CEL access shape**:
  - Single artifact: `sequence.myop1.outputs.artifacts['foo.txt']` returns an opaque artifact handle with metadata fields (e.g., `.name`, `.size_bytes`) but no byte content.
  - Artifact tree (glob): `sequence.myop1.outputs.artifacts['foo/*']` or `sequence.myop1.outputs.artifacts['*']` returns an opaque artifact-tree handle representing all matching artifacts with relative paths.
- **Interpolation rules**: `{{ sequence.myop1.outputs.artifacts['foo.txt'] }}` is invalid in string contexts; artifacts cannot be stringified. Metadata fields can be interpolated (`{{ sequence.myop1.outputs.artifacts['foo.txt'].size_bytes }}`). Artifact-tree handles cannot be stringified; only their aggregate metadata (if exposed) is interpolable.

## Authoring Examples (conceptual)
- Declaring an artifact input:
  ```yaml
  inputs:
    build_log: # type: artifact (enforced by schema/types)
      from: "{{ sequence.build.outputs.artifacts['build.log'] }}"
  ```
- Declaring an artifact-tree input (glob):
  ```yaml
  inputs:
    reports_dir: # type: artifact_tree
      from: "{{ sequence.aggregate.outputs.artifacts['reports/*'] }}"
  ```
- Referencing metadata (allowed):
  ```yaml
  when: "sequence.build.outputs.artifacts['build.log'].size_bytes < 10_000_000"
  ```
- Invalid (stringify artifact):
  ```yaml
  # error: artifact used in string interpolation
  note: "See artifact: {{ sequence.build.outputs.artifacts['build.log'] }}"
  ```

## Validation Rules (UX-level)
- Artifact-typed inputs must resolve to single artifacts; non-artifact-typed inputs must not.
- Artifact-tree inputs must resolve from one or more glob expressions into artifacts; selecting zero artifacts may be allowed or not per field config.
- Referenced artifact names/patterns must exist in the visible scope (produced upstream).
- Interpolation of an artifact or artifact-tree handle into a string is a validation error; only metadata fields are allowed.
- When an op declares `consumes` artifact names/patterns, template validation ensures those are available in scope before execution.

## Merging Multiple Artifact Trees
- Some inputs may need multiple artifact trees (e.g., `artifacts_needed`). Authors can merge glob results to create a single composite tree.
- Merge semantics: later entries overwrite earlier ones on path collisions; non-colliding paths are unified.
- CEL helper (proposed): `artifacts.merge(tree1, tree2, ...)` returning an artifact-tree handle with merged contents and relative paths preserved.
- Authoring example:
  ```yaml
  inputs:
    artifacts_needed: # type: artifact_tree
      from: "{{ artifacts.merge(
        sequence.build.outputs.artifacts['reports/*'],
        sequence.test.outputs.artifacts['coverage/*']
      ) }}"
  ```
  If both trees contain `foo/bar.txt`, the value from the latter argument (`coverage/*` above) wins.

## Open UX Questions
- Syntax: use bracket form `artifacts['foo.txt']` (no auto-sanitized dot variant).
- Metadata exposure: only `name` and `size_bytes` are visible in templates.
- Artifact-tree shape: return an opaque handle (no list/map in templates).
- Glob semantics: align with gitignore-style patterns (`*`, `**`, `?`, path separators honored).
- Artifact-tree inputs: a single glob per field (no multiple globs per field).***
