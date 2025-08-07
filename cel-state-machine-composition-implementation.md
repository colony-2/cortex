# State Machine Composition Implementation Plan

## Overview
Implement workflow composition within state machine states, allowing arbitrary nesting of sequential, parallel, and conditional execution patterns.

## Implementation Tasks

### Phase 1: Core Types and Structures

#### 1.1 Update Type Definitions
```go
// server/activity/pkg/statemachine/types.go

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

### Phase 2: Workflow Execution Engine

#### 2.1 Create Workflow Executor
```go
// server/activity/pkg/statemachine/workflow_executor.go

type WorkflowExecutor struct {
    activity *StateMachineActivity
    celEnv   *cel.Env
}

func NewWorkflowExecutor(activity *StateMachineActivity) *WorkflowExecutor {
    return &WorkflowExecutor{
        activity: activity,
        celEnv:   activity.celEnv,
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

### Phase 4: Update Main Activity

#### 4.1 Modify executeState Method
```go
// server/activity/pkg/statemachine/activity.go

func (s *StateMachineActivity) executeState(ctx context.Context, 
    state StateDefinition, 
    stateCtx *StateContext) (map[string]interface{}, error) {
    
    // Terminal states
    if state.Terminal {
        return state.Outputs, nil
    }
    
    // NEW: Check for workflow composition
    if state.Workflow != nil {
        executor := NewWorkflowExecutor(s)
        return executor.Execute(ctx, state.Workflow, stateCtx)
    }
    
    // Backward compatibility: single activity/recipe
    inputs := state.Inputs
    if inputs == nil {
        inputs = stateCtx.Inputs
    }
    
    if state.Activity != "" {
        return s.executor.ExecuteActivity(ctx, state.Activity, inputs)
    } else if state.Recipe != "" {
        return s.executor.ExecuteRecipe(ctx, state.Recipe, inputs)
    }
    
    return nil, fmt.Errorf("state must specify 'activity', 'recipe', or 'workflow'")
}
```

### Phase 5: Testing

#### 5.1 Unit Tests
```go
// server/activity/pkg/statemachine/workflow_executor_test.go

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

### Week 1: Core Types and Basic Sequential
- Update type definitions
- Implement sequential workflow execution
- Basic template evaluation

### Week 2: Parallel and Conditional
- Implement parallel execution with dependency resolution
- Add conditional workflow support
- Enhanced CEL integration

### Week 3: Advanced Features
- Nested state machine support
- Retry logic for steps
- Performance optimizations

### Week 4: Testing and Documentation
- Comprehensive test coverage
- Performance benchmarks
- Documentation and examples

## Success Criteria

1. **Backward Compatibility**: All existing state machines work unchanged
2. **Performance**: Parallel execution shows measurable improvement
3. **Test Coverage**: >90% coverage for new code
4. **Documentation**: Complete examples and migration guide
5. **Integration**: Works with existing recipe system without modifications

## Risk Mitigation

1. **Complexity**: Keep simple cases simple, compose only when needed
2. **Debugging**: Enhanced logging and tracing for composed workflows
3. **Performance**: Careful resource management for parallel execution
4. **Migration**: Automated tools to assist migration