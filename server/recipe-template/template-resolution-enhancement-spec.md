# Template Resolution Enhancement Specification

## Overview

This specification enhances the current template resolution system to support **string interpolation with embedded CEL expressions** for all fields except `when` conditions, which must remain pure CEL expressions.

## Current Behavior (Problem)

Currently, the template resolver treats any field value containing `{{ }}` as a complete CEL expression that gets fully replaced. This means:

```yaml
# Current: This fails or returns unexpected results
message: "Hello {{ inputs.name }}, your ID is {{ inputs.user_id }}"
# Result: Error or only evaluates first expression

# Current: Must use CEL string concatenation
message: '{{ "Hello " + inputs.name + ", your ID is " + inputs.user_id }}'
# Result: "Hello Alice, your ID is 123"
```

## Desired Behavior

Support natural string interpolation with multiple embedded expressions:

```yaml
# Desired: Multiple expressions in a single string
message: "Hello {{ inputs.name }}, your ID is {{ inputs.user_id }}"
# Result: "Hello Alice, your ID is 123"

# Desired: Mix of static text and expressions
url: "https://{{ inputs.domain }}/api/{{ inputs.version }}/users/{{ inputs.user_id }}"
# Result: "https://example.com/api/v2/users/123"

# Desired: Expressions anywhere in the string
log: "[{{ scope.timestamp }}] User {{ inputs.user_id }} performed {{ inputs.action }}"
# Result: "[2024-01-01T12:00:00Z] User 123 performed login"
```

## Implementation Strategy

### 1. Two Resolution Modes

```go
type ResolutionMode int

const (
    // For input/output fields - supports string interpolation
    ModeInterpolation ResolutionMode = iota
    
    // For when conditions - pure CEL expression only
    ModePureCEL
)
```

### 2. Minimal Custom Parser (Not stdlib Fork)

After analysis, forking stdlib's lexer would require too many changes. Instead, we'll write a minimal custom parser (~200 lines) that handles our specific needs.

```go
// Simple types for our parser
type Segment interface {
    isSegment()
    Position() int  // For error reporting
}

type TextSegment struct {
    Text string
    Pos  int
}

type ExpressionSegment struct {
    Expression string  // Raw CEL expression
    Pos        int     // Start position in original input
}

func (TextSegment) isSegment()       {}
func (ExpressionSegment) isSegment() {}
func (t TextSegment) Position() int  { return t.Pos }
func (e ExpressionSegment) Position() int { return e.Pos }
```

### 3. Quote Handling Rules for CEL Compatibility

#### CEL String Literal Types We Must Handle:

1. **Double-quoted strings**: `"hello"` with escape sequences (`\n`, `\t`, `\"`, `\\`)
2. **Single-quoted strings**: `'hello'` with literal contents (only `''` for escaping)
3. **Both quote types in expressions**: `inputs.type == 'active' || inputs.code == "ABC"`

#### Parser Quote Rules:

```go
// Quote tracking state
type QuoteState struct {
    InString   bool
    Delimiter  rune  // Either ' or "
}

// Rules for quote handling:
// 1. When we see " or ' and NOT in a string -> enter string mode
// 2. When in string with delimiter '"':
//    - Backslash escapes next character (\n, \t, \", \\)
//    - Only '"' ends the string (unless escaped)
// 3. When in string with delimiter '\'':
//    - Backslash does NOT escape (literal)
//    - Two single quotes ('') is an escaped single quote
//    - Single ' ends the string (unless followed by another ')
// 4. Never interpret }} inside any string as delimiter
```

#### Examples That Must Parse Correctly:

```yaml
# Double quotes with escapes
expr1: {{ "text with }} inside" }}              # }} inside string ignored
expr2: {{ "escaped: \"quote\"" }}               # Escaped quotes
expr3: {{ "path: C:\\Users\\file" }}           # Escaped backslashes

# Single quotes (literal, CEL-style)
expr4: {{ 'text with }} inside' }}              # }} inside string ignored  
expr5: {{ 'doesn''t fail' }}                    # '' = escaped single quote
expr6: {{ 'literal \n stays' }}                 # No escape interpretation

# Mixed quotes in CEL expressions
expr7: {{ inputs.status == 'active' }}          # Common CEL pattern
expr8: {{ inputs.name in ["Alice", "Bob"] }}    # Mixed quote usage
expr9: {{ 'single' + "double" }}                # Both in one expression
```

### 4. Parser Implementation

