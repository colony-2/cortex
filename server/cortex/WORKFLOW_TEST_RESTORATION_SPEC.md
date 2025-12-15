# Workflow Test Suite Restoration Specification

## Overview
This specification outlines the restoration of Temporal Workflow Test Suite tests that were disabled during the legacy cleanup. These tests can run without a Temporal server using `testsuite.WorkflowTestSuite`.

## Current State

### Disabled Workflow Tests
From analysis of disabled test files:

1. **workflow_test.go.disabled** (3 functions)
   - TestNestedCompositionWorkflow
   - TestStateTransitionsWorkflow  
   - TestRetryPolicyWorkflow

2. **integration_test.go.disabled** (3 functions)
   - TestComplexDocumentProcessingIntegration
   - TestWithTemporalDevServer
   - TestErrorHandlingIntegration

### Existing Test Pattern
The codebase already has a working pattern in `worker_integration_test.go`:
```go
type WorkerIntegrationTestSuite struct {
    suite.Suite
    testsuite.WorkflowTestSuite
    env *testsuite.TestWorkflowEnvironment
}
```

## Proposed Test Structure

### Location
Create new test file: `/Users/jnadeau/src/colony2/server/cortex/internal/shared/workflow_test.go`

### Test Suite Structure

```go
package shared

import (
    "context"
    "testing"
    "github.com/stretchr/testify/suite"
    "go.temporal.io/sdk/testsuite"
    "go.temporal.io/sdk/workflow"
)

type SharedWorkflowTestSuite struct {
    suite.Suite
    testsuite.WorkflowTestSuite
    env *testsuite.TestWorkflowEnvironment
}
```

## Test Cases to Implement

### 1. Nested Composition Workflow Tests

#### TestNestedSequentialParallelComposition
**Purpose**: Test deeply nested sequential and parallel compositions
**Coverage**:
- Sequential > Parallel > Sequential nesting
- Data flow between nested levels
- Output template resolution across nesting
- Scope isolation in nested contexts

**Test Scenarios**:
```yaml
sequence:
  - id: prepare
    op: prepare_data
  - id: process_parallel
    parallel:
      - id: branch1
        sequence:
          - id: transform1
            op: transform
          - id: validate1
            op: validate
      - id: branch2
        op: direct_process
  - id: aggregate
    op: aggregate_results
```

#### TestDeeplyNestedStateMachines
**Purpose**: Test state machines within compositions
**Coverage**:
- State machine inside sequence
- State machine inside parallel branches
- Transitions based on nested outputs
- Retry policies in nested contexts

### 2. State Transition Workflow Tests

#### TestComplexStateTransitions
**Purpose**: Test complex state transition logic with CEL expressions
**Coverage**:
- CEL-based transition conditions
- Multiple transition paths
- Default transitions
- Terminal state detection
- Transition with nested outputs

**Test Scenarios**:
```yaml
states:
  initial: validation
  states:
    validation:
      op: validate_document
      transitions:
        - to: approved
          when: "score >= 80 && confidence > 0.9"
        - to: review
          when: "score >= 60"
        - to: rejected
```

#### TestStateMachineWithRetries
**Purpose**: Test retry logic within state machines
**Coverage**:
- Retry policy execution
- Exponential backoff calculation
- Max attempts enforcement
- Retry with state transitions
- Error propagation after retries

### 3. Integration Workflow Tests

#### TestDocumentProcessingPipeline
**Purpose**: End-to-end document processing workflow
**Coverage**:
- Complete workflow from input to output
- Multiple activity executions
- Error handling and recovery
- Conditional branches
- Final output aggregation

**Test Flow**:
1. Document ingestion
2. Parallel processing (OCR, metadata extraction)
3. State machine for review process
4. Conditional approval flow
5. Final document storage

#### TestErrorHandlingAndRecovery
**Purpose**: Test error handling at various levels
**Coverage**:
- Activity-level errors
- State transition errors
- Parallel branch failures
- Retry exhaustion
- Compensation logic

#### TestTimeoutHandling
**Purpose**: Test timeout scenarios
**Coverage**:
- Activity timeouts
- Workflow timeouts
- State machine timeout transitions
- Parallel branch timeout handling

### 4. Template and CEL Integration Tests

