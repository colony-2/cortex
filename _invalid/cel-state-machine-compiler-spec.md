# State Machine with Workflow Composition - Compiler Extension

## Overview

This specification extends the recipe compiler to support state machines with workflow composition as a first-class execution type, alongside existing `sequential` and `parallel` execution types.

## Motivation

Current recipe execution types:
- `sequential`: Execute steps one after another
- `parallel`: Execute steps concurrently

Missing capability:
- **Conditional workflows**: Execute different steps based on runtime conditions
- **Retry loops**: Repeat steps with different conditions
- **State-based execution**: Complex workflows with multiple phases and transitions

## Design

### Recipe YAML Extension

Add `state_machine` as a new execution type alongside `sequential` and `parallel`:

```yaml
# Current recipe format
recipe:
  name: document_processing
  inputs:
    document: string
  
  # NEW: State machine execution type
  execution:
    type: state_machine
    config:
      initial_state: preparation
      states:
        preparation:
          # Single activity (current pattern)
          uses: extract_text
          inputs:
            document: "{{ .Inputs.document }}"
          transitions:
            - to: enrichment
              when: ".Outputs.success == true"
        
        enrichment:
          # NEW: Workflow composition within state
          workflow:
            type: parallel
            steps:
              - id: classify
                uses: document_classifier
                inputs:
                  text: "{{ .States.preparation.outputs.text }}"
              
              - id: summarize
                uses: summarizer
                inputs:
                  text: "{{ .States.preparation.outputs.text }}"
          
          transitions:
            - to: finalize
              when: ".Outputs.classify.confidence > 0.8"
        
        finalize:
          terminal: true
          outputs:
            result: "{{ .States.enrichment.outputs }}"
```

### Compiler Architecture Extension

#### New Types in `recipe-core/pkg/yaml`

```go
// ExecutionConfig defines how the recipe executes
type ExecutionConfig struct {
    Type             ExecutionType           `yaml:"type"`
    Steps            []Step                  `yaml:"steps,omitempty"`      // For sequential/parallel
    StateMachineSpec *StateMachineSpec       `yaml:"config,omitempty"`     // For state_machine
}

type ExecutionType string

const (
    ExecutionTypeSequential   ExecutionType = "sequential"
    ExecutionTypeParallel     ExecutionType = "parallel"
    ExecutionTypeStateMachine ExecutionType = "state_machine" // NEW
)

// StateMachineSpec defines the state machine configuration
type StateMachineSpec struct {
    InitialState string                    `yaml:"initial_state"`
    States       map[string]StateSpec      `yaml:"states"`
    Timeout      string                    `yaml:"timeout,omitempty"`
}

// StateSpec defines a single state
type StateSpec struct {
    // Single activity mode (existing pattern)
    Uses        string                 `yaml:"uses,omitempty"`
    Config      map[string]interface{} `yaml:"config,omitempty"`
    
    // Workflow composition mode (NEW)
    Workflow    *WorkflowSpec          `yaml:"workflow,omitempty"`
    
    // State properties
    Terminal    bool                   `yaml:"terminal,omitempty"`
    Error       string                 `yaml:"error,omitempty"`
    Inputs      map[string]interface{} `yaml:"inputs,omitempty"`
    Outputs     map[string]interface{} `yaml:"outputs,omitempty"`
    Retry       *RetryPolicy           `yaml:"retry,omitempty"`
    Transitions []TransitionSpec       `yaml:"transitions,omitempty"`
}

// WorkflowSpec defines workflow composition within a state
type WorkflowSpec struct {
    Type    ExecutionType `yaml:"type"`     // sequential or parallel
    Steps   []Step        `yaml:"steps"`    // Reuse existing Step type!
    Timeout string        `yaml:"timeout,omitempty"`
}

// TransitionSpec defines state transitions with CEL expressions
type TransitionSpec struct {
    To   string `yaml:"to"`
    When string `yaml:"when"` // CEL expression
}
```

#### Compiler Extension in `recipe-worker/pkg/compiler`

