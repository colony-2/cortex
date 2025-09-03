# YAML Files Catalog - VibeThis Project (Non-Moon Files)

## Overview
This document catalogs YAML files in the VibeThis project, excluding Moon build system files, categorizing them by type and documenting key learnings about their structure and purpose.

## File Categories

### 1. Recipe Files (Workflow Definitions)
These files define workflows with steps, activities, inputs, outputs, and state management.

#### State Machine Recipes
- **./example-error-handling-recipe.yaml** - Complex error handling example with state transitions, demonstrates:
  - Multiple error types (timeout, auth, rate limit, validation, etc.)
  - State-based error recovery strategies
  - Retry with exponential backoff
  - Terminal states for different failure scenarios

- **./server/recipe-worker/examples/simple_state_machine.yaml** - Basic state machine implementation
- **./server/recipe-worker/examples/state_machine_composition.yaml** - Advanced state composition

#### Data Pipeline Recipes
- **./server/recipe-worker/examples/unified_data_pipeline.yaml** - Demonstrates:
  - Unified activity model with shared activities
  - Parallel batch processing
  - LLM integration for data analysis
  - Template-based data transformation

- **./server/recipe-core/examples/unified_data_pipeline.yaml** - Core data pipeline implementation

#### Parallel Processing Recipes
- **./server/recipe-worker/examples/parallel_recipe.yaml** - Parallel execution patterns
- **./server/ops/examples/recipe-invocation/parallel-child-invocation.yaml** - Shows:
  - Multiple child recipes running in parallel
  - Aggregation of parallel results
  - Quality checking across parallel outputs
  - Report generation from combined data

#### Simple Workflow Recipes
- **./server/recipe-worker/examples/simple_echo.yaml** - Minimal recipe with command execution
- **./server/recipe-worker/examples/echo_example.yaml** - Echo activity example
- **./server/recipe-core/examples/simple_workflow.yaml** - Simple workflow pattern
- **./server/recipe-core/examples/minimal_workflow.yaml** - Minimal workflow example
- **./test-recipe.yaml** - Test recipe showing LLM and command execution integration

#### Complex Composition Recipes
- **./server/recipe-worker/examples/nested_composition.yaml** - Nested recipe composition
- **./server/recipe-worker/examples/template_features.yaml** - Template feature demonstrations
- **./server/ops/examples/recipe-invocation/parent-child-basic.yaml** - Parent-child recipe relationships
- **./server/ops/examples/recipe-invocation/context-propagation.yaml** - Context propagation patterns
- **./server/ops/examples/recipe-invocation/data-aggregation-pattern.yaml** - Data aggregation patterns

#### AI/LLM Integration Recipes
- **./server/recipe-worker/examples/gemini_recipe.yaml** - Gemini AI integration

#### Research Project Recipes
- **./server/recipe-worker/examples/research_project/recipe.yaml** - Research project main recipe
- **./server/recipe-worker/examples/research_project/project.yaml** - Project configuration
- **./server/recipe-worker/examples/research_project/agents.yaml** - Agent definitions
- **./server/recipe-worker/examples/research_project/ops.yaml** - Operations definitions

#### Test Fixture Recipes
- **./server/cortex/test-fixtures/simple-recipe.yaml** - Simple test recipe
- **./server/cortex/test-fixtures/inputs.yaml** - Input test fixtures
- **./server/cortex/test-fixtures/test-execute.yaml** - Execution test recipe
- **./server/cortex/test-fixtures/test-invalid.yaml** - Invalid recipe for testing
- **./server/cortex/test-fixtures/test-missing-fields.yaml** - Missing fields test
- **./server/cortex/test-recipe.yaml** - Cortex test recipe
- **./server/nucleus/cmd/nucleus/testdata/recipes/simple-recipe.yaml** - Nucleus simple recipe
- **./server/nucleus/cmd/nucleus/testdata/recipes/invalid-recipe.yaml** - Invalid nucleus recipe

### 2. API Specification Files
- **./api/openapi/vibethis-api.yaml** - Main VibeThis REST API specification (OpenAPI 3.0.3)
  - Comprehensive cell management API
  - Git operations endpoints
  - Container/DevContainer management
  - File system operations
  - Graph visualization support

### 3. Code Generation Configuration
- **./server/openapi/codegen.yml** - OpenAPI code generation settings for:
  - Model generation
  - Embedded specifications
  - Client library generation

