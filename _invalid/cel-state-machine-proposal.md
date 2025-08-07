# Proposal: CEL Integration for State Machine Recipes

## Overview

This proposal outlines the integration of Google's Common Expression Language (CEL) into the recipe system to enable declarative, type-safe conditional logic for state transitions and branching behavior in recipes. State machines will become a core execution type in the recipe system, alongside sequential and parallel workflows.

## Motivation

Current limitations:
- Recipe workflows are limited to sequential and parallel execution types
- No standardized way to express retry loops or conditional branching
- No support for complex decision logic in recipes
- Need for a safe, sandboxed expression language for user-defined conditions

## Proposed Solution

### 1. CEL Integration

Use CEL as the standard expression language for:
- State transition conditions
- Retry loop conditions
- Activity input transformations
- Output validations
- Recipe-to-recipe invocation conditions

### 2. State Machine as Core Execution Type

State machines will be implemented as a first-class execution type in the core recipe system, enabling complex workflows with conditional branching and loops.

#### Workflow Definition
```yaml
workflow:
  type: state_machine
  config:
    initial_state: reviewing
    states:
          reviewing:
            activity: critique_activity
            inputs:
              document: "{{ .Inputs.document }}"
            transitions:
              - to: approved
                when: ".Outputs.score >= 80"
              - to: revising
                when: ".Outputs.score < 80 && .State.attempts < 3"
              - to: rejected
                when: ".State.attempts >= 3"
          revising:
            activity: improve_activity
            transitions:
              - to: reviewing
                when: ".Outputs.improved == true"
          approved:
            terminal: true
            outputs:
              result: "{{ .Outputs.final_result }}"
          rejected:
            terminal: true
            error: "Failed review after {{ .State.attempts }} attempts"
```

### 3. Key Features

#### Conditional State Transitions
- CEL expressions for complex transition logic
- Access to full execution context in transition conditions
- Support for retry loops with configurable backoff

#### State Types
- **Activity States**: Execute any registered activity
- **Recipe States**: Invoke other recipes (including other state machines)
- **Terminal States**: Success or error termination points
- **Nested State Machines**: States can invoke other state machine activities

#### Context Management
- Automatic state tracking and attempt counting
- State outputs preserved across transitions
- Access to inputs, outputs, and execution metadata in CEL expressions

#### Integration Benefits
- **No Core Changes**: Works within existing RegisterableActivity framework
- **Composable**: State machines can be reused across multiple recipes
- **Testable**: Each state machine is a standalone activity with defined inputs/outputs
- **Versioned**: State machine definitions can be versioned like any activity

### 4. CEL Context Variables

CEL expressions in transitions have access to the full execution context, using the same dot notation as Go templates:

```yaml
# Current activity/state outputs
.Outputs:
  <field>: any              # Fields from current activity output
  
# State-specific context
.State:
  name: string              # Current state name
  attempts: int             # Number of attempts in current state
  entered_at: timestamp     # When state was entered
  
# Previous states in state machine
.States:
  <state_name>:
    outputs: any            # Outputs from previous states that have executed
    
# Recipe inputs
.Inputs:
  <input_name>: any         # Original recipe inputs

# Recipe context (from RecipeContext)
.Context:
  Recipe:
    Name: string
    Version: string
    ExecutionID: string
  Environment:
    Name: string
    Region: string
  Execution:
    StartedAt: timestamp
    Timeout: duration
```

Note: CEL expressions use the same variable naming convention as Go templates for consistency.

### 5. Implementation

#### Architecture Overview

```go
// StateMachineActivity implements RegisterableActivity interface
type StateMachineActivity struct {
    executor activities.ExecutorImplementation
    celEnv   *cel.Env
}

// Implements: RegisterableActivity[StateMachineConfig, map[string]interface{}, map[string]interface{}]
func (s *StateMachineActivity) Execute(ctx context.Context, 
    config StateMachineConfig, 
    inputs map[string]interface{}) (map[string]interface{}, error) {
    
    // Initialize state context
    stateCtx := &StateContext{
        CurrentState: config.InitialState,
        Inputs:       inputs,
        StateOutputs: make(map[string]map[string]interface{}),
        Attempts:     make(map[string]int),
    }
    
    // Execute state machine
    for !isTerminal(stateCtx.CurrentState, config.States) {
        state := config.States[stateCtx.CurrentState]
        
        // Execute state activity
        outputs, err := s.executeStateActivity(ctx, state, stateCtx)
        if err != nil {
            return nil, err
        }
        
        // Store outputs
        stateCtx.StateOutputs[stateCtx.CurrentState] = outputs
        
        // Evaluate transitions
        nextState, err := s.evaluateTransitions(state.Transitions, outputs, stateCtx)
        if err != nil {
            return nil, err
        }
        
        stateCtx.CurrentState = nextState
    }
    
    // Return terminal state outputs
    return stateCtx.StateOutputs[stateCtx.CurrentState], nil
}
```

