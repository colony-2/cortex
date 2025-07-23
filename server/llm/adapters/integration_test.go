//go:build integration
// +build integration

package llmadapters

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration for each adapter
var (
	// Use smaller, faster models for integration tests
	testModels = map[string]string{
		"openai":    "o4-mini-2025-04-16",
		"anthropic": "claude-3-5-haiku-20241022",
		"gemini":    "gemini-2.5-flash-lite",
		"bedrock":   "amazon.titan-text-lite-v1",
	}

	// Simple test prompt
	testPrompt = "What is 2+2? Reply with just the number."

	// Test prompt for structured output
	structuredPrompt = "Generate a person with name John Doe, age 30, and occupation Software Engineer."

	// Test prompt for tool calling
	toolPrompt = "What's the weather in San Francisco?"
)

// Helper function to skip test if API key is not set
func failNoKey(t *testing.T, envVar string) {
	if os.Getenv(envVar) == "" {
		t.Errorf("%s not set", envVar)
	}
}

// Helper function to create test context with timeout
func testContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// TestOpenAIIntegration tests the OpenAI adapter with real API calls
func TestOpenAIIntegration(t *testing.T) {
	failNoKey(t, "OPENAI_API_KEY")

	adapter, err := NewOpenAIAdapter("")
	require.NoError(t, err)
	require.NotNil(t, adapter)

	t.Run("Generate", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:       testModels["openai"],
			Temperature: 0.0,
			MaxTokens:   50,
		}

		response, err := adapter.Generate(ctx, testPrompt, config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, response.Content, "4")
		assert.Greater(t, response.Usage.TotalTokens, 0)
		assert.NotEmpty(t, response.Model)
		assert.NotEmpty(t, response.FinishReason)
	})

	t.Run("GenerateWithSystemPrompt", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:        testModels["openai"],
			Temperature:  0.0,
			MaxTokens:    50,
			SystemPrompt: "You are a helpful math tutor. Always explain your reasoning.",
		}

		response, err := adapter.Generate(ctx, "What is 10 divided by 2?", config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, strings.ToLower(response.Content), "5")
	})

	t.Run("GenerateJSON", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:          testModels["openai"],
			Temperature:    0.0,
			MaxTokens:      100,
			ResponseFormat: "json",
			SystemPrompt:   "Always respond with valid JSON containing 'answer' field.",
		}

		response, err := adapter.Generate(ctx, "What is 2+2?", config)
		require.NoError(t, err)

		// Verify it's valid JSON
		var result map[string]interface{}
		err = json.Unmarshal([]byte(response.Content), &result)
		assert.NoError(t, err)
		assert.NotNil(t, result["answer"])
	})

	t.Run("GenerateWithTools", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		tools := []Tool{
			{
				Name:        "get_weather",
				Description: "Get the current weather for a location",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						},
						"unit": {
							"type": "string",
							"enum": ["celsius", "fahrenheit"],
							"description": "Temperature unit"
						}
					},
					"required": ["location"]
				}`),
			},
		}

		config := Config{
			Model:       testModels["openai"],
			Temperature: 0.0,
			MaxTokens:   100,
		}

		response, err := adapter.GenerateWithTools(ctx, toolPrompt, tools, config)
		require.NoError(t, err)

		// Should either return a tool call or explain it would need to call the tool
		if len(response.ToolCalls) > 0 {
			assert.Equal(t, "get_weather", response.ToolCalls[0].Name)
			assert.NotEmpty(t, response.ToolCalls[0].Arguments)

			// Verify arguments are valid JSON
			var args map[string]interface{}
			err = json.Unmarshal(response.ToolCalls[0].Arguments, &args)
			assert.NoError(t, err)
			assert.NotEmpty(t, args["location"])
		} else {
			assert.NotEmpty(t, response.Content)
		}
	})

	t.Run("StreamGenerate", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:       testModels["openai"],
			Temperature: 0.0,
			MaxTokens:   50,
		}

		stream, err := adapter.StreamGenerate(ctx, "Count from 1 to 5", config)
		require.NoError(t, err)
		require.NotNil(t, stream)

		var fullContent strings.Builder
		tokenCount := 0

		for token := range stream {
			if token.Error != nil {
				t.Fatalf("Stream error: %v", token.Error)
			}
			fullContent.WriteString(token.Content)
			tokenCount++
		}

		assert.Greater(t, tokenCount, 0)
		assert.NotEmpty(t, fullContent.String())
		// Should contain numbers 1-5
		content := fullContent.String()
		assert.Contains(t, content, "1")
		assert.Contains(t, content, "2")
		assert.Contains(t, content, "3")
		assert.Contains(t, content, "4")
		assert.Contains(t, content, "5")
	})
}

// TestAnthropicIntegration tests the Anthropic adapter with real API calls
func TestAnthropicIntegration(t *testing.T) {
	failNoKey(t, "ANTHROPIC_API_KEY")

	adapter, err := NewAnthropicAdapter("")
	require.NoError(t, err)
	require.NotNil(t, adapter)

	t.Run("Generate", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:       testModels["anthropic"],
			Temperature: 0.0,
			MaxTokens:   50,
		}

		response, err := adapter.Generate(ctx, testPrompt, config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, response.Content, "4")
		assert.Greater(t, response.Usage.TotalTokens, 0)
		assert.NotEmpty(t, response.Model)
		assert.NotEmpty(t, response.FinishReason)
	})

	t.Run("GenerateWithSystemPrompt", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:        testModels["anthropic"],
			Temperature:  0.0,
			MaxTokens:    100,
			SystemPrompt: "You are a helpful assistant. Be concise.",
		}

		response, err := adapter.Generate(ctx, "What is the capital of France?", config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, strings.ToLower(response.Content), "paris")
	})

	t.Run("GenerateWithTools", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		tools := []Tool{
			{
				Name:        "calculator",
				Description: "Perform basic math operations",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"operation": {
							"type": "string",
							"enum": ["add", "subtract", "multiply", "divide"],
							"description": "The math operation to perform"
						},
						"a": {
							"type": "number",
							"description": "First number"
						},
						"b": {
							"type": "number",
							"description": "Second number"
						}
					},
					"required": ["operation", "a", "b"]
				}`),
			},
		}

		config := Config{
			Model:       testModels["anthropic"],
			Temperature: 0.0,
			MaxTokens:   100,
		}

		response, err := adapter.GenerateWithTools(ctx, "What is 15 multiplied by 7?", tools, config)
		require.NoError(t, err)

		// Claude should recognize it needs to use the calculator tool
		if len(response.ToolCalls) > 0 {
			assert.Equal(t, "calculator", response.ToolCalls[0].Name)

			var args map[string]interface{}
			err = json.Unmarshal(response.ToolCalls[0].Arguments, &args)
			assert.NoError(t, err)
			assert.Equal(t, "multiply", args["operation"])
			assert.Equal(t, float64(15), args["a"])
			assert.Equal(t, float64(7), args["b"])
		}
	})

	t.Run("StreamGenerate", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:       testModels["anthropic"],
			Temperature: 0.0,
			MaxTokens:   50,
		}

		stream, err := adapter.StreamGenerate(ctx, "Say 'Hello, World!'", config)
		require.NoError(t, err)
		require.NotNil(t, stream)

		var fullContent strings.Builder
		tokenCount := 0

		for token := range stream {
			if token.Error != nil {
				t.Fatalf("Stream error: %v", token.Error)
			}
			fullContent.WriteString(token.Content)
			tokenCount++
		}

		assert.Greater(t, tokenCount, 0)
		content := fullContent.String()
		assert.NotEmpty(t, content)
		assert.Contains(t, strings.ToLower(content), "hello")
		assert.Contains(t, strings.ToLower(content), "world")
	})
}

