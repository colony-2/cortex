# Schema Generation Analysis: Hand-Written vs Automated

## Executive Summary

The current schema generation in `schema.go` is largely hand-written because it serves as a **bridge between runtime type information and a domain-specific schema language** for workflow orchestration. While some aspects could be automated using struct reflection, the majority of the schema requires manual definition due to workflow semantics, validation rules, and the need for a clean, user-facing API that doesn't directly mirror internal implementation details.

## What CAN Be Automated (Using Struct Types)

### 1. Activity Input/Output Schemas (~30% of current code)
The activity-specific schemas (Config, Input, Output structs) can be automatically generated from Go structs using reflection:

```go
// These structs with json tags can be auto-generated into JSON schemas
type CommandExecutionInput struct {
    Run              string            `json:"run"`
    WorkingDirectory string            `json:"working_directory"`
    Shell            string            `json:"shell"`
    Env              map[string]string `json:"env"`
    ContinueOnError  bool              `json:"continue_on_error"`
    Timeout          string            `json:"timeout"`
}
```

**Current Implementation:**
- Lines 46-99 in `schema.go` attempt this with `generateSchemasForActivity()`
- Uses `jsonschema.Schema` library and reflection
- Already partially automated

**Potential Improvements:**
- Better handling of interface types
- Automatic extraction of validation rules from struct tags
- Generation of examples from struct defaults

### 2. Basic Type Definitions (~10% of current code)
Simple type mappings can be automated:
- Go `string` → JSON Schema `"type": "string"`
- Go `int` → JSON Schema `"type": "integer"`
- Go `bool` → JSON Schema `"type": "boolean"`
- Go struct fields → JSON Schema properties

## What CANNOT Be Automated (Requires Manual Definition)

### 1. Workflow Control Flow Semantics (~40% of current code)

The workflow engine has specific semantics that don't map to Go types:

```yaml
# This YAML structure doesn't have a direct Go struct representation
sequence:
  - op: command_execution
    inputs:
      run: "echo 'Step 1'"
  - op: command_execution
    inputs:
      run: "echo 'Step 2'"
```

**Why Manual:**
- `sequence`, `parallel`, `states` are workflow concepts, not data structures
- The `oneOf` constraints for node types (lines 201-206) represent mutually exclusive workflow patterns
- State machine transitions (lines 393-410) encode workflow logic, not data

### 2. Discriminated Union Types (~15% of current code)

The schema uses `const` fields for type discrimination:

```json
{
  "op": { "const": "sleep" },
  "inputs": { ... }
}
```

**Why Manual:**
- Go doesn't have discriminated unions
- The `const` pattern is a JSON Schema idiom for polymorphism
- Each operation type needs custom validation rules

### 3. Cross-Type References and Composition (~10% of current code)

The schema uses JSON Schema `$ref` for type composition:

```json
{
  "retry": { "$ref": "#/definitions/RetryPolicy" },
  "timeout": { "type": "string" }
}
```

**Why Manual:**
- Shared definitions across different node types
- Recursive references (nodes can contain nodes)
- Schema organization doesn't mirror code organization

### 4. Domain-Specific Validation Rules (~5% of current code)

Many validation rules are workflow-specific:

```json
{
  "timeout": {
    "type": "integer",
    "minimum": 1,
    "maximum": 3600
  }
}
```

**Why Manual:**
- Business logic constraints
- Format specifications (e.g., duration strings like "30s")
- Conditional requirements based on other fields

## Recommended Hybrid Approach

### Phase 1: Enhanced Automation for Activity Schemas
1. **Improve struct reflection** to handle:
   - Complex nested types
   - Interface types with known implementations
   - Automatic example generation from test fixtures

2. **Add struct tags** for validation:
   ```go
   type CommandExecutionInput struct {
       Run     string `json:"run" required:"true" description:"Command to execute"`
       Timeout string `json:"timeout" pattern:"^[0-9]+(s|m|h)$"`
   }
   ```

3. **Generate activity schemas** completely from structs:
   - Eliminate manual `buildCommandExecutionOperation()` type methods
   - Auto-generate from registered activities

### Phase 2: Template-Based Workflow Schema
1. **Keep manual definition** for workflow constructs
2. **Use templates** to reduce repetition:
   ```go
   func generateOperationSchema(opType string, inputs, outputs interface{}) map[string]interface{} {
       // Common template for all operations
   }
   ```

3. **Externalize** workflow rules to configuration:
   ```yaml
   workflow_types:
     sequence:
       description: "Sequential execution of nodes"
       min_items: 1
       allows_nesting: true
   ```

## Implementation Estimate

### Fully Automated Approach (Not Recommended)
- **Feasibility:** ~40% of schema can be automated
- **Effort:** High (requires DSL or complex annotations)
- **Maintenance:** Harder (abstractions hide schema details)
- **Result:** Loss of fine control over user-facing API

### Hybrid Approach (Recommended)
- **Feasibility:** 100% (combines automation with manual control)
- **Effort:** Medium (enhance existing reflection, add templates)
- **Maintenance:** Better (clear separation of concerns)
- **Result:** Reduced code duplication while maintaining control

## Conclusion

The schema is largely hand-written because:

1. **Workflow semantics** don't map directly to Go types
2. **User-facing API** needs to be cleaner than internal implementations
3. **Validation rules** are domain-specific and complex
4. **Type discrimination** patterns don't exist in Go

The recommended approach is to:
- **Automate** activity schema generation from structs (30% reduction in manual code)
- **Template** common patterns (20% reduction in repetition)
- **Keep manual** workflow semantics and validation rules (necessary for correctness)

This hybrid approach balances automation benefits with the need for precise control over the schema's user-facing API.