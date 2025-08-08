# Op Provider System

The op provider system allows you to extend recipe-worker with custom op types that integrate seamlessly with the recipe-core op type registry.

## Overview

Op providers implement the logic for executing specific op types. When a recipe-worker creates workers for recipes, it uses the provider registry to determine how to execute each op based on its type.

## Creating a Custom Op Provider

### 1. Basic Provider Interface

```go
type OpProvider interface {
    GetType() string
    Execute(ctx context.Context, args ...interface{}) (interface{}, error)
}
```

Example implementation:

```go
type CustomDatabaseProvider struct{}

func (p *CustomDatabaseProvider) GetType() string {
    return "database"
}

func (p *CustomDatabaseProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
    // args[0] is the op config (from recipe definition)
    // args[1] is the op inputs (runtime values)
    
    config := args[0].(map[string]interface{})
    inputs := args[1].(map[string]interface{})
    
    // Implement your op logic here
    return map[string]interface{}{
        "success": true,
        "result": "data",
    }, nil
}
```

### 2. Typed Provider (Recommended)

For better type safety, use the typed provider pattern:

```go
// Define your types
type MyConfig struct {
    Endpoint string `json:"endpoint"`
    APIKey   string `json:"apiKey"`
}

type MyInput struct {
    Query string `json:"query"`
}

type MyOutput struct {
    Results []string `json:"results"`
}

// Create typed provider
provider := recipeworker.NewTypedProvider("my_activity", 
    func(ctx context.Context, config MyConfig, input MyInput) (MyOutput, error) {
        // Type-safe implementation
        return MyOutput{
            Results: []string{"result1", "result2"},
        }, nil
    })
```

## Registering Providers

### With WorkerManager

```go
workerManager := worker.NewWorkerManager(logger, temporalClient)

// Register provider
provider := &CustomDatabaseProvider{}
err := workerManager.RegisterProvider(provider)
```

### With ProviderRegistry Directly

```go
registry := recipeworker.NewProviderRegistry()

// Register basic provider
err := registry.Register(provider)

// Register typed provider
err := recipeworker.RegisterTyped(registry, "my_activity",
    func(ctx context.Context, config MyConfig, input MyInput) (MyOutput, error) {
        // Implementation
    })
```

## Integration with recipe-core

The provider system integrates with recipe-core's activity type registry. To fully integrate a custom activity:

1. Register the activity type definition in recipe-core:
```go
registry := activity.NewActivityTypeRegistry()
err := registry.RegisterType(activity.ActivityTypeDefinition{
    Type: "my_custom_type",
    ConfigSchema: configSchema,
    InputSchema: inputSchema,
    OutputSchema: outputSchema,
})
```

2. Register the corresponding provider in recipe-worker:
```go
provider := NewMyCustomProvider()
err := workerManager.RegisterProvider(provider)
```

## Built-in Providers

The following providers are available out of the box:

- `http` - HTTP request provider (see `providers/http_provider.go`)
- Additional built-in providers for `ai_prompt`, `script`, `function`, and `grpc` can be implemented similarly

## Activity Execution Flow

1. Recipe parser validates activity definitions against registered types in recipe-core
2. WorkerManager creates workers for recipes and registers activities
3. When an activity executes:
   - The system checks if a provider is registered for the activity type
   - If found, the provider's Execute method is called with config and inputs
   - If not found, it falls back to built-in implementations (if any)
   - The provider returns outputs that flow to the next workflow step

## Example: Complete Custom Provider

See `examples/custom_provider.go` for a complete example that demonstrates:
- Creating a custom database provider
- Using typed providers for weather API
- Integrating with the HTTP provider
- Starting a worker with custom providers

## Testing Providers

```go
func TestMyProvider(t *testing.T) {
    provider := &MyProvider{}
    
    result, err := provider.Execute(context.Background(),
        map[string]interface{}{"config": "value"},
        map[string]interface{}{"input": "data"})
    
    require.NoError(t, err)
    assert.Equal(t, expectedResult, result)
}
```

See `provider_test.go` and `providers/http_provider_test.go` for more test examples.