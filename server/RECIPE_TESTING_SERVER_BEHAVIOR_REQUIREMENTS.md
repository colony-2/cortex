# Recipe Testing Server Behavior Requirements (Functional / Per-Case)

## Purpose

Define server runtime behavior for per-case testing APIs where no test-run state is persisted.

If implemented with the CLI and OpenAPI specs, the system is fully functional end-to-end.

Implementation detail companion:

1. `RECIPE_TESTING_SERVER_DETAILED_SPEC.md` provides normative runtime mechanics for
   - mock insertion
   - `op_case` node scoping
   - passthrough dependency contracts
   - policy option handling

## Core model

1. Server receives one test case request at a time.
2. Server validates or executes that case synchronously.
3. Server returns full case result in response.
4. Server stores no reusable suite definitions and no run records.
5. Server can evaluate case outcomes using deterministic pattern checks and optional LLM judges.

## Persistence requirements

Server must not persist:

1. suite definitions
2. test runs
3. per-run artifact registries

Server may persist only:

1. operational logs/metrics
2. temporary execution artifacts needed during request handling

Temporary data must be cleaned after request completion/failure.

## Recipe resolution behavior

Each request must resolve `target_recipe` independently.

`server_ref` mode:

1. require `name`
2. require exactly one of `version` or `ref`
3. resolve to immutable recipe snapshot before case processing

`inline_recipe` mode:

1. parse recipe content from declared format
2. validate recipe schema before case processing
3. treat recipe as request-scoped only

## Validate endpoint behavior

For `POST /v1/recipe-tests/cases/validate`, server must:

1. resolve target recipe
2. validate case schema
3. validate case semantics:
   - case type valid
   - `op_case` has `target.node_path`
   - assertions syntactically valid
   - evaluations syntactically valid
   - mock matchers structurally valid
4. return structured `errors`/`warnings`
5. compute `case_hash` for traceability

No execution is performed in validate.

## Execute endpoint behavior

For `POST /v1/recipe-tests/cases/execute`, server must:

1. perform same validation steps as validate endpoint
2. create isolated request-scoped execution sandbox
3. execute case in test mode
4. evaluate assertions
5. evaluate configured outcome evaluations
6. return terminal case result payload

`status` must be one of:

1. `passed`
2. `failed`
3. `timed_out`
4. `canceled`

## Isolation and safety behavior

Default: `mode=isolated`.

In isolated mode server must:

1. block side-effecting ops unless explicitly allowed by case options
2. block interactive waits for `input` ops unless mocked
3. prevent unintended production mutations by default

Minimum blocked-by-default ops:

1. `ticket.manage`
2. `squashrebasemerge`

## Mock behavior requirements

Supported mock scopes:

1. op mocks
2. child recipe mocks
3. function mocks (`cells()`, etc.)
4. context overrides
5. seeded artifacts

Supported mock modes:

1. `return`
2. `fail`
3. `passthrough`
4. `record_passthrough`
5. `replay`

Matching precedence:

1. highest specificity:
   - `node_path + op`
   - `node_path`
   - `op`
2. declaration order tie-breaker

Server must include mock hit/miss info in execute response diagnostics.

## Child recipe behavior

When executing child recipe ops in test mode:

1. inherit test mode and relevant mocks by default
2. if matched by child-recipe mock, do not execute real child workflow
3. include child execution/mocking details in response diagnostics

## Assertion behavior

Required assertion types:

1. `output_equals`
2. `output_matches`
3. `artifact_exists`
4. `artifact_json_equals`
5. `node_executed`
6. `node_not_executed`
7. `status_is`
8. `cel_true`

Rules:

1. assertions evaluate after execution reaches terminal state
2. if any assertion fails, case `status=failed`
3. response includes per-assertion expected/actual and failure message

## Outcome evaluation behavior

Case evaluations are independent checks over outputs/artifacts/traces.

Supported evaluation types:

1. `text_pattern`
2. `llm_judge`

Evaluation source kinds:

1. `artifact`
2. `artifact_glob`
3. `output_path`
4. `trace`

Pipeline order:

1. resolve evaluation sources
2. run `text_pattern` evaluators
3. run `llm_judge` evaluators
4. aggregate evaluation summary
5. apply pass/fail policy (`enforce` vs `report_only`)

`text_pattern` requirements:

1. support `forbid_regex`
2. support `require_regex`
3. report matched snippets/locations when possible

`llm_judge` requirements:

1. invoke configured provider/model with explicit prompt + source content
2. support structured `response_schema`
3. evaluate `pass_when` expression against judge output
4. return raw judge output (subject to redaction limits)
5. default `temperature=0` when unset

Pass/fail policy:

1. evaluator `mode=enforce`: fail case when evaluator fails.
2. evaluator `mode=report_only`: include findings but do not fail case by itself.
3. request-level `execution.evaluation_mode=report_only` forces all evaluators to report-only for that call.

Evaluator errors:

1. deterministic evaluator error -> case failed in enforce mode, reported otherwise.
2. llm judge timeout/error -> recorded as evaluator error with category `evaluator_error`.

## Sensitive source handling

Some trace/artifact sources may contain sensitive model outputs.

Rules:

1. evaluation sources marked sensitive require explicit `allow_sensitive=true`.
2. if sensitive source requested without opt-in, evaluator is rejected as validation error.
3. response redaction settings apply to evaluator findings and raw outputs.
4. example sensitive sources include assistant trace streams (such as internal monologue/reasoning trace artifacts).

## Response artifact behavior

Because there is no persisted run artifact API, execute response must support inline artifact return.

Behavior:

1. respect `artifact_mode` request option:
   - `none`: no artifact payloads
   - `inline`: include selected artifact payloads
2. enforce `artifact_max_bytes` limit
3. include truncation metadata when limits are exceeded
4. redact sensitive values when configured

## Timeouts and limits

Server must enforce:

1. request payload size limits
2. execution timeout from request or safe default
3. artifact inline size caps
4. bounded memory/CPU usage for request sandbox
5. evaluator-specific timeout/token limits

Timeout behavior:

1. set `status=timed_out`
2. include timeout failure entry in response

## Observability requirements

Server logs/metrics must include:

1. request ID
2. case ID
3. resolved recipe ref/version (or inline hash)
4. duration
5. terminal status
6. failure category (`validation_error`, `runtime_error`, `assertion_failure`, `evaluation_failure`, `evaluator_error`, `policy_blocked`, `timeout`)
7. evaluation status summary (`passed`, `failed`, `errors`)

## Backward compatibility

1. Existing workflow/ticket APIs remain unchanged.
2. Non-test recipe execution behavior remains unchanged.

## Minimum acceptance tests (server team)

1. validate endpoint works for both `server_ref` and `inline_recipe`.
2. execute endpoint returns full case result without creating run records.
3. isolated mode blocks unmocked `ticket.manage`.
4. mocked `input` op avoids interactive wait.
5. failed assertion sets case status to `failed`.
6. inline artifact response respects byte limit and truncation flags.
7. no API exists for stored suites or run status retrieval.
8. forbidden regex pattern in an artifact fails case when evaluator mode is `enforce`.
9. LLM judge evaluation result is returned with score/findings and affects case status per evaluation mode.
