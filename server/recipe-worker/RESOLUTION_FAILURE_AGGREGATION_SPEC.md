# Spec: Aggregate Template Resolution Failures During Validation

## Summary

Validation currently aborts on the first template/CEL resolution error. This
spec defines how to collect all resolution failures across a recipe in
validation mode and return them as structured data, while preserving existing
fail-fast behavior for non-validation runs.

## Goals

- Collect all template resolution failures during validation.
- Return errors in a structured, machine-readable format with paths and context.
- Preserve existing runtime behavior outside validation mode.
- Ensure validation continues through the full recipe graph, even after
  encountering resolution failures.

## Non-goals

- Changing runtime (non-validation) execution semantics.
- Introducing new template syntax or new resolution scopes.
- Reworking unrelated validation logic (schema validation, op existence, etc.).

## Current Behavior

- `ResolveMap` and `EvaluateCEL` return on the first error.
- Validation mode stops at the first resolution failure.
- Error strings are not structured, making it difficult to report multiple
  issues in a single response.

## Proposed Changes

### 1) Add structured resolution error type (recipe-template)

Add a structured error type that can accumulate multiple failures:

```go
type ResolutionFailureKind string

const (
    ResolutionFailureUnknown     ResolutionFailureKind = "unknown"
    ResolutionFailureInvalidExpr ResolutionFailureKind = "invalid_expr"
    ResolutionFailureMissingRef  ResolutionFailureKind = "missing_reference"
    ResolutionFailureTypeError   ResolutionFailureKind = "type_error"
    ResolutionFailureEvalError   ResolutionFailureKind = "eval_error"
)

type ResolutionFailure struct {
    Path      string               // JSON-ish field path; e.g. "sequence[2].inputs.user_id"
    Expr      string               // Raw expression string when applicable
    Scope     ScopeType            // Scope where failure occurred
    NodePath  string               // Optional; e.g. "sequence[2]", "states.review"
    Kind      ResolutionFailureKind
    Message   string               // Human readable summary
}

type ResolutionErrors struct {
    Failures []ResolutionFailure
}

func (e ResolutionErrors) Error() string { ... } // concise summary
```

`ResolutionErrors` should be JSON-serializable and safe to return through API
and CLI surfaces.

### 2) Add `CollectAll` to resolution options

Extend `template.ResolutionOptions` with:

```go
CollectAll bool // when true, traverse entire input/output structure
```

If `CollectAll` is `true`, the resolver:
- continues walking the entire map/tree even after a failure,
- returns a partially resolved map (best-effort),
- returns `ResolutionErrors` containing all failures.

If `CollectAll` is `false`, preserve existing fail-fast behavior.

### 3) Add diagnostics-friendly resolution methods

Update the template resolver to return structured errors:

- `ResolveMap(...)` should surface `ResolutionErrors` when `CollectAll` is true.
- `EvaluateCEL(...)` should return a structured failure when parsing or
  evaluating the expression fails. In validation mode, these failures are
  collected and evaluation continues where possible.

If the underlying resolver cannot classify a failure, use
`ResolutionFailureUnknown`.

### 4) Compiler validation flow aggregates failures

Add a compiler-side collector for validation:

```go
type ValidationErrors struct {
    Failures []template.ResolutionFailure
}

func (e ValidationErrors) Error() string { ... }
```

During `ExecutionModeValidate`, the compiler:
- sets `CollectAll = true` in resolution options,
- captures `ResolutionErrors` from `ResolveMap` and `EvaluateCEL`,
- appends them to the collector,
- continues validating the rest of the recipe graph.

After validation completes:
- If any failures exist, return `ValidationErrors`.
- If no failures, return resolved outputs as usual.

### 5) Validation mode control flow adjustments

- Sequence and state machine inputs/outputs: If `ResolveMap` returns
  `ResolutionErrors`, continue with the returned best-effort map and record all
  failures.
- Transition conditions: In `ValidateAll` mode, evaluate all transition
  expressions and collect failures without short-circuiting.
- ValidatePathOnly: keep current behavior unless `CollectAll` is true; if
  `CollectAll` is true, record failures and continue.

### 6) Structured response format

The validation error returned by the compiler should marshal to:

```json
{
  "message": "validation failed",
  "failures": [
    {
      "path": "sequence[1].inputs.user_id",
      "expr": "{{ sequence.fetch.outputs.user.id }}",
      "scope": "sequence",
      "nodePath": "sequence[1]",
      "kind": "missing_reference",
      "message": "sequence.fetch.outputs.user is not defined"
    }
  ]
}
```

The API layer (if any) should pass this through unchanged.

## Detailed Behavior

### Resolution failure classification

The resolver should map common failure types to `ResolutionFailureKind`:

- CEL parse error -> `invalid_expr`
- CEL eval error -> `eval_error`
- Missing field/unknown identifier -> `missing_reference`
- Type mismatch (e.g., indexing into non-list) -> `type_error`
- Anything else -> `unknown`

When possible, include both `Path` (field being resolved) and `Expr` (raw CEL).

### Node path derivation

Where available, attach a `NodePath` to errors for quick identification:

- Sequence nodes: `sequence[<index>]`
- States: `states.<name>`
- Ops: `op:<opName>` or `<nodeID>` when a node ID is present

If a node path is not available, omit it.

## Compatibility

- Default behavior (non-validation or `CollectAll=false`) remains unchanged.
- Validation callers that expect a single error will now receive a structured
  error with multiple failures when `CollectAll=true`.

## Test Plan

1) Sequence resolution: recipe with two broken input expressions in different
   nodes -> returns two failures.
2) State machine transitions: invalid `when` expressions in multiple states ->
   all failures returned.
3) Output templates: invalid outputs in both sequence and state machine outputs
   -> all failures returned.
4) ValidatePathOnly: ensure behavior unchanged when `CollectAll=false`.
5) Runtime mode: ensure fail-fast behavior unchanged during normal execution.

## Open Questions

- Should `ResolveMap` return partial resolved values for failed expressions as
  `nil`, zero values, or raw strings? (Spec defaults to best-effort with `nil`
  or zero values, whichever the resolver currently uses.)
- Should validation errors be merged with other validation errors
  (schema/structural) into a unified error response?
