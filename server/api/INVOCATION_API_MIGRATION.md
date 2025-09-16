# Invocation API V2 Migration (Deterministic Invocation Context)

This document defines an ordered, barriered migration to introduce an explicit, deterministic Invocation API (V2) in recipe-core, have recipe-worker pass invocation metadata to every op call, and migrate ops and API tests to the new pattern. It is written for LLM consumption and remains human‑readable.

Notes
- Run the detect command for each step in the root of the listed directories.
- A step is complete when its detect command outputs nothing (no matches) in all affected directories.
- The framework should enforce a barrier: do not proceed to the next step until all components pass the current step.

## Affected Directories by Step

| Step ID | Description | Directories |
|---|---|---|
| core-add-v2-api | Add Invocation type + V2 constructors/signatures | server/recipe-core |
| worker-pass-invocation | Pass Invocation to ops (inline/activity) | server/recipe-worker |
| ops-use-v2-constructors | Migrate ops to V2 constructors/signatures | server/ops, server/activity, server/git |
| api-tests-require-id | Require id in respond/cancel test bodies | server/api |
| core-remove-v1-api | Remove legacy V1 constructors/signatures | server/recipe-core |
| openapi-update-schemas | Require id in respond/cancel schemas/clients | api/openapi, server/openapi, web/openapi |
| cleanup-dead-code | Remove remaining V1/deprecated code | server/recipe-core, server/recipe-worker, server/ops, server/activity, server/git, server/api, api/openapi, server/openapi, web/openapi |

---

## 1) core-add-v2-api (server/recipe-core)

- Change (LLM concise):
  - Add ops.Invocation {RecipeID, NodePath, InvokeSeq, BoxID, ActivityID, ID} with method Key().
  - Add V2 handler types that accept Invocation explicitly:
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
  - Compute Invocation {RecipeID, NodePath, InvokeSeq, BoxID, ActivityID} per op call (deterministic).
  - Optionally precompute inv.ID = inv.Key().
  - Invoke ops via V2 signatures:
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

- Detect (no output means done):
```
bash -lc 'grep -RIn --exclude-dir vendor --exclude-dir node_modules -E "New(InlineOp|ActivityMappedOp)(WithManagement)?\s*\[" server/ops server/activity server/git || true'
```

---

## 4) api-tests-require-id (server/api)

- Change (LLM concise):
  - Ensure POST /api/user-inputs/{workflowID}/respond and /cancel request bodies include an "id" field in all tests.
  - Remove or fix tests that post without id.

- Detect (no output means done):
```
bash -lc 'grep -RIn --exclude-dir vendor --exclude-dir node_modules -E "/api/user-inputs/.+/respond|/api/user-inputs/.+/cancel" server/api | grep -v ""id"" || true'
```

---

## 5) core-remove-v1-api (server/recipe-core)

- Change (LLM concise):
  - Remove legacy V1 constructors (NewInlineOp, NewActivityMappedOp) and any V1 handler types.
  - Ensure all ops have migrated to V2 before removal.

- Detect (no output means done):
```
bash -lc 'grep -RIn --exclude-dir vendor --exclude-dir node_modules -E "func\s+New(InlineOp|ActivityMappedOp)\[|type\s+InlineHandler\s*func|type\s+ActivityHandler\s*func" server/recipe-core || true'
```

---

## 6) openapi-update-schemas (api/openapi, server/openapi, web/openapi)

- Change (LLM concise):
  - Update OpenAPI respond/cancel request schemas to require "id".
  - Regenerate server/openapi (oapi-codegen) and web/openapi (TS client) and api/openapi if applicable.

- Detect (no output means done):
```
bash -lc 'grep -RIn --exclude-dir node_modules --exclude-dir vendor -E "user-inputs/.+/respond|user-inputs/.+/cancel" api/openapi server/openapi web/openapi | grep -v ""id"" || true'
```

---

## 7) cleanup-dead-code (all components)

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
