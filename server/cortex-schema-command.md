# Cortex Recipe Schema and Validation Commands Specification

## Overview
This document specifies two new cortex commands:
1. `cortex schema` - Provides JSON schema definitions for recipes, including configuration for all available activity types
2. `cortex validate` - Validates recipe files against the generated schema

Both commands use runtime reflection to automatically discover activity configurations from registered Go types. The validation command leverages the same schema generation system and uses standard JSON Schema validation libraries.

## Command Interfaces

### 1. Schema Command

#### Basic Usage
```bash
cortex schema [flags]
```

#### Flags
- `--format, -f` : Output format (`json`, `yaml`, `openapi`). Default: `json`
- `--activity, -a` : Filter to specific activity type (e.g., `llm`, `command`, `recipe`)
- `--version, -v` : Include version information in schema
- `--output, -o` : Output file path (default: stdout)
- `--include-examples` : Include example configurations in the schema

#### Examples
```bash
# Get complete JSON schema for all recipe types
cortex schema

# Get schema in YAML format
cortex schema -f yaml

# Get schema for specific activity type
cortex schema -a llm

# Output schema to file with examples
cortex schema -o recipe-schema.json --include-examples
```

### 2. Validate Command

#### Basic Usage
```bash
cortex validate <recipe-file> [flags]
```

#### Flags
- `--schema, -s` : Path to custom schema file (optional, uses built-in schema by default)
- `--strict` : Enable strict validation mode (fail on warnings)
- `--format, -f` : Output format for validation results (`text`, `json`, `junit`). Default: `text`
- `--output, -o` : Output file path for validation results (default: stdout)

#### Examples
```bash
# Validate a single recipe file
cortex validate my-recipe.yaml

# Validate with custom schema
cortex validate my-recipe.yaml --schema custom-schema.json

# Validate multiple recipe files
cortex validate recipes/*.yaml

# Validate with strict mode (warnings become errors)
cortex validate my-recipe.yaml --strict

# Output validation results as JSON
cortex validate my-recipe.yaml -f json

# Output JUnit format for CI integration
cortex validate recipes/*.yaml -f junit -o test-results.xml
```

## Implementation Architecture

### 1. Activity Discovery System

The command will implement automatic discovery of activity types through:

```go
package schema

import (
    "github.com/divisive-ai/vibethis/server/activity/pkg/activity"
    "github.com/divisive-ai/vibethis/server/activity/pkg/types"
)

type ActivityDiscovery struct {
    activities []types.RegisterableActivity
}

func (d *ActivityDiscovery) DiscoverActivities() error {
    // Use activity.GetAll() to retrieve all registered activities
    allActivities := activity.GetAll()
    
    // Process each activity to extract metadata and schema
    for _, act := range allActivities {
        // Use reflection to determine Config, Input, Output types
        // Generate JSON schema from Go struct tags
    }
    
    return nil
}
```

### 2. Schema Generation

The schema generator uses the same approach as the existing recipe-worker module. It leverages the `github.com/invopop/jsonschema` library which is already used in the codebase:

```go
import (
    "github.com/invopop/jsonschema" // Already used in recipe-worker
    "reflect"
)

// DefaultSchemaGenerator uses invopop/jsonschema for schema generation
// This matches the implementation in recipe-worker/pkg/worker/schema_generator.go
type DefaultSchemaGenerator struct {
    reflector *jsonschema.Reflector
}

func NewDefaultSchemaGenerator() *DefaultSchemaGenerator {
    reflector := &jsonschema.Reflector{
        // Don't allow additional properties by default
        AllowAdditionalProperties: false,
        // Anonymous fields should be expanded
        ExpandedStruct: true,
        // Require explicit tagging
        RequiredFromJSONSchemaTags: true,
    }
    return &DefaultSchemaGenerator{reflector: reflector}
}

func (g *DefaultSchemaGenerator) GenerateSchema(typ reflect.Type) (*jsonschema.Schema, error) {
    // Create a value from the type
    val := reflect.New(typ).Interface()
    
    // Use invopop/jsonschema to generate the schema
    schema := g.reflector.Reflect(val)
    
    // Schema automatically includes:
    // - Type information from Go types
    // - Required fields from jsonschema:"required" tags
    // - Validation rules from jsonschema tags (min, max, pattern, enum)
    // - Default values from jsonschema:"default=value" tags
    // - Descriptions from jsonschema:"description=text" tags
    
    return schema, nil
}
```

