# State Machine Composition Specification

## Overview

This specification extends the CEL state machine implementation to support arbitrary composition within states. Instead of limiting each state to a single activity, states can now contain full workflow definitions with sequential, parallel, and nested step execution - mirroring the capabilities available at the top level of recipes.

## Motivation

Current limitations:
- Each state can only execute a single activity or recipe
- No support for parallel operations within a state
- No ability to compose multiple activities before transitioning
- Complex workflows require many states for what could be a single logical step

Benefits of composition:
- States represent logical workflow phases, not individual operations
- Parallel execution within states for better performance
- Reuse existing recipe workflow patterns
- Simplified state machines with fewer states

## Design

### Enhanced State Definition

States can now contain either a single activity/recipe (backward compatible) OR a complete workflow definition with steps:

```yaml
states:
  data_preparation:
    # NEW: Workflow definition within a state
    workflow:
      type: parallel  # sequential, parallel, or conditional
      steps:
        - id: fetch_data
          activity: fetch_from_api
          inputs:
            endpoint: "{{ .Inputs.api_url }}"
        
        - id: load_cache
          activity: load_from_cache
          inputs:
            key: "{{ .Inputs.cache_key }}"
        
        - id: validate
          activity: validate_data
          inputs:
            data: "{{ .Steps.fetch_data.outputs.data }}"
          depends_on: ["fetch_data"]
    
    # Transitions evaluate against the combined workflow outputs
    transitions:
      - to: processing
        when: ".Outputs.validate.valid == true"
      - to: error
        when: ".Outputs.validate.valid == false"
```

### Backward Compatibility

Existing single-activity states continue to work:

```yaml
states:
  simple_state:
    activity: my_activity  # Still supported
    transitions:
      - to: next_state
        when: ".Outputs.result == 'success'"
```

### Complete Example

```yaml
activities:
  - name: document_processing_pipeline
    implementation:
      type: state_machine
      config:
        initial_state: preparation
        states:
          preparation:
            # Parallel data preparation within the state
            workflow:
              type: parallel
              steps:
                - id: extract_text
                  activity: text_extractor
                  inputs:
                    document: "{{ .Inputs.document }}"
                
                - id: extract_metadata
                  activity: metadata_extractor
                  inputs:
                    document: "{{ .Inputs.document }}"
                
                - id: extract_images
                  activity: image_extractor
                  inputs:
                    document: "{{ .Inputs.document }}"
            
            transitions:
              - to: enrichment
                when: ".Outputs.extract_text.success == true"
              - to: fallback_extraction
                when: ".Outputs.extract_text.success == false"
          
          enrichment:
            # Sequential enrichment pipeline
            workflow:
              type: sequential
              steps:
                - id: classify
                  activity: document_classifier
                  inputs:
                    text: "{{ .States.preparation.outputs.extract_text.text }}"
                    metadata: "{{ .States.preparation.outputs.extract_metadata.metadata }}"
                
                - id: enrich
                  activity: llm_enrichment
                  inputs:
                    text: "{{ .States.preparation.outputs.extract_text.text }}"
                    classification: "{{ .Steps.classify.outputs.category }}"
                
                - id: generate_summary
                  activity: summarizer
                  inputs:
                    enriched_text: "{{ .Steps.enrich.outputs.enriched_text }}"
            
            transitions:
              - to: quality_check
                when: ".Outputs.generate_summary.confidence >= 0.8"
              - to: manual_review
                when: ".Outputs.generate_summary.confidence < 0.8"
          
          quality_check:
            # Nested state machine within a state
            workflow:
              type: sequential
              steps:
                - id: automated_checks
                  activity: quality_validator
                  inputs:
                    document: "{{ .States.enrichment.outputs }}"
                
                - id: review_if_needed
                  activity: conditional_review_state_machine  # Another state machine
                  inputs:
                    document: "{{ .States.enrichment.outputs }}"
                    validation: "{{ .Steps.automated_checks.outputs }}"
                  when: ".Steps.automated_checks.outputs.requires_review == true"
            
            transitions:
              - to: publish
                when: ".Outputs.automated_checks.passed == true"
              - to: revision
                when: ".Outputs.review_if_needed.approved == false"
          
          publish:
            terminal: true
            outputs:
              result: "{{ .States.enrichment.outputs }}"
              metadata: "{{ .States.preparation.outputs.extract_metadata }}"
```

## Implementation

