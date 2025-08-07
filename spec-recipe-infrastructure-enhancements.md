# Recipe Infrastructure Enhancements Specification

## Overview

This specification defines enhancements to the recipe infrastructure to support recipe-to-recipe invocation, enabling a parent recipe to invoke and monitor child recipes across distributed execution environments.

## Current State

The existing recipe infrastructure supports:
- Recipe definitions in YAML
- Activity execution within recipes
- Single execution environment
- Basic recipe inputs/outputs

## Required Enhancements

### 1. Recipe Invocation

#### New Activity Type: `recipe`

A new activity implementation type that invokes other recipes as child processes:

```yaml
implementation:
  type: recipe
  recipe: "data-processing/transform"      # Recipe to execute
  timeout: "30m"                           # Execution timeout
  retry_policy:
    maximum_attempts: 3
    initial_interval: "5s"
    backoff_coefficient: 2.0
```

#### Activity Definition Example

```yaml
activities:
  - name: process_data
    description: Processes data using a child recipe
    timeout: 35m  # Slightly longer than recipe timeout
    inputs:
      - name: dataset_id
        type: string
        required: true
        description: ID of the dataset to process
      - name: format
        type: string
        required: false
        description: Output format for processing
    outputs:
      - name: execution_id
        type: string
        description: ID of the recipe execution
      - name: result
        type: object
        description: Result from the recipe execution
      - name: status
        type: string
        description: Final status of the recipe
    implementation:
      type: recipe
      recipe: "data-processing/transform"

### 2. System Context Variables

#### Automatic Context Population

The recipe system automatically provides context variables similar to GitHub Actions:

```yaml
# Available in all recipe templates via {{ .context }}
context:
  recipe:
    name: "current-recipe-name"
    version: "1.0.0"
    execution_id: "unique-execution-id"
    parent_execution_id: "parent-id-if-sub-recipe"
  
  environment:
    name: "production|staging|development"
    region: "us-west-2"
    cluster: "cluster-name"
  
  execution:
    host: "execution-host"           # Automatically determined
    namespace: "execution-namespace" # Inherited from parent
    task_queue: "task-queue"        # Inherited from parent
    started_at: "2024-01-01T00:00:00Z"
    timeout: "30m"
  
  auth:
    identity: "service-identity"
    token: "auth-token"             # Securely managed
```

#### Using Context Variables

```yaml
activities:
  - name: log_context
    implementation:
      type: recipe
      recipe: "monitoring/log-execution"
    inputs:
      - name: execution_id
        value: "{{ .context.recipe.execution_id }}"
      - name: environment
        value: "{{ .context.environment.name }}"
```

#### Implementation Requirements

1. **Connection Pooling**: Reuse execution client connections when possible
2. **Health Checking**: Verify execution environment availability
3. **Timeout Handling**: Properly propagate context timeouts
4. **Error Mapping**: Convert execution errors to recipe activity errors

### 3. Recipe Result Handling

#### Enhanced Output Structure

```yaml
outputs:
  - name: result
    type: object
    schema:
      type: object
      properties:
        recipe_outputs:
          type: object
          description: Outputs from the executed recipe
        execution_metadata:
          type: object
          properties:
            start_time:
              type: string
              format: date-time
            end_time:
              type: string
              format: date-time
            duration_ms:
              type: integer
            attempt_count:
              type: integer
```

### 4. Recipe Versioning

#### Version Support

Recipes can optionally specify versions:

```yaml
implementation:
  type: recipe
  recipe: "data-processing/transform@v2.1.0"  # Optional version
  timeout: "30m"
```

### 5. Monitoring and Observability

#### Activity Execution Tracking

Enhanced tracking for recipe-to-recipe executions:

```go
type RecipeExecutionEvent struct {
    ParentExecutionID  string
    ParentRecipe       string
    ChildExecutionID   string
    ChildRecipe        string
    Environment        string
    Status             string
    StartTime          time.Time
    EndTime            *time.Time
}
```

## Implementation Plan

### Phase 1: Core Activity Implementation
1. Create `recipe` activity type
2. Implement basic recipe invocation
3. Add result polling and retrieval
4. Populate system context variables automatically

### Phase 2: Execution Management
1. Implement connection pooling
2. Add health checking
3. Support secure communication

### Phase 3: Enhanced Features
1. Add recipe discovery
2. Implement advanced error handling
3. Add monitoring/metrics
4. Support recipe versioning

## Testing Requirements

1. **Unit Tests**: Mock recipe execution interactions
2. **Integration Tests**: Test with embedded execution environments
3. **E2E Tests**: Full parent-to-child recipe execution chains

## Security Considerations

1. **Authentication**: Automatic token propagation from parent context
2. **Authorization**: Validate recipe execution permissions
3. **Network Security**: Secure communication between execution environments
4. **Input Validation**: Sanitize recipe inputs

## Backwards Compatibility

All enhancements must maintain compatibility with existing recipe definitions. The new `recipe` activity type is additive and doesn't affect existing activities.

## Convention Over Configuration

The recipe invocation pattern emphasizes simplicity:
- No manual configuration of execution hosts, namespaces, or task queues
- Automatic context inheritance from parent recipes
- System-managed authentication and connection details
- Focus on recipe logic rather than infrastructure concerns