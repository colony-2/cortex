package yaml

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResearchProjectExample(t *testing.T) {
	// Parse the example research project
	parser := NewParser()
	projectPath := filepath.Join("..", "..", "example", "research_project")
	
	project, err := parser.ParseProject(projectPath)
	require.NoError(t, err)
	require.NotNil(t, project)
	
	// Verify project manifest
	assert.NotNil(t, project.Manifest)
	assert.Equal(t, "research_report_project", project.Manifest.Name)
	assert.Equal(t, "1.0", project.Manifest.Version)
	
	// Verify workflow
	assert.NotNil(t, project.Workflow)
	assert.Equal(t, "research_report_workflow", project.Workflow.Name)
	assert.Equal(t, "Research a topic and generate a comprehensive report", project.Workflow.Description)
	
	// Check inputs
	assert.Len(t, project.Workflow.Inputs, 2)
	assert.Equal(t, "topic", project.Workflow.Inputs[0].Name)
	assert.True(t, project.Workflow.Inputs[0].Required)
	assert.Equal(t, "max_sources", project.Workflow.Inputs[1].Name)
	assert.Equal(t, 10, project.Workflow.Inputs[1].Default)
	
	// Check workflow steps
	assert.Equal(t, "sequential", project.Workflow.Workflow.Type)
	assert.Len(t, project.Workflow.Workflow.Steps, 3)
	
	// Verify step sequence
	assert.Equal(t, "research", project.Workflow.Workflow.Steps[0].ID)
	assert.Equal(t, "research_activity", project.Workflow.Workflow.Steps[0].Activity)
	
	assert.Equal(t, "analyze", project.Workflow.Workflow.Steps[1].ID)
	assert.Equal(t, "analyze_activity", project.Workflow.Workflow.Steps[1].Activity)
	
	assert.Equal(t, "write_report", project.Workflow.Workflow.Steps[2].ID)
	assert.Equal(t, "write_report_activity", project.Workflow.Workflow.Steps[2].Activity)
	
	// Verify activities
	assert.Len(t, project.Activities, 3)
	
	// Find research activity
	var researchActivity *ActivityDefinition
	for _, act := range project.Activities {
		if act.Name == "research_activity" {
			researchActivity = &act
			break
		}
	}
	require.NotNil(t, researchActivity)
	assert.Equal(t, "http", researchActivity.Implementation.Type)
	
	// Find AI prompt activity
	var aiActivity *ActivityDefinition
	for _, act := range project.Activities {
		if act.Name == "write_report_activity" {
			aiActivity = &act
			break
		}
	}
	require.NotNil(t, aiActivity)
	assert.Equal(t, "ai_prompt", aiActivity.Implementation.Type)
	assert.Equal(t, "gpt-4", aiActivity.Implementation.Config["model"])
	
	// Verify agents
	assert.Len(t, project.Agents, 3)
	assert.Contains(t, project.Agents, "researcher")
	assert.Contains(t, project.Agents, "analyst")
	assert.Contains(t, project.Agents, "writer")
	
	// Check researcher agent
	researcher := project.Agents["researcher"]
	assert.Equal(t, "Senior Research Analyst", researcher.Role)
	assert.Contains(t, researcher.Capabilities, "web_search")
	assert.Contains(t, researcher.Goals, "Find comprehensive, accurate information")
}

func TestParseSimpleWorkflowExample(t *testing.T) {
	// Parse the single-file example
	parser := NewParser()
	filePath := filepath.Join("..", "..", "example", "simple_workflow.yaml")
	
	project, err := parser.ParseProject(filePath)
	require.NoError(t, err)
	require.NotNil(t, project)
	
	// Verify workflow
	assert.NotNil(t, project.Workflow)
	assert.Equal(t, "simple_research", project.Workflow.Name)
	
	// Check workflow structure
	assert.Len(t, project.Workflow.Inputs, 1)
	assert.Equal(t, "query", project.Workflow.Inputs[0].Name)
	
	assert.Len(t, project.Workflow.Workflow.Steps, 2)
	assert.Equal(t, "search", project.Workflow.Workflow.Steps[0].ID)
	assert.Equal(t, "summarize", project.Workflow.Workflow.Steps[1].ID)
	
	// Verify activities
	assert.Len(t, project.Activities, 2)
	
	// Check activity names
	activityNames := make([]string, len(project.Activities))
	for i, act := range project.Activities {
		activityNames[i] = act.Name
	}
	assert.Contains(t, activityNames, "quick_search")
	assert.Contains(t, activityNames, "summarize_results")
}