### Type Updates

```go
// StateDefinition extends to support workflow composition
type StateDefinition struct {
    // Single activity/recipe (backward compatible)
    Activity     string                 `json:"activity,omitempty"`
    Recipe       string                 `json:"recipe,omitempty"`
    
    // NEW: Workflow definition for composition
    Workflow     *WorkflowDefinition    `json:"workflow,omitempty"`
    
    // Existing fields
    Terminal     bool                   `json:"terminal,omitempty"`
    Error        string                 `json:"error,omitempty"`
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
    Outputs      map[string]interface{} `json:"outputs,omitempty"`
    Retry        *StateRetryPolicy      `json:"retry,omitempty"`
    Transitions  []TransitionSpec       `json:"transitions,omitempty"`
}

// WorkflowDefinition defines the workflow within a state
type WorkflowDefinition struct {
    Type    string         `json:"type"`    // sequential, parallel, conditional
    Steps   []WorkflowStep `json:"steps"`
    Timeout string         `json:"timeout,omitempty"`
}

// WorkflowStep defines a step within a state's workflow
type WorkflowStep struct {
    ID          string                 `json:"id"`
    Activity    string                 `json:"activity,omitempty"`
    Recipe      string                 `json:"recipe,omitempty"`
    StateMachine string                `json:"state_machine,omitempty"`
    Inputs      map[string]interface{} `json:"inputs,omitempty"`
    DependsOn   []string               `json:"depends_on,omitempty"`
    When        string                 `json:"when,omitempty"` // CEL condition
    Retry       *RetryPolicy           `json:"retry,omitempty"`
}
```

### Execution Logic

```go
func (s *StateMachineActivity) executeState(ctx context.Context, state StateDefinition, stateCtx *StateContext) (map[string]interface{}, error) {
    // Terminal states don't execute anything
    if state.Terminal {
        return state.Outputs, nil
    }
    
    // Check for workflow definition (new composition feature)
    if state.Workflow != nil {
        return s.executeStateWorkflow(ctx, state.Workflow, stateCtx)
    }
    
    // Backward compatibility: single activity/recipe
    if state.Activity != "" {
        return s.executor.ExecuteActivity(ctx, state.Activity, prepareInputs(state, stateCtx))
    } else if state.Recipe != "" {
        return s.executor.ExecuteRecipe(ctx, state.Recipe, prepareInputs(state, stateCtx))
    }
    
    return nil, fmt.Errorf("state must specify either 'activity', 'recipe', or 'workflow'")
}

func (s *StateMachineActivity) executeStateWorkflow(ctx context.Context, workflow *WorkflowDefinition, stateCtx *StateContext) (map[string]interface{}, error) {
    switch workflow.Type {
    case "sequential":
        return s.executeSequentialWorkflow(ctx, workflow, stateCtx)
    case "parallel":
        return s.executeParallelWorkflow(ctx, workflow, stateCtx)
    case "conditional":
        return s.executeConditionalWorkflow(ctx, workflow, stateCtx)
    default:
        return nil, fmt.Errorf("unknown workflow type: %s", workflow.Type)
    }
}

func (s *StateMachineActivity) executeSequentialWorkflow(ctx context.Context, workflow *WorkflowDefinition, stateCtx *StateContext) (map[string]interface{}, error) {
    stepOutputs := make(map[string]interface{})
    
    for _, step := range workflow.Steps {
        // Check if step should be executed (CEL condition)
        if step.When != "" && !s.evaluateCondition(step.When, stepOutputs, stateCtx) {
            continue
        }
        
        // Execute the step
        output, err := s.executeWorkflowStep(ctx, step, stepOutputs, stateCtx)
        if err != nil {
            if step.Retry != nil && s.shouldRetryStep(step.Retry, err) {
                // Retry logic
                output, err = s.retryStep(ctx, step, stepOutputs, stateCtx)
            }
            if err != nil {
                return nil, fmt.Errorf("step '%s' failed: %w", step.ID, err)
            }
        }
        
        stepOutputs[step.ID] = output
    }
    
    return stepOutputs, nil
}

func (s *StateMachineActivity) executeParallelWorkflow(ctx context.Context, workflow *WorkflowDefinition, stateCtx *StateContext) (map[string]interface{}, error) {
    // Group steps by dependencies
    groups := s.groupStepsByDependencies(workflow.Steps)
    stepOutputs := make(map[string]interface{})
    
    for _, group := range groups {
        // Execute steps in parallel within each group
        results := make(chan stepResult, len(group))
        
        for _, step := range group {
            go func(step WorkflowStep) {
                output, err := s.executeWorkflowStep(ctx, step, stepOutputs, stateCtx)
                results <- stepResult{ID: step.ID, Output: output, Error: err}
            }(step)
        }
        
        // Collect results
        for range group {
            result := <-results
            if result.Error != nil {
                return nil, fmt.Errorf("parallel step '%s' failed: %w", result.ID, result.Error)
            }
            stepOutputs[result.ID] = result.Output
        }
    }
    
    return stepOutputs, nil
}
```

