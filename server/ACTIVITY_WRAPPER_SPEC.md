# Activity Wrapper Specification for Recipe-Worker

## Overview

This specification defines how activities in the `./activity` module expose themselves for consumption by `recipe-worker` through YAML definitions. The goal is to establish a standardized interface that allows any activity to be registered and executed within the recipe system using Go generics for type safety and automatic schema derivation.

## Architecture Overview

### Dependency Flow

```
┌─────────────┐     ┌───────────────┐     ┌──────────────┐
│   activity  │────▶│  recipe-core  │◀────│recipe-worker │
└─────────────┘     └───────────────┘     └──────────────┘
     defines              common              consumes
  activities &           schemas           activities &
   interface                              implements logic
```

**Key Principles:**
- `activity` module defines the `RegisterableActivity` interface and activity implementations
- `activity` module has NO dependency on `recipe-worker`
- `recipe-worker` imports and consumes activities from the `activity` module
- `recipe-core` contains only common types/schemas used by both
- All registration, schema generation, and provider bridging logic lives in `recipe-worker`

## Architecture

### 1. Core Interface Definition (activity module)

The generic interface will be defined in `./activity` that standardizes how activities expose themselves:

```go
// Package activity defines interfaces for activities that can be consumed by recipe-worker
// Located in: ./activity/pkg/types/activity.go
package types

import (
    "context"
    "time"
)

// RegisterableActivity defines the contract for activities that can be consumed
// by external systems like recipe-worker via YAML definitions
type RegisterableActivity[TConfig any, TInput any, TOutput any] interface {
    // GetMetadata returns activity metadata for registration
    GetMetadata() ActivityMetadata
    
    // Execute runs the activity with provided configuration and inputs
    Execute(ctx context.Context, config TConfig, inputs TInput) (TOutput, error)
}

// ActivityMetadata describes the activity for registration and documentation
type ActivityMetadata struct {
    Type           string         // Unique identifier for the activity type
    Name           string         // Human-readable name
    Description    string         // Detailed description
    Version        string         // Semantic version
    DefaultTimeout time.Duration  // Default execution timeout
    RetryPolicy    *RetryPolicy   // Default retry configuration
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
    MaximumAttempts        int32
    InitialInterval        time.Duration
    BackoffCoefficient     float64
    MaximumInterval        time.Duration
    NonRetryableErrorTypes []string
}
```

### 2. Activity Implementation Pattern (activity module)

Activities in the `./activity` module implement `RegisterableActivity`:

```go
// Located in: ./activity/pkg/llm/wrapper.go
package llm

import (
    "context"
    "time"
    "github.com/divisive-ai/vibethis/server/activity/pkg/types"
)

// Define typed structs for LLM activity - ALL fields MUST have json tags
type LLMConfig struct {
    Provider    string  `json:"provider"`    // Required by consumer
    Model       string  `json:"model"`       // Required by consumer
    Temperature float64 `json:"temperature"` // Optional with validation
}

type LLMInput struct {
    Prompt    string `json:"prompt"`     // Required
    System    string `json:"system"`     // Optional
    MaxTokens int    `json:"max_tokens"` // Optional
}

type LLMOutput struct {
    Response string                 `json:"response"`
    Usage    map[string]interface{} `json:"usage"`
}

// LLMActivity implements the RegisterableActivity interface
type LLMActivity struct {
    client *Client // Internal LLM client
}

// Ensure we implement the interface
var _ types.RegisterableActivity[LLMConfig, LLMInput, LLMOutput] = (*LLMActivity)(nil)

func NewLLMActivity() types.RegisterableActivity[LLMConfig, LLMInput, LLMOutput] {
    return &LLMActivity{
        client: NewClient(),
    }
}

func (a *LLMActivity) GetMetadata() types.ActivityMetadata {
    return types.ActivityMetadata{
        Type:           "llm_inference",
        Name:           "LLM Inference",
        Description:    "Executes LLM inference with various providers",
        Version:        "1.0.0",
        DefaultTimeout: 5 * time.Minute,
        RetryPolicy: &types.RetryPolicy{
            MaximumAttempts:    3,
            InitialInterval:    1 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    30 * time.Second,
        },
    }
}

func (a *LLMActivity) Execute(ctx context.Context, config LLMConfig, input LLMInput) (LLMOutput, error) {
    // Direct execution with typed parameters
    response, err := a.client.Complete(ctx, &CompletionRequest{
        Provider:    config.Provider,
        Model:       config.Model,
        Temperature: config.Temperature,
        Prompt:      input.Prompt,
        System:      input.System,
        MaxTokens:   input.MaxTokens,
    })
    
    if err != nil {
        return LLMOutput{}, err
    }
    
    return LLMOutput{
        Response: response.Text,
        Usage:    response.Usage,
    }, nil
}
```

