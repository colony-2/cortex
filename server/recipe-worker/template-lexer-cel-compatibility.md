# Template Lexer vs CEL String Compatibility Analysis

## CEL String Literal Specification

CEL supports these string literal formats:

```cel
// Double-quoted strings (most common)
"Hello, World"
"Line 1\nLine 2"        // Escape sequences
"Quote: \"nested\""     // Escaped quotes
"Path: C:\\Users\\file" // Escaped backslashes

// Single-quoted strings (raw strings - no escape sequences)
'Hello, World'
'No escapes: \n stays literal'
'Single quote: '' (doubled to escape)'

// Triple-quoted strings (multiline raw strings)
'''
Multiple
Lines
'''

"""
Multiple lines
with "quotes" inside
"""

// Bytes literals (prefixed with 'b')
b"byte string"
b'\x00\xFF'
```

### CEL Escape Sequences (in double-quoted strings only):
- `\n` - newline
- `\r` - carriage return  
- `\t` - tab
- `\\` - backslash
- `\"` - double quote
- `\'` - single quote (though not needed in double-quoted)
- `\xHH` - hex byte
- `\uHHHH` - Unicode code point
- `\UHHHHHHHH` - Unicode code point (32-bit)

## Go Template Lexer String Handling

The stdlib lexer recognizes:

```go
// From text/template/parse/lex.go
func lexQuote(l *lexer) stateFn {
    // Handles only double-quoted strings
    // Recognizes these escapes:
    // \" \\ \n \r \t
    // Does NOT handle:
    // - Single-quoted strings
    // - Triple-quoted strings  
    // - Hex escapes (\xHH)
    // - Unicode escapes (\uHHHH, \UHHHHHHHH)
    // - Byte string prefix (b"...")
}
```

## Compatibility Issues

### 1. String Delimiter Mismatch

```yaml
# CEL supports multiple quote styles
expr1: {{ 'single quoted string' }}          # ❌ Lexer won't recognize
expr2: {{ "double quoted string" }}          # ✅ Works
expr3: {{ '''triple quoted''' }}             # ❌ Lexer won't recognize
expr4: {{ inputs.field == 'value' }}         # ❌ Problem!
```

### 2. Escape Sequence Differences

```yaml
# CEL hex/unicode escapes
expr1: {{ "Unicode: \u4E16\u754C" }}         # ❌ Lexer won't handle \u correctly
expr2: {{ "Hex: \x41\x42" }}                 # ❌ Lexer won't handle \x
expr3: {{ "Tab: \t" }}                       # ✅ Works
```

### 3. Common CEL Patterns That Break

```cel
// These common CEL expressions will fail lexing:
{{ inputs.status == 'active' }}              // Single quotes
{{ inputs.name in ['Alice', 'Bob'] }}        // Single quotes in list
{{ b'bytes' }}                               // Byte literals
{{ matches(inputs.text, r'regex.*') }}       // Raw string for regex
```

## Solution: Modified Lexer Approach

### Option 1: Minimal String Awareness (Recommended)

Don't try to parse string contents at all - just track quote balancing:

```go
func lexInsideExpression(l *lexer) stateFn {
    var quoteStack []rune
    
    for {
        // Check for }} only when not inside ANY quotes
        if len(quoteStack) == 0 && strings.HasPrefix(l.input[l.pos:], "}}") {
            if l.pos > l.start {
                l.emit(itemRawExpression)  // Entire CEL expression as-is
            }
            return lexRightDelim
        }
        
        switch r := l.next(); {
        case r == eof:
            return l.errorf("unclosed expression")
            
        case r == '\\':
            // Skip next character when inside quotes
            if len(quoteStack) > 0 {
                l.next()
            }
            
        case r == '"' || r == '\'':
            if len(quoteStack) == 0 {
                // Starting a quoted section
                quoteStack = append(quoteStack, r)
            } else if quoteStack[len(quoteStack)-1] == r {
                // Ending current quoted section
                quoteStack = quoteStack[:len(quoteStack)-1]
            }
            // Different quote type inside another is fine
        }
    }
}
```

### Option 2: Full CEL String Support (Complex)

Implement complete CEL string literal parsing:

```go
func lexInsideExpression(l *lexer) stateFn {
    for {
        if !l.inString && strings.HasPrefix(l.input[l.pos:], "}}") {
            // Handle end of expression
        }
        
        switch {
        case strings.HasPrefix(l.input[l.pos:], "'''"):
            l.scanTripleQuoted('\'')
        case strings.HasPrefix(l.input[l.pos:], `"""`):
            l.scanTripleQuoted('"')
        case strings.HasPrefix(l.input[l.pos:], "b'"):
            l.scanByteString('\'')
        case strings.HasPrefix(l.input[l.pos:], `b"`):
            l.scanByteString('"')
        case l.peek() == '\'':
            l.scanSingleQuoted()
        case l.peek() == '"':
            l.scanDoubleQuoted()
        }
    }
}
```

## Recommendation: Use Option 1

The lexer should be **quote-aware but not quote-parsing**:

1. **Track quote state** - Know when we're inside strings
2. **Handle escapes minimally** - Just skip the next char after `\`
3. **Support both quote types** - Both `'` and `"` as CEL uses both
4. **Don't parse contents** - Let CEL handle escape sequences

This gives us:
- ✅ Correct `}}` detection (not inside strings)
- ✅ Support for all CEL string styles
- ✅ Simple implementation
- ✅ No need to duplicate CEL's string parsing

## Updated Lexer Implementation

```go
func lexInsideExpression(l *lexer) stateFn {
    // Simple quote tracking - just track depth and type
    var inString bool
    var stringDelim rune
    
    for {
        // Only check for }} when not in a string
        if !inString && strings.HasPrefix(l.input[l.pos:], "}}") {
            if l.pos > l.start {
                l.emit(itemRawExpression)
            }
            return lexRightDelim
        }
        
        switch r := l.next(); {
        case r == eof:
            return l.errorf("unclosed expression")
            
        case r == '\\' && inString:
            // In any string, backslash escapes next char
            // Don't interpret it, just skip next char
            if l.next() == eof {
                return l.errorf("unexpected EOF in string escape")
            }
            
        case (r == '"' || r == '\'') && !inString:
            // Starting a string
            inString = true
            stringDelim = r
            
        case r == stringDelim && inString:
            // Check if it's escaped for single quotes CEL-style
            if stringDelim == '\'' && l.peek() == '\'' {
                // Two single quotes = escaped single quote in CEL
                l.next()
            } else {
                // End of string
                inString = false
            }
        }
    }
}
```

## Test Cases for Validation

```yaml
# These should all lex correctly:
test1: {{ "double quoted with }} inside" }}
test2: {{ 'single quoted with }} inside' }}  
test3: {{ inputs.status == 'active' }}
test4: {{ inputs.name in ["Alice", "Bob"] }}
test5: {{ 'escaped: '' quote' }}  # CEL single quote escape
test6: {{ "escaped: \" quote" }}   # Standard escape
test7: {{ "Unicode: Hello" }}      # CEL will handle \u later
test8: {{ inputs.path == "C:\\Users\\file" }}  # Backslash handling
```

## Key Decisions

1. **Lexer only tracks quote boundaries** - doesn't interpret escape sequences
2. **Support both `'` and `"` quotes** - as CEL uses both frequently  
3. **Handle `''` for single-quote escape** - CEL convention
4. **Pass raw expression to CEL** - let CEL handle all escape interpretation
5. **No triple-quote support needed** - unlikely in our template contexts