# Ticket: SWF Embedded Harness for Tests (swf-go)

## Context
- Embedded Temporal harness is being removed; SWF migration requires a lightweight test harness for integration/integration-like tests.
- Harness should live in `/swf-go` as an embedded package usable by server components (recipe-worker, ops, api, nucleus, cortex).

## Requirements
- Provide an in-process harness that:
  - Spins up Postgres (temp DB) and a Strata-compatible endpoint (mock or lightweight service).
  - Builds an `SWFEngine` via `EngineBuilder` with configurable workers/capabilities.
  - Exposes helpers to start/stop, clean DB/state, and return engine/DSN/Strata endpoints to callers.
- Offer chapter inspection utilities (read stories/chapters, decode envelopes) for assertions.
- Support capability watchers needed for input remote tasks (`FindTasksWaitingForCapability` polling loop helper).
- Allow configurable max concurrency and tenant ID for isolation across tests.
- Provide Go test helpers/examples demonstrating a minimal job with a remote task completion and a recipe-style job worker.

## Deliverables
- New package under `/swf-go` (e.g., `pkg/embedded`) with the harness implementation.
- Tests validating job start, remote task completion, chapter creation, and teardown stability.
- Documentation (README) describing setup, APIs, and sample usage for downstream repos.
