package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestCompileSimpleRecipe(t *testing.T) {
	// Create a simple recipe definition using unified format
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:        "test-recipe",
		Description: "Test recipe",
		Version:     "1.0",
		Steps: []yamlpkg.Step{
			{
				ID:   "step1",
				Uses: "test_activity",
				Config: map[string]interface{}{
					"type": "function",
				},
				Inputs: map[string]interface{}{
					"param1": "test_value",
				},
				Outputs: map[string]string{
					"result": "activity_output",
				},
			},
		},
	}

	// Create activity registry and register test activity
	registry := NewActivityRegistry()
	registry.RegisterActivity("test_activity")

	// Compile recipe
	compiler := NewCompiler(registry)
	workflowFunc, err := compiler.CompileWorkflow(recipeDef)
	require.NoError(t, err)
	assert.NotNil(t, workflowFunc)
}

func TestTemplateResolver(t *testing.T) {
	state := &WorkflowState{
		Inputs: map[string]interface{}{
			"topic":       "AI Agents",
			"max_sources": 5,
		},
		Steps: map[string]StepResult{
			"research": {
				Outputs: map[string]interface{}{
					"sources": []string{"source1", "source2"},
					"summary": "Research summary",
				},
			},
		},
	}

	resolver := NewTemplateResolver(state)

	tests := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "simple input reference",
			template: "{{ .Inputs.topic }}",
			expected: "AI Agents",
		},
		{
			name:     "step output reference",
			template: "{{ .Steps.research.outputs.summary }}",
			expected: "Research summary",
		},
		{
			name:     "no template",
			template: "plain text",
			expected: "plain text",
		},
		{
			name:     "complex template",
			template: "Research on {{ .Inputs.topic }} with {{ .Inputs.max_sources }} sources",
			expected: "Research on AI Agents with 5 sources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.Resolve(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParallelRecipeCompilation(t *testing.T) {
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:    "parallel-recipe",
		Version: "1.0",
		Steps: []yamlpkg.Step{
			{
				ID: "parallel_tasks",
				Parallel: &yamlpkg.ParallelSpec{
					Steps: []yamlpkg.Step{
						{
							ID:   "task_a",
							Uses: "activity_a",
							Config: map[string]interface{}{
								"type": "function",
							},
							Outputs: map[string]string{
								"result": "output_a",
							},
						},
						{
							ID:   "task_b",
							Uses: "activity_b",
							Config: map[string]interface{}{
								"type": "function",
							},
							Outputs: map[string]string{
								"result": "output_b",
							},
						},
					},
				},
			},
		},
	}

	registry := NewActivityRegistry()
	registry.RegisterActivity("activity_a")
	registry.RegisterActivity("activity_b")

	compiler := NewCompiler(registry)
	workflowFunc, err := compiler.CompileWorkflow(recipeDef)
	require.NoError(t, err)
	assert.NotNil(t, workflowFunc)
}

func TestActivityRegistry(t *testing.T) {
	registry := NewActivityRegistry()

	// Register activity by name
	registry.RegisterActivity("test_activity")

	// Check if activity is registered
	hasActivity := registry.HasActivity("test_activity")
	assert.True(t, hasActivity)

	// Non-existent activity
	notFound := registry.HasActivity("non_existent")
	assert.False(t, notFound)
}

func TestRecipeWithSharedActivities(t *testing.T) {
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:    "shared-recipe",
		Version: "1.0",
		Shared: map[string]yamlpkg.SharedActivity{
			"my_llm": {
				Uses: "llm",
				Config: map[string]interface{}{
					"type":  "ai_prompt",
					"model": "gpt-4",
				},
			},
		},
		Steps: []yamlpkg.Step{
			{
				ID:   "analyze",
				Uses: "shared/my_llm",
				Inputs: map[string]interface{}{
					"prompt": "Analyze this data",
				},
			},
		},
	}

	registry := NewActivityRegistry()
	registry.RegisterActivity("llm")
	registry.RegisterActivity("shared/my_llm")

	compiler := NewCompiler(registry)
	workflowFunc, err := compiler.CompileWorkflow(recipeDef)
	require.NoError(t, err)
	assert.NotNil(t, workflowFunc)
}