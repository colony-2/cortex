VibeThis Bug Reports (from cortex)

1) Test ops leaking into production schema

- Summary: Running `cortex schema` returns both production and test ops in the JSON schema output.
- Impact: Schema consumers (UI, validators, generators) see dozens of test-only activity types (e.g., context_logger, batch_validator, test-activity, etc.) alongside real ops like command_execution, recipe, llm, git ops. This breaks production contracts and confuses users.
- Root Cause: The package `server/recipe-worker/pkg/ops` contains files `test_activities.go` and `test_activities_typed.go` that register many test activities in an `init()` function. These files are not test-only (they do not have `_test.go` suffix nor build tags), so they are compiled in normal builds. The `cortex` binary imports `server/recipe-worker/pkg/ops` via `cmd/cortex/execute.go` (needed for `ActivityRegistry`), which causes those `init()` functions to run at process startup, registering test ops into the global registry in `server/recipe-core/pkg/ops`.
- Evidence:
  - `server/recipe-worker/pkg/ops/test_activities.go` and `test_activities_typed.go` call `recipeops.Register(...)` in `init()` for many synthetic ops.
  - `server/cortex/cmd/cortex/execute.go` imports `github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops`, which triggers the init-time registrations when the cortex binary starts, even if the `execute` command isn’t used.
- Recommended Fix (upstream – outside cortex):
  - Make test activities test-only by renaming to `_test.go` or adding an exclusion build tag, e.g. add to both files:
    //go:build testonly
    // +build testonly

    and ensure that only tests build with the `-tags testonly` flag, or move registrations into test helpers under `test-fixtures`.
  - Alternatively, move the test registrations into a separate package (e.g., `server/recipe-worker/pkg/ops/testops`) and import it only from test files using a blank import.
  - As a more robust design, avoid global registration via init for tests; instead expose explicit `RegisterTestOps()` used by tests.
- Mitigation applied in cortex: To prevent leaking test ops at runtime, cortex now clears the global ops registry before registering production ops from `server/ops`, `server/recipe-worker/pkg/export`, and `server/git` (see `server/cortex/internal/shared/registry.go`). This ensures `cortex schema` only emits production ops even if test ops were pre-registered by init.

