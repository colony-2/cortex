# Downstream Migration Guide: Runtime-Based Engine Construction

## Summary

SWF now supports backend-agnostic engine construction through `WorkflowRuntime`.

This is the new preferred direction:

- `swf` public engine construction should not require direct `strata` or `pgwf` objects
- production-style engines should be created with a runtime implementation
- test/in-memory engines should also be created with a runtime implementation

The old APIs still exist for compatibility, but the backend-leaking pieces are now deprecated.

## What Changed

New preferred APIs:

- `EngineBuilder.WithRuntime(...)`
- `EngineBuilder.BuildEngine()`
- `pkg/swf/runtime/direct`
- `pkg/swf/runtime/toy`

Deprecated APIs and patterns:

- `EngineBuilder.WithStrata(...)`
- `EngineBuilder.WithStrataAPIKey(...)`
- `EngineBuilder.WithPostgresDSN(...)`
- `EngineBuilder.Build(builder Builder)`
- `swf.Builder`
- `swf.Lease`
- `swf.Dependencies`
- `JobKey.ToStoryKey()`
- `JobKeyFromStoryKey(...)`
- `WorkSet.Capabilities`
- `swf.FromStrataArtifact(...)`
- `swf.ToStrataArtifact(...)`
- `swf.PgwfMetadataPredicates(...)`

## Engine Creation

### Old: direct builder config + impl.Builder

This is the old construction style:

```go
import (
    "log/slog"
    "time"

    "github.com/colony-2/swf-go/pkg/swf"
    "github.com/colony-2/swf-go/pkg/swf/impl"
)

engine, err := swf.NewEngineBuilder().
    WithPostgresDSN(postgresDSN).
    WithStrata(strataBaseURL).
    WithStrataAPIKey(strataAPIKey).
    WithLogger(slog.Default()).
    WithAwaitRecycleThreshold(5 * time.Minute).
    PlusWorkers(jobWorker, taskWorkers...).
    Build(impl.Builder)
if err != nil {
    return err
}
```

### New: direct runtime

Preferred replacement:

```go
import (
    "log/slog"
    "time"

    "github.com/colony-2/swf-go/pkg/swf"
    directruntime "github.com/colony-2/swf-go/pkg/swf/runtime/direct"
)

runtime, err := directruntime.NewFromConfig(postgresDSN, strataBaseURL, strataAPIKey)
if err != nil {
    return err
}

engine, err := swf.NewEngineBuilder().
    WithRuntime(runtime).
    WithLogger(slog.Default()).
    WithAwaitRecycleThreshold(5 * time.Minute).
    PlusWorkers(jobWorker, taskWorkers...).
    BuildEngine()
if err != nil {
    return err
}
```

### New: direct runtime with already-created clients

If your application already manages DB and Strata clients:

```go
import (
    "github.com/colony-2/swf-go/pkg/swf"
    directruntime "github.com/colony-2/swf-go/pkg/swf/runtime/direct"
)

runtime := directruntime.New(gormDB, strataClient)

engine, err := swf.NewEngineBuilder().
    WithRuntime(runtime).
    PlusWorkers(jobWorker, taskWorkers...).
    BuildEngine()
if err != nil {
    return err
}
```

### New: toy runtime

For local tests and in-memory execution:

```go
import (
    "github.com/colony-2/swf-go/pkg/swf"
    toyruntime "github.com/colony-2/swf-go/pkg/swf/runtime/toy"
)

engine, err := swf.NewEngineBuilder().
    WithRuntime(toyruntime.New()).
    PlusWorkers(jobWorker, taskWorkers...).
    BuildEngine()
if err != nil {
    return err
}
```

## Backward Compatibility

If you set `WithRuntime(...)`, the legacy `Build(...)` entry point will use the runtime-based path:

```go
engine, err := swf.NewEngineBuilder().
    WithRuntime(runtime).
    PlusWorkers(jobWorker, taskWorkers...).
    Build(nil)
```

That works today, but `BuildEngine()` is the intended replacement and should be preferred in downstream code.

## Specific API Migration Notes

### 1. Engine builder configuration

Old:

- `WithPostgresDSN(...)`
- `WithStrata(...)`
- `WithStrataAPIKey(...)`
- `Build(impl.Builder)`

New:

- construct a runtime
- `WithRuntime(runtime)`
- `BuildEngine()`

Practical rule:

- if you are building an engine, stop passing backend connection details through `swf.EngineBuilder`
- put those details into the runtime implementation instead

### 2. `swf.Builder`

Old:

```go
type Builder func(db *gorm.DB, strataClient *strataclient.Client, workers []WorkSet, logger *slog.Logger) (SWFEngine, error)
```

This is deprecated because it exposes a Strata client directly and ties construction to the old direct backend model.

New:

- downstream users should not implement or depend on `swf.Builder`
- if you need a custom backend, implement `swf.WorkflowRuntime`

### 3. `JobKey.ToStoryKey()` and `JobKeyFromStoryKey(...)`

These are deprecated because they expose Strata `story.Key` in the public SWF surface.

Old:

```go
storyKey := jobKey.ToStoryKey()
jobKey = swf.JobKeyFromStoryKey(storyKey)
```

New guidance:

- keep using `swf.JobKey` in downstream code
- if you still have direct Strata integration, isolate that translation in your direct-runtime-specific layer
- do not make `story.Key` part of your application-facing workflow code

### 4. `swf.Lease` and `swf.Dependencies`

These are deprecated because they re-export `pgwf` types through the SWF API.

If you are using them directly today, that code is coupled to the old direct backend internals.

New guidance:

- do not write new code against `swf.Lease` or `swf.Dependencies`
- move worker scheduling / reschedule / completion logic behind the runtime abstraction
- if you need direct pgwf behavior, keep it in code that is explicitly direct-runtime-specific

### 5. `WorkSet.Capabilities`

`WorkSet.Capabilities` is deprecated because it exposes `pgwf.Capability`.

Most downstream users should not need this field directly.

Old pattern:

```go
for _, cap := range workSet.Capabilities {
    ...
}
```

New guidance:

- treat `WorkSet` as a registration container for job and task workers
- do not depend on pgwf capability strings from outside runtime/direct internals

### 6. Artifact bridge helpers

Deprecated:

- `swf.FromStrataArtifact(...)`
- `swf.ToStrataArtifact(...)`

These exist only to bridge SWF and Strata artifact types.

Old:

```go
swfArt := swf.FromStrataArtifact(strataArt)
rawStrataArt := swf.ToStrataArtifact(swfArt)
```

New guidance:

- downstream code should use `swf.Artifact`
- artifact creation should use SWF constructors:
  - `swf.NewArtifactFromBytes(...)`
  - `swf.NewArtifactFromReader(...)`
  - `swf.NewArtifactFromFile(...)`
  - `swf.NewArtifact(...)`
- if conversion to/from Strata is still unavoidable, keep it in direct-runtime-specific code only

Example:

```go
artifact := swf.NewArtifactFromBytes("output.json", outputBytes)
taskData := swf.NewTaskDataOrPanic(payload, artifact)
```

### 7. `swf.PgwfMetadataPredicates(...)`

This helper is deprecated because it exposes `pgwf.MetadataPredicate`.

Downstream guidance:

- keep using `swf.MetadataFilter` and `swf.ListJobsRequest`
- do not write new code that depends on `pgwf.MetadataPredicate` through SWF
- if you must convert SWF filters to pgwf filters, do it only in direct-runtime-specific code

## Recommended Migration Sequence

### For application code

1. Replace engine construction with `WithRuntime(...)` + `BuildEngine()`.
2. Switch production engines to `runtime/direct`.
3. Switch in-memory/test engines to `runtime/toy`.
4. Stop using deprecated bridge helpers and direct backend types in workflow-facing code.

### For libraries built on top of SWF

1. Remove references to `impl.Builder`.
2. Remove references to `story.Key`, `pgwf.Lease`, and `pgwf.JobDependencies` from your public surface.
3. Keep backend-specific adaptation in a dedicated direct-runtime integration layer.

## Example Before / After

### Before

```go
builder := swf.NewEngineBuilder().
    WithPostgresDSN(cfg.PostgresDSN).
    WithStrata(cfg.StrataBaseURL).
    WithStrataAPIKey(cfg.StrataAPIKey).
    PlusWorkers(myJobWorker, myTaskWorker)

engine, err := builder.Build(impl.Builder)
if err != nil {
    return err
}
```

### After

```go
runtime, err := directruntime.NewFromConfig(
    cfg.PostgresDSN,
    cfg.StrataBaseURL,
    cfg.StrataAPIKey,
)
if err != nil {
    return err
}

engine, err := swf.NewEngineBuilder().
    WithRuntime(runtime).
    PlusWorkers(myJobWorker, myTaskWorker).
    BuildEngine()
if err != nil {
    return err
}
```

## Current Compatibility Expectation

At this stage:

- the deprecated APIs still exist
- the new runtime APIs are functional
- both `direct` and `toy` runtimes are available

Downstream users should migrate now, even if the old APIs still compile, because the next step is removal of the deprecated backend-leaking surface.
