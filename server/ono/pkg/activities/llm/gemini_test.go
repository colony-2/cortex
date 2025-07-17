package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
)

func TestLoadGeminiAPIKey(t *testing.T) {
	// This test will pass if the key file exists, skip otherwise
	apiKey, err := LoadGeminiAPIKey()
	if err != nil {
		t.Skipf("Skipping test - Gemini API key not found: %v", err)
	}
	
	assert.NotEmpty(t, apiKey)
	assert.NotContains(t, apiKey, "\n", "API key should not contain newlines")
}

func TestGeminiProviderCreation(t *testing.T) {
	provider := NewGeminiProvider("test-key", "gemini-2.0-flash-exp")
	assert.NotNil(t, provider)
	assert.Equal(t, "test-key", provider.apiKey)
	assert.Equal(t, "gemini-2.0-flash-exp", provider.model)
}

func TestLLMExecutorCreation(t *testing.T) {
	executor := NewExecutor()
	assert.NotNil(t, executor)
	assert.NotNil(t, executor.providers)
}

func TestRenderPrompt(t *testing.T) {
	tests := []struct {
		name     string
		template string
		inputs   map[string]interface{}
		expected string
	}{
		{
			name:     "simple substitution",
			template: "Generate a report about {{ .topic }}",
			inputs:   map[string]interface{}{"topic": "AI"},
			expected: "Generate a report about AI",
		},
		{
			name:     "multiple substitutions",
			template: "Research {{ .topic }} with {{ .depth }} analysis",
			inputs: map[string]interface{}{
				"topic": "Machine Learning",
				"depth": "comprehensive",
			},
			expected: "Research Machine Learning with comprehensive analysis",
		},
		{
			name:     "no substitutions",
			template: "Static prompt text",
			inputs:   map[string]interface{}{},
			expected: "Static prompt text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderPrompt(tt.template, tt.inputs)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteAIPromptActivityValidation(t *testing.T) {
	executor := NewExecutor()
	
	// Register a mock provider to avoid "unsupported provider" error
	executor.RegisterProvider("openai", &mockProvider{})
	
	// Test missing prompt
	activityDef := &recipe.ActivityDefinition{
		Name: "test_activity",
		Implementation: yamlpkg.ActivityImplementation{
			Type:   "ai_prompt",
			Config: map[string]interface{}{
				// missing prompt
			},
		},
	}
	
	_, err := executor.ExecuteAIPromptActivity(context.Background(), activityDef, map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}

// mockProvider is a test provider
type mockProvider struct{}

func (m *mockProvider) ExecutePrompt(ctx context.Context, prompt string, config map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"result": "mock result"}, nil
}