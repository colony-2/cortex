# State Machine Composition Specification

## Overview

This specification enhances the CEL state machine to support **fully composable conditional workflows** within states. The core goal is to enable complex conditional logic and dynamic execution paths through unlimited nesting of sequential, parallel, and conditional compositions. Any composition type can contain any other type, creating a powerful system for expressing complex business logic.

Key changes:
1. States can contain complete workflow compositions (not just single activities)
2. All composition types (sequential, parallel, conditional) are fully nestable
3. Simplified YAML structure with direct type declarations
4. Clean reference syntax for accessing nested outputs
5. State machine moves from activity implementation to core compiler

## Design

### Enhanced State Definition

States maintain the existing structure but can now contain complete compositions. The composition type is declared directly without a wrapper:

```yaml
states:
  # Simple activity state (backward compatible)
  simple_state:
    uses: my_activity
    inputs:
      data: "{{ .Inputs.raw_data }}"
    transitions:
      - to: next_state
        when: ".Outputs.result == 'success'"
  
  # Parallel composition state
  data_preparation:
    parallel:
      - id: fetch_data
        uses: fetch_from_api
        inputs:
          endpoint: "{{ .Inputs.api_url }}"
      
      - id: load_cache
        uses: load_from_cache
        inputs:
          key: "{{ .Inputs.cache_key }}"
      
      - id: validate
        uses: validate_data
        inputs:
          data: "{{ .Steps.fetch_data.outputs.data }}"
        depends_on: [fetch_data]
    
    transitions:
      - to: processing
        when: ".Outputs.validate.valid == true"
      - to: error
        when: ".Outputs.validate.valid == false"
  
  # Sequential composition state
  processing:
    sequential:
      - id: transform
        uses: data_transformer
        inputs:
          data: "{{ .States.data_preparation.outputs.fetch_data.data }}"
      
      - id: enrich
        uses: data_enricher
        inputs:
          data: "{{ .Steps.transform.outputs.result }}"
      
      - id: save
        uses: data_saver
        inputs:
          data: "{{ .Steps.enrich.outputs.enriched_data }}"
    
    transitions:
      - to: complete
        when: ".Outputs.save.success == true"
  
  # Conditional composition state
  decision_point:
    conditional:
      - when: ".Inputs.data_size > 1000000"
        parallel:  # Nested parallel in conditional
          - id: split_1
            uses: batch_processor
            inputs:
              batch: "{{ .Inputs.data[0:500000] }}"
          
          - id: split_2
            uses: batch_processor
            inputs:
              batch: "{{ .Inputs.data[500000:] }}"
      
      - when: ".Inputs.priority == 'high'"
        uses: fast_processor
        inputs:
          data: "{{ .Inputs.data }}"
      
      - default:
        uses: standard_processor
        inputs:
          data: "{{ .Inputs.data }}"
    
    transitions:
      - to: aggregation
        when: ".Outputs != null"
```

### Full Composability

Any composition type can contain any other type, enabling complex nested structures:

```yaml
states:
  complex_processing:
    sequential:
      - id: prepare
        parallel:  # Parallel within sequential
          - id: clean
            uses: data_cleaner
            inputs:
              data: "{{ .Inputs.raw_data }}"
          
          - id: validate
            uses: data_validator
            inputs:
              data: "{{ .Inputs.raw_data }}"
      
      - id: process
        conditional:  # Conditional within sequential
          - when: ".Steps.prepare.outputs.validate.is_valid"
            sequential:  # Sequential within conditional within sequential
              - id: ml_process
                uses: ml_pipeline
                inputs:
                  data: "{{ .Steps.prepare.outputs.clean.data }}"
              
              - id: postprocess
                conditional:  # Another conditional nested deeper
                  - when: ".Steps.ml_process.outputs.confidence > 0.9"
                    uses: high_confidence_handler
                    inputs:
                      result: "{{ .Steps.ml_process.outputs }}"
                  - default:
                    uses: manual_review
                    inputs:
                      result: "{{ .Steps.ml_process.outputs }}"
          
          - default:
            uses: error_handler
            inputs:
              error: "Validation failed"
    
    transitions:
      - to: complete
        when: ".Outputs.process != null"

### Reference Syntax

Reference syntax follows the existing CEL variable patterns:

```yaml
# Within a state, use standard template patterns:
inputs:
  # Step outputs within current state
  data: "{{ .Steps.transform.outputs.result }}"
  
  # Nested step outputs
  cleaned: "{{ .Steps.prepare.outputs.clean.data }}"
  
  # State outputs from previous states
  previous: "{{ .States.preparation.outputs.extract_text.content }}"
  
  # Current state inputs
  original: "{{ .Inputs.document }}"
  
  # Recipe context
  user: "{{ .Context.user_id }}"

