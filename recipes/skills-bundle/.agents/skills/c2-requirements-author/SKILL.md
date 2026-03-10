---
name: c2-requirements-author
description: Produce a requirements bundle that is testable, scoped to cells, and compatible.
---

# c2-requirements-author

## Purpose

Produce a requirements bundle that defines goals, constraints, compatibility expectations, and dependency candidates.

## Required outputs (`/src/outbox/requirements`)

- `index.md`
- `plan.json` with:
  - `goals`
  - `non_goals`
  - `constraints`
  - `compatibility_requirements`
  - `candidate_dependencies`
- optional requirement detail markdown files

## Guardrails

- No backward-incompatible requirements.
- Keep requirements testable and outcome-focused.
