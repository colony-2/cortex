# Implementation Recipe Spec (Historical)

## Purpose
Provide an agent-guided, single-cell implementation workflow that:
- Generates acceptance test statements for a ticket.
- Implements the requested change set.
- Runs local and dependent tests.
- Validates outputs (compatibility, spec coverage, alignment, style).
- Produces a summary and captures git status.

This spec describes the intent of the removed `impl-recipe` for reference only.

## Inputs (expected)
- ticket_id: unique ticket identifier.
- cell: target cell/module name.
- spec_doc: path to the specification document.
- spec_content: optional inline spec content.
- git_state: git worktree mode (shared or isolated).
- dependency_snapshot: optional dependency metadata.
- validator_script: optional path to lint/validator script.
- validation_threshold: numeric threshold for validator scoring.
- max_validation_attempts: max retries for validation loop.
- dependency_ticket_recipe: recipe name to use for dependency tickets.
- codex_context: worktree/blobstore context for codex sessions.
- stub_mode: if true, generate stub artifacts without running codex.

## Workflow (intended)
1) Bootstrap
   - Ensure working directories exist for generated specs and plans.
   - Seed spec file if stub content is provided.
   - Compute paths and defaults (tests doc, summary doc, lint script, codex context).

2) Test Statements
   - Generate acceptance test statements with codex (or stub text).
   - Write statements to tests doc path.
   - Lint test statements (best effort).

3) Implementation
   - Use codex to implement changes based on the spec and test statements.
   - Write implementation summary.
   - Run local tests and dependent tests (best effort).
   - Track pending dependencies and whether another implementation pass is needed.

4) Dependency Resolution
   - Record pending dependency information for follow-up.

5) Validation
   - Run codex-based checks for compatibility, spec coverage, test alignment, and style.
   - Produce a validation score map and a review requirement flag.

6) Wrap Up
   - Produce a final summary (codex or stub).
   - Capture git status for context.
   - Emit final outputs for downstream consumption.

## Outputs (intended)
- test_statements_doc: path to the generated test statements document.
- implementation_summary: path to the implementation summary document.
- pending_dependencies: list of pending dependency items.
- validation_scores: map of validation scores.
- git_context_patch: git status output or patch context.
- needs_human_review: boolean indicating manual review requirement.
