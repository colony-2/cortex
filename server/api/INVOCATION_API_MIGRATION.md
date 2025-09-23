# Invocation API V2 Migration (Deterministic Invocation Context)

This document defines an ordered, barriered migration to introduce an explicit, deterministic Invocation API (V2) in recipe-core, have recipe-worker pass invocation metadata to every op call, and migrate ops and API tests to the new pattern. It is written for LLM consumption and remains human‑readable.

Guiding principles
- We pass new arguments explicitly to handlers (Invocation as a first parameter), we do NOT stuff custom values into workflow.Context or context.Context.
- RegisterableOp gains V2 execution methods; legacy methods simply call the V2 methods with an empty Invocation as a temporary shim.
- Steps MUST be followed in order. Do not start a later step until the current step has zero matches in all affected directories.

Notes
- Run the detect command for each step in the root of the listed directories.
- A step is complete when its detect command outputs nothing (no matches) in all affected directories.
- The framework should enforce a barrier: do not proceed to the next step until all components pass the current step.

## Affected Directories by Step

| Step | Step ID | Description | Directories |
|---|---|---|
| 1 | core-add-v2-api | Add Invocation type + V2 constructors/signatures | server/recipe-core |
| 2 | worker-pass-invocation | Pass Invocation to ops (inline/activity) | server/recipe-worker |
| 3 | ops-use-v2-constructors | Migrate ops to V2 constructors/signatures | server/ops, server/activity, server/git |
| 4 | core-remove-v1-api | Remove legacy V1 constructors/signatures | server/recipe-core |
| 5 | cleanup-dead-code | Remove remaining V1/deprecated code | server/recipe-core, server/recipe-worker, server/ops, server/activity, server/git, server/api, api/openapi, server/openapi, web/openapi |

---

## 1) core-add-v2-api (server/recipe-core)

- Change (LLM concise):
  - Add ops.Invocation {RecipeID, NodePath, InvokeSeq, BoxID, ActivityID, ID} with method InvocationContext().
  - Add Invocation.Hash() which is a short deterministic hash of the invocation object.
  - Add V2 handler types that accept Invocation explicitly (arguments, not context injection):
    - InlineHandlerV2(inv Invocation, wctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, in In) (Out, error)
    - ActivityHandlerV2(inv Invocation, actx context.Context, in In) (Out, error)
  - Add constructors NewInlineOpV2, NewInlineOpWithManagementV2 and NewActivityMappedOpV2 that bind these handler types.

- Detect (no output means done):
```
bash -lc '(! grep -RIl --exclude-dir vendor --exclude-dir node_modules -E "type\s+Invocation\s+struct" server/recipe-core >/dev/null) || (! grep -RIl --exclude-dir vendor --exclude-dir node_modules -E "New(InlineOpV2|ActivityMappedOpV2)\s*\[" server/recipe-core >/dev/null) && echo core_v2_api_missing'
```

---

## 2) worker-pass-invocation (server/recipe-worker)

- Change (LLM concise):
  - Compute Invocation based on Invocation.Hash()
  - Optionally precompute inv.ID = inv.Hash().
  - Invoke ops via V2 signatures and V2 execution methods on RegisterableOp (compiler passes extra arg):
    - Inline: handler(inv, workflow.Context, ...)
    - Activity: handler(inv, context.Context, ...)
  - Remove Invocation‑less calls.

- Detect (no output means done):
```
bash -lc 'grep -RIn --exclude-dir vendor --exclude-dir node_modules -E "ExecuteInline\(\s*ctx\s*,|Execute\(\s*ctx\s*," server/recipe-worker || true'
```

---

## 3) ops-use-v2-constructors (server/ops, server/activity, server/git)

- Change (LLM concise):
  - Replace NewInlineOp/NewActivityMappedOp(/WithManagement) with NewInlineOpV2/NewActivityMappedOpV2.
  - Update handlers to accept Invocation as first parameter and use it as needed.
  - Ensure RegisterableOp implementations expose ExecuteInlineV2/ExecuteV2 and route legacy methods through the V2 methods.
  - Include wrappers (e.g., GetOp returning ops.RegisterableOp) in migration: any GetOp that does not use V2 constructors must be updated.

- Detect (no output means done):
```
bash -lc '( grep -RIn --exclude-dir vendor --exclude-dir node_modules -E "ops\.New(InlineOp|ActivityMappedOp)(WithManagement)?\s*\[" server/ops server/activity server/git || true;   grep -RIl --exclude-dir vendor --exclude-dir node_modules -E "func\s+GetOp\s*\(\)\s*ops\.RegisterableOp" server/ops server/activity server/git   | xargs -r grep -L -E "New(InlineOpV2|ActivityMappedOpV2)\s*\[" || true )'
```

---

## 4) core-remove-v1-api (server/recipe-core)

- Change (LLM concise):
  - Remove legacy V1 constructors (NewInlineOp, NewActivityMappedOp) and any V1 handler types.
  - Ensure all ops have migrated to V2 before removal.

- Detect (no output means done):
```
bash -lc 'grep -RIn --exclude-dir vendor --exclude-dir node_modules -E "func\s+New(InlineOp|ActivityMappedOp)\[|type\s+InlineHandler\s*func|type\s+ActivityHandler\s*func" server/recipe-core || true'
```
---

## 5) cleanup-dead-code (all components)

- Change (LLM concise):
  - Remove any remaining adapters, legacy invocation paths, or shims related to V1 invocation.
  - Eliminate all references to V1 constructors or Invocation‑less signatures.

- Detect (no output means done):
```
bash -lc 'grep -RIn --exclude-dir vendor --exclude-dir node_modules -E "Invocation-less|LegacyInvocation|V1Invocation|New(InlineOp|ActivityMappedOp)(WithManagement)?\s*\[" server/recipe-core server/recipe-worker server/ops server/activity server/api api/openapi server/openapi web/openapi || true'
```

---

Completion
- When all steps report no matches across their affected directories, the migration to Invocation API V2 is complete.