# In CEL expressions (transitions, when conditions):
transitions:
  - to: next_state
    when: ".Outputs.validate.is_valid && .Outputs.process.score > 0.8"
  
  # For conditional compositions, the selected branch output is in .Outputs
  - to: success
    when: ".Outputs.success == true"
```

### Complete Example: Advanced Document Processing Pipeline

```yaml
activities:
  - name: intelligent_document_processor
    implementation:
      type: state_machine
      config:
        initial_state: intake
        states:
          intake:
            conditional:
              - when: ".Inputs.document_type == 'structured'"
                parallel:
                  - id: extract_fields
                    uses: field_extractor
                    inputs:
                      doc: "{{ .Inputs.document }}"
                  
                  - id: validate_schema
                    uses: schema_validator
                    inputs:
                      doc: "{{ .Inputs.document }}"
              
              - when: ".Inputs.document_type == 'unstructured'"
                sequential:
                  - id: detect_language
                    uses: language_detector
                    inputs:
                      doc: "{{ .Inputs.document }}"
                  
                  - id: extract_content
                    conditional:
                      - when: ".Steps.detect_language.outputs.language == 'en'"
                        uses: english_extractor
                      - when: ".Steps.detect_language.outputs.language == 'es'"
                        uses: spanish_extractor
                      - default:
                        uses: universal_extractor
                    inputs:
                      doc: "{{ .Inputs.document }}"
              
              - default:
                uses: auto_classifier
                inputs:
                  doc: "{{ .Inputs.document }}"
            
            transitions:
              - to: processing
                when: ".Outputs != null"
          
          processing:
            sequential:
              - id: prepare_data
                parallel:
                  - id: clean
                    uses: data_cleaner
                    inputs:
                      data: "{{ .States.intake.outputs }}"
                  
                  - id: normalize
                    uses: data_normalizer
                    inputs:
                      data: "{{ .States.intake.outputs }}"
                  
                  - id: enhance_metadata
                    sequential:
                      - id: extract_entities
                        uses: entity_extractor
                        inputs:
                          data: "{{ .States.intake.outputs }}"
                      
                      - id: link_entities
                        uses: entity_linker
                        inputs:
                          entities: "{{ .Steps.extract_entities.outputs.entities }}"
              
              - id: analyze
                conditional:
                  - when: ".Inputs.analysis_depth == 'deep'"
                    parallel:
                      - id: sentiment
                        uses: sentiment_analyzer
                        inputs:
                          text: "{{ .Steps.prepare_data.outputs.clean.text }}"
                      
                      - id: topics
                        uses: topic_modeler
                        inputs:
                          text: "{{ .Steps.prepare_data.outputs.clean.text }}"
                      
                      - id: ml_pipeline
                        sequential:
                          - id: feature_extract
                            uses: feature_extractor
                            inputs:
                              data: "{{ .Steps.prepare_data.outputs.normalize.data }}"
                          
                          - id: classify
                            uses: ml_classifier
                            inputs:
                              features: "{{ .Steps.feature_extract.outputs.features }}"
                          
                          - id: confidence_check
                            conditional:
                              - when: ".Steps.classify.outputs.confidence > 0.95"
                                uses: high_confidence_processor
                              - when: ".Steps.classify.outputs.confidence > 0.7"
                                uses: medium_confidence_processor
                              - default:
                                parallel:
                                  - id: human_review
                                    uses: send_to_review_queue
                                  - id: uncertainty_sampling
                                    uses: active_learning_sampler
                            inputs:
                              classification: "{{ .Steps.classify.outputs.result }}"
                  
                  - when: ".Inputs.analysis_depth == 'quick'"
                    uses: fast_analyzer
                    inputs:
                      data: "{{ .Steps.prepare_data.outputs.clean.text }}"
                  
                  - default:
                    sequential:
                      - id: basic_analysis
                        uses: standard_analyzer
                        inputs:
                          data: "{{ .Steps.prepare_data.outputs.clean.text }}"
                      
                      - id: should_enhance
                        conditional:
                          - when: ".Steps.basic_analysis.outputs.complexity_score > 0.8"
                            uses: enhanced_analyzer
                            inputs:
                              data: "{{ .Steps.prepare_data.outputs.clean.text }}"
                              initial: "{{ .Steps.basic_analysis.outputs.results }}"
                          - default:
                            uses: finalize_basic
                            inputs:
                              results: "{{ .Steps.basic_analysis.outputs.results }}"
            
            transitions:
              - to: quality_assurance
                when: ".Outputs.analyze != null"
              - to: error_handling
                when: ".Outputs.analyze.error != null"
          
          quality_assurance:
            parallel:
              - id: validate_results
                sequential:
                  - id: schema_check
                    uses: result_schema_validator
                    inputs:
                      results: "{{ states.processing.analyze }}"
                  
                  - id: business_rules
                    uses: business_rule_engine
                    inputs:
                      results: "{{ states.processing.analyze }}"
                      rules: "{{ inputs.business_rules }}"
              
              - id: generate_outputs
                conditional:
                  - when: "inputs.output_format == 'report'"
                    sequential:
                      - id: generate_report
                        uses: report_generator
                        inputs:
                          data: "{{ states.processing.analyze }}"
                      
                      - id: add_visualizations
                        parallel:
                          - id: charts
                            uses: chart_generator
                            inputs:
                              data: "{{ states.processing.analyze }}"
                          
                          - id: tables
                            uses: table_generator
                            inputs:
                              data: "{{ states.processing.analyze }}"
                  
                  - when: "inputs.output_format == 'api'"
                    uses: api_formatter
                    inputs:
                      data: "{{ states.processing.analyze }}"
                  
                  - default:
                    uses: json_formatter
                    inputs:
                      data: "{{ states.processing.analyze }}"
            
            transitions:
              - to: delivery
                when: "validate_results.business_rules.passed == true"
              - to: manual_review
                when: "validate_results.business_rules.requires_review == true"
              - to: rejection
                when: "validate_results.business_rules.failed == true"
          
          delivery:
            terminal: true
            outputs:
              result: "{{ states.quality_assurance.generate_outputs }}"
              metadata:
                processing_time: "{{ context.elapsed_time }}"
                confidence: "{{ states.processing.analyze.confidence }}"
                validations: "{{ states.quality_assurance.validate_results }}"