#### TestTemplateResolutionInWorkflow
**Purpose**: Test template resolution during workflow execution
**Coverage**:
- Input template resolution
- Output template resolution
- CEL expressions in templates
- Cross-node references
- Nested scope references

#### TestCELEvaluationInTransitions
**Purpose**: Test CEL evaluation for state transitions
**Coverage**:
- Complex CEL expressions
- Type conversions
- Error handling in CEL
- Performance under load

## Implementation Plan

### Phase 1: Core Test Suite Setup
1. Create `workflow_test.go` in shared package
2. Implement base test suite structure
3. Set up mock activity executor
4. Create helper functions for test workflows

### Phase 2: Composition Tests
1. Implement nested composition tests
2. Add sequential/parallel combination tests
3. Test data flow between nested levels
4. Verify scope isolation

### Phase 3: State Machine Tests
1. Implement state transition tests
2. Add retry policy tests
3. Test complex CEL conditions
4. Verify terminal state handling

### Phase 4: Integration Tests
1. Create end-to-end workflow tests
2. Add error handling scenarios
3. Implement timeout tests
4. Test compensation logic

### Phase 5: Template/CEL Tests
1. Test template resolution in workflows
2. Verify CEL evaluation in transitions
3. Test performance scenarios
4. Add edge case coverage

## Mock Activity Implementations

### Required Mock Activities
```go
// Mock activities for testing
var mockActivities = map[string]func(context.Context, map[string]interface{}) (map[string]interface{}, error){
    "validate_document": validateDocumentActivity,
    "process_data": processDataActivity,
    "transform": transformActivity,
    "aggregate_results": aggregateResultsActivity,
    "retry_activity": retryableActivity,
}
```

## Success Criteria

1. **Coverage Restoration**: All 6 disabled workflow test functions replaced with equivalent or better coverage
2. **No Server Dependency**: All tests run using WorkflowTestSuite without Temporal server
3. **Comprehensive Scenarios**: Each test covers multiple edge cases and error conditions
4. **Performance**: Tests complete within reasonable time (< 5 seconds per test)
5. **Maintainability**: Clear test structure with reusable components

## Benefits

1. **Immediate Testing**: No need for Temporal server infrastructure
2. **Fast Execution**: Tests run in-memory with time skipping
3. **Deterministic**: Reproducible test results
4. **Complete Coverage**: All workflow patterns tested
5. **CI/CD Ready**: Can run in any environment

## Example Test Implementation

```go
func (s *SharedWorkflowTestSuite) TestNestedCompositionWithStateTransitions() {
    // Setup
    s.env.RegisterActivity(mockActivities["validate_document"])
    s.env.RegisterActivity(mockActivities["process_data"])
    
    // Create workflow definition
    recipeDef := &yamlpkg.RecipeDefinition{
        Name: "nested-state-workflow",
        States: &yamlpkg.StateMap{
            Initial: "process",
            States: map[string]yamlpkg.State{
                "process": {
                    Sequential: []yamlpkg.Node{
                        {ID: "validate", Op: "validate_document"},
                        {ID: "transform", Op: "process_data"},
                    },
                    Transitions: []yamlpkg.Transition{
                        {To: "complete", When: "outputs.valid == true"},
                        {To: "error", When: "outputs.valid == false"},
                    },
                },
            },
        },
    }
    
    // Register workflow
    workflowFunc := CreateStateMachineWorkflow(recipeDef)
    s.env.RegisterWorkflow(workflowFunc)
    
    // Execute
    s.env.ExecuteWorkflow(workflowFunc, map[string]interface{}{
        "document": "test.pdf",
    })
    
    // Verify
    s.True(s.env.IsWorkflowCompleted())
    s.NoError(s.env.GetWorkflowError())
    
    var result map[string]interface{}
    s.NoError(s.env.GetWorkflowResult(&result))
    s.Equal("complete", result["final_state"])
}
```

## Timeline

- **Phase 1**: 2 hours - Setup and helpers
- **Phase 2**: 3 hours - Composition tests
- **Phase 3**: 3 hours - State machine tests
- **Phase 4**: 4 hours - Integration tests
- **Phase 5**: 2 hours - Template/CEL tests

**Total Estimated Time**: 14 hours

## Notes

- All tests should use the unified recipe format (sequence, parallel, states, op)
- Mock activities should simulate realistic behavior including delays and errors
- Tests should verify both happy path and error scenarios
- Use table-driven tests where appropriate for better coverage