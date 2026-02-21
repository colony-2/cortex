# Recipe Testing Specs Overview (Start Here)

## Purpose

The c2 recipe testing framework will allow users of the c2 recipe cli to work with the server to build and test recipes.

## Key Specifications

1. `/src/cli/RECIPE_TESTING_CLI_SPEC.md`
   - CLI command behavior
   - local suite compilation
   - per-case parallel orchestration and live reporting
2. `/src/api/RECIPE_TESTING_OPENAPI_UPDATES_SPEC.md`
   - request/response contract for server APIs (core openapi spec in /src/api/openapi. generators in /src/cli, /src/server/openapi, /src/web/openapi)
   - schema definitions for cases, mocks, assertions, evaluations
3. `/src/server/RECIPE_TESTING_SERVER_BEHAVIOR_REQUIREMENTS.md`
   - runtime semantics for validation/execution (implement in combination of /src/server/recipe-worker and /src/server/api and ensure exposed in both the /src/server/api's testserver and /src/server/cortext server)
   - isolation, mocking, evaluations, and policy handling

## Shared architectural decisions

1. Per-case functional API model:
   - `POST /v1/recipe-tests/cases/validate`
   - `POST /v1/recipe-tests/cases/execute`
2. No server persistence for:
   - reusable suites
   - run objects / run reports
3. Target recipe supports both:
   - server reference (`name` + `version`)
   - inline recipe content
4. CLI owns:
   - suite fan-out
   - parallel execution
   - live progress reporting
   - local summary/report artifacts
5. Cases can include outcome evaluations:
   - deterministic text pattern evaluators
   - LLM judge evaluators

## Recommended reading order

1. Read `/src/api/RECIPE_TESTING_OPENAPI_UPDATES_SPEC.md` for contract shape.
2. Read `/src/server/RECIPE_TESTING_SERVER_BEHAVIOR_REQUIREMENTS.md` for runtime semantics.
3. Read `/src/cli/RECIPE_TESTING_CLI_SPEC.md` for orchestration and UX behavior.

## Team implementation split

1. API/server schema team:
   - implement OpenAPI contract from `RECIPE_TESTING_OPENAPI_UPDATES_SPEC.md`
2. Server runtime team:
   - implement behavior from `RECIPE_TESTING_SERVER_BEHAVIOR_REQUIREMENTS.md`
3. CLI team:
   - implement command/workflow behavior from `RECIPE_TESTING_CLI_SPEC.md`

## Completion criteria

The system is considered complete when:

1. per-case validate/execute endpoints work for both recipe source modes
2. server returns full case result (assertions + evaluations) without run persistence
3. CLI can execute suites in parallel and report case outcomes as they complete
4. CLI writes local summary/report artifacts for the full suite
