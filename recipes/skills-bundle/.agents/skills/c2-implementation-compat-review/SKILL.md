---
name: c2-implementation-compat-review
description: Contrarian compatibility and sequencing review for implementation plans.
---

# c2-implementation-compat-review

## Purpose

Verify implementation plan compatibility and rollout sequencing safety.

## Required outputs (`/src/outbox/implementation`)

- `compat-review.json` with:
  - `ok` (bool)
  - `blocking_issues` (list)
  - `feedback` (list or string)

## Guardrails

- Block plans with backward-incompatible or unsafe sequencing.
- Keep output machine-routable.
