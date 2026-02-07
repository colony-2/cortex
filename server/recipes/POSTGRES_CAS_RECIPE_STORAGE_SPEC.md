# Postgres Content-Addressable Recipe Storage (CAS) — Spec & Plan

## Context / Problem

The current recipe service uses git as the content store. Even with ephemeral shallow/sparse workspaces, high-frequency recipe reads/writes still trigger expensive git operations (clone/fetch/checkout/log), which is causing unacceptable latency and resource usage.

This document proposes replacing git-backed recipe storage with a Postgres-backed, content-addressable storage (CAS) + explicit version tables. Publishing remains independent per recipe, and version history remains queryable without relying on git.

Assumption: we do **not** need to preserve or migrate any existing recipes.

---

## Goals

- **Eliminate git checkouts** from the hot path for create/update/get/list/history.
- **Immutable versions**: every update creates a new, immutable recipe version.
- **Independent publishing**: each `(project_id, recipe_name)` has at most one “published” version at a time, updatable independently.
- **Deduplicated storage**: identical recipe content stored once (CAS).
- **Fast reads**: fetching published recipes is a single indexed DB query (plus optional in-memory cache).
- **Auditability**: author/message/timestamps for versions and publishes.
- **Concurrency safety**: optimistic concurrency for updates and publish pointer changes.

## Non-goals

- Preserving git-like branch semantics (branches, `HEAD~1`, arbitrary ref expressions).
- Supporting partial history import from git (explicitly out of scope).
- Large-scale binary artifact storage (this is for recipe YAML bytes).

---

## High-level Design

Replace the “git is source of truth” model with:

1) **CAS blob table** keyed by a content digest (e.g. `sha256` of raw bytes).
2) **Recipe identity table** keyed by `(project_id, name)`.
3) **Recipe events table** (append-only) where each row can represent:
   - a **save** (new immutable version),
   - a **publish** (publish some saved content), or
   - an **unpublish** (clear publish state),
   and where publish/unpublish history is what enables “published as-of X”.
4) (Optional) **Denormalized pointers** on `recipes` for fast “current published” and “latest saved”.
5) (Optional) **Tags** per recipe mapping human-friendly strings (e.g. `v1.2.3`) to a saved version.

All operations become DB transactions; the service no longer needs `git.Repository`, ephemeral workspaces, or `.c2/recipes` paths.

---

## Identifiers and “Ref” Syntax

### Identifiers

- **Content digest**: `sha256:<hex>` of the raw YAML bytes as provided to the service.
- **Event ID**: KSUID (or UUID) generated per event row.
- **Saved ordinal**: per-recipe monotonic integer (`1..N`) assigned only to **save** events for cheap “history” paging and stable human references (`v1`, `v2`, ...).

### `name@ref` resolution (replacement for git refs)

The service should keep the `GetRecipe(projectID, name, ref)` shape, but redefine `ref`:

- `""` (empty): return **published** version if present; else return **latest saved** version (highest `saved_ordinal`).
- `sha256:<hex>`: return the saved version with that digest (if multiple saved versions reference the same digest, return the one with the highest `saved_ordinal`; alternatively require disambiguation via event id).
- `ver:<ksuid>` (or bare KSUID): return that exact saved version (event id).
- `v<ordinal>` (e.g. `v12`): return by ordinal.
- `asof:<rfc3339nano>`: return the version that was published as-of that time (based on publish/unpublish events).

This preserves the ergonomics of “pinning” and “testing unpublished versions” while dropping git-specific ref parsing.

### “Published as-of” refs (project-wide snapshot semantics)

If you want `recipe1@asof:...` and `recipe2@asof:...` to mean “the version of each recipe that was published *as of the same moment*”, you need an **append-only publish event log** and a project-scoped time coordinate.

We will use **database time** only:

- Each publish/unpublish event records `event_at = transaction_timestamp()` (DB time, not client time).
- `recipe@asof:2026-04-01T12:12:12.123456Z` resolves to “the last publish/unpublish event for that recipe with `event_at <= asof`”.
  - If that event is a **publish**, return the published version.
  - If that event is an **unpublish** (or there is no event), treat as **not published** at that time.
- For deterministic results when multiple events share the same timestamp, add a stable tie-breaker (`ORDER BY event_at DESC, id DESC`).

#### Consistent “snapshot” usage

To get a consistent view across multiple recipes (“all recipes as-of the same moment”), clients should not invent their own timestamps. Instead:

- The service should return a DB-derived timestamp (e.g. `snapshot_at = transaction_timestamp()`) in list/snapshot responses.
- Clients then pass that exact `snapshot_at` back as `@asof:<snapshot_at>` in subsequent calls.

This avoids clock skew between app servers, clients, and the database, and gives a stable anchor for “this moment”.