### 4. Development Tool Configuration
- **./.pre-commit-config.yaml** - Pre-commit hooks configuration
  - Forbids binary files (except images)
  - Code quality checks

### 5. Moon System Configuration (Excluded from detailed analysis)
- **Moon workspace configs**: `.moon/workspace.yml`, `.moon/toolchain.yml`
- **Moon task definitions**: `.moon/tasks/node.yml`, `.moon/tasks/go.yml`
- **Project-specific moon.yml files**: 44 files across various directories

## Key Recipe Patterns & Learnings

### 1. Two Main Recipe Formats

#### State Machine Format
```yaml
name: recipe-name
initialState: startState
variables:
  var1: value
states:
  - name: stateName
    activities:
      - name: activityName
        input: {...}
        output: varName
    transitions:
      - condition: 'CEL expression'
        target: nextState
        actions:
          - set: variable
            value: 'expression'
  - name: terminalState
    terminal: true
```

#### Workflow/Steps Format
```yaml
name: recipe-name
inputs:
  - name: input1
    type: string
    required: true
steps:
  - id: step1
    uses: activity_name
    inputs:
      param: "{{ .Inputs.input1 }}"
    outputs:
      result: output_var
outputs:
  - name: final_output
    value: "{{ .Steps.step1.outputs.result }}"
```

### 2. Activity Types Discovered

- **command_execution**: Execute shell commands
- **llm_inference**: LLM/AI operations with model selection
- **recipe**: Invoke child recipes
- **document_loader**: Load and parse documents
- **validate_sources**: Data validation
- **process_data**: Data processing
- **combine_results**: Result aggregation
- **report_generator**: Report generation
- **analysis_validator**: Quality checking
- **shared/*** : Reference to shared, reusable activities

### 3. Advanced Features

#### Error Handling
- Retry policies with exponential backoff
- Multiple error type detection using CEL expressions
- Graceful degradation paths
- Terminal states for different failure modes

#### Parallel Processing
- Parallel step blocks for concurrent execution
- Result aggregation from parallel branches
- Independent error handling per parallel branch

#### Template System
- Variable interpolation with `{{ .Inputs.varname }}`
- Step output references: `{{ .Steps.stepId.outputs.field }}`
- Array slicing and manipulation in templates
- Complex expressions in template variables

#### Configuration Options
- Timeouts at activity level
- Retry policies with customizable parameters
- Model selection for AI activities
- Parallel execution flags

### 4. Recipe Invocation Patterns

#### Parent-Child Relationships
- Parent recipes can invoke child recipes as activities
- Context and data propagation between parent and child
- Parallel child invocation for scalability
- Result aggregation from multiple children

#### Data Flow
- Input validation and transformation
- Pipeline-style data processing
- Batch processing with parallel execution
- Output formatting and aggregation

### 5. Testing Patterns

- Valid recipe fixtures for positive testing
- Invalid recipes for error handling validation
- Missing field tests for schema validation
- Input variation testing

## Recommendations

### Recipe Development Best Practices

1. **Choose the Right Format**
   - Use state machines for complex control flow and error handling
   - Use workflow/steps for simpler sequential or parallel tasks
   - Consider hybrid approaches for complex scenarios

2. **Error Handling**
   - Implement comprehensive error detection with specific conditions
   - Provide fallback paths for recoverable errors
   - Use terminal states to clearly indicate failure modes
   - Implement retry strategies with appropriate backoff

3. **Modularity**
   - Create shared activities for common operations
   - Use child recipes for complex reusable workflows
   - Keep individual recipes focused on specific tasks
   - Use clear naming conventions for activities and steps

4. **Performance**
   - Leverage parallel processing where possible
   - Set appropriate timeouts for activities
   - Use batch processing for large datasets
   - Consider resource constraints in parallel execution

5. **Testing**
   - Create comprehensive test fixtures
   - Test both success and failure paths
   - Validate input handling and edge cases
   - Test parallel execution scenarios

### API Design (from OpenAPI spec)

1. **RESTful Principles**
   - Clear resource-based paths
   - Proper HTTP method usage
   - Comprehensive error responses
   - Consistent response formats

2. **Feature Coverage**
   - Complete CRUD operations for resources
   - Batch operations where appropriate
   - Filtering and pagination support
   - WebSocket support for real-time updates (if needed)

3. **Documentation**
   - Detailed descriptions for all endpoints
   - Example requests and responses
   - Clear error code documentation
   - Version management strategy