# Cortex Schema Generation Fix Specification v2

## Problem Statement

The current `cortex schema` command output is incomplete and doesn't provide enough information for users to understand how to properly invoke activities. Additionally, there's significant code duplication between the schema, execute, and validate commands that should be eliminated.

### Core Issues

1. **Missing Required Fields**: The schema doesn't indicate which fields are required vs optional for each activity
2. **No Field Details**: Field descriptions, types, and constraints are not shown  
3. **Unclear Activity Configuration**: Users cannot determine from the schema what configuration is needed (e.g., that `run` field is required for command_execution)
4. **Lazy Schema Generation**: Schemas are only generated when accessed, but the cortex schema command doesn't trigger this generation
5. **Code Duplication**: All three commands (schema, execute, validate) independently:
   - Create and configure activity registries
   - Register activities from opsactivity.GetAll()
   - Parse and validate recipe YAML files
   - Handle recipe inputs/outputs

## Current Code Duplication Analysis

### Duplicated in All Three Commands
```go
// Activity registry initialization (schema.go:42-52, execute.go:484-493, validate.go:99-109)
registry := worker.NewActivityRegistry()
activities := activity.GetAll()
for _, act := range activities {
    if err := registry.RegisterGeneric(act); err != nil {
        if !strings.Contains(err.Error(), "already registered") {
            return fmt.Errorf("failed to register activity: %w", err)
        }
    }
}
```

### Duplicated Between Execute and Validate
- Recipe YAML parsing and validation
- Input validation logic
- Recipe structure validation

### Opportunities for Shared Components
1. **Registry Manager**: Single place to initialize and configure activity registry (used by all 3 commands)
2. **Recipe Validator**: Common recipe structure validation (used by execute and validate)
3. **Schema Manager**: Centralized schema generation and management (used by schema and validate)

## Proposed Architecture

### 1. Create Shared Components Package

Create `server/cortex/internal/shared` package with:

```go
// registry.go
package shared

import (
    "github.com/divisive-ai/vibethis/server/ops/pkg/activity"
    "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
)

type RegistryManager struct {
    registry *worker.ActivityRegistry
    executor *executor.StandaloneExecutor
}

func NewRegistryManager(logger *zap.Logger) (*RegistryManager, error) {
    registry := worker.NewActivityRegistry()
    activities := activity.GetAll()
    
    for _, act := range activities {
        if err := registry.RegisterGeneric(act); err != nil {
            if !strings.Contains(err.Error(), "already registered") {
                return nil, fmt.Errorf("failed to register activity: %w", err)
            }
        }
    }
    
    // Also create standalone executor if needed
    exec, err := executor.NewStandaloneExecutor(logger)
    if err != nil {
        return nil, err
    }
    
    return &RegistryManager{
        registry: registry,
        executor: exec,
    }, nil
}

func (rm *RegistryManager) GetRegistry() *worker.ActivityRegistry {
    return rm.registry
}

func (rm *RegistryManager) GetExecutor() *executor.StandaloneExecutor {
    return rm.executor
}
```

```go
// validator.go
package shared

import (
    yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

type RecipeValidator struct {
    registryManager *RegistryManager
}

func NewRecipeValidator(rm *RegistryManager) *RecipeValidator {
    return &RecipeValidator{registryManager: rm}
}

func (rv *RecipeValidator) ValidateRecipeStructure(recipe *yamlpkg.RecipeDefinition) error {
    if recipe.Name == "" {
        return fmt.Errorf("recipe name is required")
    }
    
    if len(recipe.Steps) == 0 {
        return fmt.Errorf("recipe must have at least one step")
    }
    
    // Validate each step has required fields
    for i, step := range recipe.Steps {
        if step.ID == "" {
            return fmt.Errorf("step %d: id is required", i)
        }
        if step.Uses == "" && step.Parallel == nil {
            return fmt.Errorf("step %s: must specify uses or parallel", step.ID)
        }
        
        // Validate activity exists
        if step.Uses != "" {
            if _, exists := rv.registryManager.GetRegistry().Get(step.Uses); !exists {
                return fmt.Errorf("step %s: unknown activity type %s", step.ID, step.Uses)
            }
        }
    }
    
    return nil
}

func (rv *RecipeValidator) ValidateInputs(recipe *yamlpkg.RecipeDefinition, inputs map[string]interface{}) error {
    // Check required inputs
    for _, input := range recipe.Inputs {
        if input.Required {
            if _, exists := inputs[input.Name]; !exists {
                return fmt.Errorf("required input '%s' not provided", input.Name)
            }
        }
    }
    
    // TODO: Add type validation
    
    return nil
}
```