#### Why invopop/jsonschema?
- **Already in use**: Used in recipe-worker module for consistency
- **Rich tag support**: Comprehensive struct tag support for validation rules
- **Type safety**: Generates accurate schemas from Go types
- **Active maintenance**: Well-maintained library with regular updates

### 3. Output Schema Structure

The generated schema will follow this structure:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Recipe Schema",
  "type": "object",
  "properties": {
    "version": {
      "type": "string",
      "default": "1.0"
    },
    "name": {
      "type": "string",
      "description": "Recipe name"
    },
    "description": {
      "type": "string"
    },
    "steps": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {
            "type": "string"
          },
          "activity": {
            "type": "object",
            "oneOf": [
              {
                "$ref": "#/definitions/activities/llm"
              },
              {
                "$ref": "#/definitions/activities/command"
              },
              {
                "$ref": "#/definitions/activities/recipe"
              },
              {
                "$ref": "#/definitions/activities/gitshallow"
              }
            ]
          }
        }
      }
    }
  },
  "definitions": {
    "activities": {
      "llm": {
        "type": "object",
        "properties": {
          "type": {
            "const": "llm"
          },
          "config": {
            "$ref": "#/definitions/configs/llm"
          },
          "inputs": {
            "$ref": "#/definitions/inputs/llm"
          }
        }
      },
      "command": {
        "type": "object",
        "properties": {
          "type": {
            "const": "command_execution"
          },
          "config": {
            "$ref": "#/definitions/configs/command"
          },
          "inputs": {
            "$ref": "#/definitions/inputs/command"
          }
        }
      },
      "recipe": {
        "type": "object",
        "properties": {
          "type": {
            "const": "recipe"
          },
          "config": {
            "$ref": "#/definitions/configs/recipe"
          },
          "inputs": {
            "$ref": "#/definitions/inputs/recipe"
          }
        }
      },
      "gitshallow": {
        "type": "object",
        "properties": {
          "type": {
            "const": "git_shallow"
          },
          "config": {
            "$ref": "#/definitions/configs/gitshallow"
          },
          "inputs": {
            "$ref": "#/definitions/inputs/gitshallow"
          }
        }
      }
    },
    "configs": {
      // Auto-generated from Config struct types
    },
    "inputs": {
      // Auto-generated from Input struct types
    },
    "outputs": {
      // Auto-generated from Output struct types
    }
  }
}
```

### 4. Nested Composition Schemas

The schema includes definitions for nested compositions with encapsulation:

```json
{
  "definitions": {
    "compositions": {
      "sequential": {
        "type": "object",
        "properties": {
          "inputs": {
            "type": "array",
            "items": { "$ref": "#/definitions/inputDefinition" },
            "description": "Explicit inputs from parent context (required for nested compositions)"
          },
          "outputs": {
            "type": "object",
            "additionalProperties": { "type": "string" },
            "description": "Explicit outputs accessible to parent context"
          },
          "steps": {
            "type": "array",
            "items": { "$ref": "#/definitions/step" }
          }
        },
        "required": ["steps"]
      },
      "parallel": {
        "type": "object",
        "properties": {
          "inputs": { "type": "array", "items": { "$ref": "#/definitions/inputDefinition" } },
          "outputs": { "type": "object", "additionalProperties": { "type": "string" } },
          "steps": { "type": "array", "items": { "$ref": "#/definitions/step" } }
        },
        "required": ["steps"]
      },
      "conditional": {
        "type": "object",
        "properties": {
          "inputs": { "type": "array", "items": { "$ref": "#/definitions/inputDefinition" } },
          "outputs": { "type": "object", "additionalProperties": { "type": "string" } },
          "branches": { "type": "array", "items": { "$ref": "#/definitions/conditionalBranch" } }
        },
        "required": ["branches"]
      }
    },
    "inputDefinition": {
      "type": "object",
      "properties": {
        "name": { "type": "string", "description": "Internal name for the input" },
        "from": { "type": "string", "description": "Template expression from parent context" },
        "type": { 
          "type": "string",
          "enum": ["string", "number", "boolean", "object", "array", "steps", "states"],
          "description": "Optional type hint for context passing"
        }
      },
      "required": ["name", "from"]
    }
  }
}
```

## Runtime Type Discovery

### 1. Struct Tag Processing

Ops use standard Go struct tags for schema generation:

```go
type LLMConfig struct {
    Model       string        `json:"model" jsonschema:"required,enum=gpt-4,enum=gpt-3.5-turbo,enum=claude-3"`
    MaxTokens   int          `json:"max_tokens" jsonschema:"minimum=1,maximum=4096,default=1000"`
    Temperature float64      `json:"temperature" jsonschema:"minimum=0,maximum=2,default=0.7"`
    Timeout     time.Duration `json:"timeout" jsonschema:"default=30s"`
}
```

### 2. Runtime Metadata Extraction

The system extracts metadata using runtime reflection only:

- **Struct tags**: JSON and jsonschema tags for field configuration
- **Type information**: Go types mapped to JSON Schema types
- **Activity metadata**: `GetMetadata()` method provides activity-level information
- **Reflection**: Runtime type inspection without source code parsing

### 3. Dynamic Type Registry

Registry built at runtime from registered activities:

```go
type TypeRegistry struct {
    activities map[string]ActivitySchema
}

