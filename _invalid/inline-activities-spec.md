# Unified Activity Model Specification

## Executive Summary

This specification defines a unified model where everything in a recipe workflow is a registerable activity. There are no special concepts - state machines, recipes, LLM personas, and standard activities all follow the same pattern. Activities can be defined inline with their configuration or referenced from shared definitions.

## Motivation

### Current Problems
1. **Cognitive Overhead**: Developers must define activities in one section and reference them in another
2. **Conceptual Proliferation**: Too many concepts (activities, recipes, state machines, personas, functions, HTTP calls)
3. **Inconsistent Patterns**: Different syntax for different activity types
4. **Navigation Complexity**: Understanding requires jumping between sections

### Proposed Benefits
1. **One Pattern**: Everything uses `uses: activity_name` with optional `config:`
2. **Conceptual Simplicity**: Everything is just a registerable activity
3. **Inline Flexibility**: Define complex behaviors inline or extract to shared
4. **No Special Syntax**: No prefixes, no special handling

## Specification

### Core Principle: Everything is an Activity

All executable components in a workflow are registerable activities:
- **Standard Activities**: validation_activity, command_execution, etc.
- **Recipe Activities**: Activities that invoke other recipes (e.g., data-processor)
- **State Machine Activities**: Activities that execute state machines
- **LLM Activities**: Activities that interact with language models
- **User Input Activities**: Activities that gather user input

The implementation of each activity determines its behavior, not special syntax.

### Syntax: One Pattern for Everything

```yaml
steps:
  - id: step_name
    uses: activity_name      # The registered activity to use
    config:                  # Optional: activity-specific configuration
      key: value
    inputs:                  # Optional: runtime inputs
      key: "{{ expression }}"
    outputs:                 # Optional: output mapping
      key: ".path.to.value"
```

### Examples

#### 1. Standard Activity
```yaml
steps:
  - id: validate
    name: Validate Input
    uses: validation_activity
    config:
      validation_type: "strict"
    inputs:
      datasets: "{{ .Inputs.dataset_ids }}"
```

#### 2. Recipe Invocation (Just Another Activity)
```yaml
steps:
  - id: process
    name: Process Data
    uses: data-processor        # This happens to be a recipe
    inputs:
      data: "{{ .Steps.validate.outputs }}"
      mode: "enhanced"
    config:
      timeout: "10m"
      retry:
        max_attempts: 3
```

#### 3. Inline State Machine
```yaml
steps:
  - id: review_process
    name: Document Review with Retry
    uses: state_machine
    config:
      initial_state: reviewing
      states:
        reviewing:
          uses: critique_activity
          inputs:
            document: "{{ .Inputs.document }}"
          transitions:
            - to: approved
              when: ".Outputs.score >= 80"
            - to: improving
              when: ".Outputs.score < 80 && .State.attempts < 3"
            - to: rejected
              when: ".State.attempts >= 3"
        
        improving:
          uses: llm
          config:
            model: "gpt-4"
            system_prompt: "Improve this document for clarity."
          inputs:
            prompt: "Improve: {{ .State.document }}"
          transitions:
            - to: reviewing
              when: ".Outputs.improved == true"
        
        approved:
          terminal: true
          outputs:
            document: "{{ .State.final_document }}"
        
        rejected:
          terminal: true
          error: "Failed after {{ .State.attempts }} attempts"
    inputs:
      document: "{{ .Inputs.document }}"
      threshold: 80
```

#### 4. LLM with Inline Persona
```yaml
steps:
  - id: analyze
    name: Analyze Data
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.3
      system_prompt: |
        You are a senior data analyst specializing in statistical analysis.
        Provide detailed technical analysis with actionable insights.
    inputs:
      prompt: "Analyze this dataset: {{ .Steps.fetch.outputs }}"
```