```go
// schema.go
package shared

import (
    "github.com/invopop/jsonschema"
)

type SchemaManager struct {
    registryManager *RegistryManager
}

func NewSchemaManager(rm *RegistryManager) *SchemaManager {
    return &SchemaManager{registryManager: rm}
}

func (sm *SchemaManager) GenerateCompleteSchema(filterActivity string, includeVersion bool, includeExamples bool) (map[string]interface{}, error) {
    registry := sm.registryManager.GetRegistry()
    
    // Ensure all schemas are generated
    sm.ensureSchemasGenerated(registry)
    
    // Build complete schema with all details
    return sm.buildCompleteSchema(registry, filterActivity, includeVersion, includeExamples)
}

func (sm *SchemaManager) ensureSchemasGenerated(registry *worker.ActivityRegistry) {
    // Force schema generation for all registered activities
    for activityType, registration := range registry.GetAll() {
        if registration.ConfigSchema == nil || registration.InputSchema == nil {
            sm.generateSchemasForActivity(&registration)
            registry.UpdateRegistration(activityType, registration)
        }
    }
}

func (sm *SchemaManager) generateSchemasForActivity(registration *worker.ActivityRegistration) {
    // Use reflection to extract types from Execute method
    activityValue := reflect.ValueOf(registration.Activity)
    executeMethod := activityValue.MethodByName("Execute")
    
    if executeMethod.IsValid() {
        methodType := executeMethod.Type()
        if methodType.NumIn() >= 3 && methodType.NumOut() >= 1 {
            // Generate schemas from types
            generator := worker.NewDefaultSchemaGenerator()
            
            configType := methodType.In(1)
            registration.ConfigSchema, _ = generator.GenerateSchema(configType)
            
            inputType := methodType.In(2)
            registration.InputSchema, _ = generator.GenerateSchema(inputType)
            
            outputType := methodType.Out(0)
            registration.OutputSchema, _ = generator.GenerateSchema(outputType)
        }
    }
}
```

### 2. Update Commands to Use Shared Components

#### Schema Command
```go
func runSchema(cmd *cobra.Command, args []string) error {
    logger := createLogger()
    
    // Use shared registry manager
    rm, err := shared.NewRegistryManager(logger)
    if err != nil {
        return err
    }
    
    // Use shared schema manager
    sm := shared.NewSchemaManager(rm)
    schema, err := sm.GenerateCompleteSchema(schemaActivity, schemaVersion, schemaIncludeExamples)
    if err != nil {
        return err
    }
    
    // Output schema...
}
```

#### Execute Command
```go
func runExecute(cmd *cobra.Command, args []string) error {
    logger := setupLogger(executeLogLevel, executeLogFormat, executeNoColor)
    
    // Use shared registry manager
    rm, err := shared.NewRegistryManager(logger)
    if err != nil {
        return err
    }
    
    // Load recipe directly (no shared loader needed)
    recipeData, err := os.ReadFile(recipeFile)
    if err != nil {
        return fmt.Errorf("failed to read recipe file: %w", err)
    }
    
    var recipe yamlpkg.RecipeDefinition
    if err := yaml.Unmarshal(recipeData, &recipe); err != nil {
        return fmt.Errorf("failed to parse recipe YAML: %w", err)
    }
    
    // Use shared validator
    validator := shared.NewRecipeValidator(rm)
    if err := validator.ValidateRecipeStructure(&recipe); err != nil {
        return err
    }
    
    // Parse inputs (specific to execute command)
    inputs, err := parseInputs(executeInputs, executeInputFile)
    if err != nil {
        return err
    }
    
    if err := validator.ValidateInputs(&recipe, inputs); err != nil {
        return err
    }
    
    // Execute using the standalone executor from registry manager
    executor := rm.GetExecutor()
    outputs, err := executor.Execute(ctx, recipe, inputs)
    // ...
}
```

