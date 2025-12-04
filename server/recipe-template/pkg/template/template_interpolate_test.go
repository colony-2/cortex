package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpolateString(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	ctx := newSequenceCtx(t, recipeCtx, "test", map[string]interface{}{
		"name":     "Alice",
		"user_id":  123,
		"domain":   "example.com",
		"version":  "v2",
		"priority": "HIGH",
		"order_id": "ORD-456",
		"action":   "login",
		"status":   "active",
		"code":     "ABC",
	})

	addOpOutput(t, ctx, "fetch", map[string]interface{}{
		"status": 200,
		"body":   map[string]interface{}{"data": "test"},
	})
	addOpOutput(t, ctx, "check", map[string]interface{}{
		"valid":    true,
		"is_valid": true,
	})
	addOpOutput(t, ctx, "count", map[string]interface{}{
		"total": 42,
	})
	addOpOutput(t, ctx, "timer", map[string]interface{}{
		"duration": 1500,
	})

	tests := []struct {
		name     string
		template string
		mode     ResolutionMode
		expected interface{}
		isString bool // Whether result should be a string
	}{
		// Single expression tests (backward compatibility)
		{
			name:     "single expression returns raw type - string",
			template: "{{ inputs.name }}",
			mode:     ModeInterpolation,
			expected: "Alice",
			isString: true,
		},
		{
			name:     "single expression returns raw type - number",
			template: "{{ inputs.user_id }}",
			mode:     ModeInterpolation,
			expected: int64(123),
			isString: false,
		},
		{
			name:     "single expression returns raw type - boolean",
			template: "{{ sequence.check.outputs.valid }}",
			mode:     ModeInterpolation,
			expected: true,
			isString: false,
		},
		{
			name:     "single expression returns raw type - map",
			template: "{{ sequence.fetch.outputs.body }}",
			mode:     ModeInterpolation,
			expected: map[string]interface{}{"data": "test"},
			isString: false,
		},

		// Interpolation tests
		{
			name:     "multiple expressions interpolation",
			template: "Hello {{ inputs.name }}, your ID is {{ inputs.user_id }}",
			mode:     ModeInterpolation,
			expected: "Hello Alice, your ID is 123",
			isString: true,
		},
		{
			name:     "URL construction",
			template: "https://{{ inputs.domain }}/api/{{ inputs.version }}/users/{{ inputs.user_id }}",
			mode:     ModeInterpolation,
			expected: "https://example.com/api/v2/users/123",
			isString: true,
		},
		{
			name:     "log message format",
			template: "[{{ inputs.priority }}] Order {{ inputs.order_id }} - User {{ inputs.user_id }} performed {{ inputs.action }}",
			mode:     ModeInterpolation,
			expected: "[HIGH] Order ORD-456 - User 123 performed login",
			isString: true,
		},
		{
			name:     "mixed static and dynamic",
			template: "Order {{ inputs.order_id }} status: {{ inputs.status }}",
			mode:     ModeInterpolation,
			expected: "Order ORD-456 status: active",
			isString: true,
		},
		{
			name:     "processed items summary",
			template: "Processed {{ sequence.count.outputs.total }} items in {{ sequence.timer.outputs.duration }}ms",
			mode:     ModeInterpolation,
			expected: "Processed 42 items in 1500ms",
			isString: true,
		},

		// Plain text (no expressions)
		{
			name:     "plain text no markers",
			template: "Hello World",
			mode:     ModeInterpolation,
			expected: "Hello World",
			isString: true,
		},

		// CEL expressions in strings
		{
			name:     "CEL string concat still works",
			template: `{{ "Hello " + inputs.name }}`,
			mode:     ModeInterpolation,
			expected: "Hello Alice",
			isString: true,
		},
		{
			name:     "CEL arithmetic",
			template: "{{ inputs.user_id + 100 }}",
			mode:     ModeInterpolation,
			expected: int64(223),
			isString: false,
		},

		// Quotes in expressions
		{
			name:     "double quotes with }} inside",
			template: `{{ "text with }} inside" }}`,
			mode:     ModeInterpolation,
			expected: "text with }} inside",
			isString: true,
		},
		{
			name:     "single quotes with }} inside",
			template: `{{ 'text with }} inside' }}`,
			mode:     ModeInterpolation,
			expected: "text with }} inside",
			isString: true,
		},
		{
			name:     "mixed quotes in CEL",
			template: `{{ inputs.status == 'active' || inputs.code == "ABC" }}`,
			mode:     ModeInterpolation,
			expected: true,
			isString: false,
		},

		// Empty and whitespace
		{
			name:     "empty expression",
			template: "{{ }}",
			mode:     ModeInterpolation,
			expected: "",
			isString: true,
		},
		{
			name:     "whitespace preserved",
			template: "  {{ inputs.name }}  ",
			mode:     ModeInterpolation,
			expected: "  Alice  ", // Multiple segments due to whitespace
			isString: true,
		},
		{
			name:     "whitespace preserved in interpolation",
			template: "  Hello {{ inputs.name }}  ",
			mode:     ModeInterpolation,
			expected: "  Hello Alice  ",
			isString: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.interpolateString(tt.template, tt.mode)
			require.NoError(t, err)

			if tt.isString {
				assert.IsType(t, "", result)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInterpolateString_Errors(t *testing.T) {
	ctx := newSequenceCtx(t, newRecipeCtx(t, nil), "test", map[string]interface{}{})

	tests := []struct {
		name      string
		template  string
		mode      ResolutionMode
		expectErr string
	}{
		{
			name:      "unclosed expression",
			template:  "Hello {{ inputs.name",
			mode:      ModeInterpolation,
			expectErr: "template parse error",
		},
		{
			name:      "undefined field",
			template:  "{{ inputs.nonexistent }}",
			mode:      ModeInterpolation,
			expectErr: "no such key: nonexistent",
		},
		{
			name:      "invalid CEL syntax",
			template:  "{{ inputs.name + }}",
			mode:      ModeInterpolation,
			expectErr: "failed to compile CEL expression",
		},
		{
			name:      "multiple expressions with error",
			template:  "Hello {{ inputs.name }}, ID: {{ inputs.missing }}",
			mode:      ModeInterpolation,
			expectErr: "expression error at position",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.interpolateString(tt.template, tt.mode)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectErr)
			assert.Nil(t, result)
		})
	}
}

func TestResolveValueWithMode(t *testing.T) {
	ctx := newSequenceCtx(t, newRecipeCtx(t, nil), "test", map[string]interface{}{
		"name":  "Alice",
		"count": 5,
	})

	tests := []struct {
		name     string
		input    interface{}
		mode     ResolutionMode
		expected interface{}
	}{
		{
			name:     "string interpolation",
			input:    "Hello {{ inputs.name }}",
			mode:     ModeInterpolation,
			expected: "Hello Alice",
		},
		{
			name: "map with templates",
			input: map[string]interface{}{
				"greeting": "Hello {{ inputs.name }}",
				"count":    "{{ inputs.count }}",
				"static":   "no template",
			},
			mode: ModeInterpolation,
			expected: map[string]interface{}{
				"greeting": "Hello Alice",
				"count":    int64(5),
				"static":   "no template",
			},
		},
		{
			name: "slice with templates",
			input: []interface{}{
				"{{ inputs.name }}",
				"Count: {{ inputs.count }}",
				"static",
			},
			mode: ModeInterpolation,
			expected: []interface{}{
				"Alice",
				"Count: 5",
				"static",
			},
		},
		{
			name: "nested structure",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"name":    "{{ inputs.name }}",
					"message": "Hello {{ inputs.name }}!",
				},
				"items": []interface{}{
					"Item {{ inputs.count }}",
				},
			},
			mode: ModeInterpolation,
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"name":    "Alice",
					"message": "Hello Alice!",
				},
				"items": []interface{}{
					"Item 5",
				},
			},
		},
		{
			name:     "non-string types pass through",
			input:    42,
			mode:     ModeInterpolation,
			expected: 42,
		},
		{
			name:     "boolean pass through",
			input:    true,
			mode:     ModeInterpolation,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.ResolveValueWithMode(tt.input, tt.mode)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPureCELMode(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	ctx := newStateMachineCtx(t, recipeCtx, "test", map[string]interface{}{
		"retry_count": 2,
		"max_retries": 3,
	})

	seqCtx := newSequenceCtx(t, ctx, "validate-seq", ctx.TemplateData.ContainerInputs)

	addOpOutput(t, seqCtx, "validate", map[string]interface{}{
		"valid": true,
	})

	tests := []struct {
		name      string
		expr      string
		expected  bool
		expectErr bool
	}{
		{
			name:     "simple boolean",
			expr:     "true",
			expected: true,
		},
		{
			name:     "CEL comparison",
			expr:     "inputs.retry_count < inputs.max_retries",
			expected: true,
		},
		{
			name:     "CEL logical AND",
			expr:     "sequence.validate.outputs.valid == true && inputs.retry_count < 3",
			expected: true,
		},
		{
			name:     "CEL logical OR",
			expr:     "inputs.retry_count > 5 || sequence.validate.outputs.valid == true",
			expected: true,
		},
		{
			name:      "invalid CEL",
			expr:      "invalid syntax {{",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := seqCtx.EvaluateCEL(tt.expr)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
