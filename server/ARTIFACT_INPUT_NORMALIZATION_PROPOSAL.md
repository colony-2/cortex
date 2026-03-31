# Artifact Input Normalization Proposal

## Goal

Replace the current artifact-specific reflection and runtime type-guessing with a cleaner, schema-guided input normalization step.

The immediate targets are:

1. `recipe-core/pkg/ops/registerable_op.go`
2. `recipe-worker/pkg/compiler/artifact_keys.go`
3. `recipe-worker/pkg/compiler/compiler.go`
4. `recipe-worker/pkg/compiler/op_input_validation.go`

## Problem

Artifact handling currently leaks across multiple generic layers.

### 1. Decode Hook Knows About Artifact Semantics

`DecodeHookMapDecoder(...)` currently includes a special case for `swf.ArtifactKey`.

That means:

1. a generic map decode hook now contains artifact-domain behavior
2. artifact coercion happens late and implicitly
3. the hook has to inspect runtime values like `map[string]interface{}` and try to infer intent

This is the wrong boundary. The decode hook should stay generic.

### 2. Artifact Dependency Discovery Is A Separate Heuristic Pass

After input resolution, we walk the resolved `map[string]interface{}` again with `collectArtifactKeys(...)`.

That means:

1. artifact dependency extraction is detached from typed input decoding
2. we recursively guess which runtime values are artifact-like
3. the code grows more `switch` and type-assertion cases over time

This is a symptom of missing normalization at the compiler boundary.

### 3. Artifact Semantics Are Inferred From Runtime Shapes

Today we rely on patterns like:

1. `swf.ArtifactKey`
2. `artifacts.Ref`
3. slices/maps of either
4. `ArtifactKey()` providers
5. `map[string]interface{}` that happen to look like a ref

That is flexible, but it is not clean. The compiler should not need to rediscover artifact meaning from arbitrary values after template resolution.

## Root Cause

We are missing an explicit boundary between:

1. template/CEL evaluation
2. typed op input decoding
3. artifact dependency collection

Without that boundary, artifact behavior gets reimplemented in:

1. map decode hooks
2. compiler scans
3. CEL helper normalization
4. ad hoc coercion helpers

## Proposed Direction

Add a schema-guided normalization step before validation and execution.

Suggested shape:

```go
type NormalizedOpInput struct {
	Data               map[string]any
	StoredArtifactKeys []swf.ArtifactKey
}

func NormalizeOpInput(inputType reflect.Type, raw map[string]any) (NormalizedOpInput, error)
```

This function should:

1. walk the declared input type
2. normalize artifact values into the expected runtime representation
3. collect stored artifact dependencies during the same walk
4. reject invalid type combinations explicitly

The key change is that artifact handling becomes schema-driven instead of value-guessing-driven.

## What `NormalizeOpInput` Should Do

### For Legacy Stored-Artifact Inputs

If a field is typed as `swf.ArtifactKey`:

1. accept a stored `artifacts.Ref`
2. extract its underlying key
3. add that key to `StoredArtifactKeys`
4. reject external refs with a clear error

This preserves existing behavior while making the rule explicit.

### For Recipe-Native Ref Inputs

If a field is typed as `artifacts.Ref`:

1. accept an `artifacts.Ref`
2. accept legacy `swf.ArtifactKey` and wrap as stored ref if we still want backward compatibility
3. collect the stored key only if the ref is stored

### For Slices And Maps

If a field is:

1. `[]swf.ArtifactKey`
2. `[]artifacts.Ref`
3. `map[string]swf.ArtifactKey`
4. `map[string]artifacts.Ref`

then normalization should recurse through the declared element type, not through arbitrary runtime values.

### For Non-Artifact Fields

Leave them alone and let the normal decode/validate path handle them.

## Compiler Flow After Refactor

The compile path should become:

1. resolve templates into `resolvedNodeInputs`
2. call `NormalizeOpInput(chain[0].InputType, resolvedNodeInputs)`
3. validate the normalized data against the input type
4. use `StoredArtifactKeys` for prefetch
5. send the normalized `Data` as the task input

