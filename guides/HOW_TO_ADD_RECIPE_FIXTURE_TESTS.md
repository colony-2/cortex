# How to add recipe fixture tests

Use the fixture harness under `test-fixtures` to run YAML recipe definitions with table-driven test cases. The harness automatically picks up `.test.yaml` files and looks for a matching recipe file by name. Recommended pattern: write fixture tests in your own project and import the harness instead of adding tests directly here, unless you are specifically validating recipe-worker compiler/core behavior.

## 1) Add a recipe definition

Create `test-fixtures/recipes/<recipe-name>.yaml`.

Guidelines:
- Keep the recipe `id` aligned with the filename (`<recipe-name>`) to avoid confusion.
- Use real ops and templates the same way production recipes do.

Example:
```yaml
id: basic-test
desc: Basic test recipe
version: "1.0"
input_schema:
  message:
    type: string
    required: true
inputs:
  message: "{{ inputs.message }}"
sequence:
  - id: cmd
    op: command_execution
    inputs:
      run: "{{ \"echo '\" + inputs.message + \"'\" }}"
outputs:
  result: "{{ sequence.cmd.outputs.stdout }}"
```

## 2) Add a fixture test file

Create `test-fixtures/recipes/<recipe-name>.test.yaml`. The harness derives the recipe path from the test filename, so the base names must match.

Test case schema:
```yaml
recipes:
  - child-simple.yaml
  - child-artifact.yaml

tests:
  - name: case_name
    description: Optional description
    inputs: {}
    jobContext: {}           # optional overrides to the default job context
    gitContext: {}           # optional overrides for parent/persist hash or ref
    want: {}
    wantErr: false
    wantErrContains: Optional substring
    wantArtifacts:
      - optional-artifact-name
    wantJobArtifacts:
      - optional-job-artifact-name
```

Notes:
- `recipes` is optional; list additional recipe YAML files (relative to `recipes/`) when the main recipe invokes other recipes (e.g., via `recipe-child` ops).
- `want` is compared after pruning `context` and `git_persist_hash`. The framework also prunes actual output down to keys present in `want` and ignores zero-value extras.
- Numeric comparisons are type-flexible (`1` equals `1.0`).
- Set `wantErr: true` to assert errors; use `wantErrContains` for substring matching.
- Set `wantArtifacts` to assert artifact names; this switches execution to the toy engine so artifacts can be captured.
- Set `wantJobArtifacts` to assert artifact names on the final job result (also uses the toy engine).
- `jobContext` lets you override any subset of the default job context (ticket ID, actor, cell path/name, job ID, project ID, environment paths, git base fields, ticket metadata). Only fields you set are replaced; everything else keeps the defaults.
- `gitContext` lets you override `parent_ref`, `parent_hash`, or `hash` (persist hash) used for the run.

Example:
```yaml
tests:
  - name: simple_passthrough
    inputs:
      message: Hello Test
    want:
      result: "Hello Test"
    wantErr: false
```

## 3) Run the tests

From `/src/server/recipe-worker`:
```bash
go test ./test-fixtures
```

Optional filtering:
```bash
go test ./test-fixtures -run TestAllRecipes
```

## Useful context from the harness

The harness builds a temporary git repo with a few seeded cells (for recipes that need git or cell context). The job context uses:
- `CellName: cells/test-cell`
- `CellPath: cells/test-cell`
- `TicketID: TEST-TICKET`
- `ActorName: test-actor`
- `ActorEmail: test-actor@colony2`
- `JobID: test-job-id`

If your recipe relies on git state or repo paths, leverage these defaults.

## Recommended: use the framework in your own project

If you have a project-specific recipe suite, prefer running fixture tests from your own module/package and import this harness.

Example test file in your project:
```go
package recipetests

import (
	"testing"

	testfixtures "github.com/colony-2/colony2/server/recipe-worker/test-fixtures"
)

func TestProjectRecipes(t *testing.T) {
	testfixtures.RunTestOnAllRecipes("test-fixtures/recipes/*.test.yaml", t)
}
```

Layout recommendation in your project:
```
your-project/
  test-fixtures/
    recipes/
      my-recipe.yaml
      my-recipe.test.yaml
  recipe_fixtures_test.go
```

This keeps recipe tests close to the project that owns them while reusing the same execution harness.

## Appendix: tests that belong in recipe-worker

Only add new fixture tests under `/src/server/recipe-worker/test-fixtures/recipes` when they specifically validate recipe-worker compiler or core workflow behavior (parsing, templating, execution semantics, artifact capture, etc.). For feature tests tied to your project or recipe definitions, place them in your project and import the harness instead.
