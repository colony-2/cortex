# Temporal Ticket Queue & API Orchestration Spec

## Purpose
Ensure TicketRecipe executions obey core rules while Temporal remains the system of record for ticket lifecycle:

1. A ticket may mutate only its owning cell.
2. Cells only request work from direct or transitive dependencies.
3. Each cell processes TicketRecipe runs sequentially (FIFO) with deterministic ordering.

We rely on Temporal primitives (signals, long-lived workflows, durable history) to coordinate queueing, ticket creation, and locking without introducing new database tables. This spec also defines the REST surface that marshals requests into Temporal, keeping the workflows as the source of truth.

## Key Requirements
- **Per-cell serialization**: Exactly one TicketRecipe runs per cell at any time; additional ticket requests wait in FIFO order.
- **Temporal-first ticket source of truth**: Ticket metadata (cell, title, description, actor, etc.) is submitted via signals; the TicketRecipe workflow creates/updates the relational ticket record using `ticket.manage` as part of execution.
- **Signal-buffer queueing**: Use Temporal’s signal buffering to maintain order instead of persisting queue state in Postgres.
- **Rewind aware**: Rewinds restart the active ticket workflow without losing its place and without disturbing later signals.
- **History management**: Queue workflow self-resets via `ContinueAsNew` after `N` requests or `M` hours to keep history bounded.
- **Dependency validation**: Continue to block cross-cell work not rooted in the caller’s dependency graph.
- **Failure resilience**: Recover automatically if workflows crash or workers restart; queued work is not lost; no divergence between Temporal and the database.
- **Observability**: Operators can inspect active/queued tickets via Temporal queries surfaced through the ticket service/API.

## Temporal-Centric Architecture

### CellQueue Workflow
- Workflow ID pattern: `cellQueue/{cell}` (`WorkflowIdReusePolicy = AllowDuplicate`).
- Maintains minimal state:
  - `activeTicketWorkflow` — workflow id of the ticket currently executing (empty when idle).
  - `queuedCount` — approximate number of buffered enqueue signals (for telemetry only).
  - `requestsSinceReset` — counter used to trigger `ContinueAsNew`.
  - `startedAt` — timestamp when the current run began.
- **Signals**
  - `EnqueueTicket(request)` — payload includes ticket metadata (`request_id`, `tempTicketId`, `title`, `description`, `actor`, `attachments`, `created_at`). Signals buffer in FIFO order while another ticket is active.
  - `TicketCompleted(result)` — sent by the running TicketRecipe workflow with outcome details (success, blocked, retry requested, final summary metadata).
  - `RestartActive(restart)` — used for rewind; contains `tempTicketId` and checkpoint info. Cancels and restarts the matching ticket workflow.
- **Queries**
  - `QueueState` — exposes `activeTicketWorkflow`, `queuedCount`, and a sample of most recent completion metadata for UI/monitoring.
- **Execution loop**
  1. If `activeTicketWorkflow` is set, block on the `TicketCompleted` channel; otherwise block on `EnqueueTicket`.
  2. Upon receiving an `EnqueueTicket` while idle, decrement `queuedCount`, derive deterministic workflow id (e.g., `ticket/{cell}/{tempTicketId}`), and start a new TicketRecipe workflow via `StartWorkflow` with the provided metadata. Set `activeTicketWorkflow` to the new workflow id and increment `requestsSinceReset`.
  3. Additional enqueue signals arriving during execution remain buffered automatically by Temporal.
  4. When `TicketCompleted` arrives, clear `activeTicketWorkflow`. If the payload requests immediate retry (e.g., rewind), the queue workflow restarts the same TicketRecipe (using `ContinueAsNew` signal or `SignalWithStart`). Otherwise, it proceeds to the next buffered `EnqueueTicket` signal.
  5. After each completion, evaluate history guard: if `requestsSinceReset >= N` or `time.Since(startedAt) >= M`, emit a `ContinueAsNew` with the remaining buffered requests (Temporal automatically replays pending signals). Reset counters in the new run.

### TicketRecipe Workflow
- Workflow ID pattern assigned by the cell queue (e.g., `ticket/{cell}/{tempTicketId}`) ensures uniqueness per request.
- **Bootstrap state**
  - First action is `ticket.manage` with `create_ticket` using metadata from the enqueue signal. The resulting database ticket id becomes canonical; the workflow stores it in search attributes and subsequent signals.
  - For rewinds, metadata includes `existing_ticket_id`; the workflow skips creation and uses `reset_ticket`/`append_workflow_event` instead.
- **Execution**
  - After creating the ticket record, the workflow runs the TicketRecipe state machine (Plan/Impl/etc.).