```go
// Extend existing Compiler to handle state machines
func (c *Compiler) ExecuteWorkflow(ctx workflow.Context, def *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
    state := &WorkflowState{
        Inputs:  inputs,
        Steps:   make(map[string]StepResult),
        Outputs: make(map[string]interface{}),
    }

    switch def.Execution.Type {
    case yamlpkg.ExecutionTypeSequential:
        return c.executeSequentialSteps(ctx, def.Execution.Steps, state)
    case yamlpkg.ExecutionTypeParallel:
        return c.executeParallelSteps(ctx, def.Execution.Steps, state)
    case yamlpkg.ExecutionTypeStateMachine: // NEW
        return c.executeStateMachine(ctx, def.Execution.StateMachineSpec, state)
    }
}

// NEW: State machine execution
func (c *Compiler) executeStateMachine(ctx workflow.Context, spec *yamlpkg.StateMachineSpec, state *WorkflowState) (map[string]interface{}, error) {
    sm := &StateMachineExecutor{
        compiler: c,
        celEnv:   c.createCELEnvironment(), // NEW: CEL integration
    }
    
    return sm.Execute(ctx, spec, state)
}
```

#### State Machine Executor (NEW)

```go
// StateMachineExecutor handles state machine execution within the compiler
type StateMachineExecutor struct {
    compiler *Compiler
    celEnv   *cel.Env
}

// Execute runs the state machine
func (sm *StateMachineExecutor) Execute(ctx workflow.Context, spec *yamlpkg.StateMachineSpec, state *WorkflowState) (map[string]interface{}, error) {
    // Add state machine context to workflow state
    state.StateMachine = &StateMachineState{
        CurrentState: spec.InitialState,
        States:       make(map[string]map[string]interface{}),
        StateInfo:    make(map[string]*StateInfo),
    }

    // Execute state machine loop
    for !sm.isTerminalState(state.StateMachine.CurrentState, spec.States) {
        stateSpec := spec.States[state.StateMachine.CurrentState]
        
        // Execute state (single activity OR workflow composition)
        outputs, err := sm.executeState(ctx, stateSpec, state)
        if err != nil {
            return nil, err
        }
        
        // Store state outputs
        state.StateMachine.States[state.StateMachine.CurrentState] = outputs
        
        // Evaluate transitions using CEL
        nextState, err := sm.evaluateTransitions(stateSpec.Transitions, outputs, state)
        if err != nil {
            return nil, err
        }
        
        state.StateMachine.CurrentState = nextState
    }
    
    // Handle terminal state
    terminalSpec := spec.States[state.StateMachine.CurrentState]
    if terminalSpec.Error != "" {
        return nil, fmt.Errorf(terminalSpec.Error)
    }
    
    return terminalSpec.Outputs, nil
}

// executeState handles both single activity and workflow composition
func (sm *StateMachineExecutor) executeState(ctx workflow.Context, stateSpec yamlpkg.StateSpec, state *WorkflowState) (map[string]interface{}, error) {
    if stateSpec.Workflow != nil {
        // NEW: Execute workflow composition within state
        return sm.executeStateWorkflow(ctx, stateSpec.Workflow, state)
    } else if stateSpec.Uses != "" {
        // Existing: Execute single activity
        return sm.executeSingleActivity(ctx, stateSpec, state)
    }
    
    return nil, fmt.Errorf("state must specify either 'uses' or 'workflow'")
}

// executeStateWorkflow leverages existing compiler step execution
func (sm *StateMachineExecutor) executeStateWorkflow(ctx workflow.Context, workflowSpec *yamlpkg.WorkflowSpec, state *WorkflowState) (map[string]interface{}, error) {
    // Create isolated step state for this workflow
    workflowState := &WorkflowState{
        Inputs:       state.Inputs,
        Steps:        make(map[string]StepResult), // Isolated from main workflow
        Outputs:      make(map[string]interface{}),
        StateMachine: state.StateMachine,          // Share state machine context
    }
    
    // REUSE existing compiler execution logic!
    switch workflowSpec.Type {
    case yamlpkg.ExecutionTypeSequential:
        err := sm.compiler.executeSequentialSteps(ctx, workflowSpec.Steps, workflowState, defaultRetryPolicy)
        return workflowState.Steps, err // Return all step outputs
    case yamlpkg.ExecutionTypeParallel:
        err := sm.compiler.executeParallelSteps(ctx, workflowSpec.Steps, workflowState, defaultRetryPolicy)
        return workflowState.Steps, err // Return all step outputs
    }
    
    return nil, fmt.Errorf("unsupported workflow type: %s", workflowSpec.Type)
}
```

#### Extended WorkflowState

