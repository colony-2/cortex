---
name: c2-triage-cell-boundary
description: Determine whether a ticket belongs to the current cell and recommend the best valid cell when it does not.
---

# c2-triage-cell-boundary

## Purpose

Determine whether the ticket belongs to the current cell and provide a valid reassignment recommendation when needed.

## Required outputs (`/src/outbox/triage`)

- `decision.md`
- `latest-status.json` with:
  - `cell_is_appropriate` (bool)
  - `recommended_cell` (string, empty when in-cell)
  - `rationale` (string)

## Guardrails

- Never invent a cell not present in `context/cells.json` or the provided `cells()` list.
- If uncertain, keep work in the current cell and explain ambiguity in rationale.
