# Ticket Temporal Storage – Phase 1: Schema Foundations

## Goals
- Add `valid_from` / `valid_until` columns (`TIMESTAMP WITHOUT TIME ZONE`) to the `tickets` table.
- Enable PostgreSQL `btree_gist` extension and add an exclusion constraint to prevent overlapping temporal ranges per ticket ID.
- Backfill existing ticket rows with appropriate temporal values.
- Leave service logic largely untouched; focus on database and model groundwork.

## Tasks
1. ALTER TABLE `tickets` to add `valid_from` & `valid_until` (defaulting to `now()` and `'infinity'` during migration/backfill).
2. Drop the legacy single-column primary key and add a composite primary key over `(id, valid_from)` to permit multiple temporal slices per ticket.
3. Enable `btree_gist` extension (no-op if already present).
4. Add constraint: `EXCLUDE USING gist (id WITH =, tsrange(valid_from, valid_until) WITH &&)`.
5. Update GORM model definitions to include the new columns/primary key (read/write support only; no behavioural change yet).
6. Add minimal tests confirming migration/backfill logic runs (e.g. verifying constraint exists).

Phase 1 lays the groundwork for subsequent service changes.
