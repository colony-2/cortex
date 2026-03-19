# Recipe Testing Git Recipe Ref Plan

## Goal

Change recipe-testing submission so requests do not embed raw recipe content.

Instead, the request should identify the recipe through a git-go-style reference that follows the same shape already used by the Codex skill refs in [`/src/server/ops/codex_skill.md`](/src/server/ops/codex_skill.md):

`<host>/<org>/<repo>/<recipe-path>@<git-ref>`

Example:

`github.com/acme/platform-recipes/.colony2/recipes/test-input.yaml@main`

## Scope Assumption

This plan assumes the targeted submission surface is the recipe testing API:

1. `POST /api/projects/{projectId}/recipe-tests/cases/validate`
2. `POST /api/projects/{projectId}/recipe-tests/cases/execute`

This is the path currently using inline recipe content via `target_recipe.mode = "inline_recipe"` in [`/src/server/api/internal/recipetesting/service.go`](/src/server/api/internal/recipetesting/service.go).

If there is another recipe-submission surface intended by this request, the implementation should stop and realign before making code changes.

## Current State

Today the request model supports two target modes:

1. `server_ref`
2. `inline_recipe`

For `inline_recipe`, the client sends:

1. `format`
2. `content`

The server then parses the submitted content directly and computes the case hash from that payload.

## Desired State

Replace raw inline recipe submission with a git-backed recipe reference mode.

Recommended request contract:

```json
{
  "target_recipe": {
    "mode": "git_ref",
    "recipe_ref": "github.com/acme/platform-recipes/.colony2/recipes/test-input.yaml@main"
  },
  "case": {
    "id": "c1",
    "type": "recipe_case"
  }
}
```

Recommended semantics:

1. `server_ref` remains supported for recipes managed by the server recipe service.
2. `inline_recipe` is removed from the request contract.
3. `git_ref` resolves a remote git ref to a concrete commit, reads the referenced recipe file, then parses that file as the test target recipe.
4. The server should preserve the resolved commit for traceability.

## Reference Format

The recipe ref should follow the same parsing rules as the Codex skill refs:

1. Split on the last `@`.
2. Require at least four slash-separated path segments before `@`.
3. Treat the first three segments as `<host>/<org>/<repo>`.
4. Treat the remaining segments as the repository-relative recipe path.
5. Reject empty path segments.
6. Reject absolute paths.
7. Reject `.` and path traversal outside repo root.

Recipe-specific rule:

1. The repository-relative path must resolve to a file, not a directory.

## Proposed Resolution Flow

For `target_recipe.mode = "git_ref"`:

1. Parse `recipe_ref`.
2. Build clone URL as `https://<host>/<org>/<repo>.git`.
3. Create a temporary git workspace.
4. `git init`
5. `git remote add origin <clone-url>`
6. `git fetch --depth 1 origin <git-ref>`
7. Resolve `FETCH_HEAD` to a concrete commit SHA.
8. `git checkout --detach <resolved-commit>`
9. Read the referenced file from the checked-out repository.
10. Parse the file as recipe YAML/JSON using the existing recipe-loading flow.
11. Return the parsed recipe plus resolved commit metadata.

This mirrors the existing skill-ref materialization model closely enough that the behavior remains consistent across both features.

## Recommended Internal Shape

Add a git-ref resolution helper inside recipe testing rather than embedding git command logic directly in the request handler path.

Suggested responsibilities:

1. `parseRecipeGitRef(raw string) (...)`
2. `resolveRecipeGitRef(ctx, raw string) (content []byte, resolvedRef string, resolvedCommit string, err error)`
3. `normalizeRecipePath(raw string) (...)`

The resolved ref should be emitted in the same input-compatible shape:

`<host>/<org>/<repo>/<recipe-path>@<resolved-commit>`

## API Contract Changes

### Request model

Update the target recipe model in [`/src/server/api/internal/recipetesting/service.go`](/src/server/api/internal/recipetesting/service.go):

