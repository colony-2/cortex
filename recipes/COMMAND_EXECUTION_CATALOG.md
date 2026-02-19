# `command_execution` usage catalog (recipes)

This doc catalogs every `op: command_execution` currently used in the published recipe set under `/src`, what it does, why it exists, and (optionally) a more “structured op” alternative.

| recipe | op name/path | what command is doing | alternative pattern (optional) |
|---|---|---|---|
| `new-ticket-triage` | `new-ticket-triage.yaml:55` (`sequence.persist_triage_json`) | Writes the LLM triage JSON (`sequence.assess_cell.outputs.response`) to the workflow outbox as `triage.json` for artifact inspection/auditing. | If artifacts aren’t required, rely on recipe outputs only. If artifacts are required, consider a dedicated “artifact.write_json” extension op to avoid inline shell. |
| `new-ticket-requirements-planning` | `new-ticket-requirements-planning.yaml:156` (`sequence.persist_plan_and_docs`) | Persists `plan.json` to `outbox/requirements/`, then renders human-reviewable markdown (`index.md` and per-requirement `REQ-*.md`) into the outbox. | Move doc rendering into an extension op (e.g. `.colony2/ops/requirements.render`) so recipes don’t embed Python; or have the LLM emit both JSON + markdown and only do minimal file writes here. |
| `new-ticket-requirements-planning` | `new-ticket-requirements-planning.yaml:311` (`sequence.persist_api_review`) | Persists contrarian API review JSON and a markdown view (`api-review.json` + `api-review.md`) into `outbox/requirements/`. | Fold into the prior persist step (single “persist artifacts” op) or move into an extension op to centralize formatting. |
| `new-ticket` | `new-ticket.yaml:181` (`state.states.analyze_scope`) | Converts the requirements list into: (a) cross-cell detection, and (b) a `ticket.manage` `actions[]` payload for child tickets (foreign-cell requirements), then prints JSON to stdout so the next state can `json_parse(...).actions`. | Have `new-ticket-requirements-planning` emit a ready-to-run `child_ticket_actions` array in its output schema; or implement “requirements → ticket actions” as an extension op to avoid embedding Python in the primary orchestration recipe. |
| `new-ticket` | `new-ticket.yaml:304` (`state.states.prepare_implement`) | Normalizes the structured “implementation session” form choice into a JSON object (`session_id`, `include_prior_summary`, `prior_session_summary`, `extra_instructions`) for the `ticket-implement` child recipe inputs. | Inline with CEL in `new-ticket.yaml` (ternaries over `states.implementation_session.outputs.fields.*`) to avoid a shell step; or move this mapping logic into `ticket-implement` entirely so the parent doesn’t need a prep step. |
| `ticket-implement` | `ticket-implement.yaml:45` (`state.states.choose`) | No-op command (`echo choose`) used only to branch into “continue session” vs “new session” states based on whether `inputs.session_id` is present. | Replace with a non-shell no-op op (e.g. `sleep` duration `0s`), or remove the `choose` state by making `continue_session` the initial state with a fallback transition when `inputs.session_id` is empty (if recipe semantics allow). |
| `ticket-implement` | `ticket-implement.yaml:156` (`state.states.persist_summary`) | Writes `implementation/assistant-summary.txt` to the outbox so the Codex summary is available as an artifact to later steps/humans. | If artifacts aren’t required, use only `ticket-implement` outputs. If artifacts are required, consider an extension op for “write small text artifact” to avoid inline shell. |
| `ticket-validate` | `ticket-validate.yaml:17` (`state.states.detect`) | Heuristically detects validation commands from repo markers (e.g. `package.json`, `go.mod`, `Cargo.toml`) and prints `{"suggested_commands": ...}` for the UI step. | Prefer cell-owned configuration (per-cell “validate” extension op) over heuristics; or use `codex.exec` to propose commands (still needs a trust/approval gate). |
| `ticket-validate` | `ticket-validate.yaml:101` (`state.states.run_provided`) | Writes the provided validation script to `outbox/validation/commands.sh`, runs it in the cell directory, and tees combined output to `outbox/validation/output.txt`. | Standardize validation via an extension op (e.g. `.colony2/ops/validate`) so users don’t provide arbitrary shell; keep `command_execution` only inside the extension op. |
| `ticket-validate` | `ticket-validate.yaml:132` (`state.states.run_suggested`) | Same as above, but runs the detected “suggested commands” instead of user-provided text; persists output as an artifact. | Same as above: make validation a dedicated op/extension per cell or per project. |
| `ticket-validate` | `ticket-validate.yaml:165` (`state.states.run_custom`) | Same as above, but runs the “custom commands” entered in the structured form; persists output as an artifact. | Same as above: reduce arbitrary shell by routing through a curated validation op and/or a fixed command allowlist per cell. |
| `ticket-validate` | `ticket-validate.yaml:198` (`state.states.summarize`) | Reads `outbox/validation/output.txt` and prints the last ~80 lines as `output_tail` for downstream display and retry decisions. | Instead of a second shell step, have the run step also emit a tail summary file; or use `codex.exec` to summarize the output artifact (better human-oriented, but adds model cost). |
| `ticket-merge` | `ticket-merge.yaml:21` (`state.states.resolve`) | Computes `LOCAL_HASH` from the git worktree and derives defaults for upstream repo/branch/commit message (from inputs or `context.git.*`), returning a JSON object used by later merge states. | If `context.git.hash`/`context.git.resolved_hash` can safely stand in for the local workspace hash, pass that directly to `squashrebasemerge` and drop this step; or add defaulting behavior to the merge op itself. |
| `ticket-merge` | `ticket-merge.yaml:146` (`state.states.done`) | No-op terminal step (`echo done`). | Remove the `done` state and make `merge` terminal (no transitions), or use a non-shell no-op op. |

