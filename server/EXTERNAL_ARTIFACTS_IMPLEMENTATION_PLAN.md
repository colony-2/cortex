# External Artifact Implementation Plan

## Goal

Implement external artifacts as a recipe-layer concept:

1. Producer ops can register an external artifact pointer `{key, url, expand}`.
2. Recipe authors reference it with the same `${{ ...artifacts["key"] }}` syntax they use today.
3. The pointer flows through recipe outputs and scope boundaries without downloading data.
4. Data is fetched only when a downstream op imports the artifact into `artifacts:` and needs it materialized into `inbox/`.

The important correction is this: external artifacts should not become a new `swf` artifact kind. `swf` continues to store real uploaded artifacts only. External artifact pointers live in recipe state and task output payloads.

## Design Principle

Use one recipe-native artifact reference model that can point at either:

1. a normal stored artifact backed by `swf.ArtifactKey`
2. an external pointer backed by `{url, expand}`

Everything above the SWF boundary should speak in terms of recipe artifact refs. Only the worker materializer decides whether a ref resolves through SWF or through external fetch logic.

## What Has To Change

### 1. Introduce A Recipe-Native Artifact Ref Type

Add a new package, for example:

`recipe-core/pkg/artifacts`

Suggested shape:

```go
type Ref struct {
	Kind     string       `json:"kind"` // "stored" | "external"
	Name     string       `json:"name"`
	Stored   *StoredRef   `json:"stored,omitempty"`
	External *ExternalRef `json:"external,omitempty"`
}

type StoredRef struct {
	Key swf.ArtifactKey `json:"key"`
}

type ExternalRef struct {
	URL    string `json:"url"`
	Expand bool   `json:"expand"`
}
```

Key rules:

1. `Ref` is the recipe-layer artifact identity.
2. Stored refs wrap `swf.ArtifactKey` and keep current behavior.
3. External refs never pretend to be `swf.ArtifactKey`.
4. `Ref` should expose helper methods like `StoredKey() (swf.ArtifactKey, bool)`, `SizeBytes() int64`, and validation helpers.
5. `Ref` should not implement `ArtifactKey() (swf.ArtifactKey, error)`, because that would cause the template layer to collapse it back into plain `swf.ArtifactKey`.

## Why This Stays Recipe-Scoped

The existing repo already has a persistence channel for recipe-native metadata: task output JSON.

Current runtime split:

1. real artifacts travel in `[]swf.Artifact`
2. structured recipe data travels in `ActivityInvocationOutput`

External artifact pointers fit naturally into the second bucket, not the first.

That means:

1. no new `swf-go` artifact kind
2. no changes to SWF artifact upload/download semantics
3. no remote runtime codec changes just to support pointers
4. external refs survive retries/replay because the output envelope is already persisted

## Recommended Architecture

### 2. Store Recipe Artifact Refs In Recipe Context, Not `swf.Artifact`

Today `contextual.StepOutput.Artifacts` and `contextual.RunOutput.Artifacts` are:

`map[string]swf.Artifact`

That should become:

`map[string]artifacts.Ref`

Files to update:

1. `recipe-core/pkg/contextual/context.go`
2. `recipe-template/pkg/template/template_resolver.go`
3. `recipe-template/pkg/template/cel_vars.go`
4. `recipe-worker/pkg/compiler/artifact_map.go`
5. `recipe-worker/pkg/compiler/validation_helpers.go`

Why this matters:

1. recipe state must be able to hold either stored refs or external refs
2. recipe state should not hold live `swf.Artifact` objects longer than needed
3. passing through `outputs:` should move refs, not bytes

### 3. Persist External Refs In `ActivityInvocationOutput`

Add a new field to the activity output payload:

```go
type ActivityInvocationOutput struct {
	GitResult    contextual.GitCommitContext `json:"git,omitempty"`
	NextTask     string                      `json:"nextTaskType,omitempty"`
	OpOutput     map[string]interface{}      `json:"output"`
	ArtifactRefs map[string]artifacts.Ref    `json:"artifact_refs,omitempty"`
}
```

Behavior:

1. normal outbox artifacts still come back through `out.GetArtifacts()`
2. external refs come back through `decoded.Activity.ArtifactRefs`
3. the compiler merges both into one `map[string]artifacts.Ref` before writing to recipe context

Files to update:

1. `recipe-worker/pkg/ops/activity_registry.go`
2. `recipe-worker/pkg/compiler/task_output.go`
3. `recipe-worker/pkg/compiler/compiler.go`
4. `workflow/internal/story/replay_recorder.go`

### 4. Add A Recipe-Layer Registration API For External Artifacts

Do not force external pointers through `swf.Artifact`.

Recommended op dependency additions:

```go
type OpDependencies interface {
	...
	AddOutputArtifact(swf.Artifact) error
	AddExternalArtifact(name string, url string, expand bool) error
	GetExternalArtifacts() map[string]artifacts.Ref
}
```

Why this is the right split:

1. `AddOutputArtifact` remains the API for real uploaded artifacts
2. `AddExternalArtifact` is explicit and recipe-scoped
3. the worker can continue to upload only real `swf.Artifact` values
4. the worker can inject external refs into the output envelope without asking SWF to persist them as blobs

