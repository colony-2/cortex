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

Inputs (JSON Schema Draft 7 compatible)
```json
{
  "type": "object",
  "required": ["prompt"],
  "properties": {
    "prompt": {"type": "string", "minLength": 1},
    "sessionId": {"type": "string"},
    "model": {"type": "string"},
    "outputSchema": {"type": "string"},
    "outputOnly": {"type": "boolean"},
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
- `outputSchema`: optional path in recipe workspace; when provided, create temp file and pass to `--output-schema`.
- `outputOnly`: toggles `-o` flag for Codex.
- `env`: extra environment variables merged into Shai-run container.
- Activity timeouts remain managed by Temporal configuration; Codex CLI is invoked without additional timeout flags.

Output Schema (normalized contract)
```json
{
  "type": "object",
  "required": ["status", "stdout", "stderr", "sessionId"],
  "properties": {
    "status": {
      "type": "string",
      "enum": ["completed", "incomplete", "error"]
    },
    "sessionId": {"type": "string"},
    "stdout": {"type": "string"},
    "stderr": {"type": "string"},
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
- `stdout`: literal aggregation of Codex STDOUT (JSONL stream + trailing lines) for audit/debug.
- `assistantSummary`: final assistant message extracted from `item.completed` events with type `assistant_message`.
- `sessionId`: thread identifier discovered from `thread.started` event or resume parameter fallback.
- `status`: `completed` when Codex produces a final assistant message with no outstanding dependency blockers; `incomplete` when Codex reports blockers or abandons the task; `error` when the CLI terminates abnormally.
- `pendingDependencies`: parsed from assistant structured output or heuristic detection (see below).
- `incompleteReason`: human-readable reason (e.g., "dependent components require updates" or "agent exited without solution").
- `incompleteCategory`: machine classifying incomplete outcomes (`dependency_blockers` vs `agent_abandoned`).
- `stderr`: literal aggregation of Codex STDERR for diagnostics.
- `errorMessage`: populated when `status == error` (stderr summary or CLI failure detail).

Execution Flow
1. Resolve execution roots using recipe context:
   - Worktree root: `ops.RecipeContext.WorktreeRoot` (or equivalent) passed to Shai `Config.WorkingDir`.
   - Current cell directory: add to `ReadWritePaths` to enable edits if Codex requests them.
2. Initialize Shai **ephemeral** runner (`server/container/pkg/shai`) with:
   - `ReadWritePaths`: `[currentCellRelativePath]` plus any derived from inputs.
   - Ephemeral mode handles container lifecycle automatically (no explicit container name).
3. Build Codex argv:
   - Always include `codex exec` and `--json` to capture event stream.
   - Append flags derived from inputs (model, structured output, etc.).
   - Force fully unsandboxed, approval-free execution by always appending `--dangerously-bypass-approvals-and-sandbox`.
   - Disable Codex repo safeguards by passing `--skip-git-repo-check`.
   - Append resume syntax when `sessionId` provided (`resume <sessionId>` before prompt argument).
   - Append `prompt` as final positional argument (quoted or via argv splits).
   - Example argv: `codex exec --json --dangerously-bypass-approvals-and-sandbox --skip-git-repo-check <flags> "<prompt>"`.
4. Run command via Shai runner `EphemeralRunner` with a non-interactive `PostSetupExec`; capture both stdout and stderr streams into buffers while also relaying to logs if needed.
   - The bundled Shai devcontainer image already includes the Codex CLI; no installation step is required.
   - Capture exit code; treat any non-zero as an `error` outcome. Timeouts are enforced by Temporal activity settings, not the CLI.
   - Configure `PostSetupExec.UseTTY = false` so Codex runs fully automated without interactive prompts.
5. Parse STDOUT JSONL stream in-process:
   - Collect events into structured list.
   - Extract session/thread id (`thread.started` event).
   - Aggregate assistant reasoning for summary fields.
   - Preserve full stdout/stderr buffers for return payload (stderr not parsed, but surfaced verbatim).
6. Determine completion status:
   - Default `completed` when CLI exit 0 and final assistant message present.
   - Inspect assistant message for structured JSON; if message contains markers like `Dependencies:` or JSON with `pendingDependencies`, parse into `pendingDependencies`, set `status` `incomplete`, and `incompleteCategory` to `dependency_blockers`.
   - If assistant message indicates abandonment (e.g., "I cannot proceed"), set `status` `incomplete` with `incompleteCategory` `agent_abandoned`.
   - If CLI exit non-zero or `turn.failed`, set `status` `error`, propagate exit status, and populate `errorMessage` from stderr.
7. Emit output struct bundling session metadata, aggregated stdout/stderr, completion flags, and any parsed dependency data.

Resume Cycle Handling
- On new sessions: save `thread.started.thread_id` to `sessionId` in output.
- On resume inputs: prefer provided sessionId; if Codex emits a new `thread.started` (e.g., resumed run ID), update to returned value.
- Expose `sessionId` back to recipe for chaining sequential steps.

Dependency Reporting Strategy
- Encourage tasks to request Codex structured output containing dependency list; when `assistantSummary` parses JSON object with `pendingDependencies`, map to schema.
- Fallback heuristic: look for markdown list prefixed by `Pending changes:` and parse `component: change` pairs.
- If dependencies present, set `status` to `incomplete` and populate `incompleteReason` with summary (e.g., "Codex identified pending dependency edits").

Error Handling
- CLI exit != 0 → `status: error`, capture `errorMessage` and stderr content; surface exit code.
- Missing `thread.started` event → treat as error; return empty sessionId and annotate message.
- Timeout at activity level → propagate Temporal timeout; annotate result when possible.

Testing Strategy
1. Unit tests (`pkg/ops/codex` package):
   - Mock Shai manager returning canned JSONL output; verify parsing of session id and completion vs incomplete rules.
2. Resume cycle test (new):
   - Simulate first run emitting `thread.started` and assistant completion; assert output.sessionId preserved.
   - Simulate second run with input sessionId; ensure command builder uses `resume <id>` and output updates `sessionId` (same or new) while capturing aggregated stdout.
3. Dependency detection test: feed assistant message with formatted dependency entries; assert `pendingDependencies` extraction and `status == incomplete`.
4. Timeout / failure test: Shai returns non-zero exit; ensure output flagged `error`, `stderr` captured, and `errorMessage` populated.
 
Observability and Telemetry
- Log codex command, sanitized flags, and execution duration at info level.
- Emit metric counters for success/incomplete/error to align with existing LLM ops telemetry, and surface stderr digests for error cases.

Open Questions
- None; current design assumes Codex CLI supports `--dangerously-bypass-approvals-and-sandbox` and `--skip-git-repo-check` as documented.

Implementation Notes
- Package layout: new directory `server/ops/pkg/codex` with `activity.go`, `command_builder.go`, `parser.go`, tests.
- Export via `server/ops/pkg/export.GetAll()` (append `codex.GetOp()`).
- Document op inputs in `server/ops/AGENTS.md` and update extension schemas if needed.