```go
// Extend existing WorkflowState to include state machine context
type WorkflowState struct {
    Inputs       map[string]interface{}
    Steps        map[string]StepResult
    Outputs      map[string]interface{}
    StateMachine *StateMachineState    // NEW: State machine context
}

// NEW: State machine context
type StateMachineState struct {
    CurrentState string
    States       map[string]map[string]interface{} // Previous state outputs
    StateInfo    map[string]*StateInfo
}

type StateInfo struct {
    Name      string
    Attempts  int
    EnteredAt time.Time
}
```

#### Enhanced Template Resolution

```go
// Extend existing TemplateResolver to include state machine variables
func (r *TemplateResolver) getTemplateData() map[string]interface{} {
    data := make(map[string]interface{})
    
    // Existing variables
    data["Inputs"] = r.state.Inputs
    data["Steps"] = r.buildStepsData()
    data["Env"] = map[string]string{}
    data["Context"] = r.buildContextData()
    
    // NEW: State machine variables (if in state machine context)
    if r.state.StateMachine != nil {
        data["States"] = r.state.StateMachine.States
        data["State"] = r.state.StateMachine.StateInfo[r.state.StateMachine.CurrentState]
    }
    
    return data
}
```

## Complete Example

```yaml
recipe:
  name: document_processing_pipeline
  inputs:
    document: string
    quality_threshold: number
  
  execution:
    type: state_machine
    config:
      initial_state: preparation
      states:
        preparation:
          # Parallel data extraction within state
          workflow:
            type: parallel
            steps:
              - id: extract_text
                uses: text_extractor
                inputs:
                  document: "{{ .Inputs.document }}"
              
              - id: extract_metadata
                uses: metadata_extractor
                inputs:
                  document: "{{ .Inputs.document }}"
              
              - id: extract_images
                uses: image_extractor
                inputs:
                  document: "{{ .Inputs.document }}"
          
          transitions:
            - to: enrichment
              when: ".Outputs.extract_text.success == true"
            - to: error_handling
              when: ".Outputs.extract_text.success == false"
        
        enrichment:
          # Sequential enrichment pipeline within state
          workflow:
            type: sequential
            steps:
              - id: classify
                uses: document_classifier
                inputs:
                  text: "{{ .States.preparation.outputs.extract_text.text }}"
                  metadata: "{{ .States.preparation.outputs.extract_metadata.metadata }}"
              
              - id: enrich
                uses: llm_enrichment
                inputs:
                  text: "{{ .States.preparation.outputs.extract_text.text }}"
                  classification: "{{ .Steps.classify.outputs.category }}"
              
              - id: quality_check
                uses: quality_validator
                inputs:
                  enriched_content: "{{ .Steps.enrich.outputs.content }}"
                  threshold: "{{ .Inputs.quality_threshold }}"
          
          transitions:
            - to: finalize
              when: ".Outputs.quality_check.score >= .Inputs.quality_threshold"
            - to: manual_review
              when: ".Outputs.quality_check.score < .Inputs.quality_threshold"
        
        manual_review:
          # Single activity state
          uses: human_reviewer
          inputs:
            content: "{{ .States.enrichment.outputs.enrich.content }}"
            quality_issues: "{{ .States.enrichment.outputs.quality_check.issues }}"
          
          transitions:
            - to: finalize
              when: ".Outputs.approved == true"
            - to: revision_needed
              when: ".Outputs.approved == false"
        
        finalize:
          terminal: true
          outputs:
            result: "{{ .States.enrichment.outputs.enrich.content }}"
            metadata: "{{ .States.preparation.outputs.extract_metadata.metadata }}"
            quality_score: "{{ .States.enrichment.outputs.quality_check.score }}"
        
        error_handling:
          terminal: true
          error: "Failed to extract text from document"
```

## Benefits

1. **First-Class Feature**: State machines are a core execution type, not an external activity
2. **Code Reuse**: Leverages all existing compiler infrastructure
3. **Consistent API**: Same template syntax, step execution, and patterns
4. **No Internal Dependencies**: Everything stays within the compiler package
5. **Composable**: Mix sequential, parallel, and conditional execution naturally
6. **Backward Compatible**: Existing recipes unchanged

## Implementation Impact

### Changes Required:
- **recipe-core/pkg/yaml**: Add state machine types
- **recipe-worker/pkg/compiler**: Add state machine executor
- **template resolution**: Extend for state machine variables

### No Changes Required:
- **Activity system**: No new activities needed
- **Temporal integration**: Uses existing workflow patterns
- **Recipe worker**: Uses existing compilation flow
- **Client libraries**: Same recipe execution API

This approach makes state machines a natural part of the recipe language itself, rather than a special-purpose activity.