```go
func parseTemplate(input string) ([]Segment, error) {
    var segments []Segment
    pos := 0
    
    for pos < len(input) {
        // Look for next {{
        idx := strings.Index(input[pos:], "{{")
        
        if idx == -1 {
            // No more expressions, rest is text
            if pos < len(input) {
                segments = append(segments, TextSegment{
                    Text: input[pos:],
                    Pos:  pos,
                })
            }
            break
        }
        
        // Add text before {{
        if idx > 0 {
            segments = append(segments, TextSegment{
                Text: input[pos : pos+idx],
                Pos:  pos,
            })
        }
        
        // Find matching }} respecting quotes
        exprStart := pos + idx + 2
        exprEnd, err := findExpressionEnd(input, exprStart)
        if err != nil {
            return nil, fmt.Errorf("at position %d: %w", exprStart, err)
        }
        
        // Add expression segment
        segments = append(segments, ExpressionSegment{
            Expression: strings.TrimSpace(input[exprStart:exprEnd]),
            Pos:        exprStart,
        })
        
        pos = exprEnd + 2  // Skip past }}
    }
    
    return segments, nil
}

func findExpressionEnd(input string, start int) (int, error) {
    pos := start
    var inString bool
    var stringDelim rune
    
    for pos < len(input) {
        // Check for }} only when not in string
        if !inString && pos+1 < len(input) && 
           input[pos] == '}' && input[pos+1] == '}' {
            return pos, nil
        }
        
        ch := rune(input[pos])
        
        switch {
        case !inString && (ch == '"' || ch == '\''):
            // Starting a string
            inString = true
            stringDelim = ch
            pos++
            
        case inString && stringDelim == '"' && ch == '\\':
            // In double-quoted string, backslash escapes next char
            pos += 2  // Skip escape sequence
            if pos > len(input) {
                return 0, fmt.Errorf("unexpected end in escape sequence")
            }
            
        case inString && stringDelim == '\'' && ch == '\'' && 
             pos+1 < len(input) && input[pos+1] == '\'':
            // In single-quoted string, '' is escaped single quote
            pos += 2
            
        case inString && ch == stringDelim:
            // End of string (unless it's '' in single-quoted)
            if !(stringDelim == '\'' && pos+1 < len(input) && input[pos+1] == '\'') {
                inString = false
            }
            pos++
            
        default:
            pos++
        }
    }
    
    return 0, fmt.Errorf("unclosed expression (missing }})") 
}
```

### 5. String Interpolation Using Parsed Segments

```go
func (rc *ResolutionContext) interpolateString(template string) (interface{}, error) {
    segments, err := parseTemplate(template)
    if err != nil {
        return nil, fmt.Errorf("template parse error: %w", err)
    }
    
    // Check if entire string is a single expression (for backward compatibility)
    if len(segments) == 1 {
        if expr, ok := segments[0].(ExpressionSegment); ok {
            // Single expression - evaluate and return raw result (could be any type)
            return rc.EvaluateCELExpression(expr.Expression)
        }
    }
    
    // Multiple segments or mixed content - interpolate as string
    var result strings.Builder
    for _, segment := range segments {
        switch s := segment.(type) {
        case TextSegment:
            result.WriteString(s.Text)
            
        case ExpressionSegment:
            value, err := rc.EvaluateCELExpression(s.Expression)
            if err != nil {
                // Include position in error for better debugging
                return nil, fmt.Errorf("expression error at position %d: %w", s.Pos, err)
            }
            result.WriteString(convertToString(value))
        }
    }
    
    return result.String(), nil
}
```

## Usage Examples

### Input/Output Fields (Interpolation Mode)

```yaml
sequence:
  - id: notify
    op: send_message
    inputs:
      # Multiple expressions in one string
      message: "Order {{ inputs.order_id }} status: {{ sequence.check.outputs.status }}"
      
      # Mixed static and dynamic content
      subject: "[{{ inputs.priority }}] Order Update - {{ inputs.order_id }}"
      
      # Single expression still returns raw type
      user_data: "{{ sequence.fetch.outputs.user }}"  # Returns map if user is a map
      
      # Explicit string concatenation still works
      alt_message: '{{ "Order " + inputs.order_id + " processed" }}'

outputs:
  # String interpolation in output mapping
  summary: "Processed {{ sequence.count.outputs.total }} items in {{ sequence.timer.outputs.duration }}ms"
  
  # Single expression returns raw type
  data: "{{ sequence.transform.outputs.result }}"  # Returns whatever type result is
```

### When Conditions (Pure CEL Mode)

```yaml
transitions:
  - to: success
    # Pure CEL - no string interpolation
    when: "sequence.validate.outputs.valid == true && inputs.retry_count < 3"
    
  - to: error  
    # INVALID - cannot use string interpolation in when
    when: "Status is {{ outputs.status }}"  # ERROR!
    
  - to: retry
    # Valid - pure CEL expression
    when: 'outputs.message == "retry needed"'
```

## Implementation Files

### Files to Create

1. **`template_parser.go`** - Minimal custom parser
   - Segment types (Text, Expression)
   - Quote-aware expression boundary detection
   - ~200 lines total
   - No external dependencies

