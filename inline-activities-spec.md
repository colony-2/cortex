# Inline Activities Specification

## Executive Summary

This specification proposes a fundamental change to how activities are defined in recipe YAML files, moving from a separate declaration model to an inline definition model inspired by GitHub Actions. This change reduces complexity, improves readability, and provides a more intuitive developer experience.

## Motivation

### Current Problems
1. **Cognitive Overhead**: Developers must define activities in one section and reference them in another
2. **Verbosity**: Simple one-off activities require full declarations
3. **Navigation Complexity**: Understanding a workflow requires jumping between sections
4. **Learning Curve**: Unlike familiar patterns (GitHub Actions), requiring additional documentation

### Proposed Benefits
1. **Simplicity**: Activities defined where they're used
2. **Flexibility**: Mix inline and referenced activities as needed
3. **Familiarity**: Follows GitHub Actions patterns that developers know
4. **Readability**: Linear flow, self-contained steps

## Specification

### Core Principles

1. **Inline by Default**: Activities should be defined inline within workflow steps
2. **Reference When Needed**: Support references for shared/complex activities
3. **Type Prefixes**: Use consistent prefixes to indicate activity types
4. **Backward Compatible**: Provide migration path from current approach

### Activity Types and Prefixes

| Type | Prefix | Description | Example |
|------|---------|-------------|---------|
| Function | none | Default activity type | `handler: search.QuickSearch` |
| Recipe | `@` | Invoke another recipe | `uses: @data-processor` |
| State Machine | `#` | Execute state machine | `uses: #review-flow` |
| HTTP | `http/` | HTTP request | `uses: http/post` |
| Shell | `run:` | Shell commands | `run: echo "Hello"` |

### Syntax Patterns

#### 1. Inline Function Activity
```yaml
steps:
  - id: validate
    name: Validate Input
    handler: validators.ValidateDatasets
    inputs:
      datasets: "{{ .Inputs.dataset_ids }}"
    outputs:
      valid: ".validation_result"
```

#### 2. Inline HTTP Activity
```yaml
steps:
  - id: fetch
    name: Fetch External Data
    uses: http/post
    with:
      url: "${API_URL}/search"
      headers:
        Authorization: "Bearer ${API_TOKEN}"
      body:
        query: "{{ .Inputs.query }}"
    outputs:
      data: ".response.results"
```

#### 3. Recipe Invocation
```yaml
steps:
  - id: process
    name: Process Data
    uses: @data-processor
    version: "2.0.0"  # Optional
    with:
      data: "{{ .Steps.fetch.outputs.data }}"
      mode: "enhanced"
    config:
      timeout: "10m"
      retry:
        max_attempts: 3
```

#### 4. State Machine
```yaml
steps:
  - id: review
    name: Document Review
    uses: #document-review-flow
    with:
      document: "{{ .Steps.process.outputs.document }}"
      threshold: 80
```

#### 5. Shell Commands
```yaml
steps:
  - id: notify
    name: Send Notification
    run: |
      echo "Processing dataset: {{ .Inputs.dataset_id }}"
      curl -X POST ${WEBHOOK_URL} \
        -H "Content-Type: application/json" \
        -d '{"status": "complete", "id": "{{ .Inputs.dataset_id }}"}'
```

#### 6. AI Prompt Activity
```yaml
steps:
  - id: summarize
    name: Generate Summary
    uses: ai/prompt
    with:
      model: "gpt-4"
      temperature: 0.7
      prompt: |
        Summarize the following data:
        {{ .Steps.analyze.outputs.results | json }}
        
        Format as bullet points.
```

### Shared Activities (Optional)

For activities used multiple times or shared across recipes:

```yaml
# Define shared activities
shared:
  standard-validation:
    handler: validators.StandardValidation
    timeout: "30s"
    retry:
      max_attempts: 2
  
  quality-check:
    uses: #quality-check-flow
    config:
      threshold: 85

# Use shared activities
steps:
  - id: validate_input
    uses: shared/standard-validation
    with:
      data: "{{ .Inputs.data }}"
  
  - id: validate_output
    uses: shared/standard-validation
    with:
      data: "{{ .Steps.process.outputs }}"
```

### Complete Example

```yaml
name: data-pipeline
version: "1.0"
description: End-to-end data processing pipeline

inputs:
  - name: dataset_ids
    type: array
    required: true
  - name: processing_mode
    type: string
    default: "standard"

steps:
  # Inline validation
  - id: validate
    name: Validate Inputs
    handler: validators.ValidateDatasets
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
          with:
            dataset_id: "{{ .dataset_id }}"
            mode: "{{ .Inputs.processing_mode }}"
  
  # HTTP request
  - id: fetch_metadata
    uses: http/get
    with:
      url: "${METADATA_API}/datasets"
      params:
        ids: "{{ .Steps.validate.outputs.valid_ids | join:',' }}"
  
  # State machine for quality check
  - id: quality_review
    uses: #quality-review-flow
    with:
      processed_data: "{{ .Steps.process_datasets.outputs }}"
      metadata: "{{ .Steps.fetch_metadata.outputs }}"
  
  # AI-powered summary
  - id: generate_report
    uses: ai/prompt
    with:
      model: "gpt-4"
      prompt: |
        Generate an executive summary for the processed datasets.
        
        Processed Data: {{ .Steps.process_datasets.outputs | json }}
        Quality Review: {{ .Steps.quality_review.outputs | json }}
  
  # Shell notification
  - id: notify
    run: |
      echo "Pipeline complete for {{ len .Inputs.dataset_ids }} datasets"
      
      # Send webhook notification
      curl -X POST ${WEBHOOK_URL} \
        -H "Content-Type: application/json" \
        -d '{
          "pipeline": "data-pipeline",
          "status": "complete",
          "datasets": {{ .Inputs.dataset_ids | json }},
          "report_url": "{{ .Steps.generate_report.outputs.url }}"
        }'

outputs:
  - name: processed_data
    value: "{{ .Steps.process_datasets.outputs }}"
  - name: quality_report
    value: "{{ .Steps.quality_review.outputs }}"
  - name: executive_summary
    value: "{{ .Steps.generate_report.outputs.summary }}"
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

## Benefits Summary

1. **Developer Experience**
   - Familiar GitHub Actions-like syntax
   - Self-contained step definitions
   - Less context switching

2. **Maintainability**
   - Clearer intent in workflow definitions
   - Easier to understand and modify
   - Better for code reviews

3. **Flexibility**
   - Mix inline and shared activities
   - Progressive complexity (simple → complex)
   - Natural composition patterns

4. **Performance**
   - No change to runtime execution
   - Simplified parsing logic
   - Reduced YAML file size for simple workflows

## Open Questions

1. Should we support YAML anchors for activity reuse within a file?
2. How do we handle activity versioning for inline definitions?
3. Should we allow mixing old and new syntax in the same file?
4. What's the timeline for deprecating the old syntax?

## Conclusion

This specification modernizes recipe definitions by adopting inline activity patterns, reducing complexity while maintaining flexibility. The approach aligns with industry standards (GitHub Actions) and provides a clear migration path from the current system.