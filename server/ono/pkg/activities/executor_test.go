package activities

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestMockResearchActivity(t *testing.T) {
	handler := NewMockActivityHandler()
	
	inputs := map[string]interface{}{
		"topic":       "Temporal Workflows",
		"max_sources": 5,
	}
	
	outputs, err := handler.ResearchActivity(context.Background(), inputs)
	require.NoError(t, err)
	
	// Check outputs structure
	researchResults, ok := outputs["research_results"].(map[string]interface{})
	require.True(t, ok)
	
	// Check sources
	sources, ok := researchResults["sources"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, sources, 2)
	
	// Check summary
	summary, ok := researchResults["summary"].(string)
	require.True(t, ok)
	assert.Contains(t, summary, "Temporal Workflows")
	
	// Check key points
	keyPoints, ok := researchResults["key_points"].([]string)
	require.True(t, ok)
	assert.Len(t, keyPoints, 3)
}

func TestMockAnalyzeActivity(t *testing.T) {
	handler := NewMockActivityHandler()
	
	inputs := map[string]interface{}{
		"data": map[string]interface{}{
			"sources": []string{"source1", "source2"},
		},
		"topic": "AI Agents",
	}
	
	outputs, err := handler.AnalyzeActivity(context.Background(), inputs)
	require.NoError(t, err)
	
	analysisResults, ok := outputs["analysis_results"].(map[string]interface{})
	require.True(t, ok)
	
	// Check trends
	trends, ok := analysisResults["trends"].([]string)
	require.True(t, ok)
	assert.Len(t, trends, 3)
	
	// Check insights
	insights, ok := analysisResults["insights"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, insights, 1)
}

func TestMockWriteReportActivity(t *testing.T) {
	handler := NewMockActivityHandler()
	
	inputs := map[string]interface{}{
		"research": map[string]interface{}{
			"sources": []string{"source1", "source2"},
		},
		"analysis": map[string]interface{}{
			"trends": []string{"trend1", "trend2"},
		},
		"topic": "Blockchain Technology",
	}
	
	outputs, err := handler.WriteReportActivity(context.Background(), inputs)
	require.NoError(t, err)
	
	report, ok := outputs["final_report"].(string)
	require.True(t, ok)
	assert.Contains(t, report, "Blockchain Technology Report")
	assert.Contains(t, report, "Executive Summary")
	assert.Contains(t, report, "Key Findings")
	assert.Contains(t, report, "References")
}

func TestExecutor(t *testing.T) {
	executor := NewExecutor()
	
	// Test HTTP activity
	httpActivity := &recipe.ActivityDefinition{
		Name:    "research_activity",
		Timeout: 5 * time.Minute,
		Implementation: yamlpkg.ActivityImplementation{
			Type: "http",
			Config: map[string]interface{}{
				"method": "POST",
				"url":    "http://example.com/research",
			},
		},
	}
	
	inputs := map[string]interface{}{
		"topic": "Cloud Computing",
	}
	
	outputs, err := executor.ExecuteActivity(context.Background(), httpActivity, inputs)
	require.NoError(t, err)
	assert.NotNil(t, outputs["research_results"])
	
	// Test AI prompt activity
	aiActivity := &recipe.ActivityDefinition{
		Name:    "write_report_activity",
		Timeout: 10 * time.Minute,
		Implementation: yamlpkg.ActivityImplementation{
			Type: "ai_prompt",
			Config: map[string]interface{}{
				"model":  "gpt-4",
				"prompt": "Generate a report",
			},
		},
	}
	
	aiInputs := map[string]interface{}{
		"research": map[string]interface{}{"data": "test"},
		"analysis": map[string]interface{}{"results": "test"},
		"topic":    "AI Ethics",
	}
	
	outputs, err = executor.ExecuteActivity(context.Background(), aiActivity, aiInputs)
	require.NoError(t, err)
	assert.NotNil(t, outputs["final_report"])
}

func TestRegisterActivities(t *testing.T) {
	executor := NewExecutor()
	
	activityDefs := []recipe.ActivityDefinition{
		{
			Name:    "test_activity_1",
			Timeout: time.Minute,
			Implementation: yamlpkg.ActivityImplementation{
				Type: "function",
			},
		},
		{
			Name:    "test_activity_2",
			Timeout: time.Minute,
			Implementation: yamlpkg.ActivityImplementation{
				Type: "http",
			},
		},
	}
	
	activities := executor.RegisterActivities(activityDefs)
	assert.Len(t, activities, 2)
	assert.Contains(t, activities, "test_activity_1")
	assert.Contains(t, activities, "test_activity_2")
}