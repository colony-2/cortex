# Recipe Testing Server Detailed Specification

## Purpose

Define the concrete server runtime for per-case recipe testing with deterministic mock insertion, scoped `op_case` execution, and explicit dependency handling for passthrough behavior.

This spec is normative for `/api/projects/{projectId}/recipe-tests/cases/validate` and `/api/projects/{projectId}/recipe-tests/cases/execute`.

## Design constraints

1. One case per request.
2. No persisted test runs.
3. Validate and execute are request-scoped.
4. Execution must be deterministic under mocks.
5. Policy must be configuration-driven by the case payload.
6. Server/client must not hardcode specific op names for allow/deny behavior.

## Request model and validation

## Structural validation

Use typed request structs with `validate` tags plus strict JSON decoding:

1. `json.Decoder.DisallowUnknownFields()`.
2. `go-playground/validator` on request graph.
3. Return `validation_error` issues with field namespace for tag failures.

## Semantic validation

After tag validation:

1. `server_ref` requires `name` and exactly one of `version` or `ref`.
2. `inline_recipe` requires non-empty `content` and valid JSON when `format=json`.
3. `op_case` requires `case.target.node_path`.
4. Each op mock must define at least one matcher key (`node_path` or `op`).
5. `replay` mock mode requires `behavior.cassette_key`.
6. `case.options.policy.required_dependencies` may only contain:
   - `database`
   - `workflow_control`
   - `sse_manager`

## Target recipe resolution

1. Resolve recipe from server (`server_ref`) or request content (`inline_recipe`).
2. Parse recipe with dynamic unknown-op stubs to allow request-time testing against ops not globally registered in-process.
3. Compute deterministic `case_hash = sha256({resolved_recipe_hash, case_payload})`.

## Execution model

Execution is implemented with a recipe-testing job context that intercepts `DoTask` calls.

## Mock insertion point

Mocks are inserted at task execution interception (`DoTask`) using invocation metadata:

1. `node_path` from invocation context.
2. op type from `taskType` prefix (`op:step`).

Matching precedence:

1. `node_path + op`
2. `node_path`
3. `op`
4. declaration order tie-break.

## Supported mock modes

1. `return`
   - Return configured outputs/artifacts as activity output.
2. `fail`
   - Return configured error.
3. `passthrough`
   - Execute real op task step with runtime dependencies.
4. `record_passthrough`
   - Execute passthrough and store result in in-memory cassette.
5. `replay`
   - Return previously recorded cassette by `cassette_key`.

Cassette scope is request-local only.

## Passthrough dependency model

Passthrough executes real op step invocation through `TaskStep.Invoke` with `OpDependencies` built from server runtime dependencies.

Dependency source:

1. `ServiceDependencies2` passed from server setup into recipe-testing service.
2. `OpDependencies` builder maps available fields:
   - database
   - workflow control
   - job tool
   - worktree path
   - input artifacts (presence tokens from artifact keys)

Dependency contract handling:

1. If case uses passthrough and dependency requirements are declared in `case.options.policy.required_dependencies`, unavailable dependencies fail validation with `dependency_unavailable`.
2. If passthrough execution fails due missing/runtime dependency behavior, execution fails with `runtime_error`.

## `op_case` scope enforcement

`op_case` is constrained by `case.target.node_path`.

Rules:

1. Only tasks emitted for the target `node_path` may execute.
2. Any task for a different node path fails with `policy_blocked` (`node outside op_case target scope`).
3. If target node never executes, case fails with `runtime_error` (`target op_case node was not executed`).

This gives deterministic single-node execution constraints without mutating recipe source.

## Policy options

Policy is case-configured under `case.options.policy`.

Supported fields:

1. `require_mocks` (bool)
   - If true, unmatched ops are blocked.
2. `blocked_ops` (`[]string`)
   - Explicitly block matched op types.
3. `required_dependencies` (`[]string`)
   - Validate passthrough preconditions.

Defaults:

1. execution `mode=isolated` implies `require_mocks=true`.
2. no default blocked op list.

## Assertions and evaluations

Assertions run after terminal execution.

Supported assertions:

1. `output_equals`
2. `output_matches`
3. `artifact_exists`
4. `artifact_json_equals`
5. `node_executed`
6. `node_not_executed`
7. `status_is`
8. `cel_true`

Evaluations:

1. `text_pattern`
2. `llm_judge` (server deterministic evaluator contract for now)

Evaluation mode:

1. per-evaluator `mode` (`enforce` or `report_only`)
2. request override `execution.evaluation_mode=report_only`

## Artifact return

1. `artifact_mode=none` omits inline payload.
2. `artifact_mode=inline` returns base64 payloads.
3. `artifact_max_bytes` truncates each artifact independently with `truncated=true`.

## Diagnostics

Response diagnostics include:

1. `mock_hits[]` (`node_path`, `op`, `mode`)
2. `mock_misses[]` (`node_path`, `op`, `reason`)

## Failure categories

Terminal `failure_category` values:

1. `validation_error`
2. `policy_blocked`
3. `runtime_error`
4. `assertion_failure`
5. `evaluation_failure`
6. `evaluator_error`
7. `timeout`

## Acceptance tests

Minimum server acceptance coverage:

1. Validate: structural tag validation and semantic validation both enforced.
2. Mock insertion precedence works (`node_path+op` > `node_path` > `op`).
3. `op_case` blocks non-target node execution.
4. `op_case` fails if target node not executed.
5. `passthrough` uses runtime dependencies and fails fast when required dependencies are unavailable.
6. `record_passthrough` followed by `replay` returns recorded output for same `cassette_key`.
7. `require_mocks=true` blocks unmatched ops.
8. inline artifact truncation flags are correct.