```

## Implementation

### Type Updates

```go
// StateDefinition extends existing structure to support compositions
type StateDefinition struct {
    // Single activity (backward compatible)
    Uses         string                 `json:"uses,omitempty"`
    Config       map[string]interface{} `json:"config,omitempty"`
    
    // NEW: Composition types (mutually exclusive with Uses)
    Sequential   []Step                 `json:"sequential,omitempty"`
    Parallel     []Step                 `json:"parallel,omitempty"`
    Conditional  []ConditionalBranch    `json:"conditional,omitempty"`
    
    // Existing fields (unchanged)
    Terminal     bool                   `json:"terminal,omitempty"`
    Error        string                 `json:"error,omitempty"`
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
    Outputs      map[string]interface{} `json:"outputs,omitempty"`
    Retry        *StateRetryPolicy      `json:"retry,omitempty"`
    Transitions  []TransitionSpec       `json:"transitions,omitempty"`
}

// TransitionSpec remains unchanged
type TransitionSpec struct {
    To   string `json:"to"`
    When string `json:"when"` // CEL expression
}

// StateRetryPolicy remains unchanged
type StateRetryPolicy struct {
    When               string  `json:"when,omitempty"`
    MaxAttempts        int     `json:"max_attempts"`
    BackoffCoefficient float64 `json:"backoff_coefficient,omitempty"`
    InitialInterval    string  `json:"initial_interval,omitempty"`
}

