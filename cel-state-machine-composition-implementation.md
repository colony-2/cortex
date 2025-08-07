# State Machine Core Integration and Composition Implementation Plan

## Overview

**IMPORTANT: This implementation will completely replace the current state machine activity (`server/activity/pkg/statemachine`) by integrating state machines as a core execution type within the workflow engine.**

State machines will become a first-class citizen alongside activities and recipes, allowing for:
- Direct state machine execution without wrapper activities
- Workflow composition within state machine states
- Arbitrary nesting of sequential, parallel, and conditional execution patterns
- Improved performance by eliminating activity overhead
- Unified execution model across all workflow types

## Current State Machine Activity Removal

The existing `StateMachineActivity` implementation will be completely removed:
- **Location to Delete**: `server/activity/pkg/statemachine/` (entire directory)
- **Related Files**: Any references to state machine as an activity type
- **Timeline**: Immediate removal upon core implementation completion

## Major Codebase Changes Required

### 1. Core Execution Engine Modifications

#### 1.1 Update Main Executor (`server/pkg/executor/`)
- Add `ExecuteStateMachine` method alongside `ExecuteActivity` and `ExecuteRecipe`
- Update execution dispatcher to recognize state machine type
- Remove dependency on state machine activity package

#### 1.2 Workflow Definition Changes (`server/pkg/workflow/`)
- Add `StateMachine` field to workflow step definition
- Update workflow parser to handle state machine definitions inline
- Modify validation logic to support state machine as execution type

#### 1.3 Registry Updates (`server/pkg/registry/`)
- Create new `StateMachineRegistry` for managing state machine definitions
- Update service registry to include state machine registry
- Remove state machine from activity registry

### 2. Database Schema Changes

#### 2.1 Execution Records (`server/pkg/storage/`)
- Add `state_machine_executions` table for tracking state machine runs
- Update `workflow_executions` to reference state machines directly
- Remove state machine references from `activity_executions`

#### 2.2 Definition Storage
- Create `state_machine_definitions` table
- Add versioning support for state machine definitions
- Migrate existing state machine definitions from activity format

### 3. API Layer Updates (`server/api/`)

#### 3.1 GraphQL Schema
```graphql
type WorkflowStep {
  id: ID!
  type: StepType!  # ACTIVITY | RECIPE | STATE_MACHINE
  activity: String
  recipe: String
  stateMachine: String  # NEW
  inputs: JSON
}

type StateMachineDefinition {
  id: ID!
  name: String!
  version: String!
  states: [StateDefinition!]!
  initialState: String!
}
```

#### 3.2 REST API Endpoints
- Add `/api/v1/state-machines` for CRUD operations
- Update workflow execution endpoints to handle state machine steps
- Remove state machine endpoints from activity namespace

### 4. File Deletions

The following files/directories will be deleted entirely:
- `server/activity/pkg/statemachine/` - entire directory
- `server/activity/examples/statemachine/` - example implementations
- Any test files specifically for state machine activity

### 5. Migration Strategy

#### 5.1 Data Migration Script
```sql
-- Migrate state machine definitions from activities to core
INSERT INTO state_machine_definitions (id, name, version, definition)
SELECT 
  activity_id,
  activity_name,
  '1.0.0',
  configuration
FROM activities
WHERE type = 'STATE_MACHINE';

-- Update workflow references
UPDATE workflow_steps
SET 
  type = 'STATE_MACHINE',
  state_machine = activity,
  activity = NULL
WHERE activity IN (SELECT name FROM activities WHERE type = 'STATE_MACHINE');
```

#### 5.2 Code Migration Steps
1. Implement core state machine executor
2. Update all imports from `server/activity/pkg/statemachine` to new location
3. Run migration script to move definitions
4. Delete old state machine activity code
5. Update all tests to use new API

## Breaking Changes and Impact Analysis

### Breaking Changes
1. **Complete Removal of State Machine Activity**
   - All workflows using `type: "STATE_MACHINE"` activities must be updated
   - State machine definitions must be migrated to new format
   - Activity-based state machine APIs will no longer exist

2. **API Changes**
   - `/api/v1/activities/state-machine/*` endpoints removed
   - New `/api/v1/state-machines/*` endpoints required
   - GraphQL schema changes for workflow steps

3. **Configuration Format Changes**
   - State machines no longer wrapped in activity configuration
   - Direct state machine definitions in workflows
   - New registry configuration required

### Impact on Existing Systems