#### 5. User Input Activity
```yaml
steps:
  - id: approval
    name: Get Approval
    uses: input
    config:
      question: "Do you approve this deployment?"
      type: "multiple_choice"
      options:
        - value: "approve"
          label: "Approve"
        - value: "reject"
          label: "Reject"
    inputs:
      context:
        artifacts: "{{ .Steps.build.outputs.artifacts }}"
```

### Shared Configurations

Extract complex or reusable configurations:

```yaml
shared:
  # Reusable validation
  strict_validator:
    uses: validation_activity
    config:
      validation_type: "strict"
      timeout: "30s"
  
  # LLM Persona: Data Analyst
  data_analyst:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.3
      system_prompt: |
        You are a senior data analyst with expertise in statistical analysis.
        Provide detailed technical analysis with actionable insights.
  
  # LLM Persona: Technical Writer  
  tech_writer:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.4
      system_prompt: |
        You are a technical writer. Create clear, concise documentation.
  
  # Reusable State Machine
  review_flow:
    uses: state_machine
    config:
      initial_state: reviewing
      states:
        reviewing:
          uses: critique_activity
          transitions:
            - to: approved
              when: ".Outputs.score >= 80"
            - to: rejected
              when: ".Outputs.score < 80"
        approved:
          terminal: true
        rejected:
          terminal: true

# Use shared configurations
steps:
  - id: validate
    uses: shared/strict_validator
    inputs:
      data: "{{ .Inputs.data }}"
  
  - id: analyze
    uses: shared/data_analyst
    inputs:
      prompt: "Analyze: {{ .Steps.validate.outputs }}"
  
  - id: review
    uses: shared/review_flow
    inputs:
      document: "{{ .Steps.analyze.outputs }}"
```

### Complete Example

