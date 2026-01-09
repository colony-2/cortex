# Op Input Default Property Resolution Specification

## Problem Statement

Operations (ops) define input fields via Go structs. Currently, there is no way to specify defaults for optional fields, including nested fields. The challenge is that default values need to support:

1. **Literal values** - Static defaults (e.g., `"main"`, `300`, `true`)
2. **Contextual values** - Dynamic values from execution context (e.g., `"{{ context.git.branch }}"`)
3. **Nested fields** - Defaults at any depth in the struct hierarchy

We already have a template resolution system in `recipe-template` that handles both literals and CEL expressions using `{{ }}` syntax. We should leverage this instead of reimplementing type coercion and expression evaluation.

## Proposed Solution

Add a `default` struct tag to op input fields at any depth. Before template resolution, inject tag values into the input map (including nested maps) for any fields that weren't explicitly provided. Then let the existing template resolver handle literal/expression evaluation and type conversion.

### Architecture

```
1. Raw YAML/JSON inputs → parsed to map[string]interface{}
2. Apply defaults: inject tag values for missing keys (at any depth)
3. Template resolution: resolve {{ }} expressions (existing system)
4. DecodeWithJsonTags: unmarshal to typed struct (existing)
5. Execute op
```

## Tag Syntax

```go
type OpInput struct {
    FieldName Type `json:"field_name" default:"value"`

    Nested NestedStruct `json:"nested"`
}

type NestedStruct struct {
    DeepField string `json:"deep_field" default:"deep_value"`
}
```

The `default` tag value is **always injected as a string** into the input map at the appropriate nesting level. It can be:
- A literal value: `default:"main"` or `default:"5432"`
- A template expression: `default:"{{ context.git.branch }}"`
- A CEL expression: `default:"{{ inputs.replicas * 2 }}"`

The template resolver will convert the string to the appropriate type during resolution.

## Examples

### Example 1: Nested Defaults

```go
type DatabaseConfig struct {
    Host     string `json:"host" default:"localhost"`
    Port     int    `json:"port" default:"5432"`
    Database string `json:"database" default:"{{ context.project_id }}"`
}

type ServiceInput struct {
    ServiceName string         `json:"service_name"`
    Config      DatabaseConfig `json:"config"`
    Replicas    int           `json:"replicas" default:"3"`
}
```

**YAML invocation:**
```yaml
type: deploy_service
service_name: "api"
config:
  host: "prod-db.example.com"
  # port defaults to 5432
  # database defaults to context.project_id (resolved via template)
# replicas defaults to 3
```

**Resulting input map after default injection:**
```go
{
    "service_name": "api",
    "config": {
        "host": "prod-db.example.com",
        "port": "5432",           // injected as string
        "database": "{{ context.project_id }}", // injected as string
    },
    "replicas": "3", // injected as string
}
```

Then template resolution converts `"5432"` → `int(5432)`, `"3"` → `int(3)`, etc.

### Example 2: Dynamic Integer Defaults

```go
type ScalingInput struct {
    MinReplicas int `json:"min_replicas" default:"{{ inputs.base_replicas }}"`
    MaxReplicas int `json:"max_replicas" default:"{{ inputs.base_replicas * 3 }}"`
    Port        int `json:"port" default:"{{ context.service.port ?: 8080 }}"`
}
```

All defaults are injected as strings containing CEL expressions, which the template resolver evaluates to integers.

## Implementation

### InjectDefaults Function

