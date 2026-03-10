---
name: c2-implementation-plan-author
description: Translate approved requirements into an ordered implementation plan with dependency ticket specs.
---

# c2-implementation-plan-author

## Purpose

Translate approved requirements into an ordered implementation plan, including dependency ticket specs for other cells.

## Required outputs (`/src/outbox/implementation`)

- `index.md`
- `plan.json` with:
  - `workstreams` (ordered)
  - `backward_compatibility_checks`
  - `requires_dependency_tickets` (bool)
  - `dependency_strategy`
- optional `dependency-ticket-specs.json`

## Guardrails

- Never plan backward-incompatible changes.
- Encode dependency order explicitly.