#### Client Applications
- Web UI must update to use new state machine APIs
- CLI tools need new commands for state machine management
- SDK updates required for all language bindings

#### Running Workflows
- In-flight workflows using state machine activities will fail
- Requires planned downtime or blue-green deployment
- Historical execution data needs migration

#### Integration Points
- Monitoring/metrics collection needs updates
- Logging format changes for state machine executions
- Webhook notifications may need schema updates

## Implementation Tasks

### Phase 1: Core Types and Structures

#### 1.1 Create New Core Types
```go
// server/pkg/statemachine/types.go (NEW LOCATION)

// Add WorkflowDefinition type
type WorkflowDefinition struct {
    Type    WorkflowType           `json:"type"`
    Steps   []WorkflowStep         `json:"steps"`
    Timeout string                 `json:"timeout,omitempty"`
}

type WorkflowType string

const (
    WorkflowTypeSequential WorkflowType = "sequential"
    WorkflowTypeParallel   WorkflowType = "parallel"
    WorkflowTypeConditional WorkflowType = "conditional"
)

// Add WorkflowStep type
type WorkflowStep struct {
    ID           string                 `json:"id"`
    Activity     string                 `json:"activity,omitempty"`
    Recipe       string                 `json:"recipe,omitempty"`
    StateMachine string                 `json:"state_machine,omitempty"`
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
    DependsOn    []string               `json:"depends_on,omitempty"`
    When         string                 `json:"when,omitempty"`
    Retry        *StepRetryPolicy       `json:"retry,omitempty"`
}

type StepRetryPolicy struct {
    MaxAttempts        int     `json:"max_attempts"`
    InitialInterval    string  `json:"initial_interval,omitempty"`
    BackoffCoefficient float64 `json:"backoff_coefficient,omitempty"`
}

// Update StateDefinition to include Workflow field
type StateDefinition struct {
    Activity    string                 `json:"activity,omitempty"`
    Recipe      string                 `json:"recipe,omitempty"`
    Workflow    *WorkflowDefinition    `json:"workflow,omitempty"` // NEW
    Terminal    bool                   `json:"terminal,omitempty"`
    Error       string                 `json:"error,omitempty"`
    Inputs      map[string]interface{} `json:"inputs,omitempty"`
    Outputs     map[string]interface{} `json:"outputs,omitempty"`
    Retry       *StateRetryPolicy      `json:"retry,omitempty"`
    Transitions []TransitionSpec       `json:"transitions,omitempty"`
}
```

### Phase 2: Core State Machine Executor

#### 2.1 Create Core State Machine Executor
```go
// server/pkg/statemachine/executor.go (NEW CORE LOCATION)

type StateMachineExecutor struct {
    registry     *Registry
    celEnv       *cel.Env
    executor     *CoreExecutor  // For executing activities and recipes
    storage      Storage
}

func NewStateMachineExecutor(registry *Registry, executor *CoreExecutor, storage Storage) *StateMachineExecutor {
    return &StateMachineExecutor{
        registry: registry,
        executor: executor,
        storage:  storage,
        celEnv:   setupCELEnvironment(),
    }
}

// Main execution dispatcher
func (w *WorkflowExecutor) Execute(ctx context.Context, 
    workflow *WorkflowDefinition, 
    stateCtx *StateContext) (map[string]interface{}, error) {
    
    switch workflow.Type {
    case WorkflowTypeSequential:
        return w.executeSequential(ctx, workflow, stateCtx)
    case WorkflowTypeParallel:
        return w.executeParallel(ctx, workflow, stateCtx)
    case WorkflowTypeConditional:
        return w.executeConditional(ctx, workflow, stateCtx)
    default:
        return nil, fmt.Errorf("unsupported workflow type: %s", workflow.Type)
    }
}
```

#### 2.2 Sequential Execution
```go
func (w *WorkflowExecutor) executeSequential(ctx context.Context,
    workflow *WorkflowDefinition,
    stateCtx *StateContext) (map[string]interface{}, error) {
    
    stepOutputs := make(map[string]interface{})
    
    for _, step := range workflow.Steps {
        // Skip if dependencies not met
        if !w.areDependenciesMet(step, stepOutputs) {
            continue
        }
        
        // Evaluate conditional execution
        if step.When != "" {
            shouldExecute, err := w.evaluateStepCondition(step.When, stepOutputs, stateCtx)
            if err != nil {
                return nil, fmt.Errorf("failed to evaluate condition for step %s: %w", step.ID, err)
            }
            if !shouldExecute {
                continue
            }
        }
        
        // Execute step with retry logic
        output, err := w.executeStepWithRetry(ctx, step, stepOutputs, stateCtx)
        if err != nil {
            return nil, fmt.Errorf("step %s failed: %w", step.ID, err)
        }
        
        stepOutputs[step.ID] = output
    }
    
    return stepOutputs, nil
}
```

