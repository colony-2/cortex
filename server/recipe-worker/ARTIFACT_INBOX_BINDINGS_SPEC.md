# Artifact Inbox Bindings Specification

**Scope:** `/src/server/recipe-worker` and `/src/server/recipe-core`

## Problem Statement

Recipes can reference artifacts today by embedding artifact refs inside `inputs`. That allows ops to receive artifact keys in typed input structs, but it does not provide an ergonomic way to stage referenced artifacts on disk before the op runs. Many ops (notably command execution) prefer a filesystem inbox. We already have an `environment.inbox` sentinel and directory in the op executor, but there is no recipe-level way to declare which artifacts should be placed there.

## Goals

- Add an `artifacts` sibling to `inputs` in recipe nodes.
- Allow `artifacts` to accept:
  - Single artifact references.
  - Artifact maps (map of artifact names to artifact refs).
- Materialize all referenced artifacts into the op executor's inbox directory **before** the op runs.
- Keep existing `inputs` artifact reference behavior intact.

## Non-Goals

- No change to op input schemas or typed decoding behavior.
- No change to how output artifacts are produced.
- No change to Temporal/SWF artifact persistence semantics.

## Data Model Changes

### Recipe Node Metadata

Add a new field to `recipe.NodeMetadata`:

```go
type NodeMetadata struct {
    ID        string       `yaml:"id,omitempty"`
    Desc      string       `yaml:"desc,omitempty"`
    Timeout   Duration     `yaml:"timeout,omitempty"`
    Retry     *RetryPolicy `yaml:"retry,omitempty"`
    Inputs    InputMap     `yaml:"inputs,omitempty"`
    Artifacts InputMap     `yaml:"artifacts,omitempty"`
    When      cel.CELExpr  `yaml:"when,omitempty"`
}
```

Notes:
- `Artifacts` uses `InputMap` to re-use YAML parsing and template resolution logic.
- `Artifacts` is **not** passed into op input structs.

### Op Metadata

Extend op metadata with a capability flag that gates artifact inbox bindings:

```go
type OpMetadata struct {
    Type              string
    Description       string
    Version           string
    DefaultTimeout    time.Duration
    AcceptsArtifacts  bool
}
```

Rules:
- Default `AcceptsArtifacts` to `false`.
- Set `AcceptsArtifacts = true` for `command_execution` and the extension op.

### Activity Invocation Envelope

Extend `ActivityInvocationRequest` with the resolved artifact bindings:

```go
type ActivityInvocationRequest struct {
    Input          map[string]interface{}        `json:"input"`
    GitTaskContext gitstate.GlobalGitTaskContext `json:"context"`
    ArtifactKeys   []swf.ArtifactKey             `json:"artifact_keys,omitempty"`
    Artifacts      map[string]swf.ArtifactKey    `json:"artifacts,omitempty"`
    Deps           ops.OpDependencies            `json:"-"`
}
```

## Recipe YAML Shape

### Artifact Bindings

```yaml
- id: consume
  op: command_execution
  inputs:
    run: "cat {{ context.environment.inbox }}/payload.json"
  artifacts:
    payload.json: "{{ sequence.emit.artifacts[\"payload\"] }}"
    config.json: "{{ sequence.emit.artifacts[\"config\"] }}"
    data.csv: "{{ sequence.emit.artifacts[\"data\"] }}"
```

Note: `emit` in the example is just the `id` of a prior node; it is not a keyword.

Materialization rules:
- Each entry writes to `<inbox>/<name>`.
- Multiple bindings are expressed by adding more entries.

## Resolution and Validation

1. Resolve `metadata.Artifacts` with the same template resolver used for `metadata.Inputs`.
2. After resolution, each value must be:
   - `swf.ArtifactKey` (or `*swf.ArtifactKey`)
3. Reject any other type with a clear validation error.
4. If the op metadata does **not** declare `AcceptsArtifacts`, reject any node that sets `artifacts`.

## Compiler Changes

### Template Resolution

- In `pkg/compiler/compiler.go`, resolve `metadata.Artifacts` (if present) with the same `ResolutionContext` used for `inputs`.
- Validate `metadata.Artifacts` is only set when the target op declares `AcceptsArtifacts`.

### Artifact Key Collection

- Extend artifact key collection to include bindings in `metadata.Artifacts`.
- De-duplicate keys as today.
- Store the resolved `Artifacts` map on the activity invocation request.

## Executor Changes

### Materialize Inbox Bindings

In `pkg/ops/op_executor.go`:

1. Rehydrate all `ArtifactKeys` as today.
2. Build a map from artifact key identity to `swf.Artifact`.
3. For each binding in `req.Artifacts`:
   - Look up the referenced artifact by key.
   - Validate the binding name:
     - Must be non-empty.
     - Must not include `..` path segments.
   - Ensure destination directories exist.
   - Save to disk using `Artifact.SaveToFile(ctx, path)` **before** invoking the op.
4. If a destination path already exists, return an error (avoid silent overwrite).

### Path Rules

- `artifacts["name.ext"] = <artifactKey>`
- Writes to `<inbox>/name.ext`
- Path separators are allowed in the binding name; they form subdirectories under the inbox.
- If the binding name ends with `/`, write to `<inbox>/<bindingName><artifact.Name()>`.

## JSON Schema Updates

Update `schema.json` generation to include `artifacts` alongside `inputs` for each node type that embeds `NodeMetadata`.

## Error Handling

- Missing or invalid artifact keys: fail compilation (validation error).
- Missing rehydrated artifact at runtime: fail the task with a clear error.
- Invalid file names or path traversal attempts: fail fast.
- Name collisions (same output path from multiple bindings): fail fast.
- Setting `artifacts` for an op that does not accept artifacts: fail compilation.

## Testing

### Unit Tests

- Validate that `metadata.Artifacts` resolves and validates like `Inputs`.
- Confirm artifact key collection includes `Artifacts`.
- Ensure executor writes artifacts to inbox with correct names.
- Verify collisions and invalid names return errors.

### Integration Tests

Add or extend fixture recipes to cover:
- Multiple artifact bindings into inbox.
- Command op consuming inbox files.

## Open Questions

1. Should inbox path collisions be allowed with last-write-wins, or always error?

## Success Criteria

- Recipes can declare `artifacts` next to `inputs`.
- All declared artifacts are written to the inbox before op execution.
- No regressions in existing artifact-in-input behavior.
- Schema and validation accurately reflect the new field.
