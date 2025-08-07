# Inline Activities Specification

## Executive Summary

This specification simplifies how activities are defined in recipe YAML files, treating all activities as registerable components that can be referenced directly or defined inline. State machines and recipes are simply types of activities, and personas can be defined inline using the LLM activity with shared configurations.

## Motivation

### Current Problems
1. **Cognitive Overhead**: Developers must define activities in one section and reference them in another
2. **Verbosity**: Simple one-off activities require full declarations
3. **Navigation Complexity**: Understanding a workflow requires jumping between sections
4. **Conceptual Confusion**: Too many different concepts (activities, recipes, state machines, personas)

### Proposed Benefits
1. **Simplicity**: Everything is just a registerable activity
2. **Flexibility**: Mix inline and referenced activities as needed
3. **Consistency**: Uniform treatment of all activity types
4. **Readability**: Linear flow, self-contained steps

## Specification

### Core Principles

1. **Everything is an Activity**: All executable components are registerable activities
2. **Inline Configuration**: Activities can be configured inline where they're used
3. **Shared Definitions**: Common configurations can be defined once and reused
4. **Simple Prefixes**: Use `@` for recipes and `#` for state machines to indicate special activity types

### Activity Types

All activities are registerable components. Special types use prefixes for clarity:

| Type | Prefix | Description | Example |
|------|---------|-------------|---------|
| Activity | none | Standard registerable activity | `uses: llm` or `uses: command_execution` |
| Recipe | `@` | Recipe activity (invokes another recipe) | `uses: @data-processor` |
| State Machine | `#` | State machine activity | `uses: #review-flow` |

### Syntax Patterns

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
    outputs:
      valid: ".validation_result"
```

#### 2. LLM Activity (Direct)
```yaml
steps:
  - id: analyze
    name: Analyze Data
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.7
      system_prompt: "You are a data analyst."
    inputs:
      prompt: "Analyze the following data: {{ .Steps.fetch.outputs.data }}"
    outputs:
      analysis: ".response"
```

#### 3. LLM Activity with Persona (Using Shared)
```yaml
# Define personas as shared LLM configurations
shared:
  analyst_persona:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.3
      system_prompt: |
        You are a senior data analyst with expertise in statistical analysis.
        Provide detailed, technical analysis with actionable insights.
  
  reviewer_persona:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.5
      system_prompt: |
        You are a quality assurance specialist.
        Focus on identifying issues, risks, and areas for improvement.

# Use personas in workflow
steps:
  - id: analyze
    name: Analyze Dataset
    uses: shared/analyst_persona
    inputs:
      prompt: "Analyze this dataset: {{ .Inputs.data }}"
  
  - id: review
    name: Review Analysis
    uses: shared/reviewer_persona
    inputs:
      prompt: "Review this analysis for accuracy: {{ .Steps.analyze.outputs }}"
```

#### 4. Recipe Invocation
```yaml
steps:
  - id: process
    name: Process Data
    uses: @data-processor
    version: "2.0.0"  # Optional
    inputs:
      data: "{{ .Steps.fetch.outputs.data }}"
      mode: "enhanced"
    config:
      timeout: "10m"
      retry:
        max_attempts: 3
```

#### 5. State Machine
```yaml
steps:
  - id: review
    name: Document Review
    uses: #document-review-flow
    inputs:
      document: "{{ .Steps.process.outputs.document }}"
      threshold: 80
```

#### 6. Command Execution Activity
```yaml
steps:
  - id: build
    name: Build Project
    uses: command_execution
    config:
      shell: "/bin/bash"
      working_directory: "./project"
    inputs:
      command: "npm run build"
      environment:
        NODE_ENV: "production"
```

### Shared Configurations

Define reusable activity configurations, including personas:

```yaml
# Define shared configurations
shared:
  # Standard validation activity
  standard_validation:
    uses: validation_activity
    config:
      validation_type: "strict"
      timeout: "30s"
  
  # Persona: Technical Writer
  technical_writer:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.4
      system_prompt: |
        You are a technical writer with expertise in creating clear,
        concise documentation. Focus on accuracy and readability.
  
  # Persona: Code Reviewer
  code_reviewer:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.2
      system_prompt: |
        You are a senior software engineer reviewing code.
        Focus on best practices, security, and performance.
  
  # State machine configuration
  quality_check:
    uses: #quality-check-flow
    config:
      threshold: 85