2. **`template_interpolate.go`** - Interpolation logic
   - Integration with ResolutionContext
   - Mode handling (Interpolation vs PureCEL)
   - ~150 lines

### Why Not Fork stdlib Lexer?

After analysis, forking `text/template/parse/lex.go` would require:
- Removing ~70% of the code (template-specific features)
- Rewriting ~20% (quote handling for CEL compatibility)
- Only keeping ~10% (basic structure)

Key incompatibilities:
- stdlib only handles `"` quotes, CEL needs both `"` and `'`
- stdlib interprets escape sequences, we need raw CEL expressions
- stdlib has ~40 token types, we need only ~2 (text and expression)

A minimal custom implementation is cleaner and more maintainable.

## Special Cases

### 1. Single Expression Handling

When a string contains only a single `{{ expression }}` with no other text:
- Evaluate the CEL expression
- Return the **raw result type** (map, array, number, boolean, etc.)
- This maintains backward compatibility and allows complex object retrieval

```yaml
# Single expression - returns raw type
data: "{{ sequence.fetch.outputs.body }}"        # Returns map/array/whatever body is
count: "{{ sequence.calc.outputs.total }}"       # Returns number
valid: "{{ sequence.check.outputs.is_valid }}"   # Returns boolean

# Multiple or mixed - always returns string
message: "Count: {{ sequence.calc.outputs.total }}"  # Returns string "Count: 42"
url: "{{ inputs.base }}{{ inputs.path }}"            # Returns concatenated string
```

### 2. Whitespace Handling

```yaml
# Significant whitespace preserved
message: "  {{ inputs.name }}  "   # Result: "  Alice  "

# Trimmed detection for single expression
data: "  {{ inputs.payload }}  "   # Single expression detected, returns raw payload
```

### 3. Error Handling

```yaml
# Expression evaluation error
message: "Hello {{ inputs.nonexistent }}"
# Result: "Hello <error: field not found: nonexistent>"

# Syntax error in expression  
message: "Value: {{ inputs.field + }}"
# Result: "Value: <error: invalid syntax>"
```

## Migration Path

### Phase 1: Implement Minimal Parser
- Write `parseTemplate` function with quote-aware expression detection
- Handle both `"` and `'` quotes per CEL conventions
- Add position tracking for error messages
- ~200 lines, no dependencies

### Phase 2: Add Interpolation Support
- Implement `interpolateString` using parsed segments
- Update `ResolveValue` to use new parser
- Maintain backward compatibility (single expressions return raw types)

### Phase 3: Update Calling Code
- Modify compiler to pass resolution mode
- Update state machine compiler to use `ModePureCEL` for `when` conditions
- Use `ModeInterpolation` for all other string fields

### Phase 4: Testing & Documentation
- Test quote handling edge cases:
  - `{{ "string with }} inside" }}`
  - `{{ 'don''t forget CEL escapes' }}`
  - `{{ inputs.status == 'active' }}`
  - Mixed quotes in same expression
- Update documentation with interpolation examples
- Update cheatsheet with quote rules

## Benefits of Minimal Custom Parser

1. **CEL-Compatible Quote Handling**: Supports both `"` and `'` quotes as CEL requires
2. **Simple and Focused**: ~200 lines vs ~600 in stdlib, easier to understand
3. **No Dependencies**: Self-contained implementation
4. **Correct String Handling**: 
   - Double quotes with backslash escapes
   - Single quotes with `''` escaping (CEL convention)
   - Properly ignores `}}` inside strings
5. **Precise Error Messages**: Position tracking for debugging
6. **Maintainable**: Clear, single-purpose code
7. **No Overhead**: No unused template language features

## Original Benefits (Still Apply)

1. **Natural Syntax**: Write templates the way users expect
2. **Less Verbose**: No need for complex CEL string concatenation
3. **Backward Compatible**: Single expressions still return raw types
4. **Clear Separation**: `when` conditions remain pure CEL for clarity
5. **Better Error Messages**: Can show exactly which expression failed in a template

## Success Criteria

1. All existing tests pass without modification
2. String interpolation works with multiple expressions
3. Single expressions return raw types (not stringified)
4. `when` conditions reject interpolation syntax
5. Error messages include position information
6. Correct CEL quote handling:
   - Double quotes: `{{ "string with }} inside" }}`
   - Single quotes: `{{ 'string with }} inside' }}`
   - Escaped quotes: `{{ "escaped \"quote\"" }}` and `{{ 'don''t' }}`
   - Mixed quotes: `{{ inputs.type == 'active' || inputs.code == "ABC" }}`
   - Backslash in double quotes: `{{ "C:\\Users\\file" }}`
   - Literal backslash in single quotes: `{{ 'C:\Users\file' }}`
7. Performance overhead < 5% for typical templates
8. Parser handles malformed input gracefully with clear errors