Files to update:

1. `recipe-core/pkg/ops/op_dependencies.go`
2. fake/stub op dependency implementations in tests
3. `recipe-worker/pkg/ops/op_executor.go`

### 5. Stop Collapsing Artifact Values To `swf.ArtifactKey` During Template Resolution

This is the most important template/compiler change.

Current behavior:

1. `resolutionTypeAdapter.NativeToValue(...)` converts any `ArtifactKey()` provider into `swf.ArtifactKey`
2. `evaluateCELExpression(...)` does the same

That behavior must be removed or narrowed so recipe artifact refs survive CEL evaluation intact.

Files to update:

1. `recipe-template/pkg/template/cel_adapter.go`
2. `recipe-template/pkg/template/template_resolver.go`
3. `recipe-template/pkg/template/template_interpolate.go`
4. `recipe-template/pkg/template/permissive_artifact_map.go`
5. `recipe-template/pkg/colonycel/artifact_functions.go`

New behavior:

1. `sequence.foo.artifacts["bar"]` resolves to `artifacts.Ref`
2. artifact helper functions operate on `artifacts.Ref`
3. string interpolation still rejects artifact refs
4. validation placeholders return placeholder refs instead of placeholder `swf.ArtifactKey`

### 6. Change `artifacts:` Bindings To Use Recipe Artifact Refs

Current binding path:

1. `resolveArtifactBindings(...)` coerces to `map[string]swf.ArtifactKey`
2. `materializeArtifactBindings(...)` assumes every binding is backed by a stored artifact key

Updated path:

1. `resolveArtifactBindings(...)` returns `map[string]artifacts.Ref`
2. `materializeArtifactBindings(...)` resolves each ref by kind

Suggested behavior by kind:

1. `stored`
   - resolve through `WorkflowControl.GetArtifactLazy(...)`
   - preserve current file materialization behavior
2. `external`
   - fetch the URL or copy from `file://`
   - if `expand: true`, extract into `inbox/<binding>/`
   - if the source is a directory, materialize as a directory tree

Files to update:

1. `recipe-worker/pkg/compiler/artifact_bindings.go`
2. `recipe-worker/pkg/ops/activity_registry.go`
3. `recipe-worker/pkg/ops/op_executor.go`

### 7. Keep `ArtifactKeys` Only For Stored-Artifact Prefetch

The current `ActivityInvocationRequest.ArtifactKeys` field still has value, but only for stored artifacts.

Recommended revised meaning:

1. `ArtifactKeys []swf.ArtifactKey` = stored artifacts referenced in inputs that may need to back `deps.GetInputArtifacts()` / `deps.FindArtifact(...)`
2. `Artifacts map[string]artifacts.Ref` = explicit inbox bindings to materialize

That means the old pattern of forcing binding refs into `ArtifactKeys` can go away. Binding resolution should happen directly from the ref itself.

Files to update:

1. `recipe-worker/pkg/compiler/artifact_keys.go`
2. `recipe-worker/pkg/compiler/compiler.go`
3. `recipe-worker/pkg/ops/op_executor.go`

## Materialization Design

### 8. Add A Shared Recipe Artifact Materializer

Create a small package, for example:

`recipe-worker/pkg/artifacts`

Responsibilities:

1. materialize stored refs via SWF
2. materialize external refs via URL fetch/copy
3. handle `file://` files and directories
4. handle `http://` and `https://`
5. expand archives when requested
6. enforce path traversal and overwrite protections

The executor should delegate to this package instead of embedding scheme logic directly into `op_executor.go`.

### 9. Define First-Cut Consumption Semantics Clearly

Recommended first-cut support:

1. full support in `artifacts:` inbox bindings
2. full support for passthrough via `outputs:`
3. full support for artifact CEL helpers like `artifact_set`, `artifact_filter`, and `artifact_names`

Recommended first-cut non-goal:

1. passing an external ref into a Go field typed as `swf.ArtifactKey`
2. using `deps.FindArtifact(swf.ArtifactKey)` with an external ref

Backward-compatible rule:

1. fields typed as `swf.ArtifactKey` continue to work for stored artifacts only
2. if we want direct typed-input parity later, we should add a new Go input type such as `artifacts.Ref` and optionally a new op-dependency helper like `FindArtifactRef(...)`

This keeps the initial implementation focused on the primary use case from `EXTERNAL_ARTIFACTS.md`: lazy inbox materialization.

## Child Recipe Propagation

### 10. Pass Recipe Artifact Refs Across Child Job Boundaries

`recipe-child` currently uses `[]swf.ArtifactKey`, which is insufficient for external refs.

Recommended changes:

1. change `recipe-child/pkg/recipe/SingleRecipe.Artifacts` from `[]swf.ArtifactKey` to `[]artifacts.Ref`
2. extend `workflowctl.StartJob` with `ArtifactRefs []artifacts.Ref`
3. keep `StartJob.Artifacts []swf.Artifact` for stored artifacts only
4. when launching a child job:
   - stored refs are mirrored into `StartJob.Artifacts` as today
   - all refs, including external refs, are carried in `StartJob.ArtifactRefs`