// TestGeminiIntegration tests the Gemini adapter with real API calls
func TestGeminiIntegration(t *testing.T) {
	failNoKey(t, "GEMINI_API_KEY")

	adapter, err := NewGeminiAdapter("")
	require.NoError(t, err)
	require.NotNil(t, adapter)

	t.Run("Generate", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:       testModels["gemini"],
			Temperature: 0.0,
			MaxTokens:   50,
		}

		response, err := adapter.Generate(ctx, testPrompt, config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, response.Content, "4")
		assert.Greater(t, response.Usage.TotalTokens, 0)
		assert.NotEmpty(t, response.Model)
		assert.NotEmpty(t, response.FinishReason)
	})

	t.Run("GenerateWithSystemPrompt", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:        testModels["gemini"],
			Temperature:  0.0,
			MaxTokens:    100,
			SystemPrompt: "You are a geography expert. Provide brief, accurate answers.",
		}

		response, err := adapter.Generate(ctx, "What is the largest ocean?", config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, strings.ToLower(response.Content), "pacific")
	})

	t.Run("GenerateStructuredOutput", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		schema := json.RawMessage(`{
			"type": "object",
			"properties": {
				"name": {"type": "string"},
				"age": {"type": "integer"},
				"occupation": {"type": "string"}
			},
			"required": ["name", "age", "occupation"]
		}`)

		config := Config{
			Model:          testModels["gemini"],
			Temperature:    0.0,
			MaxTokens:      100,
			ResponseFormat: "json",
			ResponseSchema: schema,
		}

		response, err := adapter.Generate(ctx, structuredPrompt, config)
		require.NoError(t, err)

		// Verify structured output
		var result map[string]interface{}
		err = json.Unmarshal([]byte(response.Content), &result)
		require.NoError(t, err)
		assert.Equal(t, "John Doe", result["name"])
		assert.Equal(t, float64(30), result["age"])
		assert.Equal(t, "Software Engineer", result["occupation"])
	})

	t.Run("GenerateWithTools", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		tools := []Tool{
			{
				Name:        "get_time",
				Description: "Get the current time in a specific timezone",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"timezone": {
							"type": "string",
							"description": "The timezone, e.g. America/New_York"
						}
					},
					"required": ["timezone"]
				}`),
			},
		}

		config := Config{
			Model:       testModels["gemini"],
			Temperature: 0.0,
			MaxTokens:   100,
		}

		response, err := adapter.GenerateWithTools(ctx, "What time is it in Tokyo?", tools, config)
		require.NoError(t, err)

		// Gemini should recognize it needs the time tool
		if len(response.ToolCalls) > 0 {
			assert.Equal(t, "get_time", response.ToolCalls[0].Name)

			var args map[string]interface{}
			err = json.Unmarshal(response.ToolCalls[0].Arguments, &args)
			assert.NoError(t, err)
			assert.Contains(t, args["timezone"], "Tokyo")
		} else {
			// Or it might explain it would need to use the tool
			assert.NotEmpty(t, response.Content)
		}
	})

	t.Run("StreamGenerate", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:       testModels["gemini"],
			Temperature: 0.0,
			MaxTokens:   50,
		}

		stream, err := adapter.StreamGenerate(ctx, "List three colors", config)
		require.NoError(t, err)
		require.NotNil(t, stream)

		var fullContent strings.Builder
		tokenCount := 0

		for token := range stream {
			if token.Error != nil {
				t.Fatalf("Stream error: %v", token.Error)
			}
			fullContent.WriteString(token.Content)
			tokenCount++
		}

		assert.Greater(t, tokenCount, 0)
		assert.NotEmpty(t, fullContent.String())
		// Should contain some color names
		content := strings.ToLower(fullContent.String())
		colorFound := strings.Contains(content, "red") ||
			strings.Contains(content, "blue") ||
			strings.Contains(content, "green") ||
			strings.Contains(content, "yellow") ||
			strings.Contains(content, "black") ||
			strings.Contains(content, "white")
		assert.True(t, colorFound, "Response should contain at least one color")
	})
}

// TestBedrockIntegration tests the Bedrock adapter with real API calls
func TestBedrockIntegration(t *testing.T) {
	// Bedrock requires AWS credentials, not just an API key
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		t.Error("AWS credentials not configured")
	}

	adapter, err := NewBedrockAdapter("")
	require.NoError(t, err)
	require.NotNil(t, adapter)

	t.Run("Generate", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:       testModels["bedrock"],
			Temperature: 0.0,
			MaxTokens:   50,
		}

		response, err := adapter.Generate(ctx, testPrompt, config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, response.Content, "4")
		assert.Greater(t, response.Usage.TotalTokens, 0)
		assert.NotEmpty(t, response.Model)
		assert.NotEmpty(t, response.FinishReason)
	})

	t.Run("GenerateWithSystemPrompt", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:        testModels["bedrock"],
			Temperature:  0.0,
			MaxTokens:    100,
			SystemPrompt: "You are a helpful assistant. Answer in one sentence.",
		}

		response, err := adapter.Generate(ctx, "What is photosynthesis?", config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		// Should be a brief explanation
		assert.Less(t, len(strings.Split(response.Content, ".")), 3)
	})
}

// TestRegistryIntegration tests multiple adapters through the registry
func TestRegistryIntegration(t *testing.T) {
	registry := NewRegistry()

	// Register available adapters
	availableAdapters := 0

	if os.Getenv("OPENAI_API_KEY") != "" {
		adapter, err := NewOpenAIAdapter("")
		if err == nil {
			registry.Register("openai", adapter)
			availableAdapters++
		}
	}

	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		adapter, err := NewAnthropicAdapter("")
		if err == nil {
			registry.Register("anthropic", adapter)
			availableAdapters++
		}
	}

	if os.Getenv("GEMINI_API_KEY") != "" {
		adapter, err := NewGeminiAdapter("")
		if err == nil {
			registry.Register("gemini", adapter)
			availableAdapters++
		}
	}

	t.Run("CrossAdapterConsistency", func(t *testing.T) {
		ctx, cancel := testContext()
		defer cancel()

		prompt := "What is the capital of Japan? Reply with just the city name."

		responses := make(map[string]string)

		for _, name := range registry.List() {
			adapter, err := registry.Get(name)
			require.NoError(t, err)

			config := Config{
				Model:       testModels[name],
				Temperature: 0.0,
				MaxTokens:   20,
			}

			response, err := adapter.Generate(ctx, prompt, config)
			if err != nil {
				t.Logf("Adapter %s failed: %v", name, err)
				continue
			}

			responses[name] = strings.TrimSpace(response.Content)
		}

		// All adapters should mention Tokyo
		for name, response := range responses {
			assert.Contains(t, strings.ToLower(response), "tokyo",
				"Adapter %s response doesn't contain 'tokyo': %s", name, response)
		}
	})
}

// TestErrorHandlingIntegration tests error handling with real API calls
func TestErrorHandlingIntegration(t *testing.T) {
	t.Run("InvalidModel", func(t *testing.T) {
		failNoKey(t, "OPENAI_API_KEY")

		adapter, err := NewOpenAIAdapter("")
		require.NoError(t, err)

		ctx, cancel := testContext()
		defer cancel()

		config := Config{
			Model:     "invalid-model-name-xyz",
			MaxTokens: 50,
		}

		_, err = adapter.Generate(ctx, "Hello", config)
		assert.Error(t, err)
		// Should be a model not supported error
		assert.Contains(t, err.Error(), "model")
	})

	t.Run("ContextTimeout", func(t *testing.T) {
		failNoKey(t, "GEMINI_API_KEY")

		adapter, err := NewGeminiAdapter("")
		require.NoError(t, err)

		// Create a context that times out immediately
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Wait for context to expire
		time.Sleep(2 * time.Millisecond)

		config := Config{
			Model:     testModels["gemini"],
			MaxTokens: 50,
		}

		_, err = adapter.Generate(ctx, "Hello", config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context")
	})
}