- **Completion**
  - Emits `TicketCompleted` signal back to `cellQueue` with:
    - `ticket_id` (database id)
    - `workflow_id` & `run_id`
    - `status` (success, blocked, needs_retry)
    - `queue_metrics` (start/end times, enqueue timestamp carried from request)
  - Releases any cell-scoped context so the queue can start the next request.

## API & Service Responsibilities
- **Ticket REST API**
  - Receives ticket creation request containing cell, metadata, actor, optional specs. Generates a deterministic `tempTicketId` (e.g., UUID) and `request_id` for idempotency.
  - Calls Temporal `SignalWithStart` on the per-cell queue workflow `cellQueue/{cell}`, passing `EnqueueTicket(request)`; `SignalWithStart` ensures the workflow exists.
  - Immediately responds with acknowledgement including `tempTicketId`. Queue position can be inferred via `QueueState` query (exposed through service) without waiting for DB insert.
  - No database write occurs until the TicketRecipe workflow processes the request and issues `ticket.manage`.
- **Ticket Service helpers**
  - `GetQueueState(cell)` — wraps Temporal query to expose active workflow id, approximate queue depth, and latest completion metadata.
  - `SubmitRewind(ticketID, checkpoint)` — identifies the associated `cellQueue` and `ticket` workflow, then emits `RestartActive` to queue and `RestartFromCheckpoint` to the ticket workflow.
  - `LookupTicketWorkflow(ticketID)` — maintained via search attributes emitted when the ticket is created; allows mapping DB id → workflow id for UI/automation.
- **Dependency validation**
  - Prior to enqueuing dependency tickets, higher-level services (PlanRecipe, TicketRecipe) still call `graph.IsDependent`. When dependency work is required, the owning ticket workflow emits another `EnqueueTicket` signal (via API helper) with the dependent cell and requirements metadata.

## Rewind Flow
1. User/API calls rewind for ticket `T` (known DB id). Ticket service resolves `cell` & active workflow id via search attributes.
2. Signal `ticket/{cell}/{tempTicketId}` workflow with `RestartFromCheckpoint(checkpoint)` to capture new requirements/specs.
3. Signal `cellQueue/{cell}` with `RestartActive(T, checkpoint)`.
4. Queue workflow cancels the in-flight run (if active) and restarts it immediately; since metadata already includes the canonical ticket id, the workflow skips creation and performs `ticket.manage` reset/notes before resuming.
5. If `T` is not active (e.g., waiting for dependency to resolve), the restart simply updates stored metadata and the buffered signal order remains intact.

## Failure Recovery
- **Cell queue workflow crash**: Temporal replays workflow history; buffered signals persist, so no ticket requests are lost.
- **Worker outage**: Signals accumulate on Temporal and are delivered once workers return; workflows resume automatically.
- **TicketRecipe failure**: Workflow emits `TicketCompleted` with failure status; cell queue marks request finished (or requeues if outcome flagged `needs_retry`). Human intervention occurs via `ticket.manage` + `input` inside the workflow.
- **API retries**: `SignalWithStart` is idempotent when `request_id` reused; duplicate submissions collapse into the same queued request.

## Observability
- `QueueState` query exposes `activeTicketWorkflow`, `queuedCount`, and last completion metadata. Ticket service surfaces this via REST for dashboards.
- Each TicketRecipe workflow emits search attributes: `ticket_id`, `cell`, `temp_ticket_id`, enabling cross-link between Temporal and DB.
- Structured logs on the queue workflow for each `EnqueueTicket`, `TicketCompleted`, `RestartActive`, and `ContinueAsNew` event include durations and request metadata.
- Optional background poller exports metrics:
  - `ticket_recipe_active{cell}` = 1 when `activeTicketWorkflow` non-empty.
  - `ticket_recipe_queue_depth{cell}` = `queuedCount`.
  - `ticket_recipe_queue_latency_seconds` = derived from timestamps in `TicketCompleted.queue_metrics`.
  - `ticket_recipe_queue_run_age_seconds` = `now - startedAt` to monitor when ContinueAsNew should trigger.

## Rollout Plan
1. Implement `cellQueue` workflow with signal handlers, including history guard logic for `ContinueAsNew` (tunable `N`, `M`).
2. Update TicketRecipe workflow bootstrap to create tickets via `ticket.manage` using signal metadata before proceeding with plan/impl states.
3. Modify REST API to send enqueue signals rather than inserting tickets directly.
4. Add search attributes linking Temporal workflows to ticket ids once created.
5. Build monitoring/dashboard support using `QueueState` and search attribute queries; release behind feature flag.

## Open Questions
- Pick default thresholds for `N` (requests per run) and `M` (max run age) that balance history size vs. replay cost.
- Should we allow administrators to cancel or reprioritize queued signals (additional admin signals)?
- For dependency-generated tickets, does the owning workflow send enqueue signals directly or through API middleware to reuse validation/auth logic? (Current plan: API helper proxy.)
