# Artifact Support in Template Resolution - Detailed Spec

This spec defines how artifacts flow through template resolution as first-class,
opaque values. It extends existing template semantics with artifact-aware CEL
types and input schema validation.

## Goals
- Allow templates to reference artifacts produced by upstream ops.
- Add explicit artifact output maps to step outputs and per-run outputs.
- Add a CEL-visible artifact datatype with metadata fields.
- Allow op inputs to require a single artifact or a collection of artifacts.
- Keep artifacts opaque; template authors cannot access bytes.

## Non-Goals
- Defining artifact storage, upload, or retrieval APIs.
- Defining how ops produce artifacts at execution time (covered elsewhere).
- Introducing automatic globbing or filesystem discovery (can be added later).

## Data Model Changes

### StepOutput / RunOutput
`StepOutput` and `RunOutput` gain an `Artifacts` map, keyed by artifact name.
The map holds opaque artifact handles. CEL exposes them as `swf.ArtifactKey`.

```go
type StepOutput struct {
    Outputs   map[string]interface{} `json:"outputs"`
    Artifacts map[string]swf.Artifact `json:"artifacts"`
    Runs      []RunOutput            `json:"runs"`
}

type RunOutput struct {
    Outputs   map[string]interface{} `json:"outputs"`
    Artifacts map[string]swf.Artifact `json:"artifacts"`
    RunID     string                 `json:"run_id"`
    Timestamp time.Time              `json:"timestamp"`
}
```

`StepOutput.Artifacts` reflects the latest run (mirrors `Outputs` today).
`RunOutput.Artifacts` holds per-run artifacts for loops/retries.

### ArtifactKey (template-visible)
Templates resolve artifacts to `swf.ArtifactKey`. These are opaque pointers to
stored artifacts (no bytes exposed in templates).

Fields exposed to templates:
```go
type ArtifactKey struct {
    JobId       string `json:"jobId"`
    TaskOrdinal int64  `json:"taskOrdinal"`
    Name        string `json:"name"`
    SizeBytes   int64  `json:"sizeBytes"`
}
```

## Template Data Shape
Artifacts are exposed alongside outputs:
- `sequence.<step_id>.artifacts["<name>"]`
- `states.<state_id>.artifacts["<name>"]`
- `sequence.<step_id>.runs[0].artifacts["<name>"]`

Example:
```yaml
readme_file: '{{ sequence.build.artifacts["readme.md"] }}'
```

## CEL Integration

### Types
- `artifact`: `swf.ArtifactKey` with readable fields.
- `map<string, artifact>`: artifact-key collections.

### CEL Environment
Add `swf.ArtifactKey` as a native type in the CEL adapter and allow it to be
returned as a value from expressions (derived by calling `ArtifactKey()` on
artifacts stored in step outputs).

### Allowed Operations
- Field access: `artifact.name`, `artifact.size_bytes`, etc.
- Equality/inequality by reference (optional) if needed for comparisons.
- No string conversion; artifacts are not interpolable into strings.

## Resolution Rules

### Interpolation
- `{{ <expr> }}` returns the raw value.
- Mixed string interpolation (e.g., `"x {{ expr }}"`) is only allowed if
  `expr` yields a string/number/bool; artifacts are invalid in string context.

Valid:
```yaml
readme: '{{ sequence.build.artifacts["readme.md"] }}'
size: '{{ sequence.build.artifacts["readme.md"].size_bytes }}'
```

Invalid:
```yaml
note: 'See {{ sequence.build.artifacts["readme.md"] }}'
```

### Scope Visibility
Artifact visibility mirrors outputs:
- Sequence nodes can see sibling artifacts via `sequence.<id>.artifacts`.
- State nodes can see completed states via `states.<id>.artifacts`.
- Ops see their surrounding sequence/state machine scope.

## Input Schema Changes

Extend `recipe.InputSchema.Type` with:
- `artifact`: single artifact reference
- `artifact_map`: map of artifacts keyed by name

Input validation rules:
- `artifact` requires a single `swf.ArtifactKey` value.
- `artifact_map` requires `map<string, swf.ArtifactKey>`.
- Non-artifact input types reject artifact values.

Examples:
```yaml
inputs:
  readme_file: '{{ sequence.build.artifacts["readme.md"] }}'
  build_artifacts: '{{ sequence.build.artifacts }}'
```

## Validation Behavior

### Runtime
- Missing artifact keys or wrong types raise resolution errors.
- Artifact values in string interpolation are rejected.

### Validation Mode (template-only)
- Placeholder outputs should include empty `Artifacts` maps to allow
  reference resolution without execution.
- `artifact_map` placeholders are empty maps; `artifact` placeholders are
  zero-value `swf.ArtifactKey` with empty fields.

## Backwards Compatibility
- Existing templates and outputs remain unchanged.
- `artifacts` is a new field; no change to existing output names.
- No change to current scope rules.

## Open Questions
- Should `artifact_map` allow filtering helpers (e.g., `artifacts.pick(...)`)?
- Should equality comparisons on artifacts be enabled (by `name` and metadata)?
