package yaml

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflowDefinition(t *testing.T) {
	yamlContent := `
name: test_workflow
description: A test workflow
version: "1.0"
inputs:
  - name: topic
    type: string
    required: true
    description: The topic to research
  - name: max_sources
    type: int
    default: 10
outputs:
  - name: report
    type: string
    description: The final report
workflow:
  type: sequential
  retry_policy:
    initial_interval: 1s
    maximum_attempts: 3
  steps:
    - id: research
      activity: research_activity
      inputs:
        topic: "{{ .Inputs.topic }}"
      outputs:
        data: research_results
  outputs:
    report: "{{ .Steps.research.outputs.data }}"
`

	parser := NewParser()
	workflow, err := parser.ParseWorkflowReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	assert.Equal(t, "test_workflow", workflow.Name)
	assert.Equal(t, "A test workflow", workflow.Description)
	assert.Equal(t, "1.0", workflow.Version)
	
	// Check inputs
	require.Len(t, workflow.Inputs, 2)
	assert.Equal(t, "topic", workflow.Inputs[0].Name)
	assert.Equal(t, "string", workflow.Inputs[0].Type)
	assert.True(t, workflow.Inputs[0].Required)
	
	assert.Equal(t, "max_sources", workflow.Inputs[1].Name)
	assert.Equal(t, "int", workflow.Inputs[1].Type)
	assert.Equal(t, 10, workflow.Inputs[1].Default)
	
	// Check workflow spec
	assert.Equal(t, "sequential", workflow.Workflow.Type)
	assert.Equal(t, 3, workflow.Workflow.RetryPolicy.MaximumAttempts)
	assert.Equal(t, time.Second, workflow.Workflow.RetryPolicy.InitialInterval)
	
	// Check steps
	require.Len(t, workflow.Workflow.Steps, 1)
	step := workflow.Workflow.Steps[0]
	assert.Equal(t, "research", step.ID)
	assert.Equal(t, "research_activity", step.Activity)
	assert.Equal(t, "{{ .Inputs.topic }}", step.Inputs["topic"])
	assert.Equal(t, "research_results", step.Outputs["data"])
}

func TestParseActivityDefinition(t *testing.T) {
	yamlContent := `
activities:
  - name: research_activity
    description: Research information about a topic
    timeout: 5m
    retry:
      maximum_attempts: 3
      non_retryable_errors:
        - "RATE_LIMIT_ERROR"
    inputs:
      - name: topic
        type: string
        required: true
    outputs:
      - name: research_results
        type: object
    implementation:
      type: http
      config:
        method: POST
        url: "http://example.com/research"
`

	parser := NewParser()
	activities, err := parser.ParseActivitiesReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	require.Len(t, activities, 1)
	
	activity := activities[0]
	assert.Equal(t, "research_activity", activity.Name)
	assert.Equal(t, "Research information about a topic", activity.Description)
	assert.Equal(t, 5*time.Minute, activity.Timeout)
	
	// Check retry policy
	assert.Equal(t, 3, activity.Retry.MaximumAttempts)
	assert.Contains(t, activity.Retry.NonRetryableErrors, "RATE_LIMIT_ERROR")
	
	// Check implementation
	assert.Equal(t, "http", activity.Implementation.Type)
	assert.Equal(t, "POST", activity.Implementation.Config["method"])
	assert.Equal(t, "http://example.com/research", activity.Implementation.Config["url"])
}

func TestParseParallelSteps(t *testing.T) {
	yamlContent := `
name: parallel_workflow
version: "1.0"
workflow:
  type: sequential
  steps:
    - id: parallel_group
      parallel:
        - id: task_a
          activity: activity_a
          outputs:
            result_a: output
        - id: task_b
          activity: activity_b
          outputs:
            result_b: output
    - id: merge_results
      activity: merge_activity
      inputs:
        a: "{{ .Steps.parallel_group.task_a.outputs.result_a }}"
        b: "{{ .Steps.parallel_group.task_b.outputs.result_b }}"
`

	parser := NewParser()
	workflow, err := parser.ParseWorkflowReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	require.Len(t, workflow.Workflow.Steps, 2)
	
	// Check parallel step
	parallelStep := workflow.Workflow.Steps[0]
	assert.Equal(t, "parallel_group", parallelStep.ID)
	require.Len(t, parallelStep.Parallel, 2)
	
	assert.Equal(t, "task_a", parallelStep.Parallel[0].ID)
	assert.Equal(t, "activity_a", parallelStep.Parallel[0].Activity)
	
	assert.Equal(t, "task_b", parallelStep.Parallel[1].ID)
	assert.Equal(t, "activity_b", parallelStep.Parallel[1].Activity)
	
	// Check merge step
	mergeStep := workflow.Workflow.Steps[1]
	assert.Equal(t, "merge_results", mergeStep.ID)
	assert.Equal(t, "{{ .Steps.parallel_group.task_a.outputs.result_a }}", mergeStep.Inputs["a"])
	assert.Equal(t, "{{ .Steps.parallel_group.task_b.outputs.result_b }}", mergeStep.Inputs["b"])
}

func TestParseAIPromptActivity(t *testing.T) {
	yamlContent := `
activities:
  - name: write_report_activity
    description: Generate a comprehensive report
    timeout: 10m
    implementation:
      type: ai_prompt
      config:
        model: gpt-4
        temperature: 0.7
        prompt: |
          Generate a report on {{ .topic }}
        response_format: json
        structured_output:
          schema:
            type: object
            properties:
              title:
                type: string
              summary:
                type: string
            required: ["title", "summary"]
`

	parser := NewParser()
	activities, err := parser.ParseActivitiesReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	require.Len(t, activities, 1)
	
	activity := activities[0]
	assert.Equal(t, "ai_prompt", activity.Implementation.Type)
	assert.Equal(t, "gpt-4", activity.Implementation.Config["model"])
	assert.Equal(t, 0.7, activity.Implementation.Config["temperature"])
	assert.Equal(t, "json", activity.Implementation.Config["response_format"])
	
	// Check structured output
	structuredOutput := activity.Implementation.Config["structured_output"].(map[string]interface{})
	schema := structuredOutput["schema"].(map[string]interface{})
	assert.Equal(t, "object", schema["type"])
}