---

## Data Model (Postgres)

Schema names are illustrative; use `public` unless you already namespace recipe tables.

### 1) `recipe_blobs` (CAS)

Stores recipe bytes once per digest.

- `digest` `bytea` PK (32 bytes for sha256)
- `algo` `text` NOT NULL DEFAULT `'sha256'`
- `content` `bytea` NOT NULL (optionally compressed)
- `content_encoding` `text` NOT NULL DEFAULT `'identity'` (or `'zstd'`)
- `size_bytes` `int` NOT NULL
- `created_at` `timestamptz` NOT NULL

Indexes:
- PK on `digest`

### 2) `recipes`

Identity and pointers.

- `id` `varchar(27)` PK (KSUID) or `uuid`
- `project_id` `varchar(27)` NOT NULL
- `name` `text` NOT NULL
- `created_at` `timestamptz` NOT NULL
- `updated_at` `timestamptz` NOT NULL
- `deleted_at` `timestamptz` NULL (optional soft delete)
- `latest_saved_event_id` FK → `recipe_events.id` NULL (denormalized)
- `latest_saved_ordinal` `bigint` NULL (denormalized)
- `published_event_id` FK → `recipe_events.id` NULL (denormalized; points at the last publish-affecting event)
- `row_version` `int` NOT NULL DEFAULT 0 (for optimistic concurrency if desired)

Notes:

- A recipe is considered “currently published” iff `published_event_id` points to an event with `published = TRUE`.
- Save-only events must not update `published_event_id` (so saving a draft does not unpublish).

Constraints / Indexes:
- UNIQUE `(project_id, name)`
- For prefix queries (`name LIKE 'foo/%'`): consider `btree (project_id, name text_pattern_ops)` or a trigram index on `name` (plus `project_id` filter).

### 3) `recipe_events` (required; combines versions + publish history)

Append-only per-recipe event log. This is the source of truth for:

- saved version history (events with `saved_ordinal IS NOT NULL`)
- publish/unpublish history (events where `published = TRUE` or `digest IS NULL`)
- published-as-of resolution (`@asof:...`)

Columns:

- `id` `varchar(27)` PK (KSUID) or `uuid`
- `project_id` `varchar(27)` NOT NULL
- `recipe_id` FK → `recipes.id` NOT NULL
- `digest` FK → `recipe_blobs.digest` NULL
- `saved_ordinal` `bigint` NULL
- `target_saved_ordinal` `bigint` NULL
- `published` `boolean` NOT NULL
- `event_at` `timestamptz` NOT NULL DEFAULT `transaction_timestamp()`
- `actor` `text` NULL
- `message` `text` NULL

Semantics:

- **Save-only**: `digest NOT NULL`, `saved_ordinal NOT NULL`, `published = FALSE`
- **Save+publish**: `digest NOT NULL`, `saved_ordinal NOT NULL`, `published = TRUE`
- **Publish-only** (publish a previously saved version): `target_saved_ordinal NOT NULL`, `digest NOT NULL`, `saved_ordinal NULL`, `published = TRUE`
- **Unpublish**: `target_saved_ordinal NULL`, `digest NULL`, `saved_ordinal NULL`, `published = FALSE`

Important: publishing an older saved version should **not** change what the system considers the “tip” / “latest saved” version. The “latest saved” version is defined by `max(saved_ordinal)` (or the denormalized `recipes.latest_saved_*` pointers), not by “latest event by time”.

Constraints / Indexes:

- UNIQUE `(recipe_id, saved_ordinal)` WHERE `saved_ordinal IS NOT NULL`
- Index `(project_id, recipe_id, event_at DESC, id DESC)`
- Index `(recipe_id, saved_ordinal DESC)` WHERE `saved_ordinal IS NOT NULL`
- Index `(digest)` (optional, if you need reverse lookups)
- CHECK constraints enforcing:
  - `published = TRUE` implies `digest IS NOT NULL`
  - `digest IS NULL` implies `published = FALSE AND saved_ordinal IS NULL`
  - `target_saved_ordinal IS NOT NULL` implies `saved_ordinal IS NULL`
  - (Recommended) when `target_saved_ordinal IS NOT NULL`, it must reference an existing saved version for this recipe, and `digest` must match that saved version’s digest (enforce in application code, or via trigger if desired).

## Service Behavior (API Semantics)

Below is the intended behavior; the concrete Go types can be adjusted.

### Create

Input: `(project_id, name, content, description/message, auto_publish)`