Then the child recipe runtime can expose the same artifact ref model to downstream steps without asking SWF to persist external pointers as artifacts.

Files to update:

1. `recipe-core/pkg/workflowctl/types.go`
2. `recipe-child/pkg/recipe/op.go`
3. `recipe-child/pkg/recipe/launcher.go`
4. `recipe-core/pkg/starter/start_recipe.go`
5. recipe job worker startup path in `recipe-worker/pkg/compiler/job_worker.go`

## Workflow / Story / API Visibility

### 11. Surface Recipe Artifact Refs In Workflow Views

Current workflow visibility code only sees `swf` artifacts:

1. `workflow/internal/service/service.go`
2. `workflow/internal/story/replay_recorder.go`

That means external refs would be invisible unless we explicitly read them from the output envelope.

Recommended approach:

1. aggregate stored artifacts from SWF as today
2. aggregate external refs by decoding `ActivityInvocationOutput.ArtifactRefs`
3. add a recipe-level artifact reference view type for workflow/story code
4. keep download endpoints stable, but teach them to resolve external refs when the requested artifact is recipe-native rather than SWF-backed

This is a workflow-service change, not an SWF change.

## Delivery Plan

### Phase 1: Recipe Artifact Ref Foundation

1. Add `recipe-core/pkg/artifacts`.
2. Change recipe context storage from `map[string]swf.Artifact` to `map[string]artifacts.Ref`.
3. Update template/CEL plumbing so artifact expressions return refs instead of bare `swf.ArtifactKey`.

Acceptance criteria:

1. Existing stored artifacts still resolve and render correctly in recipes.
2. `sequence.foo.artifacts["bar"]` returns a recipe ref object through CEL.
3. Artifact helper functions still work for stored artifacts after the ref migration.

### Phase 2: External Registration And Output Persistence

1. Add `AddExternalArtifact(...)` to op dependencies.
2. Add `ArtifactRefs` to `ActivityInvocationOutput`.
3. Merge stored SWF artifacts and external refs into one recipe artifact map in the compiler.

Acceptance criteria:

1. An op can emit an external ref without creating any `swf.Artifact`.
2. The external ref survives retries and replay because it is stored in the output envelope.
3. `outputs:` passthrough carries the ref without downloading data.

### Phase 3: Inbox Materialization

1. Implement the shared materializer in `recipe-worker/pkg/artifacts`.
2. Change `artifacts:` bindings to resolve from `artifacts.Ref`.
3. Support `file://`, `http://`, `https://`, directories, and `expand: true`.

Acceptance criteria:

1. No download occurs if the ref is only passed through outputs.
2. Download occurs only when the binding is materialized into `inbox/`.
3. Failures clearly identify the artifact name and source URL.

### Phase 4: Child Recipes And Cross-Scope Propagation

1. Update `recipe-child` and `workflowctl.StartJob` to carry recipe artifact refs.
2. Preserve existing stored-artifact behavior while adding external-ref propagation.

Acceptance criteria:

1. Stored artifacts still work across child recipe boundaries.
2. External refs can cross child recipe boundaries without being converted into SWF artifacts.

### Phase 5: Workflow Visibility

1. Surface external refs in workflow/story models.
2. Update download endpoints to resolve recipe-native external refs.

Acceptance criteria:

1. External refs are visible in workflow detail and story views.
2. Download endpoints work for file-like external refs.

## Test Plan

### Template / Compiler Tests

1. `sequence.*.artifacts[...]` returns `artifacts.Ref`.
2. Artifact helper CEL functions accept both stored refs and external refs.
3. `artifacts:` binding resolution returns `artifacts.Ref` instead of `swf.ArtifactKey`.
4. String interpolation still rejects artifact refs.

### Worker Tests

1. Existing stored-artifact flows still pass.
2. An external ref emitted by an op appears in downstream recipe context.
3. Importing an external ref via `artifacts:` materializes it into `inbox/`.
4. Failed materialization prevents op invocation.
5. `expand: true` and directory materialization behave correctly.

### Child Recipe Tests

1. Stored refs still cross into child jobs.
2. External refs cross into child jobs as recipe refs.
3. No eager fetch happens during child job start.

### Workflow / API Tests

1. Workflow detail includes external refs aggregated from task output payloads.
2. Story replay shows external refs for the relevant op step.
3. Artifact download endpoints resolve stored and external refs correctly.

## Explicit Non-Goals For The First Cut

1. No new `swf-go` artifact type.
2. No changes to SWF artifact upload/download semantics.
3. No attempt to make external refs masquerade as `swf.ArtifactKey`.
4. No caching or deduplication of external downloads.
5. No guarantee that existing ops with input fields typed as `swf.ArtifactKey` will accept external refs directly.

## Summary

The right implementation is:

1. keep SWF responsible for real artifacts only
2. introduce a recipe-native artifact ref model
3. persist external refs in task output JSON
4. teach template resolution, compiler state, and inbox materialization to operate on recipe refs
5. add workflow visibility on top of recipe output payloads rather than changing SWF internals
