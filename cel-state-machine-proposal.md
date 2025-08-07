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
            outputs.review.grade in ["A", "B"] && 
            outputs.review.score >= 80.0
        - to: improving
          when: |
            outputs.review.grade in ["C", "D"] && 
            state.attempts < 3
        - to: failed
          when: |
            outputs.review.grade == "F" || 
            state.attempts >= 3
```

### 3. New Recipe Features

#### Retry Loops with CEL
```yaml
states:
  compiling:
    activity: compile_code
    retry:
      when: "outputs.errors.size() > 0"
      max_attempts: 3
      backoff:
        initial: "1s"
        multiplier: 2
    transitions:
      - to: testing
        when: "outputs.success == true"
      - to: manual_fix
        when: "state.attempts >= 3"
```

#### Conditional Branching in Recipes
```yaml
states:
  code_review:
    activity: analyze_changes
    branches:
      - condition: "outputs.diff.lines_changed > 100"
        to: senior_review
      - condition: |
          outputs.diff.lines_changed <= 100 && 
          outputs.risk_score < 5
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
      states.extraction.outputs.raw_data.size() > 0 &&
      states.validation.outputs.is_valid == true
```

### 4. CEL Context Variables

Available variables in CEL expressions aligned with current recipe system:

```yaml
# State context
state:
  name: string              # Current state name
  attempts: int             # Number of attempts in current state
  entered_at: timestamp     # When state was entered
  
# Step/Activity outputs
outputs:
  <activity_output>: any    # Current activity outputs
  result: any               # Activity result map

# Previous states
states:
  <state_name>:
    outputs: any           # Outputs from previous states
    
# Recipe inputs
inputs:
  <input_name>: any        # Original recipe inputs (from InputDefinition)

# Recipe context (from RecipeContext)
context:
  recipe:
    name: string
    version: string
    execution_id: string
    parent_execution_id: string
  environment:
    name: string
    region: string
    cluster: string
  execution:
    host: string
    namespace: string
    task_queue: string
    started_at: timestamp
    timeout: duration
  auth:
    identity: string

# Metadata
metadata:
  recipe_id: string
  job_id: string
  total_transitions: int
```

### 5. Implementation Requirements

#### Go Dependencies
```go
import (
    "github.com/google/cel-go/cel"
    "github.com/google/cel-go/checker/decls"
)
```

#### Recipe Engine Changes
1. Extend `WorkflowSpec` to support `state_machine` type
2. Add CEL evaluator to recipe execution engine
3. Create context builder for CEL expressions
4. Add validation for CEL expressions at recipe parse time
5. Implement CEL-based transition evaluation
6. Integrate with existing `RecipeActivity` for recipe-to-recipe calls

#### Updated Type Definitions

Extend `server/recipe-core/pkg/yaml/types.go`:

```go
// WorkflowSpec with state machine support
type WorkflowSpec struct {
    Type         string                 `yaml:"type"` // sequential, parallel, state_machine
    RetryPolicy  RetryPolicy            `yaml:"retry_policy"`
    Steps        []Step                 `yaml:"steps"`        // For sequential/parallel
    States       map[string]StateSpec   `yaml:"states"`       // For state_machine
    InitialState string                 `yaml:"initial_state"` // For state_machine
    Outputs      map[string]string      `yaml:"outputs"`
}

// StateSpec defines a state in a state machine recipe
type StateSpec struct {
    Activity     string                 `yaml:"activity"`
    Recipe       string                 `yaml:"recipe"`      // For recipe invocation
    Inputs       map[string]interface{} `yaml:"inputs"`
    Retry        StateRetryPolicy       `yaml:"retry"`
    Transitions  []TransitionSpec       `yaml:"transitions"`
    Branches     []BranchSpec           `yaml:"branches"`
    Type         string                 `yaml:"type"`        // terminal, activity, recipe
}

// TransitionSpec defines state transitions with CEL conditions
type TransitionSpec struct {
    To   string `yaml:"to"`
    When string `yaml:"when"` // CEL expression
}

// BranchSpec defines conditional branching
type BranchSpec struct {
    Condition string `yaml:"condition"` // CEL expression
    To        string `yaml:"to"`
    Default   bool   `yaml:"default"`
}

// StateRetryPolicy with CEL conditions
type StateRetryPolicy struct {
    When               string        `yaml:"when"` // CEL expression
    MaxAttempts        int           `yaml:"max_attempts"`
    BackoffCoefficient float64       `yaml:"backoff_coefficient"`
    InitialInterval    time.Duration `yaml:"initial_interval"`
    MaximumInterval    time.Duration `yaml:"maximum_interval"`
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

### 6. Example: Complete Recipe with State Machine and Critic

```yaml
name: research_report_with_review
version: "2.0"
description: Research report generation with iterative review

inputs:
  - name: topic
    type: string
    required: true
  - name: min_grade
    type: string
    default: "B"

outputs:
  - name: final_report
    type: string
  - name: final_grade
    type: string

workflow:
  type: state_machine
  initial_state: researching
  
  states:
    researching:
      activity: research_activity
      inputs:
        topic: "inputs.topic"
      transitions:
        - to: analyzing
          when: "outputs.research_data.sources.size() > 0"
        - to: failed
          when: "outputs.research_data.sources.size() == 0"
          
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
        when: "outputs.result.extracted_text.size() > 0"
        
  data_validation:
    recipe: "validation/schema-checker"
    inputs:
      data: "states.data_extraction.outputs.result"
    retry:
      when: "outputs.result.validation_errors.size() > 0"
      max_attempts: 2
    transitions:
      - to: processing
        when: "outputs.result.is_valid == true"
```

### 8. Benefits

1. **Standardized**: Uses Google's CEL, avoiding custom expression language
2. **Type-safe**: CEL provides compile-time type checking
3. **Secure**: Sandboxed execution, no side effects
4. **Performant**: Efficient evaluation of expressions
5. **Well-documented**: Extensive documentation and community support
6. **Golang-native**: Excellent Go support via google/cel-go
7. **Recipe Composability**: Seamless integration with recipe-to-recipe invocation

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