// Step can itself be a composition or a simple activity
type Step struct {
    ID           string                 `json:"id"`
    
    // Simple activity
    Uses         string                 `json:"uses,omitempty"`
    
    // OR nested compositions (mutually exclusive)
    Sequential   []Step                 `json:"sequential,omitempty"`
    Parallel     []Step                 `json:"parallel,omitempty"`
    Conditional  []ConditionalBranch    `json:"conditional,omitempty"`
    
    // Step configuration
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
    DependsOn    []string               `json:"depends_on,omitempty"`
    When         string                 `json:"when,omitempty"` // CEL condition for step execution
    Retry        *RetryPolicy           `json:"retry,omitempty"`
}

// ConditionalBranch represents a branch in conditional logic
type ConditionalBranch struct {
    When         string                 `json:"when,omitempty"` // CEL condition (omit for default)
    Default      bool                   `json:"default,omitempty"` // Mark as default branch
    
    // Branch can be activity or composition
    Uses         string                 `json:"uses,omitempty"`
    Sequential   []Step                 `json:"sequential,omitempty"`
    Parallel     []Step                 `json:"parallel,omitempty"`
    Conditional  []ConditionalBranch    `json:"conditional,omitempty"`
    
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
}
```

### Execution Logic

```go
func (s *StateMachineCompiler) executeState(ctx context.Context, state StateDefinition, stateCtx *StateContext) (map[string]interface{}, error) {
    // Terminal states return configured outputs
    if state.Terminal {
        return state.Outputs, nil
    }
    
    // Determine execution type and delegate
    if state.Uses != "" {
        // Simple activity execution
        return s.executeActivity(ctx, state.Uses, prepareInputs(state.Inputs, stateCtx))
    } else if state.Sequential != nil {
        // Sequential composition
        return s.executeSequential(ctx, state.Sequential, stateCtx)
    } else if state.Parallel != nil {
        // Parallel composition
        return s.executeParallel(ctx, state.Parallel, stateCtx)
    } else if state.Conditional != nil {
        // Conditional composition
        return s.executeConditional(ctx, state.Conditional, stateCtx)
    }
    
    return nil, fmt.Errorf("state must specify 'uses', 'sequential', 'parallel', or 'conditional'")
}

func (s *StateMachineCompiler) executeStep(ctx context.Context, step Step, outputs map[string]interface{}, stateCtx *StateContext) (interface{}, error) {
    // Check conditional execution
    if step.When != "" && !s.evaluateCEL(step.When, outputs, stateCtx) {
        return nil, nil // Skip this step
    }
    
    // Prepare step context with current outputs
    stepCtx := s.createStepContext(step, outputs, stateCtx)
    
    // Execute based on step type
    var result interface{}
    var err error
    
    if step.Uses != "" {
        // Simple activity
        result, err = s.executeActivity(ctx, step.Uses, prepareInputs(step.Inputs, stepCtx))
    } else if step.Sequential != nil {
        // Nested sequential
        result, err = s.executeSequential(ctx, step.Sequential, stepCtx)
    } else if step.Parallel != nil {
        // Nested parallel
        result, err = s.executeParallel(ctx, step.Parallel, stepCtx)
    } else if step.Conditional != nil {
        // Nested conditional
        result, err = s.executeConditional(ctx, step.Conditional, stepCtx)
    }
    
    // Handle retry if needed
    if err != nil && step.Retry != nil {
        result, err = s.retryStep(ctx, step, outputs, stateCtx)
    }
    
    return result, err
}

