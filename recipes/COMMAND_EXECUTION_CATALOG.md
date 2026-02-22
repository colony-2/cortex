# Remaining `command_execution` challenge catalog

This catalog only lists the remaining `command_execution` uses that still represent a structural gap (vs. simple glue already removed).

| recipe | op name/path | what command is doing | alternative pattern (optional) |
|---|---|---|---|
| `ticket-implement` | `ticket-implement.yaml:149` (`state.states.persist_summary`) | Persists `assistant-summary.txt` artifact for later user review/reuse. | Add first-class `artifact.write_text` op and remove shell wrapper. |
| `ticket-validate` | `ticket-validate.yaml:18` (`state.states.detect`) | Detects suggested validation commands from repo markers (`package.json`, `go.mod`, `Cargo.toml`, etc.). | Move to cell/project-level validate config or a dedicated `validate.detect` op. |
| `ticket-validate` | `ticket-validate.yaml:102` (`state.states.run_provided`) | Runs provided validation commands, captures `validation/output.txt`, and writes `validation/output-tail.txt` artifact for UI display. | Dedicated `validate.run` op with controlled command policy and built-in artifact capture. |
| `ticket-validate` | `ticket-validate.yaml:136` (`state.states.run_suggested`) | Runs auto-detected validation commands, with full log + tail artifact capture in outbox. | Same `validate.run` op; pass a `mode: suggested` input. |
| `ticket-validate` | `ticket-validate.yaml:170` (`state.states.run_custom`) | Runs custom user-specified validation commands, with full log + tail artifact capture in outbox. | Same `validate.run` op with allowlist/policy guardrails. |
| `ticket-merge` | `ticket-merge.yaml:22` (`state.states.resolve`) | Resolves merge defaults and computes `local_hash` from git so `squashrebasemerge` has fully populated inputs. | Extend `squashrebasemerge` with default inference from `context.git`/worktree. |

## Summary

Remaining command-op usage clusters into three real gaps:

1. **Artifact write gap**  
   A small shell step remains to persist `assistant-summary.txt` because there is no first-class `artifact.write_text` op.

2. **Validation execution gap**  
   Validation still needs shell execution/capture. This is the biggest remaining structured-op gap and where a `validate.run` op would remove most command execution usage.

3. **Merge input-resolution gap**  
   One command step remains to compute/normalize merge inputs before `squashrebasemerge`.

Net: orchestration logic uses codex/artifact handoff patterns; remaining command usage is concentrated in validation execution, merge input resolution, and one small artifact-write adapter.
