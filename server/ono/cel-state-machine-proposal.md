# Proposal: CEL Integration for State Machine Workflows

## Overview

This proposal outlines the integration of Google's Common Expression Language (CEL) into the workflow system to enable declarative, type-safe conditional logic for state transitions and branching behavior.

## Motivation

Current limitations:
- No standardized way to express retry loops or conditional branching
- No support for complex decision logic in workflows
- Need for a safe, sandboxed expression language for user-defined conditions

## Proposed Solution

### 1. CEL Integration

Use CEL as the standard expression language for:
- State transition conditions
- Retry loop conditions
- Activity input transformations
- Output validations

### 2. State Machine Enhancement

#### Current State Definition
```yaml
states:
  reviewing:
    activity: critique_activity
    transitions:
      - to: completed
        when: complete
```

#### Proposed State Definition with CEL
```yaml
states:
  reviewing:
    activity: critique_activity
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

### 3. New Workflow Features

#### Retry Loops
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

#### Conditional Branching
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

### 4. CEL Context Variables

Available variables in CEL expressions:

```yaml
# State context
state:
  name: string              # Current state name
  attempts: int             # Number of attempts in current state
  entered_at: timestamp     # When state was entered
  
# Step outputs
outputs:
  <activity_output>: any    # Current activity outputs

# Previous states
states:
  <state_name>:
    outputs: any           # Outputs from previous states
    
# Workflow inputs
inputs:
  <input_name>: any        # Original workflow inputs

# Metadata
metadata:
  workflow_id: string
  execution_id: string
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

#### Workflow Engine Changes
1. Add CEL evaluator to state machine engine
2. Create context builder for CEL expressions
3. Add validation for CEL expressions at workflow parse time
4. Implement CEL-based transition evaluation

#### Activity Changes
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

### 6. Example: Complete Workflow with Critic

```yaml
name: research_report_with_review
version: "2.0"

inputs:
  - name: topic
    type: string
    required: true
  - name: min_grade
    type: string
    default: "B"

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
        report: "states.writing.outputs.report"
        grade: "states.reviewing.outputs.review.grade"
        
    failed:
      type: terminal
      error: "Workflow failed: {{ state.name }}"
```

### 7. Benefits

1. **Standardized**: Uses Google's CEL, avoiding custom expression language
2. **Type-safe**: CEL provides compile-time type checking
3. **Secure**: Sandboxed execution, no side effects
4. **Performant**: Efficient evaluation of expressions
5. **Well-documented**: Extensive documentation and community support
6. **Golang-native**: Excellent Go support via google/cel-go

### 8. Migration Path

1. Phase 1: Add CEL support alongside existing "when: complete" syntax
2. Phase 2: Convert existing workflows to use CEL expressions
3. Phase 3: Deprecate old syntax, full CEL adoption

### 9. Open Questions

1. Should we support CEL macros for common patterns?
2. How to handle CEL expression errors during execution?
3. Should we expose custom functions to CEL (e.g., `grade_to_score()`)?
4. Performance implications for complex expressions?

## Next Steps

1. Prototype CEL integration in workflow engine
2. Create comprehensive test suite for CEL expressions
3. Document CEL expression patterns and best practices
4. Build workflow validation tooling