func (s *StateMachineCompiler) executeSequential(ctx context.Context, steps []Step, stateCtx *StateContext) (map[string]interface{}, error) {
    outputs := make(map[string]interface{})
    
    for _, step := range steps {
        result, err := s.executeStep(ctx, step, outputs, stateCtx)
        if err != nil {
            return nil, fmt.Errorf("step '%s' failed: %w", step.ID, err)
        }
        
        if step.ID != "" && result != nil {
            outputs[step.ID] = result
        }
    }
    
    return outputs, nil
}

func (s *StateMachineCompiler) executeParallel(ctx context.Context, steps []Step, stateCtx *StateContext) (map[string]interface{}, error) {
    // Group by dependencies
    groups := s.groupByDependencies(steps)
    outputs := make(map[string]interface{})
    mu := &sync.Mutex{}
    
    for _, group := range groups {
        var wg sync.WaitGroup
        errors := make(chan error, len(group))
        
        for _, step := range group {
            wg.Add(1)
            go func(step Step) {
                defer wg.Done()
                
                // Check dependencies are met
                if !s.dependenciesMet(step.DependsOn, outputs) {
                    errors <- fmt.Errorf("dependencies not met for step '%s'", step.ID)
                    return
                }
                
                result, err := s.executeStep(ctx, step, outputs, stateCtx)
                if err != nil {
                    errors <- err
                    return
                }
                
                mu.Lock()
                if step.ID != "" && result != nil {
                    outputs[step.ID] = result
                }
                mu.Unlock()
            }(step)
        }
        
        wg.Wait()
        close(errors)
        
        // Check for errors
        for err := range errors {
            if err != nil {
                return nil, err
            }
        }
    }
    
    return outputs, nil
}

func (s *StateMachineCompiler) executeConditional(ctx context.Context, branches []ConditionalBranch, stateCtx *StateContext) (interface{}, error) {
    for _, branch := range branches {
        // Check condition (default branch has no condition)
        if branch.Default || (branch.When != "" && s.evaluateCEL(branch.When, nil, stateCtx)) {
            // Execute the selected branch
            if branch.Uses != "" {
                return s.executeActivity(ctx, branch.Uses, prepareInputs(branch.Inputs, stateCtx))
            } else if branch.Sequential != nil {
                return s.executeSequential(ctx, branch.Sequential, stateCtx)
            } else if branch.Parallel != nil {
                return s.executeParallel(ctx, branch.Parallel, stateCtx)
            } else if branch.Conditional != nil {
                return s.executeConditional(ctx, branch.Conditional, stateCtx)
            }
            
            return nil, fmt.Errorf("conditional branch must specify execution type")
        }
    }
    
    return nil, fmt.Errorf("no conditional branch matched")
}
```

### CEL Variable Access

CEL expressions use simplified dot notation for clean references:

```go
// Available variables in CEL expressions
type CELContext struct {
    // Direct access to current scope outputs
    outputs     map[string]interface{}  // Current state/step outputs
    
    // Named references
    inputs      interface{}             // Current inputs
    states      map[string]interface{}  // Previous state outputs
    context     map[string]interface{}  // Recipe context
    
    // Within compositions, step outputs by ID
    // Accessed directly by step ID: stepId.field
}
```

### Examples

#### Data Pipeline with Conditional Parallel Processing

```yaml
states:
  data_processing:
    conditional:
      - when: ".Inputs.batch_count > 1"
        parallel:
          - id: batch1
            uses: batch_processor
            inputs:
              data: "{{ .Inputs.batches[0] }}"
          
          - id: batch2
            uses: batch_processor
            inputs:
              data: "{{ .Inputs.batches[1] }}"
            when: ".Inputs.batch_count >= 2"
          
          - id: batch3
            uses: batch_processor
            inputs:
              data: "{{ .Inputs.batches[2] }}"
            when: ".Inputs.batch_count >= 3"
      
      - default:
        uses: single_processor
        inputs:
          data: "{{ .Inputs.batches[0] }}"
    
    transitions:
      - to: aggregation
        when: ".Outputs != null"