Conceptually:

```go
normalized, err := NormalizeOpInput(chain[0].InputType, resolvedNodeInputs)
if err != nil {
	return fmt.Errorf("normalize op input: %w", err)
}

if err := validateOpInputType(chain[0].InputType, normalized.Data, allowNulls); err != nil {
	return fmt.Errorf("op input validation failed: %w", err)
}

artifactKeys := appendArtifactKeys(normalized.StoredArtifactKeys, resolvedArtifacts)
```

This removes the need to scan the raw input map separately for artifacts.

## What To Remove Or Simplify

### 1. Remove Artifact Special-Casing From `DecodeHookMapDecoder`

`DecodeHookMapDecoder(...)` should go back to handling only actual custom map decoders.

It should not:

1. know about `swf.ArtifactKey`
2. inspect `artifacts.Ref`
3. marshal/unmarshal maps trying to infer artifact meaning

That logic belongs in normalization.

### 2. Replace `collectArtifactKeys(...)`

`collectArtifactKeys(...)` should either:

1. be removed entirely for typed ops
2. or be kept only as a narrow fallback for truly dynamic `map[string]interface{}` ops

The common path should not rely on recursive runtime type switching.

### 3. Narrow CEL Artifact Helper Inputs

CEL artifact helpers are necessarily more dynamic, but we should still narrow the accepted surface.

Prefer:

1. `artifacts.Ref`
2. `[]artifacts.Ref`
3. `map[string]artifacts.Ref`

Avoid continuing to support every artifact-ish shape forever.

## Why This Is Cleaner

### Single Responsibility

Artifact conversion becomes the responsibility of one explicit compiler step.

### Fewer Hidden Conversions

We stop converting artifact shapes inside generic decode infrastructure.

### Better Errors

Normalization can produce exact messages like:

1. field `artifact` expects stored artifact key but got external ref
2. field `artifacts[2]` expects artifact ref but got string

### Easier Evolution

If we later add:

1. `artifacts.Ref` as a first-class op input type
2. new artifact ref kinds
3. richer artifact lists/maps

we update one normalization layer instead of several unrelated helpers.

## Suggested Internal Structure

The implementation can be organized around type-directed recursion.

Example:

```go
func normalizeValue(targetType reflect.Type, value any, acc *artifactAccumulator) (any, error)
```

Where `artifactAccumulator` handles deduped stored keys:

```go
type artifactAccumulator struct {
	seen map[string]swf.ArtifactKey
}
```

This gives us:

1. one recursive walk
2. one artifact collection mechanism
3. one place for compatibility rules

## Compatibility Rules

Recommended rules:

1. `swf.ArtifactKey` fields remain stored-artifact-only
2. `artifacts.Ref` fields may accept both stored and external refs
3. raw `map[string]interface{}` ops remain permissive, but typed ops follow declared semantics

That keeps the migration incremental instead of forcing all existing ops to change at once.

## Phased Rollout

### Phase 1

Introduce `NormalizeOpInput(...)` and route typed ops through it.

Keep fallback behavior for dynamic map-based ops.

### Phase 2

Remove artifact special-casing from `DecodeHookMapDecoder(...)`.

### Phase 3

Reduce or remove `collectArtifactKeys(...)` for the normal typed-op path.

### Phase 4

Optionally encourage new ops to prefer `artifacts.Ref` over `swf.ArtifactKey`.

## Non-Goals

This proposal does not require:

1. removing `mapstructure` entirely
2. eliminating all runtime type checks from CEL helpers
3. changing recipe syntax
4. changing SWF artifact semantics

It is specifically about moving artifact coercion and dependency extraction to a cleaner compiler boundary.

## Recommendation

The cleaner pattern is:

1. normalize once
2. normalize using the declared input type
3. collect stored artifact dependencies during that same pass
4. keep generic decode hooks generic

That is a much better long-term shape than continuing to teach reflection-based helpers and recursive runtime scanners about more artifact cases.
