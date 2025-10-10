# ImplRecipe Specification

## Overview
ImplRecipe consumes a specification document and produces code/test changes that satisfy the spec within a single cell's workspace. It enforces the mandated phases: test statement authoring, implementation work, validation, and dependency escalation.

## Inputs
- `ticket_id` (string)
- `cell` (string)
- `spec_doc` (path to markdown)
- `git_state` (shared | discrete; defaults to shared for owning cell)
- `dependency_snapshot` (graph metadata for validation)

## Outputs
- `test_statements_doc` (path)
- `implementation_summary` (markdown)
- `pending_dependencies` (optional array of `{cell, requirements}`)
- `validation_scores`
- `git_context_patch`

## State Machine
```
Bootstrap -> TestStatements -> ImplementationLoop -> DependencyResolution -> ValidationLoop -> Completion
```

### Bootstrap
- Ensure workspace matches requested `git_state` (discrete for !owning cells).
- Load spec content and context (ticket, dependency snapshot).

### TestStatements
- Run `codex.exec` session `impl-test-statements` to draft updates for `.colony2/specs/{ticket_id}-tests.md`.
- Validate compliance with constraints (<=30 words, filenames, tone). Use helper script via `command_execution`.
- On failure, feed feedback back into session until compliant.

### ImplementationLoop
- Run `codex.exec` session `impl-build` with prompt referencing spec + approved test statements.
- Steps:
  1. Instruct agent to modify code/tests.
  2. After each completion, run `command_execution` to execute cell-local tests (e.g., `moon test {cell}`) and downstream tests (`moon test --dependents {cell}`).
  3. Collect outputs; if tests fail, resume session with feedback.
- Session may return `incomplete` with `pendingDependencies`; transition to `DependencyResolution`.

### DependencyResolution
- For each dependency item, call parent TicketRecipe to spawn child TicketRecipe (discrete gitstate). ImplRecipe waits on async completion and then resumes `ImplementationLoop` with dependency change summary appended.

### ValidationLoop
- Execute independent validator sessions using `codex.exec`:
  - `backwards-compatibility`
  - `spec-coverage`
  - `test-statement-alignment`
  - `style-sanity`
- Each validator returns score (1..5) and issue list.
- Aggregate issues and feed back to `ImplementationLoop` until all scores >= threshold (default 4). After N (configurable) retries, pause for human input via `input`.

### Completion
- Summarize implementation outcomes (files touched, tests run) via `codex.exec` short summary.
- Export git context patch and return outputs upstream.

## Error Handling
- Test command failure: record logs, attach to ticket via `ticket.manage` note, loop back into implementation.
- Validator non convergence: escalate via `input` request and mark `needs_human_review` in outputs.
- Git conflicts (shared gitstate) handled by parent TicketRecipe's thinpack + rebase.

## Observability
- Metrics: attempts per phase, test executions count, validator retries, dependency fan-outs.
- Emit structured logs referencing `spec_doc` and `session_id` for traceability.

## Implementation Notes
- Manage codex session IDs deterministically for resumability (`impl-test`, `impl-build`, `impl-validation-*`).
- Provide utility scripts inside repo for test statement linting and downstream test execution to avoid duplicating shell logic.
- Ensure outputs include `pending_dependencies` even when empty (normalised interface for TicketRecipe).

## Open Questions
- Should downstream tests run serially or in parallel via `recipe_set`? (Proposed: allow configuration per cell.)
- How to enforce word-count/test statement rules automatically? (Proposed: small Go utility invoked via `command_execution`.)
