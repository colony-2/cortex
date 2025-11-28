# SWF Migration Plan

## Goals
- Replace Temporal with `/swf-go` for recipe execution and remove Temporal-specific artifacts (markers, inline ops).
- Preserve recipe semantics (state/sequences/ops, git propagation, run metadata) while adopting SWF’s job/task model.
- Use SWF remote task pattern **only for user input** (input op); core recipe execution stays inline on job runners.
- Disable async child recipes during migration; sync children run inline in the same job runner.
- Remove the recipe-history and embeddedtemporal modules; rebuild equivalents later on SWF.

## Ordering
1. `02-recipe-core` – engine-neutral invocation/retry primitives (drop markers, inline concepts).
2. `03-ops` – refactor ops to SWF model; remove async child support; input op uses remote task pattern; drop inline ops.
3. `04-recipe-worker` – rewrite compiler/worker around SWF; inline child execution only; wire capabilities; no inline-op abstraction.
4. `05-recipe-history` – retire module (delete code); plan rebuild on SWF later.
5. `06-embeddedtemporal` – retire module (delete code); track SWF harness work in swf-go ticket.
6. `07-api` – expose SWF job/task endpoints; swap Temporal client.
7. `08-nucleus` – CLI drives SWF jobs/tasks and chapter views.
8. `09-cortex` – tests/commands run via SWF harness.
9. `10-ticket` – remove Temporal errors; ensure ops work under SWF contexts.
10. `11-swf-go-embedded-ticket` – swf-go work item to deliver embedded test harness.

Downstream projects may temporarily break until their step; each completed project must pass its tests.

## Known SWF Gaps vs Current Usage
- No child job API: child recipes must run inline; async mode unsupported.
- No signals/queries/search attributes: metadata + input flows must use chapters or storage.
- No built-in heartbeats/timers: compiler must implement timeouts/backoff directly.
- Limited visibility APIs: rely on `CheckJobStatus` + Strata chapters for progress.

## Async Child Usage To Disable
- `run_mode: async` in recipe op and the `recipe_set` op (which assumes async children).
- Docs/tests that assert async handles (`recipe-op-execution-modes-spec.md`, `RECIPE_OPS_REFERENCE.md`) need gating or updates.

## Input Pattern Shift
- Input op switches to SWF remote task: emit capability need, managers poll via `FindTasksWaitingForCapability` and complete with `TaskHandle.Finish`.
- Do **not** register input op in the job worker; remote task completion unblocks the paused job.

## Deliverables
- Numbered per-project specs in this directory.
- Updated docs/tests per project reflecting SWF behavior and async removal.
