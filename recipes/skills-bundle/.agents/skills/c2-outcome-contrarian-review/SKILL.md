---
name: c2-outcome-contrarian-review
description: Contrarian quality review for outcome plans, test statements, and validation command coverage.
---

# c2-outcome-contrarian-review

## Purpose

Validate that test statements and validation commands are complete, outcome-focused, and compatibility-safe.

## Required outputs (`/src/outbox/outcome`)

- `review.json` with:
  - `ok` (bool)
  - `blocking_issues` (list)
  - `feedback` (list or string)

## Guardrails

- Reject implementation-detail-heavy statements.
- Reject gaps that could hide compatibility regressions.
