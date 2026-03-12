# Extension Op Feasibility Assessment

This evaluates the ops documented in this directory against the current extension-op mechanism described in [EXTENSION_OPS.md](/src/recipes/guides/ops/EXTENSION_OPS.md).

## Current extension-op baseline

Today, an extension op is:

- A runtime-discovered external command.
- A single-step activity that gets JSON on `stdin` and returns JSON on `stdout`.
- Able to validate input/output with JSON Schema.
- Able to consume artifact bindings like other artifact-accepting ops.

Today, an extension op is **not**:

- A multi-step op with engine-managed wait/resume behavior.
- An op with injected `OpDependencies` such as database access, workflow control, job waiting, or artifact registration APIs.
- An op with a `ManagementService`, HTTP routes, SSE, or other UI control surfaces.
- An op with a standard way to return typed retryable/non-retryable workflow errors.

## Summary

| Op | Current verdict |
| --- | --- |
| `command_execution` | Yes |
| `sleep` | Yes |
| `codex.exec` | Yes |
| `llm_inference2` | Yes |
| `thinpackrebase` | Yes |
| `squashrebasemerge` | Yes |
| `cells.list` | Only with core additions |
| `ticket.manage` | Only with core additions |
| `input` | No |
| `recipe.run_and_get_result` | No |
| `recipes.run` | No |
| `recipes.run_and_wait` | No |
| `recipe.await_result` | No |
| `recipe.get_result` | No |

## `command_execution`

| Item | Assessment |
| --- | --- |
| Extension op today? | Yes |
| Why | It is already a single-step wrapper around local process execution. It does not depend on DB, workflow control, management routes, or wait/resume behavior. |
| Core changes required | None. |
| Optional framework improvements | Auto-export `workdir`/`inbox`/`outbox` paths as env vars for extension ops. |
| Recommendation | Move to extension if you want; behavior should be equivalent. |

## `sleep`

| Item | Assessment |
| --- | --- |
| Extension op today? | Yes |
| Why | Pure single-step timer behavior with no injected services. |
| Core changes required | None. |
| Optional framework improvements | None needed. |
| Recommendation | Move to extension. |

## `codex.exec`

| Item | Assessment |
| --- | --- |
| Extension op today? | Yes |
| Why | It is fundamentally a single-step wrapper around a CLI/process plus files on disk. The important contracts are in its JSON input/output and artifacts, not in engine-only primitives. |
| Core changes required | None for basic parity. |
| Required implementation details | The extension implementation must preserve `sessionId`, checkpoint/status fields, and emit `stdout.jsonl` and `stderr.txt` as output artifacts, most likely by writing them into `outbox`. |
| Optional framework improvements | Standard env vars for worktree/workdir/inbox/outbox, plus a standard extension error envelope if you want parity for non-retryable validation failures. |
| Recommendation | Good extension candidate. |

## `llm_inference2`

| Item | Assessment |
| --- | --- |
| Extension op today? | Yes |
| Why | It is a single-step service wrapper. File handling, tool execution, and response-schema validation can all live inside an extension binary/script. |
| Core changes required | None for basic parity. |
| Required implementation details | The extension must preserve the current structured-output contract, especially `response_schema` validation and the stringified JSON response behavior. |
| Optional framework improvements | A standard secret/config injection story for extension ops would make provider configuration cleaner. |
| Recommendation | Good extension candidate. |

## `thinpackrebase`

| Item | Assessment |
| --- | --- |
| Extension op today? | Yes |
| Why | It is a single-step repo-local git operation. It does not use DB, workflow control, or management routes. |
| Core changes required | None if the worker environment already has the needed git access and credentials. |
| Required implementation details | The extension must return the same `git_context_patch` shape because downstream recipes depend on that context patch contract. |
| Optional framework improvements | A standard extension error contract would help if you later want more structured failure classification. |
| Recommendation | Good extension candidate. |

## `squashrebasemerge`

| Item | Assessment |
| --- | --- |
| Extension op today? | Yes |
| Why | Same basic profile as `thinpackrebase`: single-step local git work with a structured output contract. |
| Core changes required | None if git push/fetch credentials are already available to the worker environment. |
| Required implementation details | Preserve `git_context_patch` and the existing output shape. |
| Optional framework improvements | A standard extension error envelope would help if preserving conditions like not-fast-forward as typed failures becomes important. |
| Recommendation | Good extension candidate. |

