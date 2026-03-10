---
name: c2-test-statement-curator
description: Define or update test statements and validation commands before implementation begins.
---

# c2-test-statement-curator

## Purpose

Define and update outcome test statements before implementation and emit a validation command plan.

## Required outputs (`/src/outbox/outcome`)

- `plan.json`
- `tests-index.md`
- `validation-commands.txt`
- `test-statement-delta.md`

## Guardrails

- Test statements must stay business/expectation-focused.
- Implementation stage must not edit `.c2/tests/*.md`; this stage may.
- Keep compatibility expectations explicit.
