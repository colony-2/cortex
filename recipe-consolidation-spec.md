# Recipe Test Consolidation Specification

## Overview
Consolidate all recipe tests into `server/recipe-worker/test-fixtures/` using a single test harness that executes recipes with WorkflowTestSuite and validates results using Go table-driven tests.

## Goals
1. Single location for all recipe test fixtures
2. One test harness using WorkflowTestSuite for recipe execution
3. Remove duplicate recipe tests from other services
4. Use Go table-driven tests for all test cases
5. No mocks - run real activities

## Current Recipe Locations to Consolidate
```
./example-error-handling-recipe.yaml
./test-recipe.yaml
./server/cortex/test-recipe.yaml
./server/cortex/test-fixtures/*.yaml
./server/recipe-worker/examples/*.yaml
./server/recipe-worker/examples/research_project/*.yaml
./server/recipe-core/examples/*.yaml
./server/ops/examples/recipe-invocation/*.yaml
./server/nucleus/cmd/nucleus/testdata/recipes/*.yaml
```

## New Structure
```
server/recipe-worker/test-fixtures/
├── recipes/
│   ├── error-handling.yaml
│   ├── error-handling.test.yaml
│   ├── simple-echo.yaml
│   ├── simple-echo.test.yaml
│   ├── parallel-processing.yaml
│   ├── parallel-processing.test.yaml
│   └── ... (all other recipes with their test files)
└── recipe_test.go  # Single test harness
```

## Test File Format (`*.test.yaml`)
```yaml
# simple-echo.test.yaml
tests:
  - name: "basic_echo"
    inputs:
      text: "Hello World"
    want:
      result: "Hello World"
    wantErr: false
  
  - name: "empty_string"
    inputs:
      text: ""
    want:
      result: ""
    wantErr: false
  
  - name: "missing_input"
    inputs: {}
    wantErr: true
    wantErrContains: "text is required"
```

## Test Harness Implementation
```go
// recipe_test.go
package testfixtures

import (
    "testing"
    "path/filepath"
    "os"
    "gopkg.in/yaml.v3"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "go.temporal.io/sdk/testsuite"
    "github.com/vibethis/recipe-worker/pkg/worker"
    "github.com/vibethis/recipe-worker/pkg/compiler"
)

type TestCase struct {
    Name            string                 `yaml:"name"`
    Inputs          map[string]interface{} `yaml:"inputs"`
    Want            map[string]interface{} `yaml:"want,omitempty"`
    WantErr         bool                   `yaml:"wantErr"`
    WantErrContains string                 `yaml:"wantErrContains,omitempty"`
}

type TestSuite struct {
    Tests []TestCase `yaml:"tests"`
}

func TestAllRecipes(t *testing.T) {
    // Find all .test.yaml files
    testFiles, err := filepath.Glob("recipes/*.test.yaml")
    require.NoError(t, err)

    for _, testFile := range testFiles {
        recipeName := strings.TrimSuffix(filepath.Base(testFile), ".test.yaml")
        recipePath := filepath.Join("recipes", recipeName+".yaml")

        t.Run(recipeName, func(t *testing.T) {
            // Load test cases
            testData, err := os.ReadFile(testFile)
            require.NoError(t, err)

            var suite TestSuite
            err = yaml.Unmarshal(testData, &suite)
            require.NoError(t, err)

            // Load and compile recipe
            recipeData, err := os.ReadFile(recipePath)
            require.NoError(t, err)
            
            recipe, err := compiler.CompileRecipe(recipeData)
            require.NoError(t, err)

            // Run table-driven tests
            for _, tc := range suite.Tests {
                tc := tc // capture range variable
                t.Run(tc.Name, func(t *testing.T) {
                    // Use WorkflowTestSuite for execution
                    testSuite := &testsuite.WorkflowTestSuite{}
                    env := testSuite.NewTestWorkflowEnvironment()
                    
                    // Register activities from recipe-worker
                    worker.RegisterActivities(env)
                    
                    // Execute workflow
                    env.ExecuteWorkflow(recipe.Workflow, tc.Inputs)
                    
                    // Check results
                    err := env.GetWorkflowError()
                    if tc.WantErr {
                        require.Error(t, err)
                        if tc.WantErrContains != "" {
                            assert.Contains(t, err.Error(), tc.WantErrContains)
                        }
                        return
                    }
                    
                    require.NoError(t, err)
                    
                    // Get outputs
                    var result map[string]interface{}
                    err = env.GetWorkflowResult(&result)
                    require.NoError(t, err)
                    
                    if tc.Want != nil {
                        assert.Equal(t, tc.Want, result)
                    }
                })
            }
        })
    }
}
```

## Migration Steps

### Phase 1: Setup
1. Create `server/recipe-worker/test-fixtures/recipes/` directory
2. Implement test harness in `recipe_test.go`

### Phase 2: Migrate Recipes
For each recipe:
1. Copy to `test-fixtures/recipes/<name>.yaml`
2. Create `<name>.test.yaml` with test cases
3. Verify tests pass with new harness

### Phase 3: Remove Duplicates
1. Remove recipe tests from `server/cortex/` that use same recipes
2. Remove recipe tests from `server/nucleus/` that use same recipes
3. Remove other duplicate recipe test locations
4. Keep only the centralized tests in recipe-worker

## Implementation Checklist
- [ ] Create test-fixtures/recipes directory
- [ ] Implement recipe_test.go harness using WorkflowTestSuite
- [ ] Migrate all recipes to new location
- [ ] Create .test.yaml file for each recipe
- [ ] Remove duplicate tests from cortex
- [ ] Remove duplicate tests from nucleus
- [ ] Remove old example directories
- [ ] Verify all tests pass with real activities

## Success Criteria
1. All recipes in `server/recipe-worker/test-fixtures/recipes/`
2. Single test harness using WorkflowTestSuite
3. Every recipe has a `.test.yaml` file
4. No duplicate recipe tests in other services
5. All tests run with real activities (no mocks)
6. Tests use Go table-driven pattern