```go
// In recipe-core/pkg/ops/defaults.go

import (
    "fmt"
    "reflect"
    "strings"
)

// InjectDefaults adds default values to the input map for fields that are missing.
// Defaults are injected as strings and will be resolved by the template system.
func InjectDefaults(inputType reflect.Type, inputMap map[string]interface{}) error {
    if inputType.Kind() == reflect.Ptr {
        inputType = inputType.Elem()
    }

    if inputType.Kind() != reflect.Struct {
        return nil
    }

    return injectDefaultsRecursive(inputType, inputMap)
}

func injectDefaultsRecursive(structType reflect.Type, inputMap map[string]interface{}) error {
    for i := 0; i < structType.NumField(); i++ {
        field := structType.Field(i)

        // Skip unexported fields
        if !field.IsExported() {
            continue
        }

        // Get JSON tag to determine map key
        jsonTag := field.Tag.Get("json")
        if jsonTag == "" || jsonTag == "-" {
            continue
        }

        // Extract field name (before comma, which may have omitempty etc)
        fieldName := strings.Split(jsonTag, ",")[0]
        if fieldName == "" {
            continue
        }

        // Dereference pointer types to check underlying type
        fieldType := field.Type
        if fieldType.Kind() == reflect.Ptr {
            fieldType = fieldType.Elem()
        }

        // Handle nested structs
        if fieldType.Kind() == reflect.Struct {
            // Get or create nested map
            var nestedMap map[string]interface{}
            if existing, exists := inputMap[fieldName]; exists {
                var ok bool
                nestedMap, ok = existing.(map[string]interface{})
                if !ok {
                    // User provided a non-map value, skip recursion
                    continue
                }
            } else {
                // Create nested map for defaults
                nestedMap = make(map[string]interface{})
                inputMap[fieldName] = nestedMap
            }

            // Recurse into nested struct
            if err := injectDefaultsRecursive(fieldType, nestedMap); err != nil {
                return err
            }

            // If nested map is empty after recursion, remove it
            if len(nestedMap) == 0 {
                delete(inputMap, fieldName)
            }

            continue
        }

        // For non-struct fields, check if value already provided
        if _, exists := inputMap[fieldName]; exists {
            continue
        }

        // Check for default tag
        defaultValue := field.Tag.Get("default")
        if defaultValue == "" {
            continue
        }

        // Inject default as string (template resolver will handle type conversion)
        inputMap[fieldName] = defaultValue
    }

    return nil
}
```

### Key Implementation Details

1. **Recursive Walking**: Handles nested structs at arbitrary depth
2. **String Injection**: Always injects defaults as strings, regardless of field type
3. **Precedence**: User-provided values always take precedence
4. **Empty Map Cleanup**: Removes empty nested maps created during recursion
5. **Pointer Handling**: Dereferences pointer types to check underlying type

### Wiring Into Execution Path

```go
// In recipe-template or recipe-worker, before template resolution:

err := ops.InjectDefaults(step.InputType, inputMap)
if err != nil {
    return nil, fmt.Errorf("failed to inject defaults: %w", err)
}

// Then proceed with existing template resolution:
resolvedInput, err := resolutionContext.ResolveMap(inputMap)
if err != nil {
    return nil, fmt.Errorf("failed to resolve template: %w", err)
}

// ... rest of existing code
```

## Type Handling

Type coercion is **already handled** by the existing template resolution system:

- String `"1"` → int conversion
- String `"true"` → bool conversion
- String `"5432"` → int conversion
- CEL expression `"{{ inputs.count * 2 }}"` → evaluates to int
- Nested structures and arrays

After defaults are injected as strings, the template resolver converts them to the correct types during the `ResolveMap` call.

## Testing Strategy

### Unit Tests in recipe-core

```go
func TestInjectDefaults_TopLevel(t *testing.T) {
    type TestInput struct {
        Name  string `json:"name" default:"test"`
        Count int    `json:"count" default:"5"`
    }

    inputMap := map[string]interface{}{
        "name": "override",
    }

    err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
    assert.NoError(t, err)

    assert.Equal(t, "override", inputMap["name"]) // User value preserved
    assert.Equal(t, "5", inputMap["count"])       // Default injected as string
}

func TestInjectDefaults_Nested(t *testing.T) {
    type DatabaseConfig struct {
        Host string `json:"host" default:"localhost"`
        Port int    `json:"port" default:"5432"`
    }

    type TestInput struct {
        Name   string         `json:"name" default:"test"`
        Config DatabaseConfig `json:"config"`
    }

    inputMap := map[string]interface{}{
        "config": map[string]interface{}{
            "host": "prod.example.com",
        },
    }

    err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
    assert.NoError(t, err)

    // Top-level default injected
    assert.Equal(t, "test", inputMap["name"])

    // Nested structure preserved
    configMap := inputMap["config"].(map[string]interface{})
    assert.Equal(t, "prod.example.com", configMap["host"]) // User value
    assert.Equal(t, "5432", configMap["port"])              // Default injected as string
}

func TestInjectDefaults_TemplateExpression(t *testing.T) {
    type TestInput struct {
        Branch string `json:"branch" default:"{{ context.git.branch }}"`
        Port   int    `json:"port" default:"{{ inputs.base_port + 1000 }}"`
    }

    inputMap := map[string]interface{}{}

    err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
    assert.NoError(t, err)

    // Template strings injected (will be resolved later)
    assert.Equal(t, "{{ context.git.branch }}", inputMap["branch"])
    assert.Equal(t, "{{ inputs.base_port + 1000 }}", inputMap["port"])
}

func TestInjectDefaults_EmptyNestedStruct(t *testing.T) {
    type Config struct {
        Setting string `json:"setting" default:"value"`
    }

    type TestInput struct {
        Config Config `json:"config"`
    }

    inputMap := map[string]interface{}{}

    err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
    assert.NoError(t, err)

    // Nested map created with default
    configMap := inputMap["config"].(map[string]interface{})
    assert.Equal(t, "value", configMap["setting"])
}

func TestInjectDefaults_NoDefaults(t *testing.T) {
    type Config struct {
        Setting string `json:"setting"` // no default
    }

    type TestInput struct {
        Config Config `json:"config"`
    }

    inputMap := map[string]interface{}{}

    err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
    assert.NoError(t, err)

    // No nested map created since no defaults exist
    _, exists := inputMap["config"]
    assert.False(t, exists)
}

func TestInjectDefaults_PointerFields(t *testing.T) {
    type Config struct {
        Setting string `json:"setting" default:"value"`
    }

    type TestInput struct {
        Config *Config `json:"config"`
    }

    inputMap := map[string]interface{}{}

    err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
    assert.NoError(t, err)

    // Nested map created for pointer struct
    configMap := inputMap["config"].(map[string]interface{})
    assert.Equal(t, "value", configMap["setting"])
}
```

