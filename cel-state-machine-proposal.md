# Proposal: CEL Integration for State Machine Recipes

## Overview

This proposal outlines the integration of Google's Common Expression Language (CEL) into the recipe system to enable declarative, type-safe conditional logic for state transitions and branching behavior in recipes. Note that while we refer to the system as "recipes", the YAML schema maintains the `workflow:` field for defining execution logic within recipes.

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

### 2. State Machine Enhancement to WorkflowSpec

#### Current WorkflowSpec Structure
```yaml
workflow:
  type: sequential  # or parallel
  steps:
    - id: step1
      activity: some_activity
      inputs:
        key: value
```

#### Proposed State Machine Type for WorkflowSpec
```yaml
workflow:
  type: state_machine  # New workflow type
  initial_state: reviewing
  states:
    reviewing:
      activity: critique_activity
      inputs:
        report: "states.previous.outputs.report"
      transitions:
        - to: completed
          when: |
            .Outputs.review.grade in ["A", "B"] && 
            .Outputs.review.score >= 80.0
        - to: improving
          when: |
            .Outputs.review.grade in ["C", "D"] && 
            .State.attempts < 3
        - to: failed
          when: |
            .Outputs.review.grade == "F" || 
            .State.attempts >= 3
```

### 3. Key Features of State Machine Activities

State machine activities provide powerful control flow capabilities within the existing recipe framework:

#### State Group as a Step
```yaml
workflow:
  type: sequential
  steps:
    - id: initial_processing
      activity: validate_input
      inputs:
        data: "{{ .Inputs.data }}"
    
    # Embed a state machine within a sequential workflow
    - id: review_loop
      state_group:
        initial_state: reviewing
        states:
          reviewing:
            activity: critique_activity
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
    
    - id: final_processing
      activity: publish_results
      inputs:
        approved_data: "{{ .Steps.review_loop.outputs.result }}"
```

#### Parallel State Groups
```yaml
workflow:
  type: parallel
  steps:
    - id: path_a
      state_group:
        initial_state: processing_a
        states:
          processing_a:
            activity: process_type_a
            transitions:
              - to: complete_a
                when: ".Outputs.valid == true"
              - to: error_a
                when: ".Outputs.valid == false"
          complete_a:
            terminal: true
          error_a:
            terminal: true
            error: "Type A processing failed"
    
    - id: path_b
      state_group:
        initial_state: processing_b
        states:
          processing_b:
            activity: process_type_b
            transitions:
              - to: retry_b
                when: ".Outputs.needs_retry == true && .State.attempts < 3"
              - to: complete_b
                when: ".Outputs.success == true"
          retry_b:
            activity: retry_process_b
            transitions:
              - to: processing_b
          complete_b:
            terminal: true
```

#### Nested State Groups
```yaml
workflow:
  type: state_machine
  initial_state: main_flow
  states:
    main_flow:
      type: state_group  # A state can itself be a state group
      initial_state: validate
      states:
        validate:
          activity: validate_data
          transitions:
            - to: process
              when: "outputs.valid == true"
        process:
          activity: process_data
          transitions:
            - to: complete
              when: "outputs.done == true"
        complete:
          type: terminal
      transitions:
        - to: next_phase
          when: "outputs.ready == true"
    
    next_phase:
      activity: finalize
      transitions:
        - to: done
          when: "outputs.finalized == true"
    
    done:
      type: terminal
```

### 4. New Recipe Features

#### Retry Loops with CEL
```yaml
states:
  compiling:
    activity: compile_code
    retry:
      when: ".Outputs.errors.size() > 0"
      max_attempts: 3
      backoff:
        initial: "1s"
        multiplier: 2
    transitions:
      - to: testing
        when: ".Outputs.success == true"
      - to: manual_fix
        when: ".State.attempts >= 3"
```

#### Conditional Branching in Recipes
```yaml
states:
  code_review:
    activity: analyze_changes
    branches:
      - condition: ".Outputs.diff.lines_changed > 100"
        to: senior_review
      - condition: |
          .Outputs.diff.lines_changed <= 100 && 
          .Outputs.risk_score < 5
        to: auto_merge
      - default: standard_review
```

#### Recipe-to-Recipe Invocation with Conditions
```yaml
states:
  data_processing:
    activity: recipe_invocation
    recipe: "data-processing/transform"
    inputs:
      data: "states.extraction.outputs.raw_data"
    when: |
      .States.extraction.outputs.raw_data.size() > 0 &&
      .States.validation.outputs.is_valid == true
```

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
    
# Workflow steps (for sequential/parallel workflows)
.Steps:
  <step_id>:
    outputs: any            # Outputs from completed steps
    
# Recipe inputs
.Inputs:
  <input_name>: any         # Original recipe inputs (from InputDefinition)

