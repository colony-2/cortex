# Ticket Primary Job + Ticket Creation Dependencies (Spec)

## Summary

When a ticket is created, the ticket service can auto-start a recipe job (“autostart”). Today that job can run immediately. We need to allow a new ticket to be created with **completion dependencies on other tickets**, such that the new ticket’s autostart job waits for the **primary jobs** (autostart jobs) of those upstream tickets to complete before executing.

This requires:

1) Persisting the autostart job ID on the ticket as the ticket’s **primary job**.
2) Extending all ticket creation surfaces (OpenAPI/HTTP, Go service, CLI, recipe `ticket.manage` action) to accept dependency ticket IDs.
3) Starting the autostart job with a “depends on these job IDs” list derived from upstream tickets’ primary jobs.

Related context/spec: `docs/specs/ticket-recipe-auto-start.md`.

---

## Goals

- Add `primaryJobId` to ticket read model and persist it during autostart creation.
- Add `dependsOnTicketIds` to ticket create APIs (HTTP/OpenAPI + Go service + CLI + ticket.manage action).
- Ensure dependent ticket autostart does not run until upstream tickets’ primary jobs are completed.
- Keep all changes backwards compatible (new request fields optional; new response fields nullable).

## Non-Goals

- Updating dependencies after ticket creation.
- Building a full dependency graph query API/UI.
- Advanced cycle detection/graph validation (basic self/cross-project validation only for now).

---

## Terms

- **Primary job**: The workflow/job ID of the ticket’s autostart recipe job.
- **Ticket dependency**: Ticket **T** depends on tickets **A,B** ⇒ T’s primary job must wait for completion of A’s and B’s primary jobs.

---

## Proposed Contract Changes

### OpenAPI: Ticket

Add optional field to `components/schemas/Ticket` in `api/openapi/colony2-api.yaml`:

- `primaryJobId: string, nullable: true`

Rationale:

- Existing clients keep working.
- Allows downstream systems (UI/CLI) to directly jump from ticket → primary job.

### OpenAPI: TicketCreateRequest

Add optional field to `components/schemas/TicketCreateRequest`:

- `dependsOnTicketIds: string[]`

Semantics:

- Each ID must be an existing ticket in the same project.
- The new ticket’s primary job will wait for the primary jobs of these tickets to complete (SWF prerequisites with condition `complete`).

### OpenAPI: StartWorkflowRequest (generic workflow start)

SWF now supports job prerequisites directly on job start. To expose that capability for API-driven workflow starts (and reuse it from ticket autostart), add to `components/schemas/StartWorkflowRequest`:

- `prerequisites: JobPrerequisite[]`

Add schema:

- `JobPrerequisite`:
  - `job_id: string`
  - `condition: string` enum: `complete | success`

Semantics:

- `complete`: prerequisite job must be archived (any outcome).
- `success`: prerequisite job must be archived and have `completion_status == "success"`.
- For `success` prerequisites, if a prerequisite fails, the dependent job fails immediately with a non-retryable error (per SWF behavior).

### Regenerated bindings

After OpenAPI changes:

- Regenerate Go server bindings: `server/openapi/...`
- Regenerate CLI client bindings: `cli/internal/openapi/...`
- If the web client is generated from OpenAPI in this repo, regenerate it too.

---

## Data Model Changes (server/ticket)

### Ticket: persist primary job ID

Update `server/ticket/internal/model/types.go` `Ticket` with:

- `PrimaryJobID *string` (DB column, indexed)
  - Stored on the ticket slice (the current “latest” slice where `valid_until = 9999...`).
  - Copied forward automatically across slices because ticket updates clone the prior record.

### Ticket: persist dependencies

We need to persist the dependency ticket IDs at creation. Postgres supports native string arrays, so we should use a `text[]` column rather than JSONB.

Recommended shape (GORM):

- Field type: `pq.StringArray` (or `[]string`) stored as `type:text[]`.
- Optional: a GIN index if we later need “contains ticket id” queries (not required for creation-time enforcement).

---

## Service Changes (server/ticket)

### Extend CreateInput

Update `server/ticket/internal/service/types.go`:

- Add `DependsOnTicketIDs []model.ID` (or `[]string` at the public boundary and convert internally).

### CreateTicket behavior (happy path)

Update `server/ticket/internal/service/service.go` `CreateTicket` to:

1) Validate dependency IDs (if provided):
   - trim + dedupe
   - reject empty IDs
   - reject self-dependency (not possible at create time unless caller passes generated ID; still validate if applicable)
   - ensure each dependency ticket exists in the same project
2) Resolve upstream **primary job IDs**:
   - Load the latest slice for each dependency ticket.
   - Read `PrimaryJobID`.
   - If missing, return an error indicating the dependency ticket cannot be depended upon.
