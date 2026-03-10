---
name: c2-requirements-contrarian-review
description: Contrarian review of requirements for compatibility, risk gaps, and actionable blockers.
---

# c2-requirements-contrarian-review

## Purpose

Challenge requirements quality, missing risks, and compatibility gaps before implementation planning.

## Required outputs (`/src/outbox/requirements`)

- `api-review.json` with:
  - `ok` (bool)
  - `blocking_issues` (list)
  - `feedback` (list or string)

## Guardrails

- Reject backward-incompatible proposals.
- Provide concise, actionable blockers.