### 3. Registration System with Schema Generation (recipe-worker)

The registration system and all schema generation logic lives in `recipe-worker`:

```go
// Located in: recipe-worker/pkg/worker/activity_registry.go
package worker

import (
    "fmt"
    "reflect"
    
    "github.com/invopop/jsonschema"
    "github.com/divisive-ai/vibethis/server/activity/pkg/types"
)

// ActivityRegistration holds the activity and its generated schemas
type ActivityRegistration struct {
    Activity     interface{}         // The generic activity interface
    ConfigSchema *jsonschema.Schema
    InputSchema  *jsonschema.Schema
    OutputSchema *jsonschema.Schema
    Metadata     types.ActivityMetadata
}

// ActivityRegistry manages all registered activities
type ActivityRegistry struct {
    activities map[string]ActivityRegistration
    generator  SchemaGenerator
}

// SchemaGenerator validates struct tags and generates JSON schemas
// This is internal to recipe-worker
type SchemaGenerator interface {
    GenerateSchema(typ reflect.Type) (*jsonschema.Schema, error)
    ValidateStructTags(typ reflect.Type) error
}

// Register accepts any generic RegisterableActivity from the activity module
func Register[TConfig any, TInput any, TOutput any](
    r *ActivityRegistry,
    activity types.RegisterableActivity[TConfig, TInput, TOutput],
) error {
    metadata := activity.GetMetadata()
    
    if _, exists := r.activities[metadata.Type]; exists {
        return fmt.Errorf("activity type %s already registered", metadata.Type)
    }
    
    // Generate schemas from types using reflection on the generic type parameters
    var config TConfig
    var input TInput
    var output TOutput
    
    // Validate all struct fields have json tags before generating schemas
    if err := r.generator.ValidateStructTags(reflect.TypeOf(config)); err != nil {
        return fmt.Errorf("config type validation failed: %w", err)
    }
    if err := r.generator.ValidateStructTags(reflect.TypeOf(input)); err != nil {
        return fmt.Errorf("input type validation failed: %w", err)
    }
    if err := r.generator.ValidateStructTags(reflect.TypeOf(output)); err != nil {
        return fmt.Errorf("output type validation failed: %w", err)
    }
    
    configSchema, err := r.generator.GenerateSchema(reflect.TypeOf(config))
    if err != nil {
        return fmt.Errorf("config schema generation failed: %w", err)
    }
    
    inputSchema, err := r.generator.GenerateSchema(reflect.TypeOf(input))
    if err != nil {
        return fmt.Errorf("input schema generation failed: %w", err)
    }
    
    outputSchema, err := r.generator.GenerateSchema(reflect.TypeOf(output))
    if err != nil {
        return fmt.Errorf("output schema generation failed: %w", err)
    }
    
    r.activities[metadata.Type] = ActivityRegistration{
        Activity:     activity,
        ConfigSchema: configSchema,
        InputSchema:  inputSchema,
        OutputSchema: outputSchema,
        Metadata:     metadata,
    }
    
    return nil
}

// DefaultSchemaGenerator uses invopop/jsonschema for schema generation
type DefaultSchemaGenerator struct {
    reflector *jsonschema.Reflector
}

func NewDefaultSchemaGenerator() *DefaultSchemaGenerator {
    reflector := &jsonschema.Reflector{
        // Require explicit json tags on all fields
        RequiredFromJSONSchemaTags: true,
        // Allow additional properties by default
        AllowAdditionalProperties: false,
        // Use JSON field names from tags
        FieldNameTag: "json",
    }
    
    return &DefaultSchemaGenerator{
        reflector: reflector,
    }
}

func (g *DefaultSchemaGenerator) GenerateSchema(typ reflect.Type) (*jsonschema.Schema, error) {
    // Use invopop/jsonschema to generate the schema
    schema := g.reflector.Reflect(typ)
    return schema, nil
}

func (g *DefaultSchemaGenerator) ValidateStructTags(typ reflect.Type) error {
    // Ensure type is a struct
    if typ.Kind() == reflect.Ptr {
        typ = typ.Elem()
    }
    
    if typ.Kind() != reflect.Struct {
        return nil // Non-structs don't need json tags
    }
    
    // Check each field has an explicit json tag
    for i := 0; i < typ.NumField(); i++ {
        field := typ.Field(i)
        
        // Skip unexported fields
        if !field.IsExported() {
            continue
        }
        
        // Check for json tag
        jsonTag := field.Tag.Get("json")
        if jsonTag == "" {
            return fmt.Errorf("field %s.%s is missing required json tag", typ.Name(), field.Name)
        }
        
        // If field is "-", it's explicitly ignored, which is fine
        if jsonTag == "-" {
            continue
        }
        
        // Recursively check nested structs
        fieldType := field.Type
        if fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array {
            fieldType = fieldType.Elem()
        }
        if fieldType.Kind() == reflect.Map {
            fieldType = fieldType.Elem()
        }
        
        if err := g.ValidateStructTags(fieldType); err != nil {
            return err
        }
    }
    
    return nil
}
```

