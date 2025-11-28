# SWF Migration - server/embeddedtemporal (SWF Harness)

## Objectives
- Retire the embeddedtemporal module and replace it with an SWF-focused test harness tracked via swf-go ticket.

## Scope & Plan (Temporal usage to remove)
- Delete `server/embeddedtemporal` (Temporal server/client, tests, docs).
- Update dependents (api, nucleus, tests) to drop embeddedtemporal references; point them to SWF harness ticket `11-swf-go-embedded-ticket.md`.
- Provide temporary stubs/shims for callers until the SWF harness is available.

## SWF Gaps Affecting This Project
- No in-memory mode; harness must stand up lightweight Postgres/Strata equivalents.
- No Temporal replay; rely on chapter inspection for determinism checks.