1) Validate name + content (and optionally pre-validate if `auto_publish`).
2) Compute digest of `content`.
3) Upsert blob by digest (`INSERT ... ON CONFLICT DO NOTHING`).
4) Create `recipes` row if absent; else return `ErrAlreadyExists` (since “create” is semantic creation of a new recipe name).
5) Insert a new `recipe_events` row with `saved_ordinal=1`, `digest=...`, `published=auto_publish`.
6) Set denormalized pointers:
   - `recipes.latest_saved_event_id = event.id`, `recipes.latest_saved_ordinal = 1`
   - if `auto_publish`: `recipes.published_event_id = event.id`

Idempotency option: If clients retry `Create` due to network errors, allow an optional `idempotency_key` stored in a side table to dedupe requests. If not implemented, retries may create duplicates only if “create” is allowed to create same name (it shouldn’t), so this mainly affects “version insert” for update flows.

### Update

Input: `(project_id, name, content, message, auto_publish, expected_version_id|expected_digest|expected_ordinal)`

1) Validate project + recipe exists and not deleted.
2) Concurrency check:
   - If an expected ref is supplied, ensure it matches current latest saved (`latest_saved_event_id` / `latest_saved_ordinal` / digest).
3) If content digest matches current latest digest, treat as no-op:
   - Return current latest version; if `auto_publish` and it isn’t published, publish it.
4) Upsert blob, insert a **save** event with `saved_ordinal = (max+1)`, `digest=...`, `published=auto_publish`.
5) Update denormalized pointers:
   - `latest_saved_*` to the new event
   - if `auto_publish`: `published_event_id` to the new event

Note: if a later explicit publish targets an older saved version, `latest_saved_*` must remain unchanged.

### Publish / Unpublish

Publish input: `(project_id, name, ref/saved_version_id, expected_published_event_id?)`

1) Resolve `ref` to a concrete saved version (an event with `saved_ordinal IS NOT NULL`, or a digest/ordinal/id reference).
2) Load content bytes from `recipe_blobs` via the resolved digest.
3) Validate (schema + id-match + CEL) and fail if invalid.
4) Insert a **publish-only** event row with:
   - `target_saved_ordinal = <resolved saved ordinal>`
   - `digest = <resolved digest>` (denormalized for faster content joins and to make `@asof` resolution a single-row lookup)
   - `saved_ordinal = NULL`
   - `published = TRUE`
   - `event_at = transaction_timestamp()`
5) Transactionally set `recipes.published_event_id = publish_event.id` with optimistic concurrency:
   - If `expected_published_event_id` is provided, ensure it matches current before updating.
6) The event insert + pointer update must be in the same DB transaction so they succeed/fail together.

Unpublish input: `(project_id, name, expected_published_event_id)`

1) Insert an **unpublish** event row with `digest=NULL`, `saved_ordinal=NULL`, `published=FALSE`, `event_at=transaction_timestamp()`.
2) Transactionally set `recipes.published_event_id = unpublish_event.id` (or NULL, if you prefer) if expected matches.
3) The event insert + pointer update must be in the same DB transaction so they succeed/fail together.

### Get

`GetRecipe(project_id, name, ref)`:

- If `ref` is `asof:<rfc3339nano>`, resolve via `recipe_events`:
  - Find the last publish-affecting event with `event_at <= asof`:
    - predicate: `published = TRUE OR digest IS NULL`
    - order: `ORDER BY event_at DESC, id DESC LIMIT 1`
  - If it has `published=TRUE`, return its digest/content; otherwise return `ErrNotPublished` for that time.
- Otherwise resolve to a specific saved version (published/latest/tag/digest/ordinal/id).
- Return `RecipeWithContent { Name, VersionID, Digest, ContentBytes, IsPublished, PublishedAt/By? }`
  - `VersionID` should be interpreted as the ID of the saved version (a `recipe_events.id` with `saved_ordinal IS NOT NULL`) when applicable; callers can always rely on `Digest` as the stable content identifier.

Note: if you want `PublishedAt/By`, store publish metadata either:
- in the publish-affecting event rows of `recipe_events` (source of truth: `event_at` + `actor`), optionally denormalizing the “current” event id onto `recipes` for faster default lookups.

With `recipe_events`, `PublishedAt/By` should come from the latest publish-affecting event (or the event selected by `ref`).

### List

List should become DB-native (no filesystem walks):

- Use `recipes` as the base set for a project.
- Join latest saved metadata and “current published” metadata as needed (from `recipes.latest_saved_*` and `recipes.published_event_id`).
- Apply name prefix filtering with indexed patterns.
- Return `snapshot_at` (DB time) with list responses so callers can later resolve individual recipes via `@asof:<snapshot_at>` if they need a consistent “this moment” view.

Tip semantics:

- “Latest saved” comes from `recipes.latest_saved_*` (or `max(saved_ordinal)`), and should not be affected by publishing older versions.
- “Currently published” comes from `recipes.published_event_id` (and is expected to change when you publish an older saved version).