### 4. Provider Bridge (recipe-worker)

A generic provider in `recipe-worker` bridges between the existing provider system and activities from the `activity` module:

```go
// Located in: recipe-worker/pkg/worker/activity_provider.go
package worker

import (
    "context"
    "encoding/json"
    "fmt"
    
    "github.com/divisive-ai/vibethis/server/activity/pkg/types"
)

// RegisterableActivityProvider wraps any RegisterableActivity for use in recipe-worker
type RegisterableActivityProvider[TConfig any, TInput any, TOutput any] struct {
    activity     types.RegisterableActivity[TConfig, TInput, TOutput]
    registration ActivityRegistration
}

func NewActivityProvider[TConfig any, TInput any, TOutput any](
    activity types.RegisterableActivity[TConfig, TInput, TOutput],
    registration ActivityRegistration,
) *RegisterableActivityProvider[TConfig, TInput, TOutput] {
    return &RegisterableActivityProvider[TConfig, TInput, TOutput]{
        activity:     activity,
        registration: registration,
    }
}

func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) GetType() string {
    return p.activity.GetMetadata().Type
}

func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) Execute(
    ctx context.Context,
    configMap, inputsMap map[string]interface{},
) (map[string]interface{}, error) {
    // Unmarshal config from map to typed struct
    configBytes, err := json.Marshal(configMap)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal config: %w", err)
    }
    
    var config TConfig
    if err := json.Unmarshal(configBytes, &config); err != nil {
        return nil, fmt.Errorf("failed to unmarshal config: %w", err)
    }
    
    // Unmarshal inputs from map to typed struct
    inputBytes, err := json.Marshal(inputsMap)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal inputs: %w", err)
    }
    
    var inputs TInput
    if err := json.Unmarshal(inputBytes, &inputs); err != nil {
        return nil, fmt.Errorf("failed to unmarshal inputs: %w", err)
    }
    
    // Execute with typed parameters
    output, err := p.activity.Execute(ctx, config, inputs)
    if err != nil {
        return nil, err
    }
    
    // Convert output back to map
    outputBytes, err := json.Marshal(output)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal output: %w", err)
    }
    
    var outputMap map[string]interface{}
    if err := json.Unmarshal(outputBytes, &outputMap); err != nil {
        return nil, fmt.Errorf("failed to unmarshal output: %w", err)
    }
    
    return outputMap, nil
}

func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) GetSchemas() (config, input, output *jsonschema.Schema) {
    return p.registration.ConfigSchema, p.registration.InputSchema, p.registration.OutputSchema
}
```

### 5. YAML Definition

Activities wrapped with this pattern can be used in YAML recipes:

```yaml
activities:
  - name: generate_summary
    type: llm_inference
    description: Generate a summary of the input text
    config:
      provider: openai
      model: gpt-4
      temperature: 0.7
    inputs:
      - name: text
        type: string
        required: true
    outputs:
      - name: summary
        type: string
```

### 6. Automatic Schema Derivation with invopop/jsonschema

The system uses [invopop/jsonschema](https://github.com/invopop/jsonschema) to automatically generate JSON schemas from Go structs:

```go
import "github.com/invopop/jsonschema"

// Example struct with jsonschema tags - ALL fields MUST have json tags
type CustomActivityConfig struct {
    URL         string   `json:"url" jsonschema:"title=Target URL,description=The endpoint URL,format=uri"`
    Method      string   `json:"method" jsonschema:"enum=GET,enum=POST,enum=PUT,enum=DELETE"`
    Headers     []Header `json:"headers,omitempty" jsonschema:"description=HTTP headers"`
    Timeout     int      `json:"timeout" jsonschema:"minimum=1,maximum=300,default=30,description=Timeout in seconds"`
    RetryCount  int      `json:"retry_count" jsonschema:"minimum=0,maximum=5,default=3"`
}

type Header struct {
    Name  string `json:"name" jsonschema:"required,description=Header name"`
    Value string `json:"value" jsonschema:"required,description=Header value"`
}

// Using invopop/jsonschema to generate the schema
reflector := &jsonschema.Reflector{
    RequiredFromJSONSchemaTags: true,
    AllowAdditionalProperties: false,
}
schema := reflector.Reflect(reflect.TypeOf(CustomActivityConfig{}))

// The generated schema will be:
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "title": "Target URL",
      "description": "The endpoint URL",
      "format": "uri"
    },
    "method": {
      "type": "string",
      "enum": ["GET", "POST", "PUT", "DELETE"]
    },
    "headers": {
      "type": "array",
      "description": "HTTP headers",
      "items": {
        "$ref": "#/$defs/Header"
      }
    },
    "timeout": {
      "type": "integer",
      "minimum": 1,
      "maximum": 300,
      "default": 30,
      "description": "Timeout in seconds"
    },
    "retry_count": {
      "type": "integer",
      "minimum": 0,
      "maximum": 5,
      "default": 3
    }
  },
  "required": ["url", "method", "timeout", "retry_count"],
  "$defs": {
    "Header": {
      "type": "object",
      "required": ["name", "value"],
      "properties": {
        "name": {
          "type": "string",
          "description": "Header name"
        },
        "value": {
          "type": "string",
          "description": "Header value"
        }
      }
    }
  }
}
```

#### Supported jsonschema Tags

The invopop/jsonschema library supports extensive validation tags:

- **String validations**: `minLength`, `maxLength`, `pattern`, `format` (email, uri, date-time, etc.)
- **Numeric validations**: `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`
- **Array validations**: `minItems`, `maxItems`, `uniqueItems`
- **Object validations**: `minProperties`, `maxProperties`
- **General**: `enum`, `const`, `default`, `examples`, `title`, `description`
- **Required fields**: `required` or `jsonschema_required:"true"`

#### Registration Failure Cases

Registration will fail if:
1. Any exported struct field is missing a `json` tag
2. Nested structs have fields without `json` tags
3. The schema generation encounters an unsupported type

```go
// This will FAIL registration - missing json tags
type BadConfig struct {
    URL    string // ERROR: missing json tag
    Method string `json:"method"`
}

// This will PASS registration - all fields have json tags
type GoodConfig struct {
    URL    string `json:"url"`
    Method string `json:"method"`
}

// This will also PASS - explicitly ignored field
type ConfigWithIgnored struct {
    URL      string `json:"url"`
    Method   string `json:"method"`
    Internal string `json:"-"` // Explicitly ignored
}
```

### 7. Activity Registration in recipe-worker

Recipe-worker is responsible for discovering and registering activities:

```go
// Located in: recipe-worker/cmd/worker/main.go or recipe-worker/pkg/worker/init.go
package main

import (
    "github.com/divisive-ai/vibethis/server/activity/pkg/llm"
    "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
)

func initializeActivities(registry *worker.ActivityRegistry) {
    // Import and register LLM activity
    llmActivity := llm.NewLLMActivity()
    worker.Register[llm.LLMConfig, llm.LLMInput, llm.LLMOutput](registry, llmActivity)
    
    // Import and register other activities
    // emailActivity := email.NewEmailActivity()
    // worker.Register[email.Config, email.Input, email.Output](registry, emailActivity)
}

// Alternative: Activity modules can export a registration function
// Located in: activity/pkg/llm/exports.go
package llm

import "github.com/divisive-ai/vibethis/server/activity/pkg/types"

// GetActivities returns all activities exported by this package
func GetActivities() []interface{} {
    return []interface{}{
        NewLLMActivity(),
        // Add other activities from this package
    }
}

// Then in recipe-worker:
func registerAllActivities(registry *worker.ActivityRegistry) {
    // Register LLM activities
    for _, activity := range llm.GetActivities() {
        // Use type assertion to register with proper types
        switch a := activity.(type) {
        case types.RegisterableActivity[llm.LLMConfig, llm.LLMInput, llm.LLMOutput]:
            worker.Register[llm.LLMConfig, llm.LLMInput, llm.LLMOutput](registry, a)
        }
    }
}
```

## Implementation Steps

1. **Define Core Interface in activity module**:
   - Create `RegisterableActivity` interface in `./activity/pkg/types/activity.go`
   - Define `ActivityMetadata` and `RetryPolicy` types
   - No dependencies on recipe-worker or recipe-core

2. **Update existing activities**:
   - Modify activities in `./activity` to implement `RegisterableActivity`
   - Ensure all config/input/output structs have explicit JSON tags on every field
   - Export type definitions for use by consumers

3. **Add Dependencies to recipe-worker**: 
   - Add `github.com/invopop/jsonschema` to recipe-worker's go.mod
   - Add dependency on `./activity` module

4. **Implement Registry in recipe-worker**:
   - Create activity registry in `recipe-worker/pkg/worker/activity_registry.go`
   - Implement schema generator using invopop/jsonschema with strict JSON tag validation
   - Build generic provider to bridge registerable activities

5. **Update Worker initialization**:
   - Modify worker to import activities from `./activity`
   - Register activities during startup
   - Use the registry for activity discovery

6. **Add Tests**:
   - Test registration with missing JSON tags (should fail)
   - Test schema generation
   - Test activity execution through the provider bridge

7. **Documentation**:
   - Document the registration process
   - Provide examples of creating new activities
   - Show how recipe-worker consumes activities

## Benefits

1. **Clean Architecture**: Clear separation of concerns - activities don't depend on recipe-worker
2. **Type Safety**: Full compile-time type checking with generics instead of runtime map[string]interface{} conversions
3. **Automatic Schema Generation**: JSON schemas derived automatically from Go structs with tags
4. **Enforced Standards**: Registration fails if struct fields lack JSON tags, ensuring consistency
5. **Discoverability**: Activities self-document through metadata and schemas
6. **Extensibility**: Easy to add new activities by implementing the interface in the activity module
7. **Backwards Compatibility**: Existing provider system in recipe-worker continues to work
8. **YAML Support**: Activities automatically work with YAML definitions in recipe-core
9. **Reduced Boilerplate**: No need to manually define schemas or type conversion code
10. **Testability**: Activities can be tested independently without recipe-worker dependencies

## Example Usage

```go
// Define a custom activity with typed structs
type EmailConfig struct {
    SMTPServer string `json:"smtp_server" jsonschema:"required,format=hostname"`
    Port       int    `json:"port" jsonschema:"required,minimum=1,maximum=65535"`
    UseSSL     bool   `json:"use_ssl" jsonschema:"default=true"`
}

type EmailInput struct {
    To      []string `json:"to" jsonschema:"required,minItems=1"`
    Subject string   `json:"subject" jsonschema:"required"`
    Body    string   `json:"body" jsonschema:"required"`
}

type EmailOutput struct {
    MessageID string `json:"message_id"`
    SentAt    string `json:"sent_at" jsonschema:"format=date-time"`
}

// Implement the activity
type EmailActivityWrapper struct {
    emailService *email.Service
}

func (e *EmailActivityWrapper) GetMetadata() ActivityMetadata {
    return ActivityMetadata{
        Type:           "send_email",
        Name:           "Send Email",
        Description:    "Sends an email via SMTP",
        Version:        "1.0.0",
        DefaultTimeout: 30 * time.Second,
    }
}

func (e *EmailActivityWrapper) Execute(ctx context.Context, config EmailConfig, input EmailInput) (EmailOutput, error) {
    // Type-safe execution
    messageID, err := e.emailService.Send(ctx, config, input)
    if err != nil {
        return EmailOutput{}, err
    }
    
    return EmailOutput{
        MessageID: messageID,
        SentAt:    time.Now().Format(time.RFC3339),
    }, nil
}

// Register the activity
func init() {
    registry := worker.GetGlobalActivityRegistry()
    Register[EmailConfig, EmailInput, EmailOutput](registry, &EmailActivityWrapper{
        emailService: email.NewService(),
    })
}

// Use in YAML (schemas are automatically validated)
activities:
  - name: notify_user
    type: send_email
    config:
      smtp_server: mail.example.com
      port: 587
      use_ssl: true
    inputs:
      - name: to
        value: ["user@example.com"]
      - name: subject
        value: "Task Completed"
      - name: body
        value: "Your task has been completed successfully."
```

## Migration Path

1. **Phase 1**: Define `RegisterableActivity` interface in `./activity`
2. **Phase 2**: Update activities to implement the new interface while maintaining existing functionality
3. **Phase 3**: Update recipe-worker to consume activities via the new interface
4. **Phase 4**: Existing provider system continues to work during transition
5. **Phase 5**: Once all activities are migrated, deprecate old provider implementations
6. **Note**: YAML recipes remain unchanged - only internal implementation changes