### CEL Variable Access

Within state workflows, CEL expressions have access to:

```yaml
# In workflow steps within a state
.Inputs:          # State inputs
.Steps:           # Outputs from previous steps in current workflow
  <step_id>:
    outputs: any  # Step outputs
.States:          # Outputs from previous states
  <state_name>:
    outputs: any  # State outputs
.Context:         # Recipe context
```

### Transition Evaluation

Transitions can now reference nested step outputs:

```yaml
transitions:
  - to: next_state
    when: ".Outputs.step1.result == 'success' && .Outputs.step2.score > 80"
```

## Benefits

1. **Logical Grouping**: States represent complete logical phases, not individual operations
2. **Performance**: Parallel execution within states reduces overall execution time
3. **Reusability**: Leverage existing workflow patterns and activities
4. **Flexibility**: Mix sequential, parallel, and conditional execution within states
5. **Simplicity**: Fewer states needed for complex workflows
6. **Composability**: States can contain other state machines for nested workflows

## Migration Path

### Phase 1: Backward Compatible Extension
- Add workflow field to StateDefinition
- Implement workflow execution within states
- Existing single-activity states continue to work

### Phase 2: Recipe Converter
- Tool to convert complex multi-state machines to composed states
- Identify patterns that can be consolidated

### Phase 3: Best Practices
- Guidelines for when to use composition vs separate states
- Performance optimization patterns

## Examples

### Data Pipeline with Parallel Processing

```yaml
states:
  data_processing:
    workflow:
      type: parallel
      steps:
        - id: process_batch_1
          activity: batch_processor
          inputs:
            batch: "{{ .Inputs.batches[0] }}"
        
        - id: process_batch_2
          activity: batch_processor
          inputs:
            batch: "{{ .Inputs.batches[1] }}"
        
        - id: process_batch_3
          activity: batch_processor
          inputs:
            batch: "{{ .Inputs.batches[2] }}"
    
    transitions:
      - to: aggregation
        when: "true"  # Always transition after parallel completion
```

### Conditional Processing with Fallbacks

```yaml
states:
  processing:
    workflow:
      type: sequential
      steps:
        - id: try_fast_path
          activity: fast_processor
          inputs:
            data: "{{ .Inputs.data }}"
          retry:
            max_attempts: 2
            initial_interval: "1s"
        
        - id: fallback_slow_path
          activity: slow_processor
          inputs:
            data: "{{ .Inputs.data }}"
          when: ".Steps.try_fast_path.error != null"
        
        - id: final_validation
          activity: validator
          inputs:
            result: "{{ .Steps.try_fast_path.outputs || .Steps.fallback_slow_path.outputs }}"
    
    transitions:
      - to: success
        when: ".Outputs.final_validation.valid == true"
```

### Nested State Machines

```yaml
states:
  complex_operation:
    workflow:
      type: sequential
      steps:
        - id: prepare
          activity: data_prep
        
        - id: nested_processing
          state_machine: specialized_state_machine
          inputs:
            data: "{{ .Steps.prepare.outputs.data }}"
        
        - id: finalize
          activity: finalizer
          inputs:
            result: "{{ .Steps.nested_processing.outputs.result }}"
```

## Testing Strategy

1. **Unit Tests**: Test workflow execution within states
2. **Integration Tests**: Test complex state compositions
3. **Performance Tests**: Verify parallel execution benefits
4. **Compatibility Tests**: Ensure backward compatibility

## Future Enhancements

1. **Dynamic Step Generation**: Generate steps based on input data
2. **Step Templates**: Reusable step patterns
3. **Distributed Execution**: Execute parallel steps across workers
4. **Step Caching**: Cache step outputs for reuse
5. **Visual Debugging**: Enhanced UI for debugging composed states