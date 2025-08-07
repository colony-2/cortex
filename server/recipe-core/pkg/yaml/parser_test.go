package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUnifiedRecipe(t *testing.T) {
	yamlContent := `
name: test_recipe
description: A test recipe using unified activity model
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
    value: "{{ .Steps.research.outputs.data }}"
    type: string
    description: The final report
steps:
  - id: research
    name: Research Topic
    uses: research_activity
    inputs:
      topic: "{{ .Inputs.topic }}"
      max_sources: "{{ .Inputs.max_sources }}"
    outputs:
      data: ".research_results"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	assert.Equal(t, "test_recipe", recipe.Name)
	assert.Equal(t, "A test recipe using unified activity model", recipe.Description)
	assert.Equal(t, "1.0", recipe.Version)
	
	// Check inputs
	require.Len(t, recipe.Inputs, 2)
	assert.Equal(t, "topic", recipe.Inputs[0].Name)
	assert.Equal(t, "string", recipe.Inputs[0].Type)
	assert.True(t, recipe.Inputs[0].Required)
	
	assert.Equal(t, "max_sources", recipe.Inputs[1].Name)
	assert.Equal(t, "int", recipe.Inputs[1].Type)
	assert.Equal(t, 10, recipe.Inputs[1].Default)
	
	// Check outputs
	require.Len(t, recipe.Outputs, 1)
	assert.Equal(t, "report", recipe.Outputs[0].Name)
	assert.Equal(t, "{{ .Steps.research.outputs.data }}", recipe.Outputs[0].Value)
	assert.Equal(t, "string", recipe.Outputs[0].Type)
	
	// Check steps
	require.Len(t, recipe.Steps, 1)
	step := recipe.Steps[0]
	assert.Equal(t, "research", step.ID)
	assert.Equal(t, "Research Topic", step.Name)
	assert.Equal(t, "research_activity", step.Uses)
	assert.Equal(t, "{{ .Inputs.topic }}", step.Inputs["topic"])
	assert.Equal(t, ".research_results", step.Outputs["data"])
}

func TestParseSharedActivities(t *testing.T) {
	yamlContent := `
name: recipe_with_shared
version: "1.0"
shared:
  data_analyst:
    uses: llm
    config:
      model: "gpt-4"
      temperature: 0.3
      system_prompt: "You are a data analyst."
  
  validator:
    uses: validation_activity
    config:
      validation_type: "strict"
      timeout: "30s"

steps:
  - id: validate
    uses: shared/validator
    inputs:
      data: "{{ .Inputs.data }}"
  
  - id: analyze
    uses: shared/data_analyst
    inputs:
      prompt: "Analyze: {{ .Steps.validate.outputs }}"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	// Check shared activities
	require.NotNil(t, recipe.Shared)
	require.Len(t, recipe.Shared, 2)
	
	// Check data analyst shared activity
	dataAnalyst, exists := recipe.Shared["data_analyst"]
	require.True(t, exists)
	assert.Equal(t, "llm", dataAnalyst.Uses)
	assert.Equal(t, "gpt-4", dataAnalyst.Config["model"])
	assert.Equal(t, 0.3, dataAnalyst.Config["temperature"])
	
	// Check validator shared activity
	validator, exists := recipe.Shared["validator"]
	require.True(t, exists)
	assert.Equal(t, "validation_activity", validator.Uses)
	assert.Equal(t, "strict", validator.Config["validation_type"])
	
	// Check steps reference shared activities
	require.Len(t, recipe.Steps, 2)
	assert.Equal(t, "shared/validator", recipe.Steps[0].Uses)
	assert.Equal(t, "shared/data_analyst", recipe.Steps[1].Uses)
}

func TestParseParallelProcessing(t *testing.T) {
	yamlContent := `
name: parallel_processing
version: "1.0"
steps:
  - id: process_items
    name: Process Items in Parallel
    parallel:
      for_each: "{{ .Inputs.items }}"
      as: item
      steps:
        - id: transform
          uses: transform_activity
          inputs:
            data: "{{ .item }}"
            mode: "enhanced"
        - id: validate
          uses: validation_activity
          inputs:
            data: "{{ .Steps.transform.outputs }}"
  
  - id: aggregate
    uses: aggregation_activity
    inputs:
      results: "{{ .Steps.process_items.outputs }}"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	require.Len(t, recipe.Steps, 2)
	
	// Check parallel step
	parallelStep := recipe.Steps[0]
	assert.Equal(t, "process_items", parallelStep.ID)
	assert.Equal(t, "Process Items in Parallel", parallelStep.Name)
	
	require.NotNil(t, parallelStep.Parallel)
	assert.Equal(t, "{{ .Inputs.items }}", parallelStep.Parallel.ForEach)
	assert.Equal(t, "item", parallelStep.Parallel.As)
	
	// Check parallel steps
	require.Len(t, parallelStep.Parallel.Steps, 2)
	assert.Equal(t, "transform", parallelStep.Parallel.Steps[0].ID)
	assert.Equal(t, "transform_activity", parallelStep.Parallel.Steps[0].Uses)
	assert.Equal(t, "{{ .item }}", parallelStep.Parallel.Steps[0].Inputs["data"])
	
	// Check aggregation step
	aggStep := recipe.Steps[1]
	assert.Equal(t, "aggregate", aggStep.ID)
	assert.Equal(t, "aggregation_activity", aggStep.Uses)
	assert.Equal(t, "{{ .Steps.process_items.outputs }}", aggStep.Inputs["results"])
}

func TestParseInlineStateMachine(t *testing.T) {
	yamlContent := `
name: state_machine_recipe
version: "1.0"
steps:
  - id: review_process
    name: Document Review Process
    uses: state_machine
    config:
      initial_state: reviewing
      states:
        reviewing:
          uses: critique_activity
          inputs:
            document: "{{ .Inputs.document }}"
          transitions:
            - to: approved
              when: ".Outputs.score >= 80"
            - to: improving
              when: ".Outputs.score < 80"
        
        improving:
          uses: llm
          config:
            model: "gpt-4"
            system_prompt: "Improve this document for clarity."
          inputs:
            prompt: "Improve: {{ .State.document }}"
          transitions:
            - to: reviewing
              when: ".Outputs.improved == true"
        
        approved:
          terminal: true
          outputs:
            status: "approved"
            final_document: "{{ .State.document }}"
    inputs:
      document: "{{ .Inputs.document }}"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	require.Len(t, recipe.Steps, 1)
	step := recipe.Steps[0]
	
	assert.Equal(t, "review_process", step.ID)
	assert.Equal(t, "state_machine", step.Uses)
	
	// Check state machine configuration
	require.NotNil(t, step.Config)
	assert.Equal(t, "reviewing", step.Config["initial_state"])
	
	states, ok := step.Config["states"].(map[string]interface{})
	require.True(t, ok)
	require.Len(t, states, 3)
	
	// Check reviewing state
	reviewing, ok := states["reviewing"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "critique_activity", reviewing["uses"])
	
	transitions, ok := reviewing["transitions"].([]interface{})
	require.True(t, ok)
	require.Len(t, transitions, 2)
	
	// Check first transition
	transition1, ok := transitions[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "approved", transition1["to"])
	assert.Equal(t, ".Outputs.score >= 80", transition1["when"])
}