# Recipe Test Statements

These statements are intended to drive `c2 recipe test` cases.

## Statement Catalog

| ID | Test statement | Relevant file(s) | Importance | Type / dependencies | Polarity |
|---|---|---|---|---|---|
| TS-001 | Appropriate-cell ticket continues in current cell without creating a reassignment ticket. | `new-ticket.yaml`, `new-ticket-triage.yaml` | High | Integration; deps: `c2` API, `recipe.run_and_get_result` | Positive |
| TS-002 | Out-of-cell ticket creates one reassignment child ticket and sets original ticket state to `waiting_user`. | `new-ticket.yaml`, `new-ticket-triage.yaml` | High | Integration; deps: `ticket.manage`, `c2` API | Positive |
| TS-003 | Invalid recommended cell falls back to current cell and workflow still proceeds. | `new-ticket-triage.yaml`, `new-ticket.yaml` | High | Unit (`recipe_case`) + integration; deps: `cells()` context | Negative |
| TS-004 | Triage produces `triage.json` artifact with decision, recommended cell, and rationale fields. | `new-ticket-triage.yaml` | Medium | Unit (`op_case`); deps: artifact outbox/inbox binding | Positive |
| TS-005 | Requirements planning writes `requirements/plan.json`, `requirements/index.md`, and one requirement markdown per requirement ID. | `new-ticket-requirements-planning.yaml` | High | Integration; deps: `codex.exec`, artifact outbox | Positive |
| TS-006 | Each requirement targets an existing cell and depends only on declared requirement IDs. | `new-ticket-requirements-planning.yaml` | High | Unit (`recipe_case`); deps: project cells context | Positive |
| TS-007 | Requirements review selecting `revise` loops and incorporates user feedback in next requirements artifacts. | `new-ticket.yaml`, `new-ticket-requirements-planning.yaml` | High | Integration; deps: `input`, `codex.exec` | Positive |
| TS-008 | Blocking API review must collect user feedback before requirements can be approved. | `new-ticket.yaml`, `new-ticket-requirements-planning.yaml` | High | Integration; deps: `input`, review artifacts | Negative |
| TS-009 | Approved requirements with foreign target cells create child tickets containing scope, acceptance criteria, risks, and open questions. | `new-ticket.yaml` | High | Integration; deps: `ticket.manage`, `jq()` templating | Positive |
| TS-010 | Approved requirements with only current-cell targets skip child-ticket creation and proceed to implementation session. | `new-ticket.yaml` | High | Integration; deps: state transitions | Positive |
| TS-011 | Implementation session `continue` reuses prior Codex session ID. | `new-ticket.yaml`, `ticket-implement.yaml` | High | Integration; deps: `codex.exec` session continuity | Positive |
| TS-012 | Implementation session `new_with_summary` starts a new Codex session while injecting prior summary context. | `new-ticket.yaml`, `ticket-implement.yaml` | High | Integration; deps: `codex.exec` | Positive |
| TS-013 | Implementation writes `implementation/assistant-summary.txt` artifact for reviewer visibility. | `ticket-implement.yaml` | Medium | Unit (`op_case`); deps: artifact outbox | Positive |
| TS-014 | Validation `use_suggested` runs detected commands and creates `validation/output.txt` and `validation/output-tail.txt`. | `ticket-validate.yaml` | High | Integration; deps: `command_execution`, shell runtime | Positive |
| TS-015 | Validation custom commands execute user-provided commands and persist full and tail outputs as artifacts. | `ticket-validate.yaml` | High | Integration; deps: `input`, `command_execution` | Positive |
| TS-016 | Failed validation keeps workflow open and offers retry path back to implementation session. | `new-ticket.yaml`, `ticket-validate.yaml` | High | Integration; deps: state transitions, user input | Negative |
| TS-017 | Merge review selecting `merge` runs merge workflow and records a non-empty merged hash. | `new-ticket.yaml`, `ticket-merge.yaml` | High | Integration; deps: `squashrebasemerge`, git context | Positive |
| TS-018 | Merge review selecting `skip` completes workflow without merge execution and without merged hash. | `new-ticket.yaml`, `ticket-merge.yaml` | High | Integration; deps: state transitions | Negative |
| TS-019 | Merge workflow prompts for upstream repo, branch, and commit message when any are missing. | `ticket-merge.yaml` | High | Integration; deps: `input`, `command_execution` resolve step | Positive |
| TS-020 | Merge approval selecting `cancel` exits without merge and without merged hash output. | `ticket-merge.yaml` | High | Integration; deps: `input` decision branch | Negative |
| TS-021 | Planning outputs are parsed from `requirements/plan.json` artifact, not assistant-summary text. | `new-ticket-requirements-planning.yaml` | High | Unit (`recipe_case`); deps: artifact binding/read step | Positive |
| TS-022 | API review outputs are parsed from `requirements/api-review.json` artifact, not assistant-summary text. | `new-ticket-requirements-planning.yaml` | High | Unit (`recipe_case`); deps: artifact binding/read step | Positive |
| TS-023 | Contrarian review consumes `plan.json` via inbox artifact binding and emits both JSON and markdown review artifacts. | `new-ticket-requirements-planning.yaml` | High | Integration; deps: `codex.exec`, artifact bindings | Positive |
| TS-024 | `recipe test compile` produces canonical IR JSON from suite input. | `guides/RECIPE_TESTING_CLI_USER_GUIDE.md` | Medium | CLI integration; deps: `c2 recipe test compile` | Positive |
| TS-025 | `recipe test validate` returns non-zero when any selected case is invalid or errors. | `guides/RECIPE_TESTING_CLI_USER_GUIDE.md` | Medium | CLI integration; deps: `c2 recipe test validate` | Negative |
| TS-026 | `recipe test run` writes `summary.json`, `summary.md`, and per-case `result.json` in chosen out-dir. | `guides/RECIPE_TESTING_CLI_USER_GUIDE.md` | Medium | CLI integration; deps: `c2 recipe test run` | Positive |
| TS-027 | `scenario_md` suite without a fenced YAML or JSON block fails compile. | `guides/RECIPE_TESTING_CLI_USER_GUIDE.md` | Medium | CLI integration; deps: `c2 recipe test compile` | Negative |

## Notes for Test Authoring

- Prefer `integration_case` for cross-recipe flows (`new-ticket` happy path and retry paths).
- Use `op_case` for artifact-read/output mapping checks (`TS-021`, `TS-022`).
- Use mocks sparingly; prioritize real transitions, artifacts, and ticket state effects.
