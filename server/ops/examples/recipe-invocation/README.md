# Recipe-to-Recipe Invocation Examples

This directory contains example workflows demonstrating how to use the recipe activity for parent-to-child recipe invocation.

## Overview

The recipe infrastructure supports hierarchical recipe execution where parent recipes can invoke child recipes as activities. This enables:
- Modular workflow composition
- Reusable recipe components
- Complex orchestration patterns
- Automatic context propagation
- Distributed execution with tracing

## Examples

### 1. Basic Parent-Child Pattern (`parent-child-basic.yaml`)

Demonstrates the fundamental pattern of a parent recipe invoking child recipes:
- Sequential child recipe invocation
- Passing inputs to child recipes
- Retrieving and using child outputs
- Version specification for child recipes
- Retry policies for child invocations

**Key Features:**
```yaml
activity: recipe
config:
  recipe: "child-recipe-name"
  timeout: "10m"
  retry_policy:
    maximum_attempts: 3
inputs:
  # Pass data to child
outputs:
  # Retrieve child outputs
```

### 2. Parallel Child Invocation (`parallel-child-invocation.yaml`)

Shows how to invoke multiple child recipes in parallel:
- Parallel execution of child recipes
- Different configurations per child
- Aggregating results from all children
- Quality validation of parallel results

**Use Case:** Document analysis pipeline that runs sentiment analysis, topic extraction, entity recognition, and language detection in parallel.

### 3. Data Aggregation Pattern (`data-aggregation-pattern.yaml`)

Demonstrates a complete data processing pipeline:
- Processing multiple datasets through child recipes
- Filtering results by quality thresholds
- Aggregating outputs from successful children
- Making decisions based on child outputs
- Generating insights from aggregated data

**Key Pattern:**
1. Validate inputs
2. Process each dataset via child recipe
3. Filter by quality score
4. Aggregate passing results
5. Generate insights
6. Create report

### 4. Context Propagation (`context-propagation.yaml`)

Shows how context automatically flows through recipe hierarchy:
- Automatic context propagation to children
- Accessing parent context in recipes
- Building execution traces
- Audit trail creation using context
- Nested recipe invocations with full context chain

**Context Includes:**
- Recipe metadata (name, version, execution ID)
- Parent execution ID for tracing
- Environment configuration
- Authentication and authorization info
- Execution details (host, namespace, timing)

## Recipe Activity Configuration

### Basic Structure

```yaml
activity: recipe  # Special activity type for recipe invocation
config:
  recipe: "recipe-name"          # Required: Name of child recipe
  version: "1.0.0"               # Optional: Specific version
  timeout: "10m"                 # Optional: Execution timeout
  retry_policy:                  # Optional: Retry configuration
    maximum_attempts: 3
    initial_interval: "5s"
    backoff_coefficient: 2.0
    maximum_interval: "1m"
inputs:
  # Inputs passed to child recipe
outputs:
  # Mapping of child outputs to parent variables
```

### Accessing Child Outputs

Child recipe outputs are available through the standard step output syntax:

```yaml
# Access specific output field
"{{ .Steps.child_step.outputs.field_name }}"

# Access execution metadata
"{{ .Steps.child_step.outputs.execution_id }}"
"{{ .Steps.child_step.outputs.status }}"
"{{ .Steps.child_step.outputs.execution_metadata }}"
```

## Context Propagation

Context is **automatically** propagated from parent to child recipes. No manual configuration is needed.

### What Gets Propagated

1. **Recipe Context**
   - Parent's execution ID becomes child's parent execution ID
   - Creates traceable execution hierarchy

2. **Environment Context**
   - Environment name, region, cluster
   - Ensures consistent environment across recipe chain

3. **Auth Context**
   - Identity and permissions
   - Maintains security context through invocations

4. **Execution Context**
   - Namespace, task queue
   - Ensures proper routing and execution

### Accessing Context in Recipes

```yaml
# In any recipe (parent or child)
"{{ .Context.Recipe.ExecutionID }}"          # This recipe's ID
"{{ .Context.Recipe.ParentExecutionID }}"    # Parent's ID (if child)
"{{ .Context.Environment.Name }}"            # Environment name
"{{ .Context.Environment.Region }}"          # Region
"{{ .Context.Auth.Identity }}"               # Auth identity
```

## Best Practices

### 1. Error Handling

Always configure retry policies for child recipes:

```yaml
config:
  retry_policy:
    maximum_attempts: 3
    initial_interval: "5s"
    backoff_coefficient: 2.0
    non_retryable_error_types: ["ValidationError"]
```

### 2. Timeout Configuration

Set appropriate timeouts based on expected child execution time:

```yaml
config:
  timeout: "10m"  # Should cover worst-case execution time
```

### 3. Output Validation

Validate child outputs before using them:

```yaml
- id: validate_child_output
  activity: validator
  inputs:
    data: "{{ .Steps.child_recipe.outputs.result }}"
    expected_fields: ["required_field1", "required_field2"]
```

### 4. Parallel Execution

Use parallel execution for independent child recipes:

```yaml
type: parallel
parallel:
  - id: child_1
    activity: recipe
    config:
      recipe: "processor-a"
  - id: child_2
    activity: recipe
    config:
      recipe: "processor-b"
```

### 5. Context Usage

Leverage context for:
- Audit logging
- Distributed tracing
- Environment-aware processing
- Security validation

## Testing

When testing recipes with child invocations:

1. **Unit Tests**: Mock the recipe activity
2. **Integration Tests**: Use Temporal's test framework
3. **End-to-End Tests**: Deploy actual child recipes

Example test pattern:

```go
// Mock recipe activity
env.OnActivity(recipeWrapper.Execute, mock.Anything, config, input).Return(
    RecipeOutput{
        ExecutionID: "test-child-123",
        Status: "completed",
        Result: expectedResult,
    }, nil,
)
```

## Performance Considerations

1. **Child Recipe Overhead**: Each child invocation has overhead for workflow creation
2. **Parallel Limits**: Consider worker capacity when running many children in parallel
3. **Data Transfer**: Large inputs/outputs between recipes can impact performance
4. **Context Size**: Context is passed to every child, keep it reasonable

## Debugging

To debug recipe chains:

1. Use execution IDs to trace through Temporal UI
2. Check context propagation in child recipes
3. Verify inputs/outputs at each level
4. Use structured logging with request IDs

## Migration from Direct Activities

To convert direct activity calls to recipe invocations:

**Before:**
```yaml
activity: data_processor
inputs:
  data: "{{ .Inputs.data }}"
```

**After:**
```yaml
activity: recipe
config:
  recipe: "data-processor-recipe"
inputs:
  data: "{{ .Inputs.data }}"
```

Benefits of migration:
- Better modularity and reusability
- Independent versioning and deployment
- Built-in retry and error handling
- Automatic context propagation
- Execution visibility in Temporal UI