type ActivitySchema struct {
    Type         string
    Metadata     types.ActivityMetadata
    ConfigSchema *jsonschema.Schema
    InputSchema  *jsonschema.Schema
    OutputSchema *jsonschema.Schema
}

func BuildRegistry() (*TypeRegistry, error) {
    registry := &TypeRegistry{
        activities: make(map[string]ActivitySchema),
    }
    
    // Get all activities from runtime
    activities := activity.GetAll()
    
    // Use reflection to build schemas
    generator := NewSchemaGenerator()
    for _, act := range activities {
        // Runtime reflection only, no AST parsing
        schema := generator.GenerateFromType(act)
        registry.activities[act.GetMetadata().Type] = schema
    }
    
    return registry, nil
}
```

## Validation Implementation

### 1. Validate Command Implementation

The validate command uses standard JSON Schema validation libraries:

```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/xeipuuv/gojsonschema"  // JSON Schema validation
    "gopkg.in/yaml.v3"                   // YAML parsing
    "encoding/json"
)

var validateCmd = &cobra.Command{
    Use:   "validate <recipe-file>",
    Short: "Validate recipe files against schema",
    Long:  `Validates one or more recipe YAML files against the auto-generated schema`,
    Args:  cobra.MinimumNArgs(1),
    RunE:  runValidate,
}

func runValidate(cmd *cobra.Command, args []string) error {
    // Get or generate schema
    var schema *jsonschema.Schema
    if schemaPath != "" {
        // Load custom schema
        schema = loadSchemaFromFile(schemaPath)
    } else {
        // Generate schema from registered activities
        generator := NewSchemaGenerator()
        schema = generator.GenerateCompleteSchema()
    }
    
    // Validate each file using gojsonschema
    validator := NewValidator(schema, strict)
    results := validator.ValidateFiles(args)
    
    // Output results in requested format
    formatter := GetFormatter(outputFormat)
    return formatter.Output(results, outputPath)
}
```

### 2. Validation Using Best-in-Class Libraries

Validation leverages carefully selected libraries for optimal error reporting and performance:

```go
package validation