#### Validate Command
```go
func runValidate(cmd *cobra.Command, args []string) error {
    logger := createLogger()
    
    // Use shared registry manager
    rm, err := shared.NewRegistryManager(logger)
    if err != nil {
        return err
    }
    
    // Generate schema using shared schema manager
    sm := shared.NewSchemaManager(rm)
    schemaMap, err := sm.GenerateCompleteSchema("", false, false)
    if err != nil {
        return err
    }
    
    // Use shared validator
    validator := shared.NewRecipeValidator(rm)
    
    for _, file := range files {
        // Load recipe directly
        recipeData, err := os.ReadFile(file)
        if err != nil {
            // Handle error
            continue
        }
        
        var recipe yamlpkg.RecipeDefinition
        if err := yaml.Unmarshal(recipeData, &recipe); err != nil {
            // Handle parse error
            continue
        }
        
        if err := validator.ValidateRecipeStructure(&recipe); err != nil {
            // Add to validation errors
        }
    }
    // ...
}
```

### 3. Fix Schema Generation in ActivityRegistry

Update `RegisterGeneric` in `activity_registry.go`:

```go
func (r *ActivityRegistry) RegisterGeneric(activity interface{}) error {
    // ... existing metadata extraction ...
    
    registration := ActivityRegistration{
        Activity: activity,
        Metadata: metadata,
    }
    
    // Generate schemas immediately using reflection
    r.generateSchemasForRegistration(&registration)
    
    r.activities[metadata.Type] = registration
    return nil
}

func (r *ActivityRegistry) generateSchemasForRegistration(registration *ActivityRegistration) {
    activityValue := reflect.ValueOf(registration.Activity)
    executeMethod := activityValue.MethodByName("Execute")
    
    if !executeMethod.IsValid() {
        return
    }
    
    methodType := executeMethod.Type()
    if methodType.NumIn() < 3 || methodType.NumOut() < 1 {
        return
    }
    
    // Extract types
    configType := methodType.In(1)
    inputType := methodType.In(2)
    outputType := methodType.Out(0)
    
    // Generate schemas
    if r.generator != nil {
        registration.ConfigSchema, _ = r.generator.GenerateSchema(configType)
        registration.InputSchema, _ = r.generator.GenerateSchema(inputType)
        registration.OutputSchema, _ = r.generator.GenerateSchema(outputType)
    }
}

// Add method to update registration (for schema manager)
func (r *ActivityRegistry) UpdateRegistration(activityType string, registration ActivityRegistration) {
    r.activities[activityType] = registration
}
```

### 4. Enhance Schema Output

Create enhanced schema converter that preserves all field information:

```go
func convertSchemaWithDetails(schema *jsonschema.Schema) map[string]interface{} {
    if schema == nil {
        return nil
    }
    
    result := make(map[string]interface{})
    
    // Basic type info
    if schema.Type != "" {
        result["type"] = schema.Type
    }
    
    // Description
    if schema.Description != "" {
        result["description"] = schema.Description
    }
    
    // Properties with full details
    if len(schema.Properties) > 0 {
        props := make(map[string]interface{})
        for name, propSchema := range schema.Properties {
            props[name] = convertSchemaWithDetails(propSchema)
        }
        result["properties"] = props
    }
    
    // Required fields
    if len(schema.Required) > 0 {
        result["required"] = schema.Required
    }
    
    // Additional constraints
    if schema.MinLength != nil {
        result["minLength"] = *schema.MinLength
    }
    if schema.MaxLength != nil {
        result["maxLength"] = *schema.MaxLength
    }
    if schema.Minimum != nil {
        result["minimum"] = *schema.Minimum
    }
    if schema.Maximum != nil {
        result["maximum"] = *schema.Maximum
    }
    if schema.Pattern != nil {
        result["pattern"] = *schema.Pattern
    }
    if len(schema.Enum) > 0 {
        result["enum"] = schema.Enum
    }
    
    // Handle additionalProperties
    if schema.AdditionalProperties != nil {
        if additionalSchema, ok := schema.AdditionalProperties.(*jsonschema.Schema); ok {
            result["additionalProperties"] = convertSchemaWithDetails(additionalSchema)
        } else {
            result["additionalProperties"] = schema.AdditionalProperties
        }
    }
    
    // Handle arrays
    if schema.Items != nil {
        result["items"] = convertSchemaWithDetails(schema.Items)
    }
    
    return result
}
```

## Implementation Steps

1. **Create shared package structure**:
   ```bash
   mkdir -p server/cortex/internal/shared
   ```

2. **Implement shared components**:
   - `registry.go`: RegistryManager for activity registry management
   - `validator.go`: RecipeValidator for recipe structure validation
   - `schema.go`: SchemaManager for schema generation

3. **Update activity_registry.go**:
   - Add immediate schema generation in RegisterGeneric
   - Add UpdateRegistration method
   - Ensure schemas are always available