```yaml
name: data-pipeline
version: "1.0"
description: End-to-end data processing pipeline with personas

inputs:
  - name: dataset_ids
    type: array
    required: true
  - name: processing_mode
    type: string
    default: "standard"

# Define reusable configurations including personas
shared:
  # Data analyst persona for analysis tasks
  data_analyst:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.3
      system_prompt: |
        You are a senior data analyst specializing in dataset quality assessment.
        Provide detailed technical analysis with statistical insights.
        Focus on data integrity, patterns, and anomalies.
  
  # Report writer persona for documentation
  report_writer:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.5
      system_prompt: |
        You are an executive report writer.
        Create clear, concise summaries for non-technical stakeholders.
        Focus on business impact and actionable recommendations.
  
  # Standard validation configuration
  dataset_validator:
    uses: validation_activity
    config:
      validation_type: "dataset"
      strict_mode: true
  
  # Inline state machine for review process
  quality_review_machine:
    uses: state_machine
    config:
      initial_state: analyzing
      states:
        analyzing:
          uses: quality_check_activity
          transitions:
            - to: passing
              when: ".Outputs.quality_score >= 85"
            - to: failing
              when: ".Outputs.quality_score < 85"
        passing:
          terminal: true
          outputs:
            status: "approved"
            score: "{{ .State.quality_score }}"
        failing:
          terminal: true
          outputs:
            status: "needs_improvement"
            score: "{{ .State.quality_score }}"

steps:
  # Use shared validation
  - id: validate
    name: Validate Inputs
    uses: shared/dataset_validator
    inputs:
      datasets: "{{ .Inputs.dataset_ids }}"
      mode: "{{ .Inputs.processing_mode }}"
    outputs:
      valid_ids: ".validated_ids"
  
  # Parallel processing using recipe activities
  - id: process_datasets
    name: Process Each Dataset
    parallel:
      for_each: "{{ .Steps.validate.outputs.valid_ids }}"
      as: dataset_id
      steps:
        - id: process_single
          uses: data-processor     # This is a recipe activity
          inputs:
            dataset_id: "{{ .dataset_id }}"
            mode: "{{ .Inputs.processing_mode }}"
  
  # Use data analyst persona for analysis
  - id: analyze_results
    name: Analyze Processed Data
    uses: shared/data_analyst
    inputs:
      prompt: |
        Analyze the following processed datasets for quality and completeness:
        {{ .Steps.process_datasets.outputs | json }}
        
        Provide:
        1. Statistical summary
        2. Data quality assessment
        3. Identified patterns or anomalies
        4. Recommendations for improvement
  
  # State machine for quality review
  - id: quality_review
    uses: shared/quality_review_machine
    inputs:
      data: "{{ .Steps.process_datasets.outputs }}"
      analysis: "{{ .Steps.analyze_results.outputs }}"
  
  # User approval step
  - id: get_approval
    uses: user_input_form
    config:
      title: "Review Pipeline Results"
      fields:
        - id: "approval_decision"
          type: "multiple_choice"
          question: "Do you approve these results?"
          options:
            - value: "approve"
              label: "Approve and Continue"
            - value: "reject"
              label: "Reject and Retry"
        - id: "notes"
          type: "paragraph_text"
          question: "Additional notes (optional)"
      context:
        artifacts:
          - path: "{{ .Steps.analyze_results.outputs.report_path }}"
  
  # Use report writer persona for executive summary
  - id: generate_report
    name: Generate Executive Report
    uses: shared/report_writer
    inputs:
      prompt: |
        Create an executive summary based on:
        
        Data Analysis: {{ .Steps.analyze_results.outputs }}
        Quality Review: {{ .Steps.quality_review.outputs }}
        User Decision: {{ .Steps.get_approval.outputs }}
        
        Include:
        - Key findings
        - Business impact
        - Recommendations
        - Next steps
  
  # Command execution for notification
  - id: notify
    name: Send Notifications
    uses: command_execution
    config:
      shell: "/bin/bash"
    inputs:
      command: |
        echo "Pipeline complete for {{ len .Inputs.dataset_ids }} datasets"
        
        # Send webhook notification
        curl -X POST ${WEBHOOK_URL} \
          -H "Content-Type: application/json" \
          -d '{
            "pipeline": "data-pipeline",
            "status": "complete",
            "datasets": {{ .Inputs.dataset_ids | json }},
            "report": "{{ .Steps.generate_report.outputs }}"
          }'

outputs:
  - name: processed_data
    value: "{{ .Steps.process_datasets.outputs }}"
  - name: analysis
    value: "{{ .Steps.analyze_results.outputs }}"
  - name: quality_status
    value: "{{ .Steps.quality_review.outputs.status }}"
  - name: executive_summary
    value: "{{ .Steps.generate_report.outputs }}"
```

## Migration Strategy

### Phase 1: Support Both Patterns
- Continue supporting separate `activities:` section
- Add support for inline definitions
- Emit deprecation warnings for separate declarations

### Phase 2: Migration Tools
```bash
# Automatic migration tool
vibethis migrate --unified-activities recipe.yaml

# Validation
vibethis validate --strict recipe.yaml

# Migrate all examples and tests
vibethis migrate --unified-activities server/*/examples/**/*.yaml
vibethis migrate --unified-activities server/*/testdata/**/*.yaml
```

### Phase 3: Update All Examples and Tests

All existing examples and tests must be rewritten to use the new unified pattern. Here are the files requiring updates:

#### Example Files to Update
```
server/recipe-core/examples/simple_workflow.yaml
server/recipe-core/examples/minimal_workflow.yaml
server/recipe-worker/examples/gemini_workflow.yaml
server/recipe-worker/examples/parallel_workflow.yaml
server/recipe-worker/examples/template_features.yaml
server/recipe-worker/examples/research_project/*.yaml
server/ono/example/*.yaml
server/activity/examples/recipe-invocation/*.yaml
server/nucleus/cmd/nucleus/testdata/recipes/*.yaml
```

