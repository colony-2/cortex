# Git Workspace Modernization – Phase 1 (Execution Context & Inputs)

## Overview

Phase 1 establishes the execution contract that every recipe run must satisfy before any op executes. We introduce a shared Git-aware execution context, validate mandatory inputs, and standardize the data that flows into template resolution so downstream phases can rely on consistent metadata.

## Goals

- Enforce `basegitrepo`, `basegithash`, `ticketid`, and `cellname` as required inputs for every recipe.
- Capture Git execution state in a structured `GitContext` carried throughout the workflow.
- Surface normalized keys in the recipe input map (`context.git`, `ticket_id`, `cell_name`) for templates and ops.
- Define deterministic workspace and blob-store locations that all phases will reuse.

## Non-Goals

- Performing Git clone/restore/persist work (handled by Phase 2).
- Changing recipe definition syntax or the `ops.RegisterableOp` API.
- Handling nested recipes or inline mutators (deferred to Phase 3).

## Current State

- `ExecuteRecipe` passes the original inputs map unchanged, leaving Git semantics to individual ops.
- Git helper libraries (`gitshallow`, `gitcommit`) exist but are invoked ad hoc through exported activities.
- Ops such as `command_execution` rely on bespoke `working_directory` hints with no guaranteed workspace path.

## Phase Scope

1. Create an `executionContext` struct populated at the start of `ExecuteRecipe` with:
   - `BaseRepo`, `BaseHash`, `TicketID`, `CellName`
   - `WorktreePath`
   - `BlobStoreURI`
   - `PersistHash` (initially `BaseHash`)
   - `WorkspacePrepared` flag
2. Validate required inputs. Missing values cause a deterministic workflow failure prior to scheduling any activities.
3. Augment the recipe inputs with normalized keys:
   - `context.git` – canonical git metadata (`base_repo`, `base_hash`, `persist_hash`).
   - `context.recipe` – recipe metadata (`id`, `version`, optional `node_path`, optional invocation fields).
   - `context.worktree` / `context.blobstore` / `context.ticketid` / `context.cellname` – top-level execution metadata keys (no further nesting).
   - `ticket_id`, `cell_name` – lifted out for convenience.
4. Compute deterministic paths:
   - `worktree = <workspaceRoot>/<runID>/work` isolates filesystem state per workflow attempt (`workspaceRoot` defaults to `/tmp/colony2/workflows`, configurable via `VIBETHIS_WORKSPACE_ROOT`).
   - `blobStoreURI = <persistBase>/<cellName>/<ticketID>` points to the shared blob namespace (defaults to `file:///tmp/colony2/blobstore`, can be an `s3://` URI or other adapter-supported backend).
   - Git thin packs will later live beneath `<blobStoreURI>/git/thin-packs`, leveraging `{commit}-{parent}-{root}.pack` filenames for uniqueness without run IDs.
5. Thread the `executionContext` through sequence/state helpers so every op sees the most recent `PersistHash` and metadata.

## Deliverables

- Updated compiler code to construct, validate, and attach the execution context.
- Unit tests covering input validation and the presence of the standardized keys in node inputs.
- Documentation updates referencing the new keys (`context.git`, plus exposed `ticket_id` / `cell_name`).

## Exit Criteria

- Recipes fail fast when required Git metadata is absent.
- All ops (activities and inline) can access `inputs["context"]["git"]`; there are no `git_*` aliases.
- Workspace and blob-store locations are deterministic and configurable, providing the foundation for Phase 2.
