---
name: c2-implementation-loop
description: Execute implementation iterations and return structured checkpoint status for recipe routing.
---

# c2-implementation-loop

## Purpose

Execute implementation iterations until work is ready for validation or a structured blocker status is reached.

## Required outputs (`/src/outbox/implementation`)

- `latest-status.json`
- `progress.ndjson`
- `summary.md`
- optional `questions.json`
- optional `dependency-ticket-specs.json`
- optional `test-statement-change-request.md`

## `latest-status.json` contract

- `status`: `ready_for_validation | needs_user_input | needs_dependency_tickets | needs_test_statement_update | blocked`
- `summary`: short description
- `details`: optional structured object

## Guardrails

- Never edit `.c2/tests/*.md` during implementation.
- Never apply cross-cell direct fixes; emit dependency ticket specs.
- Use structured artifacts for all downstream-critical data.
