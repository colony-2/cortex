**Unanswered external input tasks remain pending past recipe and step deadlines**

A c2j recipe waiting for user input does not terminate when its configured
timeout elapses. We reproduced this with a real c2j worker and SQLite-backed
JobDB over HTTP. A recipe with `timeout: 20s` was still pending after polling
its outcome for 45 seconds after the input prompt appeared. A separate run
with `timeout: 10s` on the input step showed the same behavior.

This prevents callers from relying on recipe deadlines to bound unattended
approval/input waits. The backend remains pending, so refreshing the browser
does not resolve the request.

Reproduced with:

- c2j `v0.0.53` (`ef65f00019724f17069d318845b0bc75ddc0dc98`).
- JobDB `v0.0.19-0.20260919034646-71b6668a65db`, commit
  `71b6668a65dbcc08ae46119412c559f54d8a4b32` (also tagged `v0.0.19`).
- Go `1.26.7`, Linux, JobDB's embedded SQLite runtime exposed through its HTTP
  server, and a separate `c2j run loop --jobdb http://127.0.0.1:18082/<tenant>`
  process.
- Cortex submits a Git recipe selector using runtime recipe resolution.
  The browser, worker, JobDB, and SSE connection are real; neither responses
  nor clocks are mocked.

Minimal recipe:

```yaml
id: browser-input
version: "1.0"
timeout: 20s
input_schema: {}
sequence:
  - id: approval
    op: input
    inputs:
      form:
        question: What should the recipe publish?
        type: short_answer
outputs:
  answer: "${{ sequence.approval.outputs.response }}"
```

To reproduce, commit this recipe to a Git repository, submit its selector to
Cortex backed by HTTP JobDB, and start the c2j worker for that tenant. Wait
until the input prompt appears, leave it unanswered, and continue polling the
job outcome beyond 20 seconds. Keep the worker running throughout. For the
step-level variant, remove the root `timeout` and put `timeout: 10s` alongside
`op: input` on the approval step.

Expected: the recipe reaches a terminal timeout failure within a reasonable
scheduling delay after the deadline. The request stops appearing as pending,
and an overdue response cannot successfully complete the expired request.

Observed: Cortex's outcome endpoint continues returning HTTP 202 with
`{"status":"pending"}` throughout the 45-second polling window. The prompt
remains unanswered and the test never reaches its checks for prompt removal
or late-response rejection. We have not established whether the request
eventually terminates under a longer, default runtime timeout.

The executable reproduction is in the Cortex checkout at commit `f231faf5`:

```bash
pnpm install --frozen-lockfile
pnpm --filter @colony2/app test:e2e --project=production \
  recipe-input.spec.ts --grep 'input timeout'
```

Use the repository's documented Go/Node prerequisites. Playwright starts the
servers, builds the pinned worker, creates a temporary Git repository, submits
the job through the browser, and verifies the prompt before testing expiry.
Worker logs and the final outcome are attached to the Playwright report.
The test currently marks only the confirmed still-pending deadline failure
as expected, so its command exits successfully while reproducing this bug.
Remove the `test.fail(stillPending, ...)` call to make it fail CI normally.

Source inspection suggests a problem in JobDB's external-task scheduling path;
this is a suspected cause, not a verified fix:

- In `pkg/workflow/worker_runner.go`, `DoTask` computes `totalDeadline`, then
  handles tasks without a local worker in the `!local` branch (around lines
  885–917). It reschedules to the external task route and exits execution
  before reaching the total-deadline check around lines 922–926.
- That branch sets `AlternateAfter` from `invocationTimeout`, without bounding
  it by the remaining total deadline.
- c2j declares `input:collect_user_input` as a no-worker task. Its compiler
  propagates operation/execution timeouts through `RunPolicy.TotalTimeout`
  (`pkg/worker/compiler/compiler.go`, around lines 446–452).
- Recipe-level timeout propagation also warrants checking for recipes resolved
  by the worker: Cortex submits without preloading the recipe, while c2j's
  `recipeRunPolicy` derives the initial job timeout from recipes available at
  submission time. The step-level reproduction avoids relying solely on that
  initial job policy.

Please cover expiry of an unanswered external task, including wake-up at the
earliest applicable deadline, worker restart while waiting, and rejection of
late completion. A response submitted before the deadline should still resume
the recipe successfully. Our separate browser tests already verify normal
responses, cancellation, worker restart, and consecutive input requests.