import (
    "github.com/santhosh-tekuri/jsonschema/v5"  // Superior error messages
    "gopkg.in/yaml.v3"                           // YAML parsing
    "encoding/json"
    "text/template"
    "regexp"
)

type Validator struct {
    schema       *jsonschema.Schema
    compiler     *jsonschema.Compiler
    strict       bool
    templateVars map[string]bool  // Track available template variables
}

func NewValidator(schemaPath string, strict bool) *Validator {
    compiler := jsonschema.NewCompiler()
    compiler.Draft = jsonschema.Draft7
    
    // Load and compile schema
    schema, err := compiler.Compile(schemaPath)
    
    return &Validator{
        schema:       schema,
        compiler:     compiler,
        strict:       strict,
        templateVars: extractAvailableVariables(),
    }
}

func (v *Validator) ValidateFile(filepath string) ValidationResult {
    // Load and parse YAML file
    data, err := os.ReadFile(filepath)
    if err != nil {
        return ValidationResult{File: filepath, Valid: false}
    }
    
    // Pre-validation: Check template variables
    templateErrors := v.validateTemplateVariables(string(data))
    
    // Convert YAML to interface{} for validation
    var doc interface{}
    yaml.Unmarshal(data, &doc)
    
    // Validate against schema
    err = v.schema.Validate(doc)
    
    result := ValidationResult{
        File:  filepath,
        Valid: err == nil && len(templateErrors) == 0,
    }
    
    // Process schema validation errors
    if err != nil {
        if validationErr, ok := err.(*jsonschema.ValidationError); ok {
            for _, e := range validationErr.Causes {
                result.Errors = append(result.Errors, ValidationError{
                    Field:       e.InstanceLocation,
                    Description: e.Message,
                    Schema:      e.SchemaLocation,
                    ErrorType:   e.ErrorType,
                })
            }
        }
    }
    
    // Add template variable errors
    result.Errors = append(result.Errors, templateErrors...)
    
    return result
}