#### Test Files with Embedded YAML
```
server/recipe-worker/pkg/worker/registry_test.go
server/recipe-worker/pkg/worker/registry_integration_test.go
server/recipe-core/pkg/recipe/parser_test.go
server/recipe-core/pkg/yaml/parser_test.go
server/ono/test/integration/recipe_discovery_test.go
```

### Phase 4: Deprecation
- Remove support for separate declarations in next major version
- Remove support for `activity:` field in favor of `uses:`
- Remove support for `implementation:` blocks

## Comprehensive Migration Examples

### Example 1: Simple Workflow

**Before:**
```yaml
workflow:
  steps:
    - id: search
      activity: quick_search
      inputs:
        query: "{{ .Inputs.query }}"
    - id: summarize
      activity: summarize_results
      inputs:
        data: "{{ .Steps.search.outputs.data }}"

activities:
  - name: quick_search
    implementation:
      type: function
      config:
        handler: search.QuickSearch
  - name: summarize_results
    implementation:
      type: function
      config:
        handler: summary.GenerateSummary
```

**After:**
```yaml
steps:
  - id: search
    uses: quick_search_activity
    inputs:
      query: "{{ .Inputs.query }}"
  
  - id: summarize
    uses: summarize_activity
    inputs:
      data: "{{ .Steps.search.outputs.data }}"
```

### Example 2: Recipe with LLM Personas

**Before:**
```yaml
workflow:
  steps:
    - id: analyze
      activity: analyze_activity
      inputs:
        data: "{{ .Inputs.data }}"

activities:
  - name: analyze_activity
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        temperature: 0.7
        prompt: "Analyze this data: {{ .data }}"
```

**After:**
```yaml
shared:
  analyst:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.7
      system_prompt: "You are a data analyst."

steps:
  - id: analyze
    uses: shared/analyst
    inputs:
      prompt: "Analyze this data: {{ .Inputs.data }}"
```

### Example 3: HTTP Activity Migration

**Before:**
```yaml
activities:
  - name: research_activity
    implementation:
      type: http
      config:
        method: POST
        url: "${RESEARCH_API_URL}/search"
        headers:
          Authorization: "Bearer ${RESEARCH_API_KEY}"

workflow:
  steps:
    - id: research
      activity: research_activity
      inputs:
        topic: "{{ .Inputs.topic }}"
```

**After:**
```yaml
steps:
  - id: research
    uses: http_client
    config:
      method: POST
      url: "${RESEARCH_API_URL}/search"
      headers:
        Authorization: "Bearer ${RESEARCH_API_KEY}"
    inputs:
      body:
        query: "{{ .Inputs.topic }}"
```

### Example 4: Recipe Invocation

**Before:**
```yaml
steps:
  - id: process
    activity: recipe
    config:
      recipe: "data-processor"
      timeout: "10m"
    inputs:
      data: "{{ .Inputs.data }}"
```

**After:**
```yaml
steps:
  - id: process
    uses: data-processor  # Recipes are just activities
    config:
      timeout: "10m"
    inputs:
      data: "{{ .Inputs.data }}"
```

### Example 5: Test File Updates

**Before (in Go test):**
```go
const testYAML = `
workflow:
  steps:
    - id: test
      activity: test_activity
      inputs:
        value: "test"
activities:
  - name: test_activity
    implementation:
      type: function
      config:
        handler: test.Handler
`
```

**After (in Go test):**
```go
const testYAML = `
steps:
  - id: test
    uses: test_activity
    inputs:
      value: "test"
`
```

### Example 6: State Machine Migration

**Before (CEL state machine proposal):**
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
```

**After:**
```yaml
steps:
  - id: review
    uses: state_machine
    config:
      initial_state: reviewing
      states:
        reviewing:
          uses: critique_activity
          transitions:
            - to: approved
              when: ".Outputs.score >= 80"
```

### Example 7: Parallel Workflow Migration

**Before:**
```yaml
workflow:
  type: parallel
  steps:
    - id: task1
      activity: process_task1
    - id: task2
      activity: process_task2