## `cells.list`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | The current implementation reads from the DB through injected services. Extension ops do not receive DB handles or an authenticated internal service client. |
| Core changes required | Add a supported service-access mechanism for extensions. The lightest version is probably an authenticated internal API/CLI contract for cell listing. |
| Behavior risk without core changes | The extension would have to rely on ambient CLI config or ad hoc network access, which is a real behavioral and operational downgrade. |
| Recommendation | Only move this after you introduce a general pattern for service-backed extension ops. |

## `ticket.manage`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | Even though it is single-step, its behavior depends on in-server ticket service semantics: batched actions in one transaction, typed workflow errors, and ticket-create auto-start behavior. The current extension-op contract cannot access that directly. |
| Core changes required | Expose a first-class ticket-actions API/SDK for extensions that preserves atomic batch semantics and ticket auto-start behavior. Also add a way for extensions to return structured retryable/non-retryable failures. |
| Behavior risk without core changes | Replacing this with a series of public API calls would lose transactionality and likely diverge on failure handling. |
| Recommendation | Possible later, but only as part of a broader "service-backed extension op" model. Not a clean move today. |

## `input`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | This is not just an activity. It is a workflow control primitive: generate form, pause on a waiting task, expose HTTP routes, stream SSE updates, collect/cancel responses, and resume execution. |
| Core changes required | Extension ops would need management services, routes, SSE, waiting-task integration, and next-task control. That is effectively making extension ops first-class engine ops. |
| Behavior risk without core changes | High. You would lose the current user-input lifecycle and durable wait/resume behavior. |
| Recommendation | Keep core. This is fundamental. |

## `recipe.run_and_get_result`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | It starts a child job, waits for it, reads the child result, and propagates child artifacts. That is orchestration, not just an activity wrapper. |
| Core changes required | Extension access to workflow control, child-job start APIs, durable waiting, result retrieval, and artifact propagation. |
| Behavior risk without core changes | High. |
| Recommendation | Keep core. This is a fundamental orchestration primitive. |

## `recipes.run`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | It is the start primitive for child recipes and depends on workflow-control-backed job creation. Even though it does not wait, it is still a core orchestration boundary. |
| Core changes required | A first-class child-job start API/SDK for extension ops. |
| Behavior risk without core changes | The extension would be a thin shim over new core APIs anyway, so moving only the wrapper buys little. |
| Recommendation | Keep core with the rest of the child-recipe family. |

## `recipes.run_and_wait`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | It combines child-job start with durable wait semantics. Current extension ops cannot do either part directly. |
| Core changes required | Everything required for `recipes.run`, plus engine-managed wait support. |
| Behavior risk without core changes | High. |
| Recommendation | Keep core. |

## `recipe.await_result`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | It waits on another job and then fetches outputs/artifacts through workflow control. That dependency surface is not available to extension ops. |
| Core changes required | Workflow-control access, durable waiting, result retrieval, and artifact propagation for extensions. |
| Behavior risk without core changes | High. |
| Recommendation | Keep core. |

## `recipe.get_result`

| Item | Assessment |
| --- | --- |
| Extension op today? | No |
| Why | It is a child-job result/artifact reader. Even without waiting, it still depends on privileged workflow-control access and artifact propagation. |
| Core changes required | A first-class child-job result API/SDK for extension ops. |
| Behavior risk without core changes | High, and keeping this separate from the other child-recipe ops would make the model inconsistent. |
| Recommendation | Keep core. |

## Practical split

If the goal is to shrink the core op surface without changing behavior, the cleanest first wave is:

- `command_execution`
- `sleep`
- `codex.exec`
- `llm_inference2`
- `thinpackrebase`
- `squashrebasemerge`

The second wave is only worth doing if you first add a general service-backed extension model:

- `cells.list`
- `ticket.manage`

The ops that should remain core are the ones that define orchestration and human-blocking semantics:

- `input`
- all child-recipe start/read/wait ops in [RUN_RECIPE.md](/src/recipes/guides/ops/RUN_RECIPE.md)
