package compiler

import (
	"fmt"
	"strings"
)

// interpolateString performs string interpolation with embedded CEL expressions
func (rc *ResolutionContext) interpolateString(template string, mode ResolutionMode) (interface{}, error) {
	// For pure CEL mode (when conditions), don't do interpolation
	if mode == ModePureCEL {
		// This should be a pure CEL expression, no {{ }} markers
		return rc.EvaluateCEL(template)
	}

	// Parse the template into segments
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
		// Single text segment - return as-is
		if text, ok := segments[0].(TextSegment); ok {
			return text.Text, nil
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

// ResolveTemplateWithMode handles expression evaluation with a specific mode
func (rc *ResolutionContext) ResolveTemplateWithMode(expr string, mode ResolutionMode) (interface{}, error) {
	// For pure CEL mode, this is a when condition - no {{ }} expected
	if mode == ModePureCEL {
		return rc.EvaluateCEL(expr)
	}

	// For interpolation mode, check if it looks like a template
	trimmed := strings.TrimSpace(expr)
	
	// If it doesn't have {{ }}, return as-is
	if !strings.Contains(trimmed, "{{") || !strings.Contains(trimmed, "}}") {
		return expr, nil
	}

	// Use interpolation
	return rc.interpolateString(expr, mode)
}

// ResolveValueWithMode recursively resolves templates in a value with a specific mode
func (rc *ResolutionContext) ResolveValueWithMode(value interface{}, mode ResolutionMode) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Resolve string templates with the given mode
		return rc.ResolveTemplateWithMode(v, mode)
	case map[string]interface{}:
		// Recursively resolve map values
		result := make(map[string]interface{})
		for key, val := range v {
			resolved, err := rc.ResolveValueWithMode(val, mode)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve key %s: %w", key, err)
			}
			result[key] = resolved
		}
		return result, nil
	case []interface{}:
		// Recursively resolve slice values
		result := make([]interface{}, len(v))
		for i, val := range v {
			resolved, err := rc.ResolveValueWithMode(val, mode)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve index %d: %w", i, err)
			}
			result[i] = resolved
		}
		return result, nil
	default:
		// For other types (numbers, bools, etc.), return as-is
		return value, nil
	}
}