// validateTemplateVariables checks for undefined Go template variables
// The recipe system uses Go text/template syntax exclusively
func (v *Validator) validateTemplateVariables(content string) []ValidationError {
    var errors []ValidationError
    
    // Recipe system uses Go templates with these variable contexts:
    // - {{.Inputs.xxx}} - workflow inputs
    // - {{.Steps.stepID.outputs.xxx}} - step outputs
    // - {{.Context.xxx}} - workflow context (workflowID, runID)
    // - {{.Env.xxx}} - environment variables (placeholder, not yet implemented)
    
    // Parse template expressions
    templatePattern := regexp.MustCompile(`\{\{([^}]+)\}\}`)
    matches := templatePattern.FindAllStringSubmatch(content, -1)
    
    for _, match := range matches {
        expr := strings.TrimSpace(match[1])
        
        // Check if it's a valid reference path
        if strings.HasPrefix(expr, ".Inputs.") {
            // Validate input reference
            inputName := strings.TrimPrefix(expr, ".Inputs.")
            if !v.isValidInput(inputName) {
                errors = append(errors, ValidationError{
                    Field:       expr,
                    Description: fmt.Sprintf("Reference to undefined input: %s", inputName),
                    ErrorType:   "undefined_input",
                })
            }
        } else if strings.HasPrefix(expr, ".Steps.") {
            // Validate step output reference
            parts := strings.Split(expr, ".")
            if len(parts) >= 4 && parts[2] == "outputs" {
                stepID := parts[1]
                outputName := strings.Join(parts[3:], ".")
                if !v.isValidStepOutput(stepID, outputName) {
                    errors = append(errors, ValidationError{
                        Field:       expr,
                        Description: fmt.Sprintf("Reference to undefined step output: %s.%s", stepID, outputName),
                        ErrorType:   "undefined_step_output",
                    })
                }
            }
        } else if strings.HasPrefix(expr, ".Context.") {
            // Validate context reference
            contextField := strings.TrimPrefix(expr, ".Context.")
            validContextFields := []string{"workflowID", "runID"}
            if !contains(validContextFields, contextField) {
                errors = append(errors, ValidationError{
                    Field:       expr,
                    Description: fmt.Sprintf("Invalid context field: %s", contextField),
                    ErrorType:   "invalid_context_field",
                })
            }
        } else if !strings.HasPrefix(expr, ".") {
            // Check for template functions like: join, json, len
            if !v.isValidTemplateFunction(expr) {
                errors = append(errors, ValidationError{
                    Field:       expr,
                    Description: fmt.Sprintf("Invalid template expression: %s", expr),
                    ErrorType:   "invalid_template_expression",
                })
            }
        }
    }
    
    return errors
}
```

#### Why santhosh-tekuri/jsonschema?

- **Superior error messages**: Provides detailed error context with JSON paths
- **Draft 7 support**: Full JSON Schema Draft 7 compliance
- **Performance**: Faster validation than xeipuuv/gojsonschema
- **Error details**: Returns structured errors with:
  - `InstanceLocation`: Exact path to the error in the document
  - `SchemaLocation`: Path to the schema rule that failed
  - `Message`: Human-readable error description
  - `ErrorType`: Type of validation failure (enum, type, required, etc.)

#### Built-in Error Types

The library provides detailed error types for precise error handling:

```go
// Error types returned by santhosh-tekuri/jsonschema
const (
    ErrorType_Enum         = "enum"          // Value not in allowed enum
    ErrorType_Type         = "type"          // Wrong type
    ErrorType_Required     = "required"      // Missing required field
    ErrorType_MinLength    = "minLength"     // String too short
    ErrorType_MaxLength    = "maxLength"     // String too long
    ErrorType_Minimum      = "minimum"       // Number below minimum
    ErrorType_Maximum      = "maximum"       // Number above maximum
    ErrorType_Pattern      = "pattern"       // String doesn't match pattern
    ErrorType_Format       = "format"        // Invalid format (email, uri, etc.)
    ErrorType_MinItems     = "minItems"      // Array too small
    ErrorType_MaxItems     = "maxItems"      // Array too large
    ErrorType_UniqueItems  = "uniqueItems"   // Array has duplicates
    ErrorType_MinProperties = "minProperties" // Object has too few properties
    ErrorType_MaxProperties = "maxProperties" // Object has too many properties
    ErrorType_Dependencies = "dependencies"  // Missing dependent properties
    ErrorType_AdditionalProperties = "additionalProperties" // Extra properties not allowed
)
```

### 3. Enhanced Error Reporting

With the santhosh-tekuri/jsonschema library, error messages include precise locations and context:

#### Text Format (Default)
```
✅ recipes/valid-recipe.yaml: VALID

❌ recipes/invalid-recipe.yaml: INVALID

SCHEMA ERRORS (3):
  ✗ /steps/0/activity/config/model [enum]
    Location: #/properties/steps/items/properties/activity/properties/config/properties/model
    Value: "gpt-5" is not one of ["gpt-4", "gpt-3.5-turbo", "claude-3"]
    
  ✗ /steps/1/activity/inputs/temperature [maximum]
    Location: #/properties/steps/items/properties/activity/properties/inputs/properties/temperature
    Value: 3.5 exceeds maximum of 2.0
    
  ✗ /steps/2/activity/config [required]
    Location: #/properties/steps/items/properties/activity/properties/config
    Missing required field: timeout

TEMPLATE ERRORS (2):
  ✗ Reference to undefined input: api_key
    Found in: steps[0].activity.inputs.key = "{{.Inputs.api_key}}"
    
  ✗ Reference to undefined step output: validate.invalid_field
    Found in: steps[3].activity.config.data = "{{.Steps.validate.outputs.invalid_field}}"