# Recipe context (from RecipeContext)
.Context:
  Recipe:
    Name: string
    Version: string
    ExecutionID: string
    ParentExecutionID: string
  Environment:
    Name: string
    Region: string
    Cluster: string
  Execution:
    Host: string
    Namespace: string
    TaskQueue: string
    StartedAt: timestamp
    Timeout: duration
  Auth:
    Identity: string

# Job metadata
.Metadata:
  RecipeID: string
  JobID: string
  TotalTransitions: int
```

Note: CEL expressions use the same variable naming convention as Go templates for consistency. The dot prefix (`.`) indicates traversal of the context object, matching the existing template syntax.

### 5. Implementation as RegisterableActivity

The state machine functionality will be implemented as a new activity type that conforms to the existing `RegisterableActivity` interface, requiring no changes to the core recipe worker.

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
    // State machine execution logic here
}
```

#### Activity Registration

State machines are registered as a new activity implementation type:

```yaml
activities:
  - name: document_review_flow
    description: Reviews document with retry loop
    implementation:
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
              - to: revision_needed
                when: ".Outputs.score < 80 && .State.attempts < 3"
          # ... more states
```

#### Go Dependencies
```go
import (
    "github.com/google/cel-go/cel"
    "github.com/google/cel-go/checker/decls"
    "github.com/divisive-ai/vibethis/server/activity/pkg/types"
)
```

#### Implementation Components

1. **StateMachineConfig**: Configuration structure for state machine definitions
2. **CEL Expression Evaluator**: Evaluates transition conditions
3. **State Executor**: Manages state transitions and activity execution
4. **Context Builder**: Builds CEL context with current state, outputs, and inputs
5. **Activity Orchestrator**: Executes nested activities within states

#### Type Definitions for State Machine Activity

```go
// StateMachineConfig defines the configuration for state machine activities
type StateMachineConfig struct {
    InitialState string                      `json:"initial_state"`
    States       map[string]StateDefinition  `json:"states"`
    Timeout      string                      `json:"timeout,omitempty"`
}

// StateDefinition defines a single state in the state machine
type StateDefinition struct {
    Activity     string                 `json:"activity,omitempty"`     // Activity to execute
    Recipe       string                 `json:"recipe,omitempty"`       // Recipe to invoke
    Terminal     bool                   `json:"terminal,omitempty"`     // Terminal state flag
    Error        string                 `json:"error,omitempty"`        // Error message for terminal states
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
    Outputs      map[string]interface{} `json:"outputs,omitempty"`      // For terminal states
    Retry        *StateRetryPolicy      `json:"retry,omitempty"`
    Transitions  []TransitionSpec       `json:"transitions,omitempty"`
    StateGroup   *StateMachineConfig    `json:"state_group,omitempty"`  // For nested state machines
}

// TransitionSpec defines state transitions with CEL conditions
type TransitionSpec struct {
    To   string `json:"to"`
    When string `json:"when"` // CEL expression
}

// StateRetryPolicy with CEL conditions
type StateRetryPolicy struct {
    When               string  `json:"when,omitempty"` // CEL expression
    MaxAttempts        int     `json:"max_attempts"`
    BackoffCoefficient float64 `json:"backoff_coefficient,omitempty"`
    InitialInterval    string  `json:"initial_interval,omitempty"`
    MaximumInterval    string  `json:"maximum_interval,omitempty"`
}

// StateContext maintains runtime state machine context
type StateContext struct {
    CurrentState string
    Attempts     map[string]int
    StateOutputs map[string]map[string]interface{}
    Inputs       map[string]interface{}
    StartTime    time.Time
}
```

#### Activity Output Validation with CEL
```yaml
activities:
  - name: critique_activity
    outputs:
      - name: review
        type: object
        schema:
          grade: string
          score: float
          suggestions: array[string]
        validation: |
          has(review.grade) && 
          review.grade in ["A", "B", "C", "D", "F"] &&
          review.score >= 0.0 && 
          review.score <= 100.0
```

### 6. Example: State Machine as Activity

This example shows how state machines are used as activities within recipes:

```yaml
# First, define the state machine as an activity in activities.yaml
activities:
  - name: document_review_flow
    description: Review document with automatic retry and improvement
    implementation:
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
                when: ".Outputs.score >= .Inputs.min_score"
              - to: improving
                when: ".Outputs.score < .Inputs.min_score && .State.attempts < 3"
              - to: rejected
                when: ".State.attempts >= 3"
          
          improving:
            activity: improve_document
            inputs:
              original: "{{ .States.reviewing.outputs.document }}"
              critique: "{{ .States.reviewing.outputs.critique }}"
            transitions:
              - to: reviewing
                when: ".Outputs.improved == true"
          
          approved:
            terminal: true
            outputs:
              final_document: "{{ .States.reviewing.outputs.document }}"
              score: "{{ .States.reviewing.outputs.score }}"
          
          rejected:
            terminal: true
            error: "Document failed review after {{ .State.attempts }} attempts"
```