#### 2.3 Parallel Execution
```go
func (w *WorkflowExecutor) executeParallel(ctx context.Context,
    workflow *WorkflowDefinition,
    stateCtx *StateContext) (map[string]interface{}, error) {
    
    // Build dependency graph
    groups := w.buildDependencyGroups(workflow.Steps)
    stepOutputs := sync.Map{}
    
    for _, group := range groups {
        var wg sync.WaitGroup
        errChan := make(chan error, len(group))
        
        for _, step := range group {
            wg.Add(1)
            go func(s WorkflowStep) {
                defer wg.Done()
                
                // Check condition
                if s.When != "" {
                    // Convert sync.Map to regular map for evaluation
                    outputs := w.syncMapToMap(&stepOutputs)
                    shouldExecute, err := w.evaluateStepCondition(s.When, outputs, stateCtx)
                    if err != nil || !shouldExecute {
                        if err != nil {
                            errChan <- err
                        }
                        return
                    }
                }
                
                outputs := w.syncMapToMap(&stepOutputs)
                output, err := w.executeStep(ctx, s, outputs, stateCtx)
                if err != nil {
                    errChan <- fmt.Errorf("step %s failed: %w", s.ID, err)
                    return
                }
                
                stepOutputs.Store(s.ID, output)
            }(step)
        }
        
        wg.Wait()
        close(errChan)
        
        // Check for errors
        for err := range errChan {
            if err != nil {
                return nil, err
            }
        }
    }
    
    return w.syncMapToMap(&stepOutputs), nil
}
```

#### 2.4 Step Execution
```go
func (w *WorkflowExecutor) executeStep(ctx context.Context,
    step WorkflowStep,
    stepOutputs map[string]interface{},
    stateCtx *StateContext) (map[string]interface{}, error) {
    
    // Prepare inputs with template evaluation
    inputs := w.prepareStepInputs(step.Inputs, stepOutputs, stateCtx)
    
    // Execute based on step type
    if step.Activity != "" {
        return w.activity.executor.ExecuteActivity(ctx, step.Activity, inputs)
    } else if step.Recipe != "" {
        return w.activity.executor.ExecuteRecipe(ctx, step.Recipe, inputs)
    } else if step.StateMachine != "" {
        // Execute nested state machine
        return w.executeNestedStateMachine(ctx, step.StateMachine, inputs)
    }
    
    return nil, fmt.Errorf("step %s must specify activity, recipe, or state_machine", step.ID)
}
```

### Phase 3: Template and CEL Integration

#### 3.1 Enhanced Variable Context
```go
type StepExecutionContext struct {
    Inputs  map[string]interface{} // State inputs
    Steps   map[string]interface{} // Step outputs within current workflow
    States  map[string]interface{} // Previous state outputs
    Context *RecipeContext         // Recipe context
}

func (w *WorkflowExecutor) buildStepContext(
    stepOutputs map[string]interface{},
    stateCtx *StateContext) *StepExecutionContext {
    
    return &StepExecutionContext{
        Inputs:  stateCtx.Inputs,
        Steps:   stepOutputs,
        States:  stateCtx.StateOutputs,
        Context: stateCtx.RecipeContext,
    }
}
```

#### 3.2 Template Evaluation for Inputs
```go
func (w *WorkflowExecutor) prepareStepInputs(
    inputTemplates map[string]interface{},
    stepOutputs map[string]interface{},
    stateCtx *StateContext) map[string]interface{} {
    
    if inputTemplates == nil {
        return stateCtx.Inputs
    }
    
    ctx := w.buildStepContext(stepOutputs, stateCtx)
    evaluated := make(map[string]interface{})
    
    for key, value := range inputTemplates {
        switch v := value.(type) {
        case string:
            // Check if it's a template
            if strings.Contains(v, "{{") && strings.Contains(v, "}}") {
                evaluated[key] = w.evaluateTemplate(v, ctx)
            } else {
                evaluated[key] = v
            }
        case map[string]interface{}:
            // Recursively evaluate nested maps
            evaluated[key] = w.prepareStepInputs(v, stepOutputs, stateCtx)
        default:
            evaluated[key] = v
        }
    }
    
    return evaluated
}
```

