package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTemplate(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  []Segment
		expectErr bool
	}{
		{
			name:  "plain text",
			input: "Hello World",
			expected: []Segment{
				TextSegment{Text: "Hello World", Pos: 0},
			},
		},
		{
			name:  "single expression",
			input: "{{ inputs.name }}",
			expected: []Segment{
				ExpressionSegment{Expression: "inputs.name", Pos: 3},
			},
		},
		{
			name:  "text and expression",
			input: "Hello {{ inputs.name }}",
			expected: []Segment{
				TextSegment{Text: "Hello ", Pos: 0},
				ExpressionSegment{Expression: "inputs.name", Pos: 9},
			},
		},
		{
			name:  "multiple expressions",
			input: "Hello {{ inputs.name }}, your ID is {{ inputs.id }}",
			expected: []Segment{
				TextSegment{Text: "Hello ", Pos: 0},
				ExpressionSegment{Expression: "inputs.name", Pos: 9},
				TextSegment{Text: ", your ID is ", Pos: 23},
				ExpressionSegment{Expression: "inputs.id", Pos: 39},
			},
		},
		{
			name:  "expression with double quotes",
			input: `{{ "text with }} inside" }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `"text with }} inside"`, Pos: 3},
			},
		},
		{
			name:  "expression with single quotes",
			input: `{{ 'text with }} inside' }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `'text with }} inside'`, Pos: 3},
			},
		},
		{
			name:  "escaped double quotes",
			input: `{{ "escaped \"quote\"" }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `"escaped \"quote\""`, Pos: 3},
			},
		},
		{
			name:  "CEL single quote escape",
			input: `{{ 'don''t' }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `'don''t'`, Pos: 3},
			},
		},
		{
			name:  "mixed quotes in expression",
			input: `{{ inputs.type == 'active' || inputs.code == "ABC" }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `inputs.type == 'active' || inputs.code == "ABC"`, Pos: 3},
			},
		},
		{
			name:  "backslash in double quotes",
			input: `{{ "C:\\Users\\file" }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `"C:\\Users\\file"`, Pos: 3},
			},
		},
		{
			name:  "literal backslash in single quotes",
			input: `{{ 'C:\Users\file' }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `'C:\Users\file'`, Pos: 3},
			},
		},
		{
			name:  "complex interpolation",
			input: `[{{ scope.timestamp }}] User {{ inputs.user_id }} performed {{ inputs.action }}`,
			expected: []Segment{
				TextSegment{Text: "[", Pos: 0},
				ExpressionSegment{Expression: "scope.timestamp", Pos: 4},
				TextSegment{Text: "] User ", Pos: 22},
				ExpressionSegment{Expression: "inputs.user_id", Pos: 32},
				TextSegment{Text: " performed ", Pos: 49},
				ExpressionSegment{Expression: "inputs.action", Pos: 63},
			},
		},
		{
			name:      "unclosed expression",
			input:     "{{ inputs.name",
			expectErr: true,
		},
		{
			name:      "unclosed string in expression",
			input:     `{{ "unclosed string }}`,
			expectErr: true,
		},
		{
			name:  "empty expression",
			input: "{{ }}",
			expected: []Segment{
				ExpressionSegment{Expression: "", Pos: 3},
			},
		},
		{
			name:  "whitespace in expression",
			input: "{{  inputs.name  }}",
			expected: []Segment{
				ExpressionSegment{Expression: "inputs.name", Pos: 4},
			},
		},
		{
			name:  "nested braces in string",
			input: `{{ "value: {{}}" }}`,
			expected: []Segment{
				ExpressionSegment{Expression: `"value: {{}}"`, Pos: 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments, err := parseTemplate(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, segments)
		})
	}
}

func TestFindExpressionEnd(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		start     int
		expected  int
		expectErr bool
	}{
		{
			name:     "simple expression",
			input:    "{{ inputs.name }}",
			start:    3,
			expected: 15,
		},
		{
			name:     "expression with double quotes",
			input:    `{{ "text with }} inside" }}`,
			start:    3,
			expected: 25,
		},
		{
			name:     "expression with single quotes",
			input:    `{{ 'text with }} inside' }}`,
			start:    3,
			expected: 25,
		},
		{
			name:     "escaped quotes",
			input:    `{{ "escaped \"}}\" quote" }}`,
			start:    3,
			expected: 26,
		},
		{
			name:     "CEL single quote escape",
			input:    `{{ 'don''t forget' }}`,
			start:    3,
			expected: 19,
		},
		{
			name:      "unclosed expression",
			input:     "{{ inputs.name",
			start:     3,
			expectErr: true,
		},
		{
			name:      "unclosed string",
			input:     `{{ "unclosed }}`,
			start:     3,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end, err := findExpressionEnd(tt.input, tt.start)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, end)
		})
	}
}

func TestIsInterpolationTemplate(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		expected bool
	}{
		{
			name: "single expression",
			segments: []Segment{
				ExpressionSegment{Expression: "inputs.name", Pos: 0},
			},
			expected: false, // Backward compatible - single expression returns raw
		},
		{
			name: "single text",
			segments: []Segment{
				TextSegment{Text: "Hello World", Pos: 0},
			},
			expected: false, // Plain text
		},
		{
			name: "text and expression",
			segments: []Segment{
				TextSegment{Text: "Hello ", Pos: 0},
				ExpressionSegment{Expression: "inputs.name", Pos: 6},
			},
			expected: true, // Mixed content needs interpolation
		},
		{
			name: "multiple expressions",
			segments: []Segment{
				ExpressionSegment{Expression: "inputs.first", Pos: 0},
				ExpressionSegment{Expression: "inputs.last", Pos: 10},
			},
			expected: true, // Multiple expressions need interpolation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInterpolationTemplate(tt.segments)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "integer",
			input:    42,
			expected: "42",
		},
		{
			name:     "int64",
			input:    int64(42),
			expected: "42",
		},
		{
			name:     "float64 integer",
			input:    float64(42),
			expected: "42",
		},
		{
			name:     "float64 decimal",
			input:    3.14159,
			expected: "3.14159",
		},
		{
			name:     "boolean true",
			input:    true,
			expected: "true",
		},
		{
			name:     "boolean false",
			input:    false,
			expected: "false",
		},
		{
			name:     "nil",
			input:    nil,
			expected: "",
		},
		{
			name:     "map",
			input:    map[string]interface{}{"key": "value"},
			expected: "map[key:value]",
		},
		{
			name:     "slice",
			input:    []interface{}{"a", "b"},
			expected: "[a b]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}