# Git State Adapter Decoupling (Spec)

## Problem
- The recipe op (`server/ops/pkg/recipe`) needs a git workspace helper, but the current adapter lives in recipe-worker and depends on Temporal types (`workflow.Context`).
- To break the ops ↔ recipe-worker dependency, we need a shared contract that is **not Temporal-specific** and can be implemented by any runtime (Temporal or otherwise).
- The git module should not gain Temporal dependencies; the adapter contract should stay neutral.

## Goals
- Provide a runtime-neutral git workspace orchestration API (no `workflow.Context`).
- Allow the recipe op to depend only on the neutral API (and recipe-core), not on recipe-worker.
- Keep recipe-worker’s Temporal implementation unchanged behaviorally, but hide its Temporal surface behind the neutral API.
- Make it easy to add other runtimes (standalone/local executor) without changing the recipe op.

## Non-goals
- Changing git workspace semantics (prepare/restore/persist) or outputs.
- Moving recipe-worker’s gitstate logic into another module.
- Refactoring other ops (input/llm/etc.).

## Proposed Design
### 1) Define a neutral API in recipe-core
- New package: `server/recipe-core/pkg/gitworkspace` (name bikesheddable).
- Types (no Temporal imports):
  - `type InlineOptions struct { ChildID string; SkipFinalize bool }`
  - `type DetachedOptions struct { ChildID string }`
  - `type Context interface { ToMap() map[string]interface{}; GetPersistHash() string; GetWorktreePath() string; GetBlobStoreURI() string; GetTicketID() string; GetCellName() string }`
  - `type InlineResult struct { Result map[string]interface{}; ContextMap map[string]interface{}; GitContext Context }`
  - `type Orchestrator interface { WithInlineWorkspace(ctx context.Context, inv ops.Invocation, inputs map[string]interface{}, opts InlineOptions, fn func(context.Context, map[string]interface{}) (map[string]interface{}, error)) (*InlineResult, error); PlanDetachedWorkspace(inv ops.Invocation, inputs map[string]interface{}, opts DetachedOptions) (Context, map[string]interface{}, error) }`
  - Optional no-op/err implementation for tests.

### 2) Temporal implementation inside recipe-worker
- New type `gitstate.TemporalOrchestrator` implementing the neutral `Orchestrator`.
  - Constructor: `NewTemporalOrchestrator(wctx workflow.Context)` stores the Temporal context internally.
  - `WithInlineWorkspace` delegates to existing gitstate inline logic, but the external signature is `context.Context`; the callback receives a background/derived `context.Context` while the orchestrator uses the stored `workflow.Context` for local activities.
  - `PlanDetachedWorkspace` wraps the existing detached logic similarly.
- Expose a thin adapter from gitstate to the neutral API; no exports of Temporal types.

### 3) Wiring for the recipe op
- In `server/ops/pkg/recipe`, replace the current gitstate adapter global with:
  - A dependency on `gitworkspace.Orchestrator` (from recipe-core).
  - Registration API: `gitworkspace.Register(orchestrator)` stays in recipe-core.
  - Calls to `withInlineWorkspace/planDetachedWorkspace` become calls to the orchestrator methods.
- Recipe-worker sets up the orchestrator during worker init (or in a dedicated init in `pkg/ops`) by registering a `gitstate.NewTemporalOrchestrator(ctx)` factory or a global instance usable by the recipe op.

### 4) Backward compatibility / safety
- Provide a default `ErrOrchestratorNotRegistered` in the neutral package; tests in ops should assert the error when not registered (mirrors today’s behavior).
- Recipe-worker tests continue to use Temporal test env; ops tests can use a fake orchestrator.

## Migration Plan
1. Add `recipe-core/pkg/gitworkspace` with the neutral API + no-op implementation.
2. Update ops recipe op to depend on the new package, remove its local adapter, and rewire calls.
3. Add `gitstate.TemporalOrchestrator` in recipe-worker implementing the neutral interface; register it during worker startup (and in tests where needed).
4. Delete the old recipe-worker → ops adapter file for good.
5. Run `go test ./...` in `server/recipe-worker`, `server/ops`, and (if added) `server/recipe-core`.

## Open Questions
- Where to register the orchestrator lifecycle-wise? Likely in worker init, but we may also need registration hooks for standalone executors/tests.
- Should the neutral API live in recipe-core or a new small module (e.g., `server/gitworkspace`)? Leaning recipe-core to avoid yet another module.
- Do we need a way to pass runtime metadata (e.g., deterministic clock) into the callback? If so, add an optional `ctx` wrapper or metadata struct without importing Temporal.
