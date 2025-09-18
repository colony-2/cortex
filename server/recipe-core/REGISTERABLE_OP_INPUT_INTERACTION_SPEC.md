**RegisterableOp Input Interaction (Inline User Input from Any Op)**

Overview
- Problem: Today, only the dedicated input op can prompt users and wait for a response. Other ops must “exit” and schedule a separate input op, which fragments logic and complicates recipes.
- Goal: Provide a recipe-core API that any op can call to request user input inline (within the same op), while consolidating all infrastructure (HTTP routes, SSE, Temporal client usage) inside recipe-worker. The existing input op stays, but becomes a thin wrapper that delegates to the new handler.

Motivation
- Keep complex flows cohesive: allow an op to collect data exactly when it needs it, without splitting state across multiple ops.
- Reuse proven mechanisms: leverage the existing workflow signal and UI patterns already used by the input op, but centralize the infra inside recipe-worker.
- Preserve determinism: limit interactive support to inline execution where workflows can safely block on signals.

Scope & Non‑Goals
- Scope: Inline user input from any op that runs inline within a Temporal workflow (i.e., uses `workflow.Context`).
- Non‑Goals: Supporting interactive prompts from Activity execution. For Activities, `RequestUserInput` returns a typed error (not supported). Activities should either run inline or ask the workflow to perform the prompt.

API Changes (Additive)
- Ownership split: recipe-core exposes the dependency-free API surface; recipe-worker provides the infrastructure and default implementation.
- New interfaces and V2 execution signatures are introduced in recipe‑core. Legacy methods remain as shims that call V2 with a no‑op interaction.

1) New types in `recipe-core/pkg/ops` (API surface)
- `type InputForm struct { ... }` and related field types live in recipe‑core, dependency-free. The shape mirrors the existing input op’s form model.
- `type UserInputResult struct { Fields map[string]interface{}; UserID string; Metadata map[string]interface{} }`
- `type InputOptions struct { Timeout time.Duration; BoxID string; ActivityID string; Context map[string]interface{} }`
- `type InputInteraction interface { RequestUserInput(wctx workflow.Context, form InputForm, opts InputOptions) (UserInputResult, error) }`
- `var ErrInteractionNotSupported = temporal.NewApplicationError("input interaction not supported in activities", "INTERACTION_UNSUPPORTED_IN_ACTIVITY")`

2) New V2 execution methods on `RegisterableOp`
- Inline (new):
  - `ExecuteInlineV2(inv Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, interaction InputInteraction, input map[string]interface{}) (map[string]interface{}, error)`
- Activity (new):
  - `ExecuteV2(inv Invocation, ctx context.Context, interaction InputInteraction, input map[string]interface{}) (map[string]interface{}, error)`
- Legacy methods keep existing signatures and are updated to call the V2 versions with `interaction = NoopInteraction`:
  - `Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)`
  - `ExecuteInline(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input map[string]interface{}) (map[string]interface{}, error)`

3) Constructors (V2)
- Add V2 constructors mirroring existing ones but binding handlers with V2 signatures (first parameter `Invocation`, plus `InputInteraction`).
  - `NewInlineOpV2[In,Out](md OpMetadata, fn func(inv Invocation, wctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, interaction InputInteraction, in In) (Out, error)) RegisterableOp`
  - `NewActivityMappedOpV2[In,Out](md OpMetadata, fn func(inv Invocation, actx context.Context, interaction InputInteraction, in In) (Out, error)) RegisterableOp`

Infrastructure Ownership
- recipe-worker implements the concrete `InputInteraction` used during execution:
  - Maintains HTTP routes (`/api/user-inputs/...`) and SSE broadcasting centrally in worker; removes per-op management service coupling.
  - Uses Temporal client or an SDK-agnostic `WorkflowControl` to signal `user-response` and to optionally mark pending/completed.
  - Handles search attribute upserts (pending/completed/timeout) inside workflow code paths where deterministic.
- recipe-core remains free of HTTP, SSE, or Temporal client dependencies; it only defines the types and function signatures.

Execution Semantics
- Inline only (supported):
  - `InputInteraction.RequestUserInput` performs the same behavior as the existing input op with worker-owned infra:
    - Within workflow code: upsert search attributes (`InputStatus`, `InputFormTitle`, etc.), and wait for `user-response` with timer-driven timeout.
    - Outside workflow code: worker-owned HTTP endpoint emits `input_pending` SSE; on submit, worker signals the workflow and emits `input_completed` or `input_cancelled`.
    - Returns `UserInputResult{Fields, UserID, Metadata}`.
- Activity (not supported):
  - `RequestUserInput` returns `ErrInteractionNotSupported`, indicating the op must run inline to prompt.