## Callout: merge is *not* performed via `command_execution`

In `ticket-merge`, the actual merge is performed by the dedicated merge op:
- `ticket-merge.yaml:117` uses `op: squashrebasemerge`.

The `command_execution` steps in `ticket-merge` are for *input/default resolution* (`resolve`) and a no-op terminal (`done`).

## Summary: why `command_execution` shows up so much

Common patterns across the catalog:

1) **Artifact persistence + rendering**
- Several steps exist purely to write JSON/markdown into the outbox (`triage.json`, `requirements/*.md`, `implementation/assistant-summary.txt`, validation logs).
- This is not “laziness” so much as: there is no first-class built-in op for “write this string/JSON to an artifact path” or “render a markdown view from a structured payload”.
- Likely new op(s): `artifact.write_text`, `artifact.write_json`, and/or a small set of purpose-built renderers like `requirements.render_bundle`.

2) **Data shaping because CEL/templating is awkward for complex transforms**
- `new-ticket` uses Python to transform `requirements[]` into `ticket.manage actions[]`.
- The core need is real (generate child tickets with structured descriptions), but the implementation is a workaround for the lack of a “map/filter/build payload” op.
- Likely new op(s): `requirements.to_child_ticket_actions` (or a generic `json.transform` op with a safe, declarative transform language).

3) **Executing validation commands (arbitrary shell)**
- `ticket-validate` uses `command_execution` to run user-provided/suggested shell scripts and to tee output.
- This is the highest-risk/least-structured use: it’s flexible, but it pushes policy and safety into “whatever shell the user typed”.
- Likely new op(s): project/cell-owned extension ops (e.g. `.colony2/ops/validate`) to centralize validation commands, allowlists, env setup, and output capture.

4) **“No-op” states used for branching or terminal completion**
- `ticket-implement.choose` (`echo choose`) and `ticket-merge.done` (`echo done`) exist only to satisfy state-machine structure.
- This is mostly mechanical boilerplate; it’s a sign we’d benefit from a non-shell no-op op (or a recipe pattern that avoids “dummy states”).
- Likely new op(s): reuse existing `sleep` with `0s` for no-op, or add an explicit `noop` op for clarity.

5) **Output summarization / tailing**
- `ticket-validate.summarize` exists to compute a tail view of logs for display + retry decisions.
- This is a recurring need in workflows (human review wants a short digest).
- Likely new op(s): `log.tail` and/or `log.summarize` (optionally model-backed) that reads a named artifact and emits a short summary + tail.

Net: most `command_execution` usage falls into a small number of reusable patterns. Converting those into first-class ops (especially artifact writes and validation) would reduce recipe complexity, improve safety, and make recipes more consistent.