SUMMARY:
  Files validated: 2
  Valid: 1
  Invalid: 1
  Schema errors: 3
  Template errors: 2
```

#### JSON Format
```json
{
  "results": [
    {
      "file": "recipes/invalid-recipe.yaml",
      "valid": false,
      "errors": [
        {
          "severity": "ERROR",
          "path": "steps[0].activity.config.model",
          "message": "Invalid enum value",
          "rule": "enum",
          "expected": ["gpt-4", "gpt-3.5-turbo", "claude-3"],
          "actual": "gpt-5",
          "fixAvailable": false
        }
      ],
      "warnings": [],
      "suggestions": []
    }
  ],
  "summary": {
    "filesValidated": 2,
    "valid": 1,
    "invalid": 1,
    "totalErrors": 2,
    "totalWarnings": 1
  }
}
```

#### JUnit Format (for CI/CD)
```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="Recipe Validation" tests="2" failures="1" errors="0">
  <testsuite name="recipes" tests="2" failures="1" errors="0">
    <testcase name="valid-recipe.yaml" classname="recipes">
      <!-- Success -->
    </testcase>
    <testcase name="invalid-recipe.yaml" classname="recipes">
      <failure message="Validation failed" type="ValidationError">
        steps[0].activity.config.model: Invalid enum value "gpt-5"
        steps[1].activity.inputs.temperature: Value 3.5 exceeds maximum of 2.0
      </failure>
    </testcase>
  </testsuite>
</testsuites>
```

### 4. JSON Schema Validation Features

The validation leverages standard JSON Schema features provided by the libraries:

- **Type checking**: Validates data types match schema definitions
- **Required fields**: Ensures all required fields are present
- **Enum constraints**: Validates values against allowed enums
- **Min/max values**: Numeric and string length constraints
- **Pattern matching**: Regex pattern validation
- **Format validation**: Standard formats (email, uri, date-time, duration)
- **Object properties**: Validates nested objects and arrays
- **Additional properties**: Prevents undefined fields when configured

## Integration with Existing Cortex Commands

### 1. Command Registration

Add both commands to the existing Cortex CLI:

```go
func init() {
    rootCmd.AddCommand(schemaCmd)
    rootCmd.AddCommand(validateCmd)
}

var schemaCmd = &cobra.Command{
    Use:   "schema",
    Short: "Generate JSON schema for recipes and activities",
    Long:  `Automatically discovers and generates JSON schemas for all available activity types`,
    RunE:  runSchema,
}

var validateCmd = &cobra.Command{
    Use:   "validate <recipe-file> [recipe-files...]",
    Short: "Validate recipe files against schema",
    Long:  `Validates one or more recipe YAML files against the auto-generated schema.
           Returns exit code 0 if all files are valid, 1 if any validation errors found.`,
    Args:  cobra.MinimumNArgs(1),
    RunE:  runValidate,
}
```

### 2. Shared Configuration

Leverage existing Cortex configuration system:

```go
func runSchema(cmd *cobra.Command, args []string) error {
    // Use existing config loading
    cfg := config.LoadConfig()
    
    // Initialize discovery with activity package
    discovery := NewActivityDiscovery(cfg)
    
    // Generate schema
    schema, err := discovery.GenerateCompleteSchema()
    
    // Output in requested format
    return OutputSchema(schema, flags)
}

