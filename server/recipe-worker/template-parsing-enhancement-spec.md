# Smart Template Parsing Enhancement Specification

## Overview

Instead of using regex to parse `{{ }}` expressions, leverage Go's `text/template/parse` package to get a proper AST, then walk the tree and evaluate expressions using CEL. This handles all edge cases correctly and provides better error reporting.

## Current Problem with Regex

```go
// Regex approach fails on:
message: `{{ "Hello {{ name }}" }}`                    // Nested braces in strings
message: `{{ /* comment */ inputs.field }}`            // Comments  
message: `{{ inputs.field | quote }}`                  // Pipes (though we don't support them)
message: `{{ "string with \" quotes" }}`               // Escaped quotes
```

## Smart Parsing Solution

### 1. Use Go's Template Parser

```go
import (
    "text/template/parse"
    "text/template"
)

func (rc *ResolutionContext) parseAndEvaluate(templateStr string) (interface{}, error) {
    // Create a dummy template to parse
    tmpl, err := template.New("temp").Parse(templateStr)
    if err != nil {
        return nil, fmt.Errorf("template parse error: %w", err)
    }
    
    // Get the parse tree
    tree := tmpl.Tree
    if tree == nil || tree.Root == nil {
        return templateStr, nil // No template content
    }
    
    // Walk the AST and evaluate
    return rc.evaluateTemplateTree(tree.Root)
}
```

### 2. AST Walker Implementation

```go
func (rc *ResolutionContext) evaluateTemplateTree(node parse.Node) (interface{}, error) {
    switch n := node.(type) {
    case *parse.ListNode:
        return rc.evaluateListNode(n)
    case *parse.TextNode:
        return n.Text, nil
    case *parse.ActionNode:
        return rc.evaluateActionNode(n)
    case *parse.IfNode:
        return nil, fmt.Errorf("if statements not supported in interpolation mode")
    case *parse.RangeNode:
        return nil, fmt.Errorf("range statements not supported in interpolation mode") 
    default:
        return nil, fmt.Errorf("unsupported node type: %T", n)
    }
}

func (rc *ResolutionContext) evaluateListNode(list *parse.ListNode) (interface{}, error) {
    if list == nil || len(list.Nodes) == 0 {
        return "", nil
    }
    
    // Check if this is a single action (for raw type return)
    if len(list.Nodes) == 1 {
        if action, ok := list.Nodes[0].(*parse.ActionNode); ok {
            return rc.evaluateSingleAction(action)
        }
    }
    
    // Multiple nodes - concatenate as string
    var result strings.Builder
    for _, node := range list.Nodes {
        value, err := rc.evaluateTemplateTree(node)
        if err != nil {
            return nil, err
        }
        result.WriteString(convertToString(value))
    }
    
    return result.String(), nil
}

func (rc *ResolutionContext) evaluateActionNode(action *parse.ActionNode) (interface{}, error) {
    if action.Pipe == nil {
        return "", nil
    }
    
    // Convert the pipeline back to CEL expression
    celExpr, err := rc.pipelineToCEL(action.Pipe)
    if err != nil {
        return nil, err
    }
    
    // Evaluate with CEL
    result, err := rc.EvaluateCELExpression(celExpr)
    if err != nil {
        return fmt.Sprintf("<error: %s>", err), nil // Return error placeholder in string context
    }
    
    return result, nil
}

func (rc *ResolutionContext) evaluateSingleAction(action *parse.ActionNode) (interface{}, error) {
    // Single action - return raw type
    if action.Pipe == nil {
        return "", nil
    }
    
    celExpr, err := rc.pipelineToCEL(action.Pipe)
    if err != nil {
        return nil, err
    }
    
    // For single expressions, return the actual result type
    return rc.EvaluateCELExpression(celExpr)
}
```

### 3. Pipeline to CEL Conversion

