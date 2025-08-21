# Recipe Core

## Overview

Recipe Core is a Go library that provides foundational data structures, parsing, and execution primitives for unified recipe definitions. It supports declarative workflow specifications using YAML with operations, sequences, parallel execution, and state machines, backed by an extensible activity type registry for custom integrations.

## Architecture

### Core Components

- **`pkg/recipe`**: Recipe parsing, validation, and data structures
  - `Parser`: High-level recipe parser with activity type registry integration
  - `Recipe`: Complete recipe representation with metadata and content hash
  - `Job`: Execution instance tracking with status, timing, and results
  - `ActivityTypeRegistry`: Extensible registry for custom activity types
  - `HashComputer`: Deterministic content hashing for versioning

- **`pkg/yaml`**: Low-level YAML parsing and unified recipe format
  - `RecipeDefinition`: Root recipe structure with embedded node properties
  - `Node`: Fundamental execution unit (operation, sequence, parallel, state machine, shared reference)
  - `StateMap/State`: State machine definitions with transitions and CEL expressions
  - `Parser`: YAML unmarshalling with validation

### Recipe Format

Unified single-file YAML format with embedded root node execution model:

```yaml
name: example-recipe
version: 1.0.0
description: Example unified recipe

# Root node (one of: op, sequence, parallel, states)
sequence:
  - id: step1
    op: http
    inputs:
      url: "https://api.example.com/data"
  - id: step2
    parallel:
      - op: ai_prompt
        inputs:
          prompt: "Analyze: ${step1.response}"
      - op: function
        inputs:
          handler: "process_data"
```

## Key Interfaces

### Recipe Parser

```go
// Create parser with activity registry
parser := recipe.NewParser(logger)
registry := parser.GetActivityTypeRegistry()

// Parse recipe from file
recipe, err := parser.ParseRecipe("/path/to/recipe.yaml")

// Access parsed content
fmt.Printf("Recipe: %s v%s\n", recipe.Name, recipe.Version)
fmt.Printf("Hash: %s\n", recipe.Hash)
```

### Activity Type Registry

```go
// Register custom activity type
registry.RegisterActivityType(&recipe.ActivityTypeDefinition{
    Type:        "custom_api",
    Description: "Custom API integration",
    ConfigSchema: recipe.JSONSchema{
        "type": "object",
        "required": []string{"endpoint"},
        "properties": map[string]interface{}{
            "endpoint": map[string]interface{}{"type": "string"},
            "timeout": map[string]interface{}{"type": "integer"},
        },
    },
    RequiredConfig: true,
    AllowAdditionalInputs: true,
})

// Validate activity configuration
err := registry.ValidateActivityConfig("custom_api", config)
```

### YAML Parser

```go
// Parse unified recipe format
yamlParser := yaml.NewParser()
definition, err := yamlParser.ParseRecipe("recipe.yaml")

// Access recipe structure
if definition.Sequence != nil {
    for _, node := range definition.Sequence {
        fmt.Printf("Node ID: %s, Op: %s\n", node.ID, node.Op)
    }
}
```

### Hash Computation

```go
// Compute deterministic recipe hash
hashComputer := recipe.NewHashComputer()
hash := hashComputer.ComputeRecipeHash(recipe)

// Hash accounts for normalized content, ignoring whitespace and ordering
```

## Usage Examples

### Basic Recipe Parsing

```go
package main

import (
    "log"
    "go.uber.org/zap"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    parser := recipe.NewParser(logger)
    r, err := parser.ParseRecipe("example.yaml")
    if err != nil {
        log.Fatalf("Parse error: %v", err)
    }

    log.Printf("Loaded: %s v%s", r.Name, r.Version)
    log.Printf("Type: %T", r.Recipe) // *yaml.RecipeDefinition
}
```

### Custom Activity Registration

```go
// Register webhook activity type
registry := parser.GetActivityTypeRegistry()
err := registry.RegisterActivityType(&recipe.ActivityTypeDefinition{
    Type: "webhook",
    Description: "HTTP webhook sender",
    ConfigSchema: recipe.JSONSchema{
        "type": "object",
        "required": []string{"url"},
        "properties": map[string]interface{}{
            "url": map[string]interface{}{
                "type": "string",
                "format": "uri",
            },
            "method": map[string]interface{}{
                "type": "string",
                "enum": []string{"POST", "PUT", "PATCH"},
                "default": "POST",
            },
        },
    },
    RequiredConfig: true,
    AllowAdditionalInputs: true,
})
```

### Job Execution Tracking

```go
// Create job instance
job := &recipe.Job{
    ID:            "job-123",
    RecipeName:    r.Name,
    RecipeVersion: r.Version,
    Status:        recipe.JobStatusRunning,
    StartTime:     time.Now(),
    Input:         map[string]interface{}{"key": "value"},
}

// Update job status
job.Status = recipe.JobStatusCompleted
job.Output = map[string]interface{}{"result": "success"}
endTime := time.Now()
job.EndTime = &endTime
```

### State Machine Context

```go
// State machine execution context
context := &yaml.StateContext{
    CurrentState: "initial",
    Inputs: map[string]interface{}{
        "user_id": "12345",
    },
    StateOutputs: make(map[string]map[string]interface{}),
    RecipeContext: &yaml.RecipeContext{
        Recipe: yaml.RecipeInfo{
            Name:    r.Name,
            Version: r.Version,
        },
    },
}
```

## Configuration

### Built-in Activity Types

- `http`: HTTP requests with method, URL, headers, body
- `ai_prompt`: LLM inference with provider, model, temperature
- `function`: Registered function handlers
- `script`: External command execution
- `grpc`: gRPC service calls

### Validation Settings

```go
// Activity type validation levels
type ActivityTypeDefinition struct {
    RequiredConfig         bool // Config section mandatory
    AllowAdditionalConfig  bool // Extra config fields allowed
    AllowAdditionalInputs  bool // Extra input fields allowed
    AllowAdditionalOutputs bool // Extra output fields allowed
}
```

### Deprecation Notice

Multi-file recipe format (directory with `recipe.yaml` manifest) is deprecated. Use unified single-file format for all new recipes.