# Recipe Worker System

## Overview

Recipe Worker is a Temporal-based workflow execution engine that dynamically discovers, loads, and executes YAML-defined recipes. It provides a hot-reloading system that monitors recipe files and automatically manages corresponding Temporal workers for each recipe.

## Architecture

The system consists of four key components working together:

### Core Components

- **Worker**: Main orchestrator that coordinates the Registry and WorkerManager
- **Registry**: File system monitor that discovers recipes and triggers worker lifecycle events
- **WorkerManager**: Temporal worker lifecycle manager that creates isolated task queues per recipe
- **Compiler**: Runtime workflow compiler that converts YAML definitions into executable Temporal workflows

### Activity System

- **ActivityProvider Interface**: Pluggable activity execution system with schema validation
- **ProviderRegistry**: Central registry for custom activity implementations
- **StandaloneExecutor**: Testing execution environment without Temporal server

## Key Interfaces

### ActivityProvider Interface
```go
type ActivityProvider interface {
    GetType() string
    Execute(ctx context.Context, args ...interface{}) (interface{}, error)
    GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{})
    GetDescription() string
    GetSchemaOptions() SchemaOptions
}
```

### WorkerManagerInterface
```go
type WorkerManagerInterface interface {
    StartWorker(recipe *recipe.Recipe) error
    StopWorker(recipeName string) error
    RestartWorker(recipeName string, recipe *recipe.Recipe) error
    StopAll()
    GetWorkerStatus(recipeName string) recipe.WorkerStatus
    GetTaskQueueForRecipe(recipeName string) string
}
```

### WorkflowExecutor Interface
```go
type WorkflowExecutor interface {
    ExecuteWorkflow(ctx workflow.Context, recipeDef *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
}
```

### Compiler ExecuteWorkflow Method
```go
func (c *Compiler) ExecuteWorkflow(ctx workflow.Context, def *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
```

### Registry Methods
```go
func (r *Registry) GetRecipe(name string) (*recipe.Recipe, error)
func (r *Registry) ListRecipes(filter *recipe.RecipeFilter) ([]*recipe.Recipe, error)
```

### StandaloneExecutor Execute Method
```go
func (e *StandaloneExecutor) Execute(ctx context.Context, recipe *yamlpkg.RecipeDefinition, inputs map[string]interface{}, opts ...ExecutionOptions) (map[string]interface{}, error)
```

## Usage Examples

### Creating and Running a Worker System
```go
// Initialize worker system
logger, _ := zap.NewDevelopment()
temporalClient, _ := client.Dial(client.Options{HostPort: "localhost:7233"})

worker, err := worker.NewWorker(logger, "/path/to/recipes", temporalClient)
if err != nil {
    log.Fatal(err)
}

// Start the system (begins file watching and worker management)
if err := worker.Start(); err != nil {
    log.Fatal(err)
}

// Stop the system
defer worker.Stop()
```

### Registering Custom Activity Provider
```go
// Create HTTP activity provider
httpProvider := providers.NewHTTPProvider()

// Register with worker manager
workerManager := worker.GetWorkerManager()
if err := workerManager.RegisterProvider(httpProvider); err != nil {
    log.Fatal(err)
}
```

### Standalone Recipe Execution (Testing)
```go
// Create standalone executor
executor, err := executor.NewStandaloneExecutor(logger)
if err != nil {
    log.Fatal(err)
}

// Define recipe
recipeDef := &yamlpkg.RecipeDefinition{
    Name: "test-recipe",
    Op:   "command_execution",
    Inputs: map[string]interface{}{
        "run": "echo 'Hello World'",
    },
}

// Execute recipe
inputs := map[string]interface{}{}
outputs, err := executor.Execute(context.Background(), recipeDef, inputs)
if err != nil {
    log.Fatal(err)
}
```

### Custom Activity Provider Implementation
```go
type CustomProvider struct{}

func (p *CustomProvider) GetType() string {
    return "custom_activity"
}

func (p *CustomProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
    config := args[0].(map[string]interface{})
    inputs := args[1].(map[string]interface{})
    
    // Custom logic here
    return map[string]interface{}{
        "result": "custom output",
        "status": "completed",
    }, nil
}

func (p *CustomProvider) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
    configSchema = map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "setting": map[string]interface{}{"type": "string"},
        },
    }
    inputSchema = map[string]interface{}{
        "type": "object",
    }
    outputSchema = map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "result": map[string]interface{}{"type": "string"},
        },
    }
    return
}

func (p *CustomProvider) GetDescription() string {
    return "Custom activity for specialized processing"
}

func (p *CustomProvider) GetSchemaOptions() recipeworker.SchemaOptions {
    return recipeworker.SchemaOptions{
        RequiredConfig: true,
        AllowAdditionalInputs: true,
    }
}
```

### Recipe YAML Structure
```yaml
name: example-recipe
version: "1.0.0"
description: Example recipe demonstrating various node types

# Sequential execution
sequence:
  - id: fetch_data
    op: http
    inputs:
      method: GET
      url: "https://api.example.com/data"
    
  - id: process_data
    op: command_execution
    inputs:
      run: "python process.py"
      input_file: "{{.Steps.fetch_data.body}}"

# Alternative: Parallel execution
# parallel:
#   - id: task1
#     op: http
#     inputs:
#       method: GET
#       url: "https://api1.example.com"
#   - id: task2
#     op: http
#     inputs:
#       method: GET
#       url: "https://api2.example.com"

# Template usage in outputs
outputs:
  result: "{{.Steps.process_data.stdout}}"
  status: "completed"
```

## Configuration

### Worker Initialization Options
- `recipesDir`: Directory path to monitor for recipe files
- `temporalClient`: Configured Temporal client connection
- `logger`: Structured logger instance

### Recipe File Discovery
- Monitors `.yaml` files in the specified directory
- Supports hot-reloading with file system watching
- Automatically manages worker lifecycle based on file changes

### Task Queue Naming
- Base task queue: `"ono-recipes"`
- Per-recipe task queue: `"ono-recipes-{recipeName}"`

### Activity Registration
- Automatic registration of shared activities from recipe definitions
- Support for custom activity providers via ProviderRegistry
- Schema validation for activity inputs/outputs/configuration