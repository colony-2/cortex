# RegisterableOp Invocation Dependency Reinjection Spec

## Background
- The recipe compiler injects runtime collaborators into `ops.Invocation.Deps` before scheduling activities (`server/recipe-worker/pkg/compiler/invocation_tracker.go:38`).
- `ops.Invocation.Deps` is marked with serialization ignores (`json:"-" yaml:"-" mapstructure:"-"`) so Temporal payloads never carry concrete handles across process boundaries (`server/recipe-core/pkg/ops/invocation.go:13`).
- The recipe worker currently serializes `workerops.ActivityInvocationRequest` without rehydrating dependencies, so activities run with `Invocation.Deps == nil` inside Temporal workers (`server/recipe-worker/pkg/ops/activity_registry.go:18`).
- Ops that rely on workflow control, SSE, or database collaborators fail or silently downgrade capabilities when `Deps` is absent.

## Problem Statement
Activity executions triggered by Temporal lose access to their required service collaborators because `Invocation.Deps` is stripped during serialization. This severs the contract between the worker runtime and ops that expect hydrated dependencies, leading to runtime errors and loss of functionality.

## Goals
- Restore dependency hydration for every `RegisterableOp.ExecuteV2` invocation executed by Temporal workers.
- Keep dependency wiring confined to the Temporal registration layer so ops and wrapper helpers stay unaware of serialization details.
- Provide test coverage ensuring `Invocation.Deps` is non-nil when dependencies have been registered with the worker.

## Non-Goals
- Changing inline execution (`ExecuteInlineV2`), which already preserves dependencies within workflow contexts.
- Altering the shape of `ServiceDependencies2` or how collaborators are provided by the worker.
- Modifying Temporal payload codecs or introducing new serialization mechanisms.

## Requirements
1. **Centralized Hydration**: The dependency reinjection must occur exactly where the worker registers activities with Temporal (i.e., within `ActivityRegistry.EnableActivitiesInWorker`). No individual op, Git wrapper, or third-party extension should need to perform hydration explicitly.
2. **Backward Compatibility**: Existing activity registration semantics (names, options, git workspace preparation) must remain intact.
3. **Graceful Absence**: If the worker has no registered dependencies, activities must continue to execute with `Invocation.Deps == nil` without panicking.
4. **Testability**: Unit tests must prove that a registered dependency container becomes visible inside activity handlers after passing through the Temporal worker boundary.

## Proposed Design
1. Introduce a helper within `server/recipe-worker/pkg/ops/activity_registry.go` (e.g., `wrapWithDependencies`) that:
   - Accepts an activity registration and shared `ServiceDependencies2` container.
   - Ensures `req.Invocation.Deps` is set to the shared container before delegating to the actual op handler.
   - Returns a closure matching Temporal's activity signature.
2. Integrate the helper within `EnableActivitiesInWorker`, composing it with existing wrappers such as `withGitWorkspace`. This preserves Git workspace preparation while guaranteeing centralized dependency hydration.
3. Ensure the registry stores the dependency container supplied via `WorkerManager.SetDependencies` so the helper can access it when the worker is constructed.

## Implementation Plan
1. **Registry Enhancements**
   - Add `wrapWithDependencies` in `activity_registry.go` and call it from `EnableActivitiesInWorker` after the Git workspace wrapper is applied.
   - Remove any ad-hoc `Invocation.Deps` assignments elsewhere to avoid duplication.
2. **Testing**
   - Extend `server/recipe-worker/pkg/ops/activity_registry_test.go` with a fake dependency container and assert the handler receives `invocation.Deps`.
   - Run `go test ./server/recipe-worker/...` to verify regression safety.
3. **Documentation & Cleanup**
   - Update any internal docs referencing the old behavior (if applicable) and ensure comments highlight the centralized hydration point.

## Alternatives Considered
- **Encoding Dependencies into Temporal Payloads**: Rejected because dependencies encapsulate live handles (DB connections, Temporal clients) that cannot be marshaled safely.
- **Per-Op Hydration**: Rejected to avoid duplication and to keep serialization concerns out of op implementations.

## Testing Strategy
- New unit test asserting dependency visibility inside wrapped activities.
- Full recipe worker suite via `go test ./server/recipe-worker/...`.

## Risks & Mitigations
- **Risk**: Additional wrapper composition could change the registered activity identity. *Mitigation*: Ensure the helper preserves options and names, and leverage existing registration tests.
- **Risk**: Future wrappers might bypass the helper. *Mitigation*: Enforce hydration inside `EnableActivitiesInWorker`, the single entry point for Temporal registration.

## Rollout
- Ship alongside recipe worker binaries; no schema or Temporal registration changes required.
- Monitor logs for dependency-aware ops (e.g., ticket operations) to confirm absence of missing dependency warnings post-rollout.

## Open Questions
- Should missing dependencies emit structured logs or metrics to aid diagnostics?
- Do we need guardrails ensuring ops avoid mutating `Invocation.Deps`, given reinjection happens immediately before execution?