```

**After:**
```yaml
parallel:
  steps:
    - id: task1
      uses: process_task1_activity
    - id: task2
      uses: process_task2_activity
```

### Example 8: Full Research Project Migration

**Before (server/ono/example/research_project/activities.yaml):**
```yaml
activities:
  - name: research_activity
    implementation:
      type: http
      config:
        method: POST
        url: "${RESEARCH_API_URL}/search"
  - name: analyze_activity
    implementation:
      type: function
      config:
        handler: analyzers.ProcessResearch
  - name: write_report_activity
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        prompt: "Generate report..."
```

**After (inline in workflow or shared section):**
```yaml
shared:
  research_api:
    uses: http_client
    config:
      method: POST
      url: "${RESEARCH_API_URL}/search"
      headers:
        Authorization: "Bearer ${RESEARCH_API_KEY}"
  
  analyzer:
    uses: analyze_activity
  
  report_writer:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.7
      system_prompt: "You are a report writer."

steps:
  - id: research
    uses: shared/research_api
    inputs:
      body:
        query: "{{ .Inputs.topic }}"
        limit: "{{ .Inputs.max_sources }}"
  
  - id: analyze
    uses: shared/analyzer
    inputs:
      data: "{{ .Steps.research.outputs }}"
  
  - id: write_report
    uses: shared/report_writer
    inputs:
      prompt: "Generate report based on: {{ .Steps.analyze.outputs }}"
```

## Benefits of Unified Activity Model

1. **Conceptual Simplicity**
   - Everything is a registerable activity
   - No special syntax or prefixes needed
   - State machines and recipes are just activities with different implementations
   - Personas are just LLM activity configurations

2. **Developer Experience**
   - One pattern to learn: `uses: activity_name`
   - Inline complex behaviors (like state machines) when needed
   - Extract to shared when reuse is needed
   - Familiar to anyone who knows GitHub Actions

3. **Maintainability**
   - Clear intent in workflow definitions
   - Reusable configurations through shared section
   - No jumping between sections to understand flow
   - Better for code reviews

4. **Flexibility**
   - Mix inline and shared configurations
   - Define personas once, use everywhere
   - Inline state machines for complex flows
   - Natural composition patterns

## Implementation Notes

### Activity Registration

All activities implement the same interface:

```go
// All activities implement the same interface
type RegisterableActivity[TConfig any, TInput any, TOutput any] interface {
    GetMetadata() ActivityMetadata
    Execute(ctx context.Context, config TConfig, input TInput) (TOutput, error)
}

// The registry knows what each activity is
registry.Register("data-processor", &RecipeActivity{})
registry.Register("state_machine", &StateMachineActivity{})
registry.Register("validation_activity", &ValidationActivity{})
registry.Register("llm", &LLMActivity{})
registry.Register("user_input_form", &UserInputActivity{})
registry.Register("command_execution", &CommandExecutionActivity{})
```

### Personas as LLM Configurations

Personas are not a separate concept but simply predefined LLM activity configurations:

```yaml
shared:
  # Each persona is just an LLM activity with specific configuration
  security_analyst:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.2
      system_prompt: "You are a security analyst..."
  
  creative_writer:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.9
      system_prompt: "You are a creative writer..."
```

### State Machines as Activities

State machines are activities that can be defined inline or extracted:

```go
type StateMachineActivity struct {
    // Implements RegisterableActivity
}

func (s *StateMachineActivity) Execute(ctx context.Context, 
    config StateMachineConfig, 
    inputs map[string]interface{}) (map[string]interface{}, error) {
    // Execute the state machine based on config
    // States can reference other activities
}
```

## Conclusion

This specification unifies all workflow components under a single concept: registerable activities. By removing special syntax and treating recipes, state machines, and personas as regular activities with different implementations, we achieve maximum simplicity while maintaining full flexibility. The approach aligns with industry standards (GitHub Actions) and provides a clear migration path from the current system.