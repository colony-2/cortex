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

States can now contain either a single activity OR a complete composition. The composition type is declared directly without a wrapper:

```yaml
states:
  # Simple activity state (backward compatible)
  simple_state:
    uses: my_activity
    transitions:
      - to: next_state
        when: "outputs.result == 'success'"
  
  # Parallel composition state
  data_preparation:
    parallel:
      - id: fetch_data
        uses: fetch_from_api
        inputs:
          endpoint: "{{ inputs.api_url }}"
      
      - id: load_cache
        uses: load_from_cache
        inputs:
          key: "{{ inputs.cache_key }}"
      
      - id: validate
        uses: validate_data
        inputs:
          data: "{{ fetch_data.data }}"
        depends_on: [fetch_data]
    
    transitions:
      - to: processing
        when: "validate.valid == true"
      - to: error
        when: "validate.valid == false"
  
  # Sequential composition state
  processing:
    sequential:
      - id: transform
        uses: data_transformer
        inputs:
          data: "{{ states.data_preparation.fetch_data.data }}"
      
      - id: enrich
        uses: data_enricher
        inputs:
          data: "{{ transform.result }}"
      
      - id: save
        uses: data_saver
        inputs:
          data: "{{ enrich.enriched_data }}"
    
    transitions:
      - to: complete
        when: "save.success == true"
  
  # Conditional composition state
  decision_point:
    conditional:
      - when: "inputs.data_size > 1000000"
        parallel:  # Nested parallel in conditional
          - id: split_1
            uses: batch_processor
            inputs:
              batch: "{{ inputs.data[0:500000] }}"
          
          - id: split_2
            uses: batch_processor
            inputs:
              batch: "{{ inputs.data[500000:] }}"
      
      - when: "inputs.priority == 'high'"
        uses: fast_processor
        inputs:
          data: "{{ inputs.data }}"
      
      - default:
        uses: standard_processor
        inputs:
          data: "{{ inputs.data }}"
    
    transitions:
      - to: aggregation
        when: "outputs != null"
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
          
          - id: validate
            uses: data_validator
      
      - id: process
        conditional:  # Conditional within sequential
          - when: "prepare.validate.is_valid"
            sequential:  # Sequential within conditional within sequential
              - id: ml_process
                uses: ml_pipeline
                inputs:
                  data: "{{ prepare.clean.data }}"
              
              - id: postprocess
                conditional:  # Another conditional nested deeper
                  - when: "ml_process.confidence > 0.9"
                    uses: high_confidence_handler
                  - default:
                    uses: manual_review
          
          - default:
            uses: error_handler
```

### Reference Syntax

Clean and intuitive reference syntax for accessing nested outputs:

```yaml
# Within a state, reference outputs using simple dot notation:
inputs:
  # Direct step reference (same level)
  data: "{{ transform.result }}"
  
  # Nested step reference (from composed steps)
  cleaned: "{{ prepare.clean.data }}"
  
  # State outputs from previous states
  previous: "{{ states.preparation.extract_text.content }}"
  
  # Current state inputs
  original: "{{ inputs.document }}"
  
  # Recipe context
  user: "{{ context.user_id }}"

# In transitions, outputs are directly accessible:
transitions:
  - to: next_state
    when: "validate.is_valid && process.score > 0.8"
  
  # For conditional compositions, the selected branch output is available
  - to: success
    when: "outputs.success == true"  # 'outputs' contains the selected branch result
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
              - when: "inputs.document_type == 'structured'"
                parallel:
                  - id: extract_fields
                    uses: field_extractor
                    inputs:
                      doc: "{{ inputs.document }}"
                  
                  - id: validate_schema
                    uses: schema_validator
                    inputs:
                      doc: "{{ inputs.document }}"
              
              - when: "inputs.document_type == 'unstructured'"
                sequential:
                  - id: detect_language
                    uses: language_detector
                    inputs:
                      doc: "{{ inputs.document }}"
                  
                  - id: extract_content
                    conditional:
                      - when: "detect_language.language == 'en'"
                        uses: english_extractor
                      - when: "detect_language.language == 'es'"
                        uses: spanish_extractor
                      - default:
                        uses: universal_extractor
                    inputs:
                      doc: "{{ inputs.document }}"
              
              - default:
                uses: auto_classifier
                inputs:
                  doc: "{{ inputs.document }}"
            
            transitions:
              - to: processing
                when: "outputs != null"
          
          processing:
            sequential:
              - id: prepare_data
                parallel:
                  - id: clean
                    uses: data_cleaner
                    inputs:
                      data: "{{ states.intake.outputs }}"
                  
                  - id: normalize
                    uses: data_normalizer
                    inputs:
                      data: "{{ states.intake.outputs }}"
                  
                  - id: enhance_metadata
                    sequential:
                      - id: extract_entities
                        uses: entity_extractor
                        inputs:
                          data: "{{ states.intake.outputs }}"
                      
                      - id: link_entities
                        uses: entity_linker
                        inputs:
                          entities: "{{ extract_entities.entities }}"
              
              - id: analyze
                conditional:
                  - when: "inputs.analysis_depth == 'deep'"
                    parallel:
                      - id: sentiment
                        uses: sentiment_analyzer
                        inputs:
                          text: "{{ prepare_data.clean.text }}"
                      
                      - id: topics
                        uses: topic_modeler
                        inputs:
                          text: "{{ prepare_data.clean.text }}"
                      
                      - id: ml_pipeline
                        sequential:
                          - id: feature_extract
                            uses: feature_extractor
                            inputs:
                              data: "{{ prepare_data.normalize.data }}"
                          
                          - id: classify
                            uses: ml_classifier
                            inputs:
                              features: "{{ feature_extract.features }}"
                          
                          - id: confidence_check
                            conditional:
                              - when: "classify.confidence > 0.95"
                                uses: high_confidence_processor
                              - when: "classify.confidence > 0.7"
                                uses: medium_confidence_processor
                              - default:
                                parallel:
                                  - id: human_review
                                    uses: send_to_review_queue
                                  - id: uncertainty_sampling
                                    uses: active_learning_sampler
                            inputs:
                              classification: "{{ classify.result }}"
                  
                  - when: "inputs.analysis_depth == 'quick'"
                    uses: fast_analyzer
                    inputs:
                      data: "{{ prepare_data.clean.text }}"
                  
                  - default:
                    sequential:
                      - id: basic_analysis
                        uses: standard_analyzer
                        inputs:
                          data: "{{ prepare_data.clean.text }}"
                      
                      - id: should_enhance
                        conditional:
                          - when: "basic_analysis.complexity_score > 0.8"
                            uses: enhanced_analyzer
                            inputs:
                              data: "{{ prepare_data.clean.text }}"
                              initial: "{{ basic_analysis.results }}"
                          - default:
                            uses: finalize_basic
                            inputs:
                              results: "{{ basic_analysis.results }}"
            
            transitions:
              - to: quality_assurance
                when: "analyze.outputs != null"
              - to: error_handling
                when: "analyze.error != null"
          
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
// StateDefinition now supports direct composition types
type StateDefinition struct {
    // Single activity (backward compatible)
    Uses         string                 `json:"uses,omitempty"`
    
    // Composition types (mutually exclusive)
    Sequential   []Step                 `json:"sequential,omitempty"`
    Parallel     []Step                 `json:"parallel,omitempty"`
    Conditional  []ConditionalBranch    `json:"conditional,omitempty"`
    
    // Common fields
    Terminal     bool                   `json:"terminal,omitempty"`
    Error        string                 `json:"error,omitempty"`
    Inputs       map[string]interface{} `json:"inputs,omitempty"`
    Outputs      map[string]interface{} `json:"outputs,omitempty"`
    Retry        *StateRetryPolicy      `json:"retry,omitempty"`
    Transitions  []TransitionSpec       `json:"transitions,omitempty"`
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
      - when: "inputs.batch_count > 1"
        parallel:
          - id: batch1
            uses: batch_processor
            inputs:
              data: "{{ inputs.batches[0] }}"
          
          - id: batch2
            uses: batch_processor
            inputs:
              data: "{{ inputs.batches[1] }}"
            when: "inputs.batch_count >= 2"
          
          - id: batch3
            uses: batch_processor
            inputs:
              data: "{{ inputs.batches[2] }}"
            when: "inputs.batch_count >= 3"
      
      - default:
        uses: single_processor
        inputs:
          data: "{{ inputs.batches[0] }}"
    
    transitions:
      - to: aggregation
        when: "outputs != null"
```

#### Conditional Processing with Nested Fallbacks

```yaml
states:
  smart_processing:
    sequential:
      - id: analyze_input
        uses: input_analyzer
        inputs:
          data: "{{ inputs.data }}"
      
      - id: process
        conditional:
          - when: "analyze_input.complexity == 'simple'"
            uses: fast_processor
            inputs:
              data: "{{ inputs.data }}"
          
          - when: "analyze_input.complexity == 'medium'"
            sequential:
              - id: preprocess
                uses: data_preprocessor
                inputs:
                  data: "{{ inputs.data }}"
              
              - id: main_process
                uses: standard_processor
                inputs:
                  data: "{{ preprocess.cleaned_data }}"
          
          - when: "analyze_input.complexity == 'complex'"
            parallel:
              - id: decompose
                uses: data_decomposer
                inputs:
                  data: "{{ inputs.data }}"
              
              - id: analyze_patterns
                uses: pattern_analyzer
                inputs:
                  data: "{{ inputs.data }}"
              
              - id: ml_process
                conditional:
                  - when: "analyze_input.ml_suitable"
                    uses: ml_pipeline
                  - default:
                    uses: heuristic_processor
                inputs:
                  data: "{{ inputs.data }}"
          
          - default:
            uses: fallback_processor
            inputs:
              data: "{{ inputs.data }}"
      
      - id: validate
        uses: result_validator
        inputs:
          result: "{{ process }}"
    
    transitions:
      - to: success
        when: "validate.is_valid"
      - to: retry
        when: "!validate.is_valid && validate.can_retry"
      - to: failure
        when: "!validate.is_valid && !validate.can_retry"
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
              source: "{{ inputs.source_a }}"
          
          - id: transform
            conditional:
              - when: "fetch.format == 'json'"
                uses: json_transformer
              - when: "fetch.format == 'xml'"
                uses: xml_transformer
              - default:
                parallel:
                  - id: detect_format
                    uses: format_detector
                  - id: parse_raw
                    uses: raw_parser
            inputs:
              data: "{{ fetch.data }}"
      
      - id: stream_b
        conditional:
          - when: "inputs.enable_stream_b"
            sequential:
              - id: connect
                uses: stream_connector
                inputs:
                  endpoint: "{{ inputs.endpoint_b }}"
              
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
                  stream: "{{ connect.stream }}"
          
          - default:
            uses: noop
    
    transitions:
      - to: merge_results
        when: "stream_a != null"
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