func runValidate(cmd *cobra.Command, args []string) error {
    // Use existing config loading
    cfg := config.LoadConfig()
    
    // Initialize validator
    validator := NewValidator(cfg, validatorFlags)
    
    // Validate files
    results := validator.ValidateFiles(args)
    
    // Output results and return appropriate exit code
    if err := OutputResults(results, outputFlags); err != nil {
        return err
    }
    
    // Return error if any files invalid (for CI/CD integration)
    if !results.AllValid() {
        os.Exit(1)
    }
    
    return nil
}
```

## Benefits

1. **Automatic Discovery**: No manual schema maintenance required
2. **Type Safety**: Schema directly derived from Go types using runtime reflection
3. **Standard Validation**: Uses established JSON Schema validation libraries
4. **CI/CD Integration**: JUnit output format and exit codes for automated testing
5. **Documentation**: Auto-generated documentation from code
6. **IDE Support**: JSON schema enables autocomplete and validation in editors
7. **Version Compatibility**: Schema includes version information for compatibility checks
8. **Extensibility**: New activities automatically included in schema

## Libraries Used

### Core Libraries

- **Schema Generation**: `github.com/invopop/jsonschema` 
  - Already used in recipe-worker module
  - Generates JSON Schema from Go types with rich struct tag support
  - Supports jsonschema tags for validation rules

- **JSON Schema Validation**: `github.com/santhosh-tekuri/jsonschema/v5`
  - Superior error messages with exact JSON paths
  - Full JSON Schema Draft 7 support
  - Detailed error types for precise error handling
  - Better performance than alternatives

- **YAML Parsing**: `gopkg.in/yaml.v3`
  - Standard YAML parsing library
  - Preserves comments and formatting where needed

- **Reflection**: Standard `reflect` package
  - Runtime type inspection
  - No AST parsing needed

- **CLI Framework**: `github.com/spf13/cobra`
  - Already used throughout the cortex CLI
  - Consistent command structure

### Template Variable Validation

The recipe system uses Go `text/template` syntax exclusively. The validator detects undefined references to:

- **Workflow Inputs**: `{{.Inputs.fieldName}}`
- **Step Outputs**: `{{.Steps.stepID.outputs.outputName}}`
- **Context Variables**: `{{.Context.workflowID}}`, `{{.Context.runID}}`
- **Template Functions**: `{{join .Inputs.separator .Inputs.items}}`, `{{json .Steps.step1.outputs.data}}`

Supported template functions (from recipe-worker/pkg/compiler/template.go):
- `json`: Marshal values to JSON
- `join`: Join array elements with separator
- `len`: Get length of arrays/slices (standard Go template function)

## Use Cases

### Development Workflow
```bash
# Generate schema to understand available activities
cortex schema --include-examples > schema.json

# Write recipe with IDE support using generated schema
# VSCode/IntelliJ will provide autocomplete

# Validate recipe during development
cortex validate my-recipe.yaml

# Validate before commit
cortex validate recipes/*.yaml --strict
```

### CI/CD Pipeline
```yaml
# GitHub Actions example
- name: Validate Recipes
  run: |
    cortex validate recipes/*.yaml --strict -f junit -o test-results.xml
    
- name: Upload Test Results
  uses: actions/upload-artifact@v2
  with:
    name: recipe-validation-results
    path: test-results.xml
```

### Pre-commit Hook
```bash
#!/bin/bash
# .git/hooks/pre-commit

# Validate all recipe files before commit
cortex validate $(git diff --cached --name-only --diff-filter=ACM | grep '\.yaml$') --strict

if [ $? -ne 0 ]; then
    echo "Recipe validation failed. Please fix errors before committing."
    exit 1
fi

## Implementation Status

As of the latest update:

- ✅ **Terminology standardization complete**: All references updated from "activities" to "ops"
- ✅ **Op discovery system**: Implemented in `/server/ops/pkg/`
- ✅ **Composition types**: Sequential, parallel, conditional fully supported
- ✅ **Nested encapsulation**: Strict boundaries enforced as per nested-composition-encapsulation-spec.md
- ✅ **Schema generation**: Using invopop/jsonschema in recipe-worker
- ⏳ **Cortex commands**: Schema and validate commands pending implementation
- ✅ **State machine support**: Full compiler in `/server/recipe-worker/pkg/compiler/statemachine/`
- ✅ **CEL expressions**: Integrated for conditional logic
```