Wire‑Up (Worker Responsibilities)
- `recipe-worker` constructs the concrete `InputInteraction` and wires infra once, centrally:
  - Inline: pass the concrete interaction into `ExecuteInlineV2`, which handles search attributes and signal waiting.
  - Activity: pass a `NoopInteraction` that always errors for `RequestUserInput`.
  - HTTP/SSE: expose `/api/user-inputs/...` endpoints and stream events from the central worker service; no per-op management service is required.

Backwards Compatibility
- All existing ops continue to work unchanged through the legacy methods.
- Ops can opt into interactive prompts by switching to V2 constructors/signatures and using the `interaction` parameter.
- The dedicated input op remains valid and stays in `server/ops`, but becomes a thin V2 inline op that delegates to `interaction.RequestUserInput`. Its previous per-op management service is superseded by the central worker service.

Illustrative API Sketch (concise)
```
// pkg/ops/types.go
type InputForm struct { /* mirrors existing input op form */ }
type UserInputResult struct {
    Fields   map[string]interface{}
    UserID   string
    Metadata map[string]interface{}
}
type InputOptions struct {
    Timeout    time.Duration
    BoxID      string
    ActivityID string
    Context    map[string]interface{}
}
type InputInteraction interface {
    RequestUserInput(wctx workflow.Context, form InputForm, opts InputOptions) (UserInputResult, error)
}

// pkg/ops/registerable_op.go
type RegisterableOp interface {
    // V1 (unchanged)
    Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
    ExecuteInline(ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input map[string]interface{}) (map[string]interface{}, error)

    // V2 (new)
    ExecuteV2(inv Invocation, ctx context.Context, interaction InputInteraction, input map[string]interface{}) (map[string]interface{}, error)
    ExecuteInlineV2(inv Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, interaction InputInteraction, input map[string]interface{}) (map[string]interface{}, error)

    // ... existing metadata/type methods ...
}

// V1 methods become shims that pass NoopInteraction and empty Invocation.
```

Example Usage (Inline Op)
```
op := ops.NewInlineOpV2[MyIn, MyOut](
    ops.OpMetadata{Type: "build_and_confirm", DefaultTimeout: 10 * time.Minute},
    func(inv ops.Invocation, wctx workflow.Context, to time.Duration, retry *temporal.RetryPolicy, ui ops.InputInteraction, in MyIn) (MyOut, error) {
        // ... do some work ...
        form := ops.InputForm{Title: "Approve deployment?", Fields: []ops.FormField{{ID: "approved", Type: ops.FieldTypeCheckboxes, Question: "Deploy now?", Required: true}}}
        res, err := ui.RequestUserInput(wctx, form, ops.InputOptions{Timeout: 5 * time.Minute, BoxID: inv.BoxID, ActivityID: inv.ActivityID})
        if err != nil { return MyOut{}, err }
        if ok, _ := res.Fields["approved"].(bool); !ok { return MyOut{}, fmt.Errorf("deployment not approved") }
        // ... continue ...
        return MyOut{Status: "deployed"}, nil
    },
)
```

Determinism & Signals
- Waiting on `workflow.GetSignalChannel(ctx, "user-response")` is deterministic. The interaction impl must keep all non‑deterministic work (HTTP calls, SSE) outside the workflow code path and confine workflow changes to:
  - Upsert of typed search attributes via Temporal’s deterministic APIs
  - Blocking on signal with a timer‑based timeout branch

OpenAPI/Frontend Impact
- No new endpoints required; the `/api/user-inputs/...` routes and SSE events live in recipe-worker’s central service.
- Frontend continues listening to `input_pending`, `input_completed`, and `input_cancelled` SSE events.

Testing Plan (High‑Level)
- Core unit tests: compile‑time tests for types and shims in recipe-core; no network/HTTP.
- Worker tests: concrete `InputInteraction` impl with workflow test env to assert pending → completed lifecycle and search attributes, plus HTTP/SSE integration.
- Integration: convert one existing op to V2 and prompt mid‑execution; verify recipe runs without inserting a separate input op.
- Regression: legacy V1 ops and the dedicated input op still pass.

Migration Notes
- This dovetails with the existing `INVOCATION_API_V2` plan: reuse Invocation and V2 constructors; add `InputInteraction` parameter.
- Phase 1 (core): add types and V2 methods in recipe-core; legacy methods shim to V2 with `NoopInteraction`.
- Phase 2 (worker): implement central `InputInteraction`, HTTP routes, and SSE events; stop relying on per-op management services for input.
- Phase 3 (ops): update the existing input op to call `interaction.RequestUserInput` and remove direct infra dependencies; migrate other ops as needed.