#### Type Definitions

```go
// StateMachineConfig defines the configuration for state machine activities
type StateMachineConfig struct {
    InitialState string                      `json:"initial_state"`
    States       map[string]StateDefinition  `json:"states"`
    Timeout      string                      `json:"timeout,omitempty"`
}

// StateDefinition defines a single state in the state machine
type StateDefinition struct {
    Activity     string                 `json:"activity,omitempty"`
    Recipe       string                 `json:"recipe,omitempty"`
    Terminal     bool                   `json:"terminal,omitempty"`
    Error        string                 `json:"error,omitempty"`
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
    Outputs      map[string]interface{} `json:"outputs,omitempty"`
    Retry        *StateRetryPolicy      `json:"retry,omitempty"`
    Transitions  []TransitionSpec       `json:"transitions,omitempty"`
}

// TransitionSpec defines state transitions with CEL conditions
type TransitionSpec struct {
    To   string `json:"to"`
    When string `json:"when"` // CEL expression
}

// StateRetryPolicy with CEL conditions
type StateRetryPolicy struct {
    When               string  `json:"when,omitempty"`
    MaxAttempts        int     `json:"max_attempts"`
    BackoffCoefficient float64 `json:"backoff_coefficient,omitempty"`
    InitialInterval    string  `json:"initial_interval,omitempty"`
}
```

### 6. Examples

#### Simple State Machine Activity
```yaml
activities:
  - name: document_review_flow
    implementation:
      type: state_machine
      config:
        initial_state: reviewing
        states:
          reviewing:
            activity: critique_activity
            transitions:
              - to: approved
                when: ".Outputs.score >= 80"
              - to: improving
                when: ".Outputs.score < 80 && .State.attempts < 3"
              - to: rejected
                when: ".State.attempts >= 3"
          improving:
            activity: improve_document
            transitions:
              - to: reviewing
                when: ".Outputs.improved == true"
          approved:
            terminal: true
          rejected:
            terminal: true
            error: "Review failed"
```

#### Using State Machine in Workflow
```yaml
workflow:
  type: sequential
  steps:
    - id: extract
      activity: extract_text
      inputs:
        path: "{{ .Inputs.document_path }}"
    
    - id: review_process
      activity: document_review_flow  # State machine activity
      inputs:
        document: "{{ .Steps.extract.outputs.text }}"
    
    - id: publish
      activity: publish_document
      inputs:
        document: "{{ .Steps.review_process.outputs.document }}"
```

#### Nested State Machines
```yaml
activities:
  - name: complex_pipeline
    implementation:
      type: state_machine
      config:
        initial_state: phase_one
        states:
          phase_one:
            # State invokes another state machine
            activity: document_review_flow
            transitions:
              - to: phase_two
                when: ".Outputs.approved == true"
          phase_two:
            activity: quality_check_flow  # Another state machine
            transitions:
              - to: complete
                when: ".Outputs.passed == true"
          complete:
            terminal: true
```

### 7. Benefits

1. **Standardized**: Uses Google's CEL, avoiding custom expression language
2. **Type-safe**: CEL provides compile-time type checking
3. **Secure**: Sandboxed execution, no side effects
4. **Performant**: Efficient evaluation of expressions
5. **Well-documented**: Extensive documentation and community support
6. **Golang-native**: Excellent Go support via google/cel-go
7. **Recipe Composability**: Seamless integration with recipe-to-recipe invocation
8. **No Core Changes**: Implemented entirely as a RegisterableActivity
9. **Gradual Adoption**: Existing recipes continue to work; state machines added as needed

## Implementation Plan

1. Implement StateMachineActivity with production-ready error handling
2. Add CEL expression evaluator with comprehensive validation
3. Create test suite with unit, integration, and end-to-end tests
4. Document CEL expression patterns and best practices for recipes
5. Build production validation tooling for state machine definitions
6. Update recipe CLI to support state machine debugging and monitoring