# Recipe Infrastructure Enhancements Specification

## Overview

This specification defines enhancements to the recipe infrastructure to support recipe-to-recipe invocation, enabling a parent recipe to invoke and monitor child recipes across distributed execution environments.

**Implementation Note**: All code changes described in this specification should be implemented in the `server/activity` directory, which contains the recipe activity system.

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

## Implementation 

**All implementation changes should be made in the `server/activity` directory.** This is where the recipe activity system is implemented and where the new `recipe` activity type will be added.

### Key Implementation Areas

#### 1. New Recipe Activity Type
- Create a new `recipe` activity type in server/activity
- **Important**: Leverage existing Temporal workflow-to-workflow invocation patterns
- Do not create new execution infrastructure - use Temporal's native child workflow capabilities
- The activity should act as a thin wrapper around Temporal's ExecuteChildWorkflow functionality

#### 2. Context Passing
- Automatically populate context variables from the parent workflow's context
- Pass execution metadata (namespace, task queue, auth tokens) transparently
- Ensure child recipes inherit parent's execution environment settings

#### 3. End-to-End Testing
- Create comprehensive E2E tests that validate parent-to-child recipe execution
- Test context inheritance and variable passing
- Verify error propagation and retry behaviors

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