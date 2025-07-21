# Custom Activity Types in Recipe-Core

The recipe parser now supports registering arbitrary activity types from external modules, with JSON schema validation for inputs, outputs, and configuration.

## How It Works

1. **Activity Type Registry**: The parser maintains a registry of known activity types
2. **JSON Schema Validation**: Each activity type can define schemas for:
   - Configuration (required fields in the `config` section)
   - Inputs (expected input parameters)
   - Outputs (expected output structure)
3. **Flexible Validation**: Support for both strict and loose validation modes

## Registering Custom Activity Types

External modules can register new activity types with the parser:

```go
import "github.com/vibethis/server/recipe-core/pkg/recipe"

func RegisterMyActivityTypes(parser *recipe.Parser) error {
    registry := parser.GetActivityTypeRegistry()
    
    // Register a custom database activity
    return registry.RegisterActivityType(&recipe.ActivityTypeDefinition{
        Type:        "database",
        Description: "Execute database queries",
        ConfigSchema: recipe.JSONSchema{
            "type":     "object",
            "required": []string{"datasource", "query"},
            "properties": map[string]interface{}{
                "datasource": map[string]interface{}{
                    "type": "string",
                    "enum": []string{"postgres", "mysql", "mongodb"},
                },
                "query": map[string]interface{}{
                    "type": "string",
                },
            },
        },
        RequiredConfig: true,
        AllowAdditionalConfig: false,
    })
}
```

## Schema Flexibility Options

### 1. Very Strict Schema
```go
ActivityTypeDefinition{
    Type: "strict_api",
    ConfigSchema: /* detailed schema */,
    InputSchema: /* detailed schema */,
    OutputSchema: /* detailed schema */,
    RequiredConfig: true,
    AllowAdditionalConfig: false,
    AllowAdditionalInputs: false,
    AllowAdditionalOutputs: false,
}
```

### 2. Moderate Schema
```go
ActivityTypeDefinition{
    Type: "flexible_api",
    ConfigSchema: /* basic schema */,
    InputSchema: nil,  // Accept any input
    OutputSchema: /* basic schema */,
    RequiredConfig: true,
    AllowAdditionalConfig: true,
    AllowAdditionalInputs: true,
    AllowAdditionalOutputs: true,
}
```

### 3. Very Loose Schema
```go
ActivityTypeDefinition{
    Type: "anything_goes",
    ConfigSchema: nil,  // No validation
    InputSchema: nil,   // No validation
    OutputSchema: nil,  // No validation
    RequiredConfig: false,
    AllowAdditionalConfig: true,
    AllowAdditionalInputs: true,
    AllowAdditionalOutputs: true,
}
```

## Using Custom Activities in YAML

Once registered, custom activities can be used in recipe YAML files:

```yaml
activities:
  - name: query_users
    description: Get active users
    implementation:
      type: database  # Your custom type
      config:
        datasource: postgres
        query: "SELECT * FROM users WHERE active = true"
    inputs:
      - name: limit
        type: integer
    outputs:
      - name: users
        type: array
```

## Built-in Activity Types

The following activity types are pre-registered:
- `http` - HTTP API calls
- `ai_prompt` - LLM/AI prompts
- `function` - Function handlers
- `script` - External scripts
- `grpc` - gRPC calls

## Integration with Parser

The parser automatically validates all activities against their registered types during parsing:

```go
parser := recipe.NewParser(logger)
recipe, err := parser.ParseRecipe("my-recipe.yaml")
// Activities are validated against registered types
```

See `registry_example.go` for more detailed examples of registering different types of activities.