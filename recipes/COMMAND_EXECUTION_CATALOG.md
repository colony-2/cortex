# Remaining `command_execution` challenge catalog

This catalog only lists the remaining `command_execution` uses that still represent a structural gap (vs. simple glue already removed).

| recipe | op name/path | what command is doing | alternative pattern (optional) |
|---|---|---|---|
| `new-ticket-triage` | `new-ticket-triage.yaml:52` (`sequence.read_triage_json`) | Reads `triage.json` artifact from inbox and emits content for typed recipe outputs. | Add first-class `artifact.read_json` output mapping support. |
| `new-ticket-requirements-planning` | `new-ticket-requirements-planning.yaml:126` (`sequence.read_plan_json_for_outputs`) | Reads `requirements/plan.json` artifact so recipe outputs can map typed fields (`summary`, `requirements`, etc.). | Add artifact-to-output mapping support (JSON path extraction). |
| `new-ticket-requirements-planning` | `new-ticket-requirements-planning.yaml:135` (`sequence.read_api_review_json_for_outputs`) | Reads `requirements/api-review.json` artifact so recipe outputs can expose review fields. | Same as above: native artifact JSON output mapping. |
| `ticket-implement` | `ticket-implement.yaml:149` (`state.states.persist_summary`) | Persists `assistant-summary.txt` artifact for later user review/reuse. | Add first-class `artifact.write_text` op and remove shell wrapper. |
| `ticket-validate` | `ticket-validate.yaml:18` (`state.states.detect`) | Detects suggested validation commands from repo markers (`package.json`, `go.mod`, `Cargo.toml`, etc.). | Move to cell/project-level validate config or a dedicated `validate.detect` op. |
| `ticket-validate` | `ticket-validate.yaml:102` (`state.states.run_provided`) | Runs provided validation commands, captures `validation/output.txt`, and writes `validation/output-tail.txt` artifact for UI display. | Dedicated `validate.run` op with controlled command policy and built-in artifact capture. |
| `ticket-validate` | `ticket-validate.yaml:136` (`state.states.run_suggested`) | Runs auto-detected validation commands, with full log + tail artifact capture in outbox. | Same `validate.run` op; pass a `mode: suggested` input. |
| `ticket-validate` | `ticket-validate.yaml:170` (`state.states.run_custom`) | Runs custom user-specified validation commands, with full log + tail artifact capture in outbox. | Same `validate.run` op with allowlist/policy guardrails. |
| `ticket-merge` | `ticket-merge.yaml:22` (`state.states.resolve`) | Resolves merge defaults and computes `local_hash` from git so `squashrebasemerge` has fully populated inputs. | Extend `squashrebasemerge` with default inference from `context.git`/worktree. |

## Summary

Remaining command-op usage clusters into three real gaps:

1. **Artifact IO + output mapping gap**  
   Shell steps remain mainly for reading artifact JSON into typed outputs and writing small text artifacts because there is no first-class artifact read/write + JSON-field mapping support.

2. **Validation execution gap**  
   Validation still needs shell execution/capture. This is the biggest remaining structured-op gap and where a `validate.run` op would remove most command execution usage.

3. **Merge input-resolution gap**  
   One command step remains to compute/normalize merge inputs before `squashrebasemerge`.

Net: orchestration logic now uses codex/artifact handoff patterns; remaining command usage is mostly adapter glue (artifact read/write mapping) plus validation execution and merge input resolution.
