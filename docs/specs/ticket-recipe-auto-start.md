# Ticket Creation Auto-Recipe Kickoff

## Overview
- When a ticket is created, the ticket service must immediately start a recipe job via `recipe-worker`’s `StartRecipeJob` helper and record the workflow event in the same database transaction.
- StartJob derivation must come directly from the persisted ticket + cell/project metadata so the recipe executes with the correct repo, branch, and actor context.
- A built-in `internal://new_ticket` recipe will serve as the final fallback and should be discoverable from an embedded registry folder.

## StartJob Derivation
- `RecipeName`: `cell.DefaultRecipe` if set ➜ `project.DefaultTicketRecipe` if set ➜ `"internal://new_ticket"`.
- `Inputs`: map keyed by `ticket` containing the freshly created ticket (ID, project_id, cell_name, cell_id, title, description, stage, state, creator, created_at/updated_at). Shape mirrors `ticket.Ticket` JSON used by the API so downstream ops (e.g., `ticket.manage`) can read the same fields.
- `JobContext.Actor`: from ticket actor (`ActorName` empty, `ActorEmail` populated when user actor provided).
- `JobContext.Workflow`: `CellName` = ticket cell; `JobID` stays empty (engine context still carries job id).
- `JobContext.GitBase.BaseRepo`: `cell.GitRepoName` when non-nil, otherwise `project.GitRepoPath`.
- `JobContext.GitBase.BaseRef`: `cell.GitBranch` when non-nil, otherwise `project.GitRepoBranch` when non-nil, otherwise `"main"`.
- `GitRef`: same branch selection as `JobContext.GitBase.BaseRef`.

## Transaction Pattern
- Wrap ticket creation, job submission, and the initial workflow event append in a single `store.WithTx` call.
- Use the gorm transaction handle from the ticket store to invoke `swf-go`’s `WithTx` engine wrapper, then call `StartRecipeJob` so the job row is created under the same transaction.
- Append a `WorkflowEvent` (`type=running`, `workflow_id=JobID`, `run_id=JobID` unless SWF exposes a run id) through `appendEventInTx` before commit so the event and job stay in sync.
- If any step fails (recipe lookup, job creation, event append), abort and roll back the entire transaction; the ticket should not be visible without an accompanying job/event.
- Upgrade to the latest `swf-go` version in all go modules (`server/ticket`, `server/recipe-worker`, `server/recipe-core`) to gain the `WithTx` support and refresh go.work/go.sum.

## Registry & Internal Recipes
- Enhance the recipe registry used by `SWFWorkflowControl` so construction optionally accepts an embedded recipe folder (e.g., `embed.FS`) and registers an `internal://` resolver that reads YAML from that folder.
- Keep existing filesystem/DB-backed recipe resolution intact; only routes `internal://*` through the embedded store when present.
- Ship an embedded `internal://new_ticket` recipe YAML in `recipe-worker` (e.g., `internalrecipes/new_ticket.yaml`) and expose the folder to the registry initializer.

## Default `internal://new_ticket` Recipe
- Minimal sequence that marks the ticket completed via `ticket.manage`:
  - Single step `op: ticket.manage` with `actions: [ { type: update_ticket, expected_version: inputs.ticket.version, stage: "__completed__", state: "waiting_user" } ]`.
  - Uses the `ticket_id` and creator supplied in `inputs.ticket`; rely on ticket service to fill `expected_version` from the created ticket.
- Add an integration test that boots a toy SWF engine + embedded registry, creates a ticket through the service, asserts a job is enqueued, and verifies the ticket event log contains the running workflow event and the stage transition to completed after the recipe runs.

## Module-by-Module Changes
- **server/ticket**: extend `ServiceConfig` to accept the workflow control/engine and recipe registry handle; build StartJob from the persisted ticket + cell/project metadata; wire transactional job start + workflow event append in `CreateTicket`; expose job id in the `CreateTicket` response DTO if needed; add tests covering happy path, missing recipe, and rollback when job/event fails.
- **server/recipe-worker**: embed `internalrecipes/new_ticket.yaml`; update the registry implementation to accept an optional embedded FS and resolve `internal://` recipes from it; ensure `SWFWorkflowControl.StartJob` uses the updated registry and propagates tx-aware engines.
- **server/recipe-core**: host the `StartRecipeJob` API (moved from recipe-worker) and expose it for consumers; update dependency wiring/builder to pass the engine/registry into ticket service initialization; ensure StartJob typing and contextual helpers document the git base fields used above.
- **swf-go dependency**: bump versions and adjust any interface changes for `WithTx`; add a smoke test using the tx-bound engine to confirm job rows land in the same transaction.

## Test Plan (unit + integration)
- **Ticket service unit tests**: validate StartJob derivation priority order (cell vs project vs fallback), branch/ref selection, repo selection, actor context mapping, and error/rollback on recipe lookup or StartRecipeJob failure.
- **Transaction unit test**: simulate failing event append or job creation inside `WithTx` to ensure ticket insert is rolled back and no event/job rows remain.
- **Registry unit tests**: confirm `internal://` recipes resolve from embedded FS and do not interfere with normal recipe loading; ensure absence of embedded folder keeps behavior unchanged.
- **Embedded recipe unit test**: load `internal://new_ticket` and verify it produces the expected sequence/op payload (stage set to `__completed__`, uses provided ticket id/version).
- **Integration (toy SWF engine, in server/api where both deps are present)**: spin up toy engine + embedded registry, create a ticket, assert `StartRecipeJob` runs, wait for completion, and verify ticket event log includes the `running` workflow event and the ticket stage/state reflect the recipe’s completion action.