4. **Refactor commands**:
   - Update schema.go to use shared components
   - Update execute.go to use shared components (already uses StandaloneExecutor)
   - Update validate.go to use shared components

5. **Add jsonschema tags** to all activity structs:
   ```go
   type CommandExecutionInput struct {
       Run string `json:"run" jsonschema:"required,description=Command to execute"`
       WorkingDirectory string `json:"working_directory" jsonschema:"description=Override working directory"`
       Shell string `json:"shell" jsonschema:"description=Shell to use,enum=bash,enum=sh,enum=powershell,enum=cmd"`
       Env map[string]string `json:"env" jsonschema:"description=Environment variables"`
       ContinueOnError bool `json:"continue_on_error" jsonschema:"description=Continue on non-zero exit"`
       Timeout string `json:"timeout" jsonschema:"description=Timeout duration,pattern=^[0-9]+(s|m|h)$"`
   }
   ```

6. **Create comprehensive tests**:
   - Test schema generation completeness
   - Test shared component functionality
   - Test command integration

## Expected Benefits

### For Users
- **Complete Schema Information**: Users see all required fields, types, descriptions, and constraints
- **Better Error Messages**: Validation can provide specific field-level errors
- **Improved Documentation**: Schema serves as living documentation
- **Consistent Behavior**: All commands use the same validation logic

### For Developers
- **Single Source of Truth**: No duplicated logic between commands
- **Easier Maintenance**: Changes in one place affect all commands
- **Better Testing**: Shared components can be unit tested independently
- **Extensibility**: Easy to add new commands that leverage shared components
- **Type Safety**: Schema generation ensures type information is preserved

## Example Output After Implementation

```json
{
  "definitions": {
    "activities": {
      "command_execution": {
        "type": "object",
        "description": "Executes arbitrary shell commands with GitHub Actions-style configuration",
        "properties": {
          "type": {
            "const": "command_execution"
          },
          "config": {
            "type": "object",
            "properties": {
              "working_dir": {
                "type": "string",
                "description": "Working directory for command execution"
              },
              "shell": {
                "type": "string",
                "description": "Shell to use",
                "enum": ["bash", "sh", "powershell", "cmd"],
                "default": "bash"
              },
              "env": {
                "type": "object",
                "additionalProperties": {"type": "string"},
                "description": "Environment variables"
              }
            }
          },
          "input": {
            "type": "object",
            "properties": {
              "run": {
                "type": "string",
                "description": "Command to execute",
                "minLength": 1
              },
              "working_directory": {
                "type": "string",
                "description": "Override working directory"
              },
              "shell": {
                "type": "string",
                "description": "Override shell",
                "enum": ["bash", "sh", "powershell", "cmd"]
              },
              "env": {
                "type": "object",
                "additionalProperties": {"type": "string"},
                "description": "Additional environment variables"
              },
              "continue_on_error": {
                "type": "boolean",
                "description": "Continue execution on non-zero exit code",
                "default": false
              },
              "timeout": {
                "type": "string",
                "description": "Execution timeout",
                "pattern": "^[0-9]+(ms|s|m|h)$",
                "default": "5m"
              }
            },
            "required": ["run"]
          },
          "output": {
            "type": "object",
            "properties": {
              "stdout": {
                "type": "string",
                "description": "Standard output from the command"
              },
              "stderr": {
                "type": "string",
                "description": "Standard error from the command"
              },
              "exit_code": {
                "type": "integer",
                "description": "Process exit code"
              },
              "success": {
                "type": "boolean",
                "description": "Whether command succeeded"
              },
              "timed_out": {
                "type": "boolean",
                "description": "Whether command timed out"
              },
              "error_message": {
                "type": "string",
                "description": "Error message if failed"
              }
            },
            "required": ["stdout", "stderr", "exit_code", "success"]
          }
        },
        "required": ["type", "input"]
      }
    }
  }
}
```

## Migration Path

1. **Phase 1**: Create shared components without breaking existing functionality
2. **Phase 2**: Update commands one by one to use shared components
3. **Phase 3**: Remove duplicated code from commands
4. **Phase 4**: Add enhanced schema generation and jsonschema tags
5. **Phase 5**: Comprehensive testing and documentation

## Success Metrics

- Zero code duplication between schema, execute, and validate commands
- Complete schema output with all field requirements and descriptions
- All existing tests pass
- New shared component tests achieve >90% coverage
- Schema validation catches all invalid recipes that would fail at runtime