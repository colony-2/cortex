//go:build integration
// +build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/divisive-ai/vibethis/server/ono/pkg/activities"
	"github.com/divisive-ai/vibethis/server/ono/pkg/activities/llm"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestResearchProjectWithGemini(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Check if API key exists
	_, err := llm.LoadGeminiAPIKey()
	if err != nil {
		t.Skip("Gemini API key not found, skipping integration test")
	}

	// Parse the research project
	parser := yamlpkg.NewParser()
	projectPath := filepath.Join("..", "..", "example", "research_project")

	project, err := parser.ParseProject(projectPath)
	require.NoError(t, err)
	require.NotNil(t, project)

	// Modify the write_report_activity to use Gemini
	for i, activity := range project.Activities {
		if activity.Name == "write_report_activity" {
			// Update to use Gemini
			project.Activities[i].Implementation.Config["provider"] = "gemini"
			project.Activities[i].Implementation.Config["model"] = "gemini-2.0-flash-exp"
			break
		}
	}

	// Create activity registry and executor
	registry := compiler.NewActivityRegistry()
	executor := activities.NewExecutor()
	executor.SetUseMock(false) // Use real LLM

	// Register activities
	for _, activityDef := range project.Activities {
		registry.RegisterActivity(&activityDef)
	}

	// Compile workflow
	workflowCompiler := compiler.NewCompiler(registry)
	_, err = workflowCompiler.CompileWorkflow(project.Workflow)
	require.NoError(t, err)

	// Prepare inputs
	inputs := map[string]interface{}{
		"topic":       "Temporal Workflow Orchestration",
		"max_sources": 5,
	}

	// Execute the workflow (simplified - in real Temporal this would be more complex)
	t.Run("ExecuteResearchWorkflow", func(t *testing.T) {
		// For this test, we'll execute activities directly
		ctx := context.Background()

		// Step 1: Research (mock)
		researchActivity := findActivity(project.Activities, "research_activity")
		researchInputs := map[string]interface{}{
			"topic":       inputs["topic"],
			"max_sources": inputs["max_sources"],
		}
		researchOutputs, err := executor.ExecuteActivity(ctx, researchActivity, researchInputs)
		require.NoError(t, err)
		assert.NotNil(t, researchOutputs["research_results"])

		// Step 2: Analyze (mock)
		analyzeActivity := findActivity(project.Activities, "analyze_activity")
		analyzeInputs := map[string]interface{}{
			"data":  researchOutputs["research_results"],
			"topic": inputs["topic"],
		}
		analyzeOutputs, err := executor.ExecuteActivity(ctx, analyzeActivity, analyzeInputs)
		require.NoError(t, err)
		assert.NotNil(t, analyzeOutputs["analysis_results"])

		// Step 3: Write Report (using Gemini)
		writeActivity := findActivity(project.Activities, "write_report_activity")
		writeInputs := map[string]interface{}{
			"research": researchOutputs["research_results"],
			"analysis": analyzeOutputs["analysis_results"],
			"topic":    inputs["topic"],
		}
		writeOutputs, err := executor.ExecuteActivity(ctx, writeActivity, writeInputs)
		require.NoError(t, err)

		// Verify the report
		report, ok := writeOutputs["final_report"].(string)
		require.True(t, ok)
		assert.NotEmpty(t, report)
		assert.Contains(t, report, "Executive Summary")
		assert.Contains(t, report, "Key Findings")
		assert.Contains(t, report, "Temporal")

		t.Logf("Generated report preview:\n%.500s...", report)
	})
}

func TestGeminiWorkflowYAML(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Check if API key exists
	_, err := llm.LoadGeminiAPIKey()
	if err != nil {
		t.Skip("Gemini API key not found, skipping integration test")
	}

	// Parse the Gemini example workflow
	parser := yamlpkg.NewParser()
	workflowPath := filepath.Join("..", "..", "example", "gemini_workflow.yaml")

	project, err := parser.ParseProject(workflowPath)
	require.NoError(t, err)
	require.NotNil(t, project)

	// Create activity executor
	executor := activities.NewExecutor()
	executor.SetUseMock(false) // Use real LLM

	// Execute the single activity
	ctx := context.Background()
	activity := &project.Activities[0]
	inputs := map[string]interface{}{
		"topic": "Cloud Native Development",
	}

	outputs, err := executor.ExecuteActivity(ctx, activity, inputs)
	require.NoError(t, err)

	// Verify the report
	report, ok := outputs["final_report"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, report)
	assert.Contains(t, report, "Executive Summary")
	assert.Contains(t, report, "Cloud Native")

	t.Logf("Generated report length: %d characters", len(report))
}

func TestAllExampleWorkflows(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	examples := []struct {
		name string
		path string
	}{
		{"Research Project", filepath.Join("..", "..", "example", "research_project")},
		{"Simple Workflow", filepath.Join("..", "..", "example", "simple_workflow.yaml")},
		{"Gemini Workflow", filepath.Join("..", "..", "example", "gemini_workflow.yaml")},
	}

	parser := yamlpkg.NewParser()

	for _, example := range examples {
		t.Run(example.name, func(t *testing.T) {
			project, err := parser.ParseProject(example.path)
			require.NoError(t, err, "Failed to parse %s", example.name)
			require.NotNil(t, project)
			require.NotNil(t, project.Workflow, "No workflow found in %s", example.name)

			t.Logf("Successfully parsed %s: %s (v%s)",
				example.name,
				project.Workflow.Name,
				project.Workflow.Version)
			t.Logf("  Steps: %d", len(project.Workflow.Workflow.Steps))
			t.Logf("  Activities: %d", len(project.Activities))
		})
	}
}

func findActivity(activities []recipe.ActivityDefinition, name string) *recipe.ActivityDefinition {
	for i := range activities {
		if activities[i].Name == name {
			return &activities[i]
		}
	}
	return nil
}