```yaml
# Then use it in workflow.yaml
name: document_processing_pipeline
version: "2.0"
description: Process documents using state machine activities

inputs:
  - name: document_path
    type: string
    required: true

workflow:
  type: sequential
  steps:
    - id: extract
      activity: extract_text
      inputs:
        path: "{{ .Inputs.document_path }}"
    
    - id: review_process
      activity: document_review_flow  # Using the state machine activity
      inputs:
        document: "{{ .Steps.extract.outputs.text }}"
        min_score: 80
    
    - id: publish
      activity: publish_document
      inputs:
        document: "{{ .Steps.review_process.outputs.final_document }}"
        metadata:
          score: "{{ .Steps.review_process.outputs.score }}"
```

### 7. Complex Example: Nested State Machines

```yaml
# Define a complex state machine with nested state machines
activities:
  - name: research_with_review
    implementation:
      type: state_machine
      config:
        initial_state: research_phase
        states:
          research_phase:
            # A state can invoke another state machine activity
            activity: research_state_machine
            inputs:
              topic: "{{ .Inputs.topic }}"
            transitions:
              - to: review_phase
                when: ".Outputs.sources.size() > 0"
          
    analyzing:
      activity: analyze_activity
      inputs:
        data: "states.researching.outputs.research_data"
      transitions:
        - to: writing
          when: "outputs.analysis.key_points.size() >= 3"
          
    writing:
      activity: write_report_activity
      inputs:
        research: "states.researching.outputs.research_data"
        analysis: "states.analyzing.outputs.analysis"
      transitions:
        - to: reviewing
          when: "outputs.report.size() > 100"
          
    reviewing:
      activity: critique_activity
      inputs:
        report: "states.writing.outputs.report"
      transitions:
        - to: completed
          when: |
            outputs.review.grade <= inputs.min_grade &&
            outputs.review.score >= 80.0
        - to: improving
          when: |
            outputs.review.grade > inputs.min_grade &&
            state.attempts < 3
        - to: failed
          when: "state.attempts >= 3"
          
    improving:
      activity: improve_report_activity
      inputs:
        original: "states.writing.outputs.report"
        critique: "states.reviewing.outputs.review"
      transitions:
        - to: reviewing
          when: "outputs.improved_report.size() > 0"
          
    completed:
      type: terminal
      outputs:
        final_report: "states.writing.outputs.report"
        final_grade: "states.reviewing.outputs.review.grade"
        
    failed:
      type: terminal
      error: "Recipe failed in state: {{ state.name }}"
```

### 7. Integration with Recipe-to-Recipe Invocation

The state machine can leverage the existing `RecipeActivity` for recipe composition:

```yaml
states:
  data_extraction:
    recipe: "data-extraction/pdf-parser"  # Invoke another recipe
    inputs:
      file_path: "inputs.document_path"
    transitions:
      - to: data_validation
        when: ".Outputs.result.extracted_text.size() > 0"
        
  data_validation:
    recipe: "validation/schema-checker"
    inputs:
      data: "states.data_extraction.outputs.result"
    retry:
      when: ".Outputs.result.validation_errors.size() > 0"
      max_attempts: 2
    transitions:
      - to: processing
        when: ".Outputs.result.is_valid == true"
```

### 8. Benefits

1. **Standardized**: Uses Google's CEL, avoiding custom expression language
2. **Type-safe**: CEL provides compile-time type checking
3. **Secure**: Sandboxed execution, no side effects
4. **Performant**: Efficient evaluation of expressions
5. **Well-documented**: Extensive documentation and community support
6. **Golang-native**: Excellent Go support via google/cel-go
7. **Recipe Composability**: Seamless integration with recipe-to-recipe invocation
8. **Flexible Mixing**: State groups allow combining sequential, parallel, and state machine logic within a single recipe
9. **Gradual Adoption**: Existing recipes continue to work; state machines can be added incrementally where needed

### 9. Migration Path

1. Phase 1: Add CEL support alongside existing sequential/parallel workflow types
2. Phase 2: Convert complex recipes to use state machine type
3. Phase 3: Enable CEL expressions in standard Step definitions for conditional execution
4. Phase 4: Full CEL adoption across recipe system

### 10. Open Questions

1. Should we support CEL macros for common recipe patterns?
2. How to handle CEL expression errors during recipe execution?
3. Should we expose custom functions to CEL (e.g., `recipe.invoke()`, `grade_to_score()`)?
4. Performance implications for complex expressions in high-throughput recipes?
5. Should state machines support nested states or hierarchical state machines?

## Implementation Plan

1. Implement CEL integration in recipe engine with production-ready error handling
2. Extend WorkflowSpec to support state_machine type with full validation
3. Create comprehensive test suite with unit, integration, and end-to-end tests
4. Document CEL expression patterns and best practices for recipes
5. Build production recipe validation tooling for state machine definitions
6. Update recipe CLI to support state machine debugging and monitoring