For “snapshot” listing (all recipes *as-of* a timestamp), compute the published version per recipe from `recipe_events`:

- For each `recipe_id`, find the last event with `event_at <= :asof` (tie-break by `id`).
- Keep only rows where `published = TRUE` (published at that time).

API shape suggestion: extend list filters with `AsOf *time.Time`.

- If `AsOf == nil`: return current view and include `snapshot_at` in the response (DB time of the query).
- If `AsOf != nil`: use it as `:asof` in the query above.

Implementation note: Postgres makes this efficient with `DISTINCT ON (recipe_id)` or a window function. Example shape:

```sql
SELECT r.project_id, r.name, e.digest, e.event_at, e.actor
FROM recipes r
JOIN LATERAL (
  SELECT *
  FROM recipe_events e
  WHERE e.project_id = r.project_id
    AND e.recipe_id = r.id
    AND e.event_at <= :asof
    AND (e.published = TRUE OR e.digest IS NULL)
  ORDER BY e.event_at DESC, e.id DESC
  LIMIT 1
) e ON TRUE
WHERE r.project_id = :project_id
  AND e.published = TRUE;
```

### History

Query saved versions from `recipe_events` ordered by `saved_ordinal DESC` (or `event_at DESC`) with pagination:

- `WHERE saved_ordinal IS NOT NULL`

For “published history” (what was published/unpublished when), query publish-affecting events:

- `WHERE published = TRUE OR digest IS NULL`

---

## Concurrency & Transactions

All mutating operations run in a single DB transaction.

Recommended locking strategy:

- For update/publish/unpublish: `SELECT ... FROM recipes WHERE project_id=? AND name=? FOR UPDATE` to serialize changes per recipe name.
- Use explicit expected fields for optimistic concurrency at the API level (mirroring the current `ExpectedCommit` behavior).

This eliminates git-level races and makes concurrency behavior deterministic.

---

## Storage, Retention, and GC

Default: retain all versions indefinitely (equivalent to “git history”).

Optional policies:

- **Soft delete recipes**: set `deleted_at`; typically also insert an unpublish event and/or set `published_event_id` to NULL; keep events for audit.
- **History truncation**: keep last `N` saved versions per recipe (requires deleting old `recipe_events` rows where `saved_ordinal IS NOT NULL`).
- **Blob GC**: periodic job deletes blobs not referenced by any `recipe_events.digest`.

Backups: rely on Postgres backup/restore; this becomes the source of truth.

---

## Observability

Metrics (minimum):

- `recipes_get_latency_ms` (by ref type: published/latest/tag/id/digest)
- `recipes_update_latency_ms`
- `recipes_publish_latency_ms`
- counts of `version_conflicts`
- blob dedupe rate: `blobs_inserted / blobs_requested`

Logging:

- structured log fields: `project_id`, `recipe_name`, `event_id`, `digest`, `saved_ordinal`, `published_event_id`.

---

## Migration / Rollout Plan (No Backwards Data Compatibility)

1) **Define schema + migrations**
   - Add the tables above; publish history lives in `recipe_events`, with optional denormalized pointers on `recipes`.
2) **Implement storage layer**
   - New store interfaces for recipes/events/blobs with transactional helpers.
3) **Refactor service to “DB as source of truth”**
   - Remove git dependencies from `ServiceConfig`.
   - Replace create/update/delete/publish/get/list/history with DB implementations.
4) **Update `name@ref` resolution**
   - Update `pkg/recipe/provider.go` parsing docs and error messages.
5) **Validation path**
   - Keep current schema/id validation and CEL validation behavior; run pre-validation on `auto_publish` and full validation on publish.
6) **Tests**
   - Add store/service tests for:
     - version append + no-op update behavior
     - publish pointer concurrency (`expected_*`)
     - `asof` resolution (including unpublish events)
     - blob dedupe correctness
7) **Remove git specs and dead code**
   - Delete/retire `.c2/recipes` path derivation, workspace helpers, git integration tests, and git command specs (or mark archived).
8) **Deploy behind a feature flag**
   - Start with new empty storage in staging.
   - Cut production to the new backend when ready.

---

## Open Decisions (Defaults Suggested)

1) **Ref format**: keep `@<ksuid>` as “version id”, and add `@v<ordinal>` + `@sha256:<hex>`.
2) **As-of syntax**: `@asof:<rfc3339nano>` vs `@at:<...>` (recommend `asof` to signal “<= time”, not equality).
3) **Publish metadata**: use publish-affecting `recipe_events` rows as source of truth; keep `recipes.published_event_id` as a denormalized “current” pointer only.
4) **Compression**: store `content` as raw `bytea` initially; add `zstd` later if storage becomes material.
