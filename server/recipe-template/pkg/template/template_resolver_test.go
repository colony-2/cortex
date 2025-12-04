package template

import (
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/contextual"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRecipeResolutionContext(t *testing.T) {
	inputs := map[string]interface{}{
		"input": "value",
	}
	ctx := newRecipeCtx(t, inputs)

	assert.Equal(t, ScopeRecipe, ctx.ScopeType)
	assert.Equal(t, inputs, ctx.TemplateData.ContainerInputs)
	assert.NotNil(t, ctx.TemplateData.Sequence)
	assert.NotNil(t, ctx.TemplateData.States)
	assert.NotNil(t, ctx.CELEnv)
	assert.Equal(t, contextual.Invocation{NodePath: "", InvokeSeq: 0}, ctx.TemplateData.Context.Invocation)
}

func TestResolveTemplate_Simple(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	seqCtx := newSequenceCtx(t, recipeCtx, "test-seq", map[string]interface{}{
		"name": "Alice",
		"age":  30,
	})

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
			result, err := seqCtx.resolveTemplate(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveTemplate_SequenceReferences(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	seqCtx := newSequenceCtx(t, recipeCtx, "test", map[string]interface{}{})

	addOpOutput(t, seqCtx, "fetch", map[string]interface{}{
		"status": 200,
		"body":   map[string]interface{}{"data": "test"},
	})
	addOpOutput(t, seqCtx, "transform", map[string]interface{}{
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
			result, err := seqCtx.resolveTemplate(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveTemplate_CELFunction(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	seqCtx := newSequenceCtx(t, recipeCtx, "test", map[string]interface{}{})

	addOpOutput(t, seqCtx, "calc", map[string]interface{}{
		"value": 10,
	})

	result, err := seqCtx.resolveTemplate(`{{ sequence.calc.outputs.value + 5 }}`)
	require.NoError(t, err)
	assert.Equal(t, int64(15), result)
}

func TestEvaluateCEL_Conditions(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	seqCtx := newSequenceCtx(t, recipeCtx, "test", map[string]interface{}{})

	addOpOutput(t, seqCtx, "validate", map[string]interface{}{
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
			result, err := seqCtx.EvaluateCEL(tt.expression)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddSequenceNode_WithRuns(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	seqCtx := newSequenceCtx(t, recipeCtx, "test", map[string]interface{}{})

	addOpOutput(t, seqCtx, "retry_node", map[string]interface{}{
		"attempt": 1,
		"status":  "failed",
	})

	node := seqCtx.TemplateData.Sequence["retry_node"]
	assert.Equal(t, 1, node.Outputs["attempt"])
	assert.Len(t, node.Runs, 0)

	addOpOutput(t, seqCtx, "retry_node", map[string]interface{}{
		"attempt": 2,
		"status":  "success",
	})

	node = seqCtx.TemplateData.Sequence["retry_node"]
	assert.Equal(t, 2, node.Outputs["attempt"])
	assert.Equal(t, "success", node.Outputs["status"])
	assert.Len(t, node.Runs, 1)
	assert.Equal(t, 1, node.Runs[0].Outputs["attempt"])
}

func TestAddStateOutput(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	smCtx := newStateMachineCtx(t, recipeCtx, "test-sm", map[string]interface{}{})

	addStateOutput(t, smCtx, "validate", map[string]interface{}{
		"valid":    true,
		"metadata": map[string]interface{}{"source": "api"},
	})

	state := smCtx.TemplateData.States["validate"]
	assert.True(t, state.Outputs["valid"].(bool))
	assert.Equal(t, "api", state.Outputs["metadata"].(map[string]interface{})["source"])
}

func TestNewChildContext(t *testing.T) {
	recipeCtx := newRecipeCtx(t, map[string]interface{}{
		"child_input": "value",
	})
	smCtx := newStateMachineCtx(t, recipeCtx, "parent-sm", recipeCtx.TemplateData.ContainerInputs)

	addStateOutput(t, smCtx, "previous_state", map[string]interface{}{
		"result": "done",
	})

	child := newStateCtx(t, smCtx, "child")

	assert.Equal(t, smCtx, child.Parent)
	assert.Equal(t, "value", child.TemplateData.ContainerInputs["child_input"])
	assert.Equal(t, "done", child.TemplateData.States["previous_state"].Outputs["result"])
}

func TestResolveValue_Recursive(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	seqCtx := newSequenceCtx(t, recipeCtx, "test", map[string]interface{}{
		"base_url": "https://api.example.com",
		"version":  "v1",
	})

	addOpOutput(t, seqCtx, "auth", map[string]interface{}{
		"token": "abc123",
	})

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

	result, err := seqCtx.resolveValue(value)
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
	seqCtx := newSequenceCtx(t, newRecipeCtx(t, nil), "test", map[string]interface{}{})

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
			err := seqCtx.validateTemplateReferences(tt.template)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCELExpression(t *testing.T) {
	seqCtx := newSequenceCtx(t, newRecipeCtx(t, nil), "test", map[string]interface{}{})

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
			err := seqCtx.validateCELExpression(tt.expr)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStateTransitionContext(t *testing.T) {
	recipeCtx := newRecipeCtx(t, nil)
	smCtx := newStateMachineCtx(t, recipeCtx, "sm", map[string]interface{}{})
	stateCtx := newStateCtx(t, smCtx, "process")
	seqCtx := newSequenceCtx(t, stateCtx, "process-seq", smCtx.TemplateData.ContainerInputs)

	addOpOutput(t, seqCtx, "validate", map[string]interface{}{
		"valid": true,
	})
	addOpOutput(t, seqCtx, "transform", map[string]interface{}{
		"success": true,
		"count":   10,
	})

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
			result, err := seqCtx.EvaluateCEL(tt.expression)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComplexStateMachineScenario(t *testing.T) {
	recipeCtx := newRecipeCtx(t, map[string]interface{}{
		"user_id": "123",
		"payload": map[string]interface{}{"data": "test"},
	})
	smCtx := newStateMachineCtx(t, recipeCtx, "workflow", recipeCtx.TemplateData.ContainerInputs)

	addStateOutput(t, smCtx, "validate", map[string]interface{}{
		"valid":    true,
		"metadata": map[string]interface{}{"checked_at": time.Now()},
	})

	processState := newStateCtx(t, smCtx, "process")
	processSeq := newSequenceCtx(t, processState, "process-seq", smCtx.TemplateData.ContainerInputs)

	template := "{{ states.validate.outputs.valid }}"
	result, err := processState.resolveTemplate(template)
	require.NoError(t, err)
	assert.Equal(t, true, result)

	addOpOutput(t, processSeq, "enrich", map[string]interface{}{
		"enriched_data": map[string]interface{}{
			"user":  "{{ inputs.user_id }}",
			"extra": "info",
		},
	})

	template = "{{ sequence.enrich.outputs.enriched_data }}"
	result, err = processSeq.resolveTemplate(template)
	require.NoError(t, err)
	assert.NotNil(t, result)

	addStateOutput(t, smCtx, "process", map[string]interface{}{
		"final_result": "completed",
	})

	finalCtx := newStateCtx(t, smCtx, "complete")

	template = "{{ states.process.outputs.final_result }}"
	result, err = finalCtx.resolveTemplate(template)
	require.NoError(t, err)
	assert.Equal(t, "completed", result)
}