### Phase 4: Integration with Core Executor

#### 4.1 Update Main Core Executor
```go
// server/pkg/executor/executor.go

type CoreExecutor struct {
    activityRegistry     *ActivityRegistry
    recipeRegistry       *RecipeRegistry
    stateMachineRegistry *StateMachineRegistry  // NEW
    stateMachineExecutor *StateMachineExecutor   // NEW
}

// NEW: Add ExecuteStateMachine method
func (e *CoreExecutor) ExecuteStateMachine(ctx context.Context, 
    name string, 
    inputs map[string]interface{}) (map[string]interface{}, error) {
    
    definition, err := e.stateMachineRegistry.Get(name)
    if err != nil {
        return nil, fmt.Errorf("state machine %s not found: %w", name, err)
    }
    
    return e.stateMachineExecutor.Execute(ctx, definition, inputs)
}

// Update workflow step execution
func (e *CoreExecutor) ExecuteWorkflowStep(ctx context.Context, step WorkflowStep) (map[string]interface{}, error) {
    switch step.Type {
    case StepTypeActivity:
        return e.ExecuteActivity(ctx, step.Activity, step.Inputs)
    case StepTypeRecipe:
        return e.ExecuteRecipe(ctx, step.Recipe, step.Inputs)
    case StepTypeStateMachine:  // NEW
        return e.ExecuteStateMachine(ctx, step.StateMachine, step.Inputs)
    default:
        return nil, fmt.Errorf("unknown step type: %s", step.Type)
    }
}
```

### Phase 5: Testing

#### 5.1 Unit Tests
```go
// server/pkg/statemachine/executor_test.go (NEW LOCATION)

func TestSequentialWorkflowExecution(t *testing.T) {
    // Test sequential step execution
    // Test step dependencies
    // Test conditional steps
}

func TestParallelWorkflowExecution(t *testing.T) {
    // Test parallel execution
    // Test dependency groups
    // Test error handling in parallel steps
}

func TestNestedStateMachines(t *testing.T) {
    // Test state machine within workflow step
    // Test context propagation
    // Test output aggregation
}
```

#### 5.2 Integration Tests
```go
func TestComposedStateWorkflow(t *testing.T) {
    // Full end-to-end test with Temporal
    // Complex workflow with mixed execution types
    // Performance validation for parallel execution
}
```

### Phase 6: Documentation and Examples

#### 6.1 Migration Guide
- How to convert multi-state sequences to composed states
- Performance optimization strategies
- Best practices for step granularity

#### 6.2 Example Recipes
- Data processing pipeline with parallel batching
- Document processing with conditional enrichment
- Nested approval workflows

## Implementation Timeline

### Phase 1: Core Infrastructure (Week 1-2)
- Create new `server/pkg/statemachine/` package structure
- Implement core state machine types and executor
- Set up state machine registry
- Integrate with core executor

### Phase 2: Migration and Removal (Week 2-3)
- Implement data migration scripts
- Update all workflow references
- Delete `server/activity/pkg/statemachine/` directory
- Remove all state machine activity references

### Phase 3: Composition Features (Week 3-4)
- Implement sequential workflow execution
- Add parallel execution with dependency resolution
- Implement conditional workflow support
- Add nested state machine support

### Phase 4: API and Integration (Week 4-5)
- Update GraphQL schema and resolvers
- Implement REST API endpoints
- Update Web UI to use new APIs
- Update CLI commands

### Phase 5: Testing and Documentation (Week 5-6)
- Comprehensive test coverage for new core implementation
- Update all integration tests
- Performance benchmarks
- Documentation and migration guide

## Success Criteria

1. **Complete Removal**: All state machine activity code deleted
2. **Core Integration**: State machines work as first-class execution type
3. **Performance**: Improved execution speed without activity overhead
4. **Test Coverage**: >90% coverage for new core implementation
5. **Zero Downtime**: Migration strategy allows seamless transition
6. **API Parity**: All functionality available through new APIs

## Risk Mitigation

1. **Data Loss**: Comprehensive backup before migration
2. **Service Disruption**: Blue-green deployment strategy
3. **Integration Issues**: Extensive integration testing phase
4. **Performance Regression**: Benchmark comparisons before/after
5. **Missing Functionality**: Feature parity checklist and validation

## Rollback Strategy

If critical issues are discovered post-deployment:
1. Revert code changes to restore state machine activity
2. Restore database from pre-migration backup
3. Re-deploy previous version with activity-based state machines
4. Document issues for resolution before retry