1. Replace `inline_recipe` with `git_ref` in the `mode` enum.
2. Remove `format`.
3. Remove `content`.
4. Add `recipe_ref`.

### Semantic validation

Validation rules should become:

1. `server_ref` requires `name` and exactly one of `version` or `ref`.
2. `git_ref` requires non-empty `recipe_ref`.
3. Invalid git-go-style refs fail with `invalid_target`.
4. Missing file, directory target, fetch failure, and parse failure fail with structured issues under `target_recipe.recipe_ref`.

## Case Hash and Traceability

The implementation should stop using client-submitted inline content as the identity source.

Recommended behavior:

1. Resolve `git_ref` to a concrete commit SHA.
2. Use the resolved identity, not the original symbolic ref, in trace data.
3. Compute `case_hash` from a deterministic recipe identity plus case payload.

Recommended recipe identity for `git_ref`:

`<host>/<org>/<repo>/<recipe-path>@<resolved-commit>`

This is preferable to a synthetic content hash because:

1. it directly reflects the submitted source of truth,
2. it is auditable,
3. it matches the skill-ref traceability model.

## Files Expected To Change During Implementation

Primary code:

1. [`/src/server/api/internal/recipetesting/service.go`](/src/server/api/internal/recipetesting/service.go)
2. new helper file under [`/src/server/api/internal/recipetesting`](/src/server/api/internal/recipetesting)
3. [`/src/server/api/internal/handlers/recipe_testing_test.go`](/src/server/api/internal/handlers/recipe_testing_test.go)
4. [`/src/server/api/internal/recipetesting/service_test.go`](/src/server/api/internal/recipetesting/service_test.go)

Docs/specs:

1. [`/src/server/RECIPE_TESTING_SERVER_DETAILED_SPEC.md`](/src/server/RECIPE_TESTING_SERVER_DETAILED_SPEC.md)
2. [`/src/server/RECIPE_TESTING_SERVER_BEHAVIOR_REQUIREMENTS.md`](/src/server/RECIPE_TESTING_SERVER_BEHAVIOR_REQUIREMENTS.md)

Optional follow-up if needed:

1. any external API documentation that currently shows `inline_recipe`

## Testing Plan

Implementation should include focused tests for:

1. valid git recipe ref parsing
2. invalid ref format rejection
3. path traversal rejection
4. empty git ref rejection
5. directory target rejection
6. handler validation for `git_ref`
7. successful validate request using resolved git recipe content
8. successful execute request using resolved git recipe content
9. case hash stability when the symbolic ref resolves to the same commit
10. resolved-commit traceability behavior

Because real network access in tests is undesirable, resolution should be testable through injection points or replaceable helper functions.

## Migration Approach

Recommended rollout:

1. First change server internals and tests to support `git_ref`.
2. Update request examples and docs.
3. Update clients to stop sending `inline_recipe`.
4. Remove any remaining inline-only tests and examples.

If rollout risk is a concern, a short-lived compatibility window can support both `inline_recipe` and `git_ref`, but that is not the preferred steady state.

## Risks

1. Pulling remote git content in request time introduces network and git-command failure paths.
2. Ref resolution latency may increase validate and execute request time.
3. Symbolic refs such as branches can drift between requests unless the resolved commit is captured immediately.
4. Reusing only the visible format from the skill-ref system without matching its validation rules would create inconsistent behavior.

## Non-Goals

This change should not:

1. modify the server-managed recipe service contract,
2. redesign recipe execution behavior,
3. introduce recipe caching unless needed for performance later,
4. widen the submission model beyond the recipe testing API without a separate decision.

## Implementation Decision To Carry Forward

The implementation should prefer a direct contract swap:

1. keep `server_ref`,
2. replace `inline_recipe` with `git_ref`,
3. use `recipe_ref` with the Codex-skill-style git-go format,
4. resolve to a concrete commit and preserve that resolved identity in traces and hashes.
