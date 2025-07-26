//go:build integration
// +build integration

package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestGeminiIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Load API key
	apiKey, err := LoadGeminiAPIKey()
	if err != nil {
		t.Skip("Gemini API key not found, skipping integration test")
	}

	// Create Gemini provider
	provider := NewGeminiProvider(apiKey, "gemini-2.0-flash-exp")

	// Test simple prompt
	ctx := context.Background()
	config := map[string]interface{}{
		"temperature": 0.7,
	}

	result, err := provider.ExecutePrompt(ctx, "Write a haiku about Temporal workflows", config)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result["result"])

	t.Logf("Gemini response: %v", result["result"])
}

func TestGeminiReportGeneration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Load API key
	apiKey, err := LoadGeminiAPIKey()
	if err != nil {
		t.Skip("Gemini API key not found, skipping integration test")
	}

	// Create LLM executor
	executor := NewExecutor()
	provider := NewGeminiProvider(apiKey, "gemini-2.0-flash-exp")
	executor.RegisterProvider("gemini", provider)

	// Create a report generation activity
	activityDef := &recipe.ActivityDefinition{
		Name:        "write_report_activity",
		Description: "Generate a comprehensive report",
		Timeout:     10 * time.Minute,
		Inputs: []yamlpkg.InputDefinition{
			{Name: "research", Type: "object"},
			{Name: "analysis", Type: "object"},
			{Name: "topic", Type: "string"},
		},
		Outputs: []yamlpkg.OutputDefinition{
			{Name: "final_report", Type: "string"},
		},
		Implementation: yamlpkg.ActivityImplementation{
			Type: "ai_prompt",
			Config: map[string]interface{}{
				"provider":    "gemini",
				"model":       "gemini-2.0-flash-exp",
				"temperature": 0.7,
				"prompt": `Generate a comprehensive report on "{{ .topic }}" based on the following:
          
Research Data:
{{ .research }}

Analysis:
{{ .analysis }}

Format the report with:
1. Executive Summary (2-3 sentences)
2. Key Findings (3-5 bullet points)
3. Conclusions (1-2 paragraphs)

Keep the report concise and professional.`,
			},
		},
	}

	// Prepare inputs
	inputs := map[string]interface{}{
		"topic": "Temporal Workflows",
		"research": map[string]interface{}{
			"sources": []string{
				"Temporal official documentation",
				"Workflow orchestration best practices",
			},
			"summary": "Temporal is a durable execution platform that enables developers to build resilient applications",
			"key_points": []string{
				"Provides durable execution guarantees",
				"Supports long-running workflows",
				"Handles failures automatically",
			},
		},
		"analysis": map[string]interface{}{
			"trends": []string{
				"Increasing adoption in microservices",
				"Growing ecosystem of integrations",
			},
			"insights": []string{
				"Simplifies complex distributed systems",
				"Reduces operational overhead",
			},
		},
	}

	// Execute the activity
	ctx := context.Background()
	result, err := executor.ExecuteAIPromptActivity(ctx, activityDef, inputs)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check the report
	report, ok := result["final_report"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, report)

	// Verify report contains expected sections
	assert.Contains(t, report, "Executive Summary")
	assert.Contains(t, report, "Key Findings")
	assert.Contains(t, report, "Conclusions")
	assert.Contains(t, report, "Temporal")

	t.Logf("Generated report:\n%s", report)
}

func TestGeminiStructuredOutput(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Load API key
	apiKey, err := LoadGeminiAPIKey()
	if err != nil {
		t.Skip("Gemini API key not found, skipping integration test")
	}

	// Create Gemini provider
	provider := NewGeminiProvider(apiKey, "gemini-2.0-flash-exp")

	// Test structured output
	ctx := context.Background()
	config := map[string]interface{}{
		"temperature": 0.1,
		"structured_output": map[string]interface{}{
			"schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type": "string",
					},
					"summary": map[string]interface{}{
						"type": "string",
					},
					"keywords": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"required": []string{"title", "summary", "keywords"},
			},
		},
	}

	prompt := `Create a JSON object with:
- title: "Introduction to Temporal Workflows"  
- summary: A one-sentence description of Temporal
- keywords: 3 relevant keywords about Temporal

Return only valid JSON.`

	result, err := provider.ExecutePrompt(ctx, prompt, config)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check structured fields
	assert.NotNil(t, result["title"])
	assert.NotNil(t, result["summary"])
	assert.NotNil(t, result["keywords"])

	t.Logf("Structured output: %+v", result)
}