3) Start the ticket’s autostart recipe job and persist `PrimaryJobID`:
   - Start the recipe job with SWF prerequisites:
     - `Prerequisites = [{JobID: upstreamPrimaryJobID, Condition: complete}, ...]`
   - Start the job inside the ticket DB transaction (`swf.WithTx(ctx, st.DB())`).
   - Store the returned job ID on the created ticket row as `PrimaryJobID`.
4) Append the initial workflow “running” event as today.

All of (ticket create + autostart job submit + primary job persist + initial event append) should remain in the same `store.WithTx` transaction.

### Error semantics

Recommended mapping (HTTP handler can translate):

- invalid dependency IDs / cross-project dependency: `ErrInvalidDependency` → 400
- dependency exists but has no resolvable primary job: `ErrDependencyMissingPrimaryJob` → 409 (or 400 if preferred)

---

## HTTP API Handlers (server/api)

### Ticket creation endpoint

Update `server/api/internal/handlers/api.go` `handleCreateTicket` to:

- Parse `dependsOnTicketIds` from request body.
- Map to `ticket.CreateInput.DependsOnTicketIDs`.

### Ticket response mapping

Update `toOpenAPITicket` to include:

- `PrimaryJobId` (new OpenAPI field) from `tk.PrimaryJobID`.

---

## Recipe Operation API (ticket.manage)

Workflows can create tickets via the `ticket.manage` operation using action payloads (see `server/ticket/internal/model/actions.go`).

Update `CreateTicketAction` to include:

- `depends_on_ticket_ids: []string` (JSON/YAML)

Update the op handler to map this to `ticket.CreateInput.DependsOnTicketIDs`.

This ensures dependent ticket creation works from:

- HTTP API
- CLI
- Workflows (recipes)

---

## Workflow/Recipe Start Plumbing

We need a mechanism to express “this recipe job depends on these other jobs” when starting the job. SWF now supports this natively via prerequisites on `StartJob`/`RestartJob`.

### Preferred implementation: SWF `StartJob.Prerequisites`

1) Update recipe/job start helpers to accept prerequisites:

- `server/recipe-core/pkg/starter.StartRecipeJob(...)` should plumb prerequisites into the `swf.StartJob{Prerequisites: ...}` request.
- The workflow HTTP start handler (`POST /api/projects/{projectId}/workflows`) should plumb `StartWorkflowRequest.prerequisites` into the engine start request.

Why this is preferred now:

- Dependencies are applied at submit-time (PGWF `wait_for`) and enforced before the job runs.
- Supports both “complete” and “success” gating.
- SWF records completion status/detail for archived jobs, enabling accurate “success” prerequisite enforcement without Strata inspection.

Notes / constraints from SWF prerequisite behavior:

- Prerequisites are stored in Strata job metadata (chapter metadata), not in PGWF payloads.
- If prerequisites are ever needed for `RestartJob`, SWF requires `ExtraTaskOutput` (and optionally `ExtraTaskInput`) so the prereqs can be attached to the `RestartExtra` chapter.

---

## CLI Changes (cli)

Update `cli/internal/cmd/ticket.go`:

- Add repeatable flag: `--depends-on-ticket <ticketId>`
  - Plumbs into `TicketCreateRequest.dependsOnTicketIds`.

If CLI supports manual workflow start (`c2 workflow run`):

- Add repeatable flag `--prereq <jobId>:<complete|success>` (or similar) plumbed into `StartWorkflowRequest.prerequisites`.

---

## Tests

### Ticket service

- New/updated tests in `server/ticket/internal/service/service_test.go`:
  - creating a ticket persists `PrimaryJobID` when engine+recipes configured
  - dependency validation (empty, duplicates, self, cross-project)

### Integration: dependency enforcement

Extend or add a test similar to `server/api/internal/handlers/ticket_autostart_test.go`:

1) Create Ticket A (auto-starts recipe, completes).
2) Create Ticket B with `dependsOnTicketIds=[A]`.
3) Assert B’s primary job does not execute the recipe until A’s primary job completes.
   - Acceptable signals:
     - job status is “PENDING_JOBS” until A completes, then becomes active/completed
     - or B’s ticket stage/state changes only after A completes

### OpenAPI/CLI bindings

- CLI unit test(s) to ensure the flag wiring produces the expected request payload.

---

## Rollout / Migration Notes

- Gorm `AutoMigrate` on `model.Ticket` will create the new column(s) (`primary_job_id`, `depends_on_ticket_ids`, etc.).
- No backfill/compat logic is required if we’re resetting/deleting existing ticket data as part of rollout.
