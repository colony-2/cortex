package template

import (
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResolutionContext(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test-seq", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)
	assert.NotNil(t, ctx)
	assert.Equal(t, "sequence", ctx.ScopeType)
	assert.Equal(t, "test-seq", ctx.ScopeID)
	assert.NotNil(t, ctx.TemplateData.Inputs)
	assert.NotNil(t, ctx.TemplateData.Sequence)
	assert.NotNil(t, ctx.TemplateData.States)
	assert.NotNil(t, ctx.CELEnv)
}

func TestResolveTemplate_Simple(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	// Set up test data
	ctx.TemplateData.Inputs = map[string]interface{}{
		"name": "Alice",
		"age":  30,
	}

	tests := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "simple input reference",
			template: "{{ inputs.name }}",
			expected: "Alice",
		},
		{
			name:     "no template",
			template: "static text",
			expected: "static text",
		},
		{
			name:     "CEL string concatenation",
			template: `{{ "Hello " + inputs.name }}`,
			expected: "Hello Alice",
		},
		{
			name:     "CEL arithmetic",
			template: "{{ inputs.age + 10 }}",
			expected: int64(40),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.resolveTemplate(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveTemplate_SequenceReferences(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	// Add sequence nodes
	ctx.AddSequenceNode("fetch", map[string]interface{}{
		"status": 200,
		"body":   map[string]interface{}{"data": "test"},
	})
	ctx.AddSequenceNode("transform", map[string]interface{}{
		"result": "processed",
		"count":  5,
	})

	tests := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "sequence node output",
			template: "{{ sequence.fetch.outputs.status }}",
			expected: int64(200),
		},
		{
			name:     "nested sequence output",
			template: "{{ sequence.transform.outputs.result }}",
			expected: "processed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.resolveTemplate(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveTemplate_CELFunction(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	ctx.AddSequenceNode("calc", map[string]interface{}{
		"value": 10,
	})

	tests := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "cel arithmetic",
			template: `{{ sequence.calc.outputs.value + 5 }}`,
			expected: int64(15),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.resolveTemplate(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEvaluateCEL_Conditions(t *testing.T) {
	ctx, err := NewResolutionContext("state_machine", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	// Set up test data
	ctx.AddSequenceNode("validate", map[string]interface{}{
		"valid": true,
		"score": 0.9,
	})

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "simple boolean check",
			expression: "sequence.validate.outputs.valid == true",
			expected:   true,
		},
		{
			name:       "numeric comparison",
			expression: "sequence.validate.outputs.score > 0.8",
			expected:   true,
		},
		{
			name:       "combined condition",
			expression: "sequence.validate.outputs.valid && sequence.validate.outputs.score > 0.5",
			expected:   true,
		},
		{
			name:       "empty expression is true",
			expression: "",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.EvaluateCEL(tt.expression)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddSequenceNode_WithRuns(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	// Add initial run
	ctx.AddSequenceNode("retry_node", map[string]interface{}{
		"attempt": 1,
		"status":  "failed",
	})

	// Check initial state
	node := ctx.TemplateData.Sequence["retry_node"]
	assert.Equal(t, 1, node.Outputs.(map[string]interface{})["attempt"])
	assert.Len(t, node.Runs, 0)

	// Add second run (retry)
	ctx.AddSequenceNode("retry_node", map[string]interface{}{
		"attempt": 2,
		"status":  "success",
	})

	// Check updated state
	node = ctx.TemplateData.Sequence["retry_node"]
	assert.Equal(t, 2, node.Outputs.(map[string]interface{})["attempt"])
	assert.Equal(t, "success", node.Outputs.(map[string]interface{})["status"])
	assert.Len(t, node.Runs, 1)
	assert.Equal(t, 1, node.Runs[0].Outputs.(map[string]interface{})["attempt"])
}

func TestAddStateOutput(t *testing.T) {
	ctx, err := NewResolutionContext("state_machine", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	// Add state output
	ctx.AddStateOutput("validate", map[string]interface{}{
		"valid":    true,
		"metadata": map[string]interface{}{"source": "api"},
	})

	// Check state was added
	state := ctx.TemplateData.States["validate"]
	stateOutputs := state.Outputs.(map[string]interface{})
	assert.Equal(t, true, stateOutputs["valid"])
	assert.Equal(t, "api", stateOutputs["metadata"].(map[string]interface{})["source"])
}

func TestNewChildContext(t *testing.T) {
	parent, err := NewResolutionContext("state_machine", "parent", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	// Add state to parent
	parent.AddStateOutput("previous_state", map[string]interface{}{
		"result": "done",
	})

	// Create child context
	child, err := parent.NewChildContext(ScopeState, "child", map[string]interface{}{
		"child_input": "value",
	})
	require.NoError(t, err)

	// Check child has access to parent states
	assert.Equal(t, parent, child.Parent)
	assert.Equal(t, "value", convertRawToMap(child.TemplateData.Inputs)["child_input"])
	assert.Equal(t, "done", convertRawToMap(child.TemplateData.States["previous_state"].Outputs)["result"])
}

func TestResolveValue_Recursive(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	ctx.TemplateData.Inputs = map[string]interface{}{
		"base_url": "https://api.example.com",
		"version":  "v1",
	}

	ctx.AddSequenceNode("auth", map[string]interface{}{
		"token": "abc123",
	})

	// Test complex nested structure
	value := map[string]interface{}{
		"url": `{{ inputs.base_url + "/" + inputs.version }}`,
		"headers": map[string]interface{}{
			"Authorization": `{{ "Bearer " + sequence.auth.outputs.token }}`,
			"Content-Type":  "application/json",
		},
		"options": []interface{}{
			"{{ inputs.version }}",
			"stable",
		},
	}

	result, err := ctx.resolveValue(value)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})
	assert.Equal(t, "https://api.example.com/v1", resultMap["url"])

	headers := resultMap["headers"].(map[string]interface{})
	assert.Equal(t, "Bearer abc123", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])

	options := resultMap["options"].([]interface{})
	assert.Equal(t, "v1", options[0])
	assert.Equal(t, "stable", options[1])
}

func TestValidateTemplateReferences(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	tests := []struct {
		name      string
		template  string
		wantError bool
	}{
		{
			name:      "valid template",
			template:  "{{ inputs.name }}",
			wantError: false,
		},
		{
			name:      "invalid syntax",
			template:  "{{ inputs.name",
			wantError: false, // Not a template anymore, just a plain string
		},
		{
			name:      "not a template",
			template:  "plain text",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ctx.validateTemplateReferences(tt.template)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCELExpression(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	tests := []struct {
		name      string
		expr      string
		wantError bool
	}{
		{
			name:      "valid expression",
			expr:      "inputs.count > 0",
			wantError: false,
		},
		{
			name:      "invalid syntax",
			expr:      "inputs.count >",
			wantError: true,
		},
		{
			name:      "empty is valid",
			expr:      "",
			wantError: false,
		},
		{
			name:      "true is valid",
			expr:      "true",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ctx.validateCELExpression(tt.expr)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStateTransitionContext(t *testing.T) {
	// This test simulates a state machine transition evaluation
	// where the current state is a sequence and we need to evaluate
	// transitions based on the sequence's node outputs

	ctx, err := NewResolutionContext("state_machine", "sm", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	// Simulate a state that is a sequence
	stateCtx, err := ctx.NewChildContext("state", "process", ctx.TemplateData.Inputs)
	require.NoError(t, err)

	// Add nodes executed in the state's sequence
	stateCtx.AddSequenceNode("validate", map[string]interface{}{
		"valid": true,
	})
	stateCtx.AddSequenceNode("transform", map[string]interface{}{
		"success": true,
		"count":   10,
	})

	// Test transition evaluation with sequence node access
	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "check sequence node output",
			expression: "sequence.transform.outputs.success == true",
			expected:   true,
		},
		{
			name:       "check multiple sequence nodes",
			expression: "sequence.validate.outputs.valid && sequence.transform.outputs.count > 5",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := stateCtx.EvaluateCEL(tt.expression)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComplexStateMachineScenario(t *testing.T) {
	// This test simulates a complex state machine with nested sequences
	// and validates the template resolution across scopes

	// Create state machine context
	smCtx, err := NewResolutionContext("state_machine", "workflow", nil, ops.TaskExecutionContext{})
	require.NoError(t, err)

	smCtx.TemplateData.Inputs = map[string]interface{}{
		"user_id": "123",
		"payload": map[string]interface{}{"data": "test"},
	}

	// Execute first state (validate)
	smCtx.AddStateOutput("validate", map[string]interface{}{
		"valid":    true,
		"metadata": map[string]interface{}{"checked_at": time.Now()},
	})

	// Create context for process state (which is a sequence)
	processCtx, err := smCtx.NewChildContext("state", "process", smCtx.TemplateData.Inputs)
	require.NoError(t, err)

	// Process state can see previous states
	template := "{{ states.validate.outputs.valid }}"
	result, err := processCtx.resolveTemplate(template)
	require.NoError(t, err)
	assert.Equal(t, true, result)

	// Add sequence nodes within process state
	processCtx.AddSequenceNode("enrich", map[string]interface{}{
		"enriched_data": map[string]interface{}{
			"user":  "{{ inputs.user_id }}",
			"extra": "info",
		},
	})

	// Next node in sequence can see previous node
	template = "{{ sequence.enrich.outputs.enriched_data }}"
	result, err = processCtx.resolveTemplate(template)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Add the process state output to parent context
	smCtx.AddStateOutput("process", map[string]interface{}{
		"final_result": "completed",
	})

	// Final state can see all previous states
	finalCtx, err := smCtx.NewChildContext("state", "complete", smCtx.TemplateData.Inputs)
	require.NoError(t, err)

	template = "{{ states.process.outputs.final_result }}"
	result, err = finalCtx.resolveTemplate(template)
	require.NoError(t, err)
	assert.Equal(t, "completed", result)
}
