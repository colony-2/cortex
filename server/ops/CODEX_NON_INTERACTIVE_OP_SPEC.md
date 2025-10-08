Codex Non-Interactive Op Specification

Context and Existing Ops
- `llm` / `llm_enhanced`: streaming and structured LLM interactions via adapters; activity-based executions with rich telemetry.
- `recipe`: orchestrates recipe-core workflows and Git workspace adapters; integrates tightly with Temporal state.
- `input`: bridges recipe-core wrappers and dynamic input routing.
- Extension ops: runtime-discovered commands executed via the extensions framework.

Problem Statement
- Recipes need a first-party op that can drive Codex CLI in non-interactive mode for automation (no approval prompts).
- Today we have only documentation (`codex_exec.md`); there is no op that captures stdout, provides resumable sessions, or normalizes completion status for downstream nodes.

Op Overview
- Name: `codex.exec` (new activity op under `server/ops/pkg/ops` namespace).
- Execution class: Activity (non-deterministic CLI invocation).
- Responsibility: run `codex exec` in non-interactive mode inside a devcontainer via Shai, capture structured JSONL output, and normalize results for recipe consumers.
- Implementation builds on the shared Codex library defined in `server/ops/CODEX_LIBRARY_SPEC.md`.

Inputs (JSON Schema Draft 7 compatible)
```json
{
  "type": "object",
  "required": ["prompt"],
  "properties": {
    "prompt": {"type": "string", "minLength": 1},
    "sessionId": {"type": "string"},
    "model": {"type": "string"},
    "env": {
      "type": "object",
      "additionalProperties": {"type": "string"}
    }
  },
  "additionalProperties": false
}
```
- `prompt`: primary user request passed as positional argument.
- `sessionId`: when present, switch to `codex exec resume <sessionId> <prompt>`.
- `model`: `--model <name>`.
- `env`: extra environment variables merged into Shai-run container.
- Activity timeouts remain managed by Temporal configuration; Codex CLI is invoked without additional timeout flags.

Output Schema (normalized contract)
```json
{
  "type": "object",
  "required": ["status", "stderr", "sessionId", "stdoutBlobUri"],
  "properties": {
    "status": {
      "type": "string",
      "enum": ["completed", "incomplete", "error"]
    },
    "sessionId": {"type": "string"},
    "stderr": {"type": "string"},
    "stdoutBlobUri": {"type": "string"},
    "assistantSummary": {"type": "string"},
    "incompleteReason": {"type": "string"},
    "incompleteCategory": {
      "type": "string",
      "enum": ["dependency_blockers", "agent_abandoned"]
    },
    "pendingDependencies": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["component", "requestedChanges"],
        "properties": {
          "component": {"type": "string"},
          "requestedChanges": {"type": "string"}
        },
        "additionalProperties": false
      }
    },
    "errorMessage": {"type": "string"}
  },
  "additionalProperties": false
}
```
- `stdoutBlobUri`: location in blobstore where full Codex stdout (JSONL stream + trailing lines) is persisted.
- `assistantSummary`: provided by the structured JSON payload in the final `assistant_message` event.
- `sessionId`: thread identifier discovered from `thread.started` event or resume parameter fallback.
- `status`: `completed` when Codex produces a final assistant message with no outstanding dependency blockers; `incomplete` when Codex reports blockers or abandons the task; `error` when the CLI terminates abnormally.
- `pendingDependencies`: sourced from the structured Codex payload (with heuristic fallback only when parsing fails).
- `incompleteReason`: human-readable reason (e.g., "dependent components require updates" or "agent exited without solution").
- `incompleteCategory`: machine classifying incomplete outcomes (`dependency_blockers` vs `agent_abandoned`).
- `stderr`: literal aggregation of Codex STDERR for diagnostics.
- `errorMessage`: populated when `status == error` (stderr summary or CLI failure detail).
- Optional string fields (`incompleteReason`, `incompleteCategory`, `errorMessage`) should be emitted as empty strings when not applicable to satisfy the shared schema contract.

Structured Output Contract
- Validation and schema enforcement live in the Codex library (`server/ops/CODEX_LIBRARY_SPEC.md`); the op consumes the parsed results without duplicating checks.
- Callers can retrieve the library’s exported schema constant when needed (e.g., for documentation or additional validation).

Execution Flow
1. Gather runtime context from the recipe invocation:
   - Worktree root, cell path, blobstore URI, workflow/activity identifiers.
2. Validate op inputs (`prompt` required) and assemble `codex.Options` with context and flags (`sessionId`, `model`, `env`).
3. Invoke `codex.Execute` with the assembled options.
4. Map `codex.Result` to op output fields (`status`, `sessionId`, `assistantSummary`, `pendingDependencies`, `stderr`, `stdoutBlobUri`, etc.).
5. Attach telemetry/logs (duration, blob URI) using existing recipe logging facilities.
6. Return structured output to recipe-core; library handles stdout persistence, schema enforcement, and error classification.

Resume Cycle Handling
- Library returns the authoritative session ID; surface it in outputs for chaining follow-up ops.
- When inputs include `sessionId`, forward it to the library; if Codex rotates the ID, propagate the new value in the result.
- Recipes can persist the returned ID to continue the conversation in subsequent invocations.

Dependency Reporting Strategy
- Depend entirely on the library’s parsed results. The library already performs fallback parsing and erroring as required (see Part 1 spec).
- The op should not mutate `pendingDependencies`; it simply forwards the slice provided by the library.

Error Handling
- If `codex.Execute` returns an error, fail the activity (Temporal non-retryable if indicated by error type).
- Otherwise, forward the `Result` verbatim; library already encodes CLI/session/schema failures into `Status`, `ErrorMessage`, and `stderr`.
- Timeouts remain managed via Temporal activity settings.

Testing Strategy
1. Input validation test: ensure missing `prompt` or invalid types produce deterministic errors without calling the library.
2. Resume cycle test: provide `sessionId` input, mock `codex.Execute` to return a different session ID, and assert output/next-step guidance reflects it.
3. Options mapping test: verify `model` and `env` values are forwarded to the library options along with context-derived paths.
4. Error propagation test: simulate library error and confirm the activity surfaces a Temporal failure with appropriate message/metadata.
5. Result mapping test: stub the library to return a populated `Result` and assert op outputs expose identical data (status, dependencies, stderr, blob URI).

Observability and Telemetry
- Record invocation start/finish with prompt hash, model, sessionId, and returned status.
- Emit metric counters for success/incomplete/error leveraging existing ops telemetry helpers.
- Bubble up library-produced metrics (e.g., blobstore bytes written) to the same namespace for consolidation.

Open Questions
- None; current design assumes Codex CLI supports `--dangerously-bypass-approvals-and-sandbox` and `--skip-git-repo-check` as documented.

Implementation Notes
- Reuse the library package (`server/ops/pkg/codex`) from Part 1; add an activity wrapper (e.g., `op.go`, `op_test.go`) that depends on the exported API.
- Register the op via `server/ops/pkg/export.GetAll()` (append `codex.GetOp()`).
- Document op inputs in `server/ops/AGENTS.md` and update extension schemas if needed.
