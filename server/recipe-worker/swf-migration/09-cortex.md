# SWF Migration - server/cortex

## Objectives
- Adapt cortex CLI/tests from Temporal workflows to SWF jobs while keeping schema/validation flows intact.

## Scope & Plan (Temporal usage to remove)
- `go.mod`/`go.sum` depend on Temporal SDK; remove.
- `internal/shared/workflow_test.go` and `internal/setup/setup.go` rely on Temporal testsuite/activities; rebuild to use SWF harness and job/task APIs.
- Specs (`WORKFLOW_TEST_RESTORATION_SPEC.md`) reference Temporal tests; update to SWF harness expectations.
- **Execution plumbing**: swap Temporal harness usage in integration tests with the SWF harness; update shared workflow helpers to start jobs and wait on chapters/status.
- **Commands**: ensure workflow-triggering commands start SWF jobs and report job IDs; adjust outputs that surface workflow/run IDs to show job IDs/ordinals.
- **Testing**: port workflow restoration/spec tests to assert against SWF chapters.

## SWF Gaps Affecting This Project
- No deterministic replay service; rely on harness + chapter assertions for determinism checks.