# Use shared configurations
steps:
  - id: validate
    uses: shared/standard_validation
    inputs:
      data: "{{ .Inputs.data }}"
  
  - id: document
    name: Generate Documentation
    uses: shared/technical_writer
    inputs:
      prompt: "Document this API: {{ .Steps.validate.outputs }}"
  
  - id: review_code
    name: Review Implementation
    uses: shared/code_reviewer
    inputs:
      prompt: "Review this code for quality: {{ .Inputs.code }}"
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
  
  # Parallel processing using recipes
  - id: process_datasets
    name: Process Each Dataset
    parallel:
      for_each: "{{ .Steps.validate.outputs.valid_ids }}"
      as: dataset_id
      steps:
        - id: process_single
          uses: @data-processor
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
    uses: #quality-review-flow
    inputs:
      processed_data: "{{ .Steps.process_datasets.outputs }}"
      analysis: "{{ .Steps.analyze_results.outputs }}"
  
  # Use report writer persona for executive summary
  - id: generate_report
    name: Generate Executive Report
    uses: shared/report_writer
    inputs:
      prompt: |
        Create an executive summary based on:
        
        Data Analysis: {{ .Steps.analyze_results.outputs }}
        Quality Review: {{ .Steps.quality_review.outputs }}
        
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
  - name: quality_report
    value: "{{ .Steps.quality_review.outputs }}"
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
vibethis migrate --inline-activities recipe.yaml

# Validation
vibethis validate --strict recipe.yaml
```

### Phase 3: Deprecation
- Remove support for separate declarations in next major version
- Provide comprehensive migration guide

### Migration Example

**Before:**
```yaml
workflow:
  steps:
    - id: search
      activity: quick_search
      inputs:
        query: "{{ .Inputs.query }}"

activities:
  - name: quick_search
    implementation:
      type: function
      config:
        handler: search.QuickSearch
```

**After:**
```yaml
steps:
  - id: search
    name: Quick Search
    handler: search.QuickSearch
    inputs:
      query: "{{ .Inputs.query }}"
```

## Implementation Details

### Parser Changes
1. Extend step parser to recognize inline activity definitions
2. Support `uses:` field for external references
3. Implement prefix detection (`@`, `#`, `http/`, etc.)
4. Maintain backward compatibility with `activity:` field

### Validation Rules
1. Steps must have either inline definition OR `uses:` reference
2. Inline steps must specify activity type (via field or prefix)
3. Referenced activities must exist in `shared:` or be valid external references
4. Validate inputs/outputs match activity interface

### Type Detection Priority
1. Explicit `type:` field
2. `uses:` prefix (`@`, `#`, `http/`)
3. Presence of type-specific fields (`handler:` = function, `run:` = shell)
4. Default to function type

## Benefits of Unified Activity Model

1. **Conceptual Simplicity**
   - Everything is a registerable activity
   - Personas are just LLM activity configurations
   - State machines and recipes are special activity types
   - No artificial distinctions between "functions", "HTTP", etc.

2. **Developer Experience**
   - Consistent pattern for all activities
   - Self-contained step definitions
   - Easy persona management through shared configs

3. **Maintainability**
   - Clearer intent in workflow definitions
   - Reusable persona definitions
   - Better separation of concerns

4. **Flexibility**
   - Mix inline and shared configurations
   - Define personas once, use everywhere
   - Natural composition patterns

## Implementation Notes

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

### Activity Registration

All activities must be registered with the system:

```go
// All activities implement the same interface
type RegisterableActivity[TConfig any, TInput any, TOutput any] interface {
    GetMetadata() ActivityMetadata
    Execute(ctx context.Context, config TConfig, input TInput) (TOutput, error)
}

// Examples of registered activities:
// - LLMActivity
// - CommandExecutionActivity
// - ValidationActivity
// - RecipeActivity (uses: @recipe-name)
// - StateMachineActivity (uses: #state-machine-name)
```

## Open Questions

1. Should we support YAML anchors for activity reuse within a file?
2. How do we handle activity versioning for shared configurations?
3. Should personas have a special syntax or remain as shared LLM configs?
4. What's the timeline for migrating existing workflows?

## Conclusion

This specification modernizes recipe definitions by adopting inline activity patterns, reducing complexity while maintaining flexibility. The approach aligns with industry standards (GitHub Actions) and provides a clear migration path from the current system.