### Integration Tests in recipe-template

Test end-to-end with actual template resolution and type conversion:

```go
func TestDefaultsWithTemplateResolution(t *testing.T) {
    type TestInput struct {
        Port int `json:"port" default:"8080"`
    }

    inputMap := map[string]interface{}{}

    // Inject defaults
    err := ops.InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
    assert.NoError(t, err)

    // port should be "8080" (string)
    assert.Equal(t, "8080", inputMap["port"])

    // Resolve templates (this will convert "8080" to int 8080)
    resolvedInput, err := resolutionContext.ResolveMap(inputMap)
    assert.NoError(t, err)

    // Decode to struct
    var input TestInput
    err = ops.DecodeWithJsonTags(resolvedInput, &input)
    assert.NoError(t, err)

    // Should be int 8080
    assert.Equal(t, 8080, input.Port)
}
```

## Edge Cases

### Empty Nested Structs

```yaml
type: my_op
# config not provided at all
```

The system should:
1. Create `config: {}` in the map during recursion
2. Inject defaults for config fields
3. Result: `config: {host: "localhost", port: "5432"}`

### Partial Nested Structs

```yaml
type: my_op
config:
  host: "custom.example.com"
```

The system should:
1. Preserve user's `config.host`
2. Inject `config.port` default
3. Result: `config: {host: "custom.example.com", port: "5432"}`

### Nested Structs Without Defaults

```go
type Config struct {
    Setting string `json:"setting"` // no default tag
}

type Input struct {
    Config Config `json:"config"`
}
```

If user provides nothing, the nested map is created, recursion happens, but since no defaults exist, the empty map is cleaned up and removed.

### User Provides Non-Map for Nested Struct

```yaml
type: my_op
config: "some-string"  # wrong type
```

The system skips recursion since the user provided a non-map value. Template resolution will fail later with a type error.

## Performance Considerations

- Reflection happens once per op invocation
- O(n) where n = total number of fields across all nesting levels
- Minimal allocations (only for nested maps with defaults)
- No library overhead, just standard library reflection

## Migration Path

### Phase 1: Implementation
1. Implement `InjectDefaults` in recipe-core/pkg/ops
2. Wire into execution path before template resolution
3. Add comprehensive unit tests

### Phase 2: Adoption
- Add `default` tags to existing ops where appropriate
- Document pattern in developer guide
- Provide examples of nested defaults

### Phase 3: Tooling (Optional)
- Linter to validate default tag syntax
- Auto-generate documentation from default tags

## Backward Compatibility

- Existing ops without tags work unchanged
- Zero values remain the fallback for fields without defaults
- No breaking changes to YAML/JSON schema
- Nested structs without defaults are unaffected

## Benefits of This Approach

1. **Handles arbitrary depth** - Works with any level of nesting
2. **Reuses existing infrastructure** - Template resolution handles type conversion
3. **No external dependencies** - Uses only standard library reflection
4. **Simple** - ~60 lines of code
5. **Powerful** - Full CEL expression support for nested fields
6. **Type-safe** - Existing type checking applies to defaults

## Summary

This specification introduces a `default` struct tag for op input fields at any depth. A recursive reflection-based function walks the struct type and injects default tag values as strings into the input map at the appropriate nesting level. The existing template/CEL system then handles all expression evaluation and type coercion, making this a simple, powerful solution with minimal code.