```go
func (rc *ResolutionContext) pipelineToCEL(pipe *parse.PipeNode) (string, error) {
    if len(pipe.Cmds) != 1 {
        return "", fmt.Errorf("only simple expressions supported, no pipes")
    }
    
    cmd := pipe.Cmds[0]
    return rc.commandToCEL(cmd)
}

func (rc *ResolutionContext) commandToCEL(cmd *parse.CommandNode) (string, error) {
    if len(cmd.Args) == 0 {
        return "", fmt.Errorf("empty command")
    }
    
    var parts []string
    for _, arg := range cmd.Args {
        part, err := rc.argToCEL(arg)
        if err != nil {
            return "", err
        }
        parts = append(parts, part)
    }
    
    // Join with spaces (basic approach - may need refinement)
    return strings.Join(parts, " "), nil
}

func (rc *ResolutionContext) argToCEL(arg parse.Node) (string, error) {
    switch n := arg.(type) {
    case *parse.FieldNode:
        // .inputs.name -> inputs.name
        return strings.TrimPrefix(strings.Join(n.Ident, "."), "."), nil
        
    case *parse.VariableNode:
        // $var -> not supported in our CEL context
        return "", fmt.Errorf("variables not supported: %s", strings.Join(n.Ident, "."))
        
    case *parse.DotNode:
        // . -> not supported in our context
        return "", fmt.Errorf("dot notation not supported in CEL context")
        
    case *parse.StringNode:
        // "string literal" -> "string literal"  
        return fmt.Sprintf(`"%s"`, n.Text), nil
        
    case *parse.NumberNode:
        // 123, 3.14 -> 123, 3.14
        return n.Text, nil
        
    case *parse.BoolNode:
        // true, false -> true, false
        return n.Text, nil
        
    case *parse.ChainNode:
        // .field1.field2 -> field1.field2
        base, err := rc.argToCEL(n.Node)
        if err != nil {
            return "", err
        }
        return base + "." + strings.Join(n.Field, "."), nil
        
    default:
        return "", fmt.Errorf("unsupported argument type: %T", n)
    }
}
```

## Enhanced Resolution Logic

```go
func (rc *ResolutionContext) ResolveTemplate(expr string) (interface{}, error) {
    // Check if it contains template syntax
    if !strings.Contains(expr, "{{") {
        return expr, nil
    }
    
    // Parse using Go template parser
    return rc.parseAndEvaluate(expr)
}
```

## Benefits Over Regex

### 1. Correct Parsing
```go
// Regex would fail, but template parser handles correctly:
message: `{{ "Hello {{ world }}" }}`                   // String with braces
message: `{{ inputs.field /* comment */ }}`            // Comments
message: `{{ inputs.field1 inputs.field2 }}`           // Multiple args (converted to CEL properly)
```

### 2. Better Error Messages
```go
// Template parser gives precise error locations:
message: `{{ inputs.field1 + }}`
// Error: "template parse error: unexpected \"}\" in command at line 1, char 20"

// vs regex approach:
// Error: "CEL compilation failed: syntax error"
```

### 3. Proper String Handling
```go
// Template parser correctly handles string literals:
message: `{{ "String with {{ braces }}" }}`           // Correctly parsed as single string
message: `{{ "String with \" quotes" }}`              // Escaped quotes handled
message: `{{ "Multi\nline\tstring" }}`                // Escape sequences
```

### 4. Future Extensibility
```go
// Easy to add supported functions later:
message: `{{ upper inputs.name }}`                    // Could map to CEL upper() function
message: `{{ inputs.items | length }}`               // Could support limited pipeline operations
```

## Implementation Strategy

### Phase 1: Core Parser Integration
```go
// Replace regex-based parsing with template parser
func (rc *ResolutionContext) ResolveTemplate(expr string) (interface{}, error) {
    if !strings.Contains(expr, "{{") {
        return expr, nil
    }
    return rc.parseAndEvaluate(expr)
}
```

### Phase 2: AST Walking
```go
// Implement comprehensive node evaluation
// Handle TextNode, ActionNode, ListNode appropriately
// Convert template expressions to CEL expressions
```

### Phase 3: Error Enhancement
```go
// Leverage parse tree for better error messages
// Include line/column information in errors
// Provide context-aware error messages
```

## Example Conversions

```yaml
# Template -> CEL Expression
"{{ .inputs.name }}"           -> "inputs.name"
"{{ .sequence.node.outputs }}" -> "sequence.node.outputs"
'{{ "literal string" }}'       -> '"literal string"'
"{{ .inputs.count }}"          -> "inputs.count"

# Complex examples:
"Hello {{ .inputs.name }}!"    -> TextNode("Hello ") + ActionNode("inputs.name") + TextNode("!")
"{{ .inputs.first .inputs.second }}" -> "inputs.first inputs.second" (needs special handling)
```

## Migration Benefits

1. **Robust Parsing**: Handles all edge cases correctly
2. **Better Errors**: Precise error locations and context
3. **Future-Proof**: Easy to extend with more template features
4. **Performance**: Template parser is optimized and well-tested
5. **Compatibility**: Leverages existing Go template knowledge

## Success Criteria

1. All current tests pass
2. Complex nested expressions parse correctly
3. String literals with braces handled properly
4. Better error messages with line/column info
5. Single expressions still return raw types
6. Performance equal or better than regex approach