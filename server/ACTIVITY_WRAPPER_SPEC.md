# Activity Wrapper Specification for Recipe-Worker

## Overview

This specification defines how to create wrappers for activities in the `./activity` module to make them usable in `recipe-worker` through YAML definitions. The goal is to establish a standardized interface that allows any activity to be registered and executed within the recipe system using Go generics for type safety and automatic schema derivation.

## Architecture

### 1. Core Interface Definition (recipe-core)

A new generic interface will be defined in `recipe-core` that standardizes how activities are shaped for registration:

```go
// RegisterableActivity defines the contract for activities that can be registered
// and used in recipe-worker via YAML definitions
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

// SchemaGenerator validates struct tags and generates JSON schemas
type SchemaGenerator interface {
    // GenerateSchema creates a JSON schema from a Go type
    // Returns error if any struct field is missing a json tag
    GenerateSchema(typ reflect.Type) (*jsonschema.Schema, error)
    
    // ValidateStructTags ensures all fields have explicit json tags
    ValidateStructTags(typ reflect.Type) error
}
```

### 2. Activity Wrapper Pattern

Each activity in the `./activity` module will have a corresponding wrapper that implements `RegisterableActivity`:

```go
// Define typed structs for LLM activity
type LLMConfig struct {
    Provider    string  `json:"provider" jsonschema:"enum=openai,enum=anthropic,enum=bedrock,required"`
    Model       string  `json:"model" jsonschema:"required"`
    Temperature float64 `json:"temperature,omitempty" jsonschema:"minimum=0,maximum=2"`
}

type LLMInput struct {
    Prompt    string `json:"prompt" jsonschema:"required"`
    System    string `json:"system,omitempty"`
    MaxTokens int    `json:"max_tokens,omitempty"`
}

type LLMOutput struct {
    Response string                 `json:"response"`
    Usage    map[string]interface{} `json:"usage,omitempty"`
}

// Example: LLM Activity Wrapper
type LLMActivityWrapper struct {
    activity *llm.Activity
}

// Implement the generic interface
var _ RegisterableActivity[LLMConfig, LLMInput, LLMOutput] = (*LLMActivityWrapper)(nil)

func NewLLMActivityWrapper() RegisterableActivity[LLMConfig, LLMInput, LLMOutput] {
    return &LLMActivityWrapper{
        activity: llm.NewActivity(),
    }
}

func (w *LLMActivityWrapper) GetMetadata() ActivityMetadata {
    return ActivityMetadata{
        Type:           "llm_inference",
        Name:           "LLM Inference",
        Description:    "Executes LLM inference with various providers",
        Version:        "1.0.0",
        DefaultTimeout: 5 * time.Minute,
    }
}

func (w *LLMActivityWrapper) Execute(ctx context.Context, config LLMConfig, inputs LLMInput) (LLMOutput, error) {
    // Direct execution with typed parameters
    response, err := w.activity.Execute(ctx, llm.Request{
        Provider:    config.Provider,
        Model:       config.Model,
        Temperature: config.Temperature,
        Prompt:      inputs.Prompt,
        System:      inputs.System,
        MaxTokens:   inputs.MaxTokens,
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

### 3. Registration System with Schema Generation

Activities will be registered with the recipe-worker system, including automatic schema generation:

```go
// In recipe-worker/pkg/worker/activity_registry.go

// ActivityRegistration holds the activity and its generated schemas
type ActivityRegistration struct {
    Activity     interface{}         // The generic activity interface
    ConfigSchema *jsonschema.Schema
    InputSchema  *jsonschema.Schema
    OutputSchema *jsonschema.Schema
    Metadata     ActivityMetadata
}

type ActivityRegistry struct {
    activities map[string]ActivityRegistration
    generator  SchemaGenerator
}

// Register accepts any generic RegisterableActivity and generates schemas
func Register[TConfig any, TInput any, TOutput any](
    r *ActivityRegistry,
    activity RegisterableActivity[TConfig, TInput, TOutput],
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

### 4. Provider Bridge

A generic provider will bridge between the existing provider system and the new registerable activities:

```go
// Generic provider that wraps any RegisterableActivity
type RegisterableActivityProvider[TConfig any, TInput any, TOutput any] struct {
    activity     RegisterableActivity[TConfig, TInput, TOutput]
    registration ActivityRegistration
}

func NewActivityProvider[TConfig any, TInput any, TOutput any](
    activity RegisterableActivity[TConfig, TInput, TOutput],
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

### 7. Auto-Registration

Activities can be auto-registered during package initialization:

```go
// In activity/pkg/llm/register.go
func init() {
    registry := worker.GetGlobalActivityRegistry()
    Register(registry, NewLLMActivityWrapper())
}

// Or with a helper function that handles the generic registration
func RegisterLLMActivity(registry *ActivityRegistry) {
    wrapper := NewLLMActivityWrapper()
    Register[LLMConfig, LLMInput, LLMOutput](registry, wrapper)
}
```

## Implementation Steps

1. **Add Dependencies**: Add `github.com/invopop/jsonschema` to go.mod
2. **Define Core Interface**: Create `RegisterableActivity` interface in `recipe-core/pkg/recipe/activity.go`
3. **Create Registry**: Implement activity registry in `recipe-worker/pkg/worker/activity_registry.go`
4. **Implement Schema Generator**: Create schema generator using invopop/jsonschema with strict JSON tag validation
5. **Build Provider Bridge**: Create generic provider to bridge registerable activities
6. **Wrap Existing Activities**: Create wrappers for activities in `./activity` module ensuring all structs have JSON tags
7. **Update Worker**: Modify worker to use the registry for activity discovery
8. **Add Tests**: Create comprehensive tests for registration, validation, and execution
9. **Documentation**: Update documentation with examples and best practices

## Required Imports

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "reflect"
    "time"
    
    "github.com/invopop/jsonschema"
)
```

## Benefits

1. **Type Safety**: Full compile-time type checking with generics instead of runtime map[string]interface{} conversions
2. **Automatic Schema Generation**: JSON schemas derived automatically from Go structs with tags
3. **Standardization**: All activities follow the same interface
4. **Discoverability**: Activities self-document through metadata and schemas
5. **Extensibility**: Easy to add new activities by implementing the interface
6. **Backwards Compatibility**: Existing provider system continues to work
7. **YAML Support**: Activities automatically work with YAML definitions
8. **Reduced Boilerplate**: No need to manually define schemas or type conversion code

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

1. Existing activities continue to work through the current provider system
2. New wrappers can be added incrementally
3. Once all activities are wrapped, the old system can be deprecated
4. YAML recipes remain unchanged, only internal implementation changes