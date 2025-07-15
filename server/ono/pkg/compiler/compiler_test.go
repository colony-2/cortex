package compiler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibethis/server/recipe-core/pkg/yaml"
)

func TestCompileSimpleWorkflow(t *testing.T) {
	// Create a simple workflow definition
	workflowDef := &yaml.WorkflowDefinition{
		Name:        "test_workflow",
		Description: "Test workflow",
		Version:     "1.0",
		Inputs: []yaml.InputDefinition{
			{Name: "input1", Type: "string", Required: true},
		},
		Outputs: []yaml.OutputDefinition{
			{Name: "output1", Type: "string"},
		},
		Workflow: yaml.WorkflowSpec{
			Type: "sequential",
			RetryPolicy: yaml.RetryPolicy{
				InitialInterval: time.Second,
				MaximumAttempts: 3,
			},
			Steps: []yaml.Step{
				{
					ID:       "step1",
					Activity: "test_activity",
					Inputs: map[string]string{
						"param1": "{{ .Inputs.input1 }}",
					},
					Outputs: map[string]string{
						"result": "activity_output",
					},
				},
			},
			Outputs: map[string]string{
				"output1": "{{ .Steps.step1.outputs.result }}",
			},
		},
	}

	// Create activity registry and register test activity
	registry := NewActivityRegistry()
	registry.RegisterActivity(&yaml.ActivityDefinition{
		Name:    "test_activity",
		Timeout: 5 * time.Minute,
	})

	// Compile workflow
	compiler := NewCompiler(registry)
	workflowFunc, err := compiler.CompileWorkflow(workflowDef)
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

func TestParallelWorkflowCompilation(t *testing.T) {
	workflowDef := &yaml.WorkflowDefinition{
		Name:    "parallel_workflow",
		Version: "1.0",
		Workflow: yaml.WorkflowSpec{
			Type: "sequential",
			Steps: []yaml.Step{
				{
					ID: "parallel_tasks",
					Parallel: []yaml.Step{
						{
							ID:       "task_a",
							Activity: "activity_a",
							Outputs: map[string]string{
								"result": "output_a",
							},
						},
						{
							ID:       "task_b",
							Activity: "activity_b",
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
	registry.RegisterActivity(&yaml.ActivityDefinition{Name: "activity_a", Timeout: time.Minute})
	registry.RegisterActivity(&yaml.ActivityDefinition{Name: "activity_b", Timeout: time.Minute})

	compiler := NewCompiler(registry)
	workflowFunc, err := compiler.CompileWorkflow(workflowDef)
	require.NoError(t, err)
	assert.NotNil(t, workflowFunc)
}

func TestActivityRegistry(t *testing.T) {
	registry := NewActivityRegistry()

	// Register activity
	activityDef := &yaml.ActivityDefinition{
		Name:        "test_activity",
		Description: "Test activity",
		Timeout:     5 * time.Minute,
	}
	registry.RegisterActivity(activityDef)

	// Retrieve activity
	retrieved := registry.GetActivity("test_activity")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test_activity", retrieved.Name)
	assert.Equal(t, 5*time.Minute, retrieved.Timeout)

	// Non-existent activity
	notFound := registry.GetActivity("non_existent")
	assert.Nil(t, notFound)
}