# SWF Migration - server/recipe-core

## Objectives
- Remove Temporal SDK dependencies and provide SWF-compatible primitives for invocation tracking, run metadata, and story logging, eliminating marker/inline-op concepts.

## Scope & Plan (Temporal usage to remove)
- `pkg/story/markers.go`: remove Temporal markers/converter usage and drop explicit marker tracking; rely on SWF/Strata chapters emitted by recipe-worker instead of replacement marker APIs.
- `pkg/ops/registerable_op.go`: inline-op signatures use `workflow.Context` and `temporal.RetryPolicy`; redesign to SWF-friendly contexts/retry config and drop inline-op abstraction.
- `pkg/recipe/schema_validate_test.go`, `pkg/ops/registerable_op_test.go`: update tests off Temporal types.
- `pkg/ops/service_deps2.go`: remove temporal namespace plumbing.
- Documentation references (`AGENTS.md`, `OPS_EXECUTE_DEPS_EXTENSION.md`, `OPS_PROVIDER_REFACTOR.md`) need SWF-aligned APIs.
- **Story logging**: rely on SWF/Strata chapters emitted by recipe-worker; remove Temporal marker-specific code/decoding.
- **Retry/error model**: adopt `swf.RetryPolicy` (from swf-go) for retries; define local error types encoding retryability; remove Temporal `ApplicationError`.
- **Invocation metadata**: update trackers to operate without `workflow.Context` or inline-op notions, using explicit sequence counters passed by recipe-worker.
- **Resume**: keep `runmetadata` engine-neutral and serializable into SWF `TaskData`/chapter clones.
- **Testing**: port logging/retry tests to chapter-based flows, removing Temporal fixtures.

## SWF Gaps Affecting This Project
- No native markers/signals; story recording must ride on chapter payloads.
- No built-in activity error taxonomy; define retryable/non-retryable semantics locally.
