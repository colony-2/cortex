# Ticket Temporal Storage & Versioning – Overview

This document provides a high-level reference for the complete temporal ticket rollout, spanning three phases. Each bullet below maps back to the detailed phase specs.

## Phase 1 – Schema Foundations
- Add `valid_from` / `valid_until` (timestamp without time zone) columns to `tickets`.
- Enable `btree_gist` and create an exclusion constraint to prevent overlapping slices per ticket ID.
- Backfill existing ticket rows and update GORM models to include the new columns.

## Phase 2 – Service & API Updates
- Update `CreateTicket` / `UpdateTicket` to manage temporal slices (close previous row, insert new version).
- Emit ticket-mutation events internally during update.
- Replace the generic `AppendEvent` with payload-specific service methods (`AppendWorkflowEvent`, `AppendMarkdownEvent`, `AppendChangeSetEvent`).
- Extend search filters with `At *time.Time` and add `GetTicketAt(ctx, id, at)`.

## Phase 3 – Reset & Point-in-Time Events
- Introduce `ResetTicket` (superseding `ResetEvents`) to coordinate event reset + ticket slice insertion in one transaction.
- Emit a `ticket_reset` event containing reset metadata.
- Extend `TicketEventFilter` / `ListEvents` with `At` support so tickets and events can be queried consistently at any timestamp.

## Cross-cutting Testing & Migration Notes
- Migration steps cover enabling `btree_gist`, adding temporal columns, applying constraints, and adjusting models.
- Each phase includes targeted unit/integration tests to validate new behaviour (temporal slices, append APIs, resets, point-in-time queries).
- Further open questions (e.g. whether reconstructed slices emit additional events) are tracked in Phase 3.

Refer to:
- `TEMPORAL_TICKETS_PHASE1.md`
- `TEMPORAL_TICKETS_PHASE2.md`
- `TEMPORAL_TICKETS_PHASE3.md`

for full implementation details per phase.