```

#### Conditional Processing with Nested Fallbacks

```yaml
states:
  smart_processing:
    sequential:
      - id: analyze_input
        uses: input_analyzer
        inputs:
          data: "{{ .Inputs.data }}"
      
      - id: process
        conditional:
          - when: ".Steps.analyze_input.outputs.complexity == 'simple'"
            uses: fast_processor
            inputs:
              data: "{{ .Inputs.data }}"
          
          - when: ".Steps.analyze_input.outputs.complexity == 'medium'"
            sequential:
              - id: preprocess
                uses: data_preprocessor
                inputs:
                  data: "{{ .Inputs.data }}"
              
              - id: main_process
                uses: standard_processor
                inputs:
                  data: "{{ .Steps.preprocess.outputs.cleaned_data }}"
          
          - when: ".Steps.analyze_input.outputs.complexity == 'complex'"
            parallel:
              - id: decompose
                uses: data_decomposer
                inputs:
                  data: "{{ .Inputs.data }}"
              
              - id: analyze_patterns
                uses: pattern_analyzer
                inputs:
                  data: "{{ .Inputs.data }}"
              
              - id: ml_process
                conditional:
                  - when: ".Steps.analyze_input.outputs.ml_suitable"
                    uses: ml_pipeline
                  - default:
                    uses: heuristic_processor
                inputs:
                  data: "{{ .Inputs.data }}"
          
          - default:
            uses: fallback_processor
            inputs:
              data: "{{ .Inputs.data }}"
      
      - id: validate
        uses: result_validator
        inputs:
          result: "{{ .Steps.process.outputs }}"
    
    transitions:
      - to: success
        when: ".Outputs.validate.is_valid"
      - to: retry
        when: "!.Outputs.validate.is_valid && .Outputs.validate.can_retry"
      - to: failure
        when: "!.Outputs.validate.is_valid && !.Outputs.validate.can_retry"
```

#### Deep Nesting Example

```yaml
states:
  orchestration:
    parallel:
      - id: stream_a
        sequential:
          - id: fetch
            uses: data_fetcher
            inputs:
              source: "{{ .Inputs.source_a }}"
          
          - id: transform
            conditional:
              - when: ".Steps.fetch.outputs.format == 'json'"
                uses: json_transformer
              - when: ".Steps.fetch.outputs.format == 'xml'"
                uses: xml_transformer
              - default:
                parallel:
                  - id: detect_format
                    uses: format_detector
                  - id: parse_raw
                    uses: raw_parser
            inputs:
              data: "{{ .Steps.fetch.outputs.data }}"
      
      - id: stream_b
        conditional:
          - when: ".Inputs.enable_stream_b"
            sequential:
              - id: connect
                uses: stream_connector
                inputs:
                  endpoint: "{{ .Inputs.endpoint_b }}"
              
              - id: process_stream
                parallel:
                  - id: realtime
                    uses: realtime_processor
                  - id: batch
                    sequential:
                      - id: buffer
                        uses: stream_buffer
                      - id: batch_process
                        uses: batch_processor
                inputs:
                  stream: "{{ .Steps.connect.outputs.stream }}"
          
          - default:
            uses: noop
    
    transitions:
      - to: merge_results
        when: ".Outputs.stream_a != null"
```

## Migration Strategy

1. **Phase 1**: Implement new types while maintaining backward compatibility
2. **Phase 2**: Migrate existing state machines to use new composition features
3. **Phase 3**: Move implementation from activity to compiler
4. **Phase 4**: Deprecate old single-activity-only states

## Testing Requirements

1. **Unit Tests**: 
   - Each composition type (sequential, parallel, conditional)
   - Nested compositions at multiple levels
   - CEL expression evaluation in all contexts
   
2. **Integration Tests**:
   - Complex multi-level compositions
   - State transitions with composed outputs
   - Error handling and retry logic in compositions
   
3. **Performance Tests**:
   - Parallel execution efficiency
   - Deep nesting performance
   - Large state machine execution

