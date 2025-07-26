//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/divisive-ai/vibethis/server/llm/adapters"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration for each adapter
var testModels = map[string]string{
	"openai":    "gpt-4o-mini",
	"anthropic": "claude-3-5-haiku-20241022",
	"gemini":    "gemini-2.5-flash-lite",
}

// adapterConfig holds the configuration for each adapter type
type adapterConfig struct {
	name          string
	envVar        string
	createAdapter func() (llmadapters.Adapter, error)
}

// Define all adapter configurations
var adapters = []adapterConfig{
	{
		name:   "openai",
		envVar: "OPENAI_API_KEY",
		createAdapter: func() (llmadapters.Adapter, error) {
			return llmadapters.NewOpenAIAdapter("")
		},
	},
	{
		name:   "anthropic",
		envVar: "ANTHROPIC_API_KEY",
		createAdapter: func() (llmadapters.Adapter, error) {
			return llmadapters.NewAnthropicAdapter("")
		},
	},
	{
		name:   "gemini",
		envVar: "GEMINI_API_KEY",
		createAdapter: func() (llmadapters.Adapter, error) {
			return llmadapters.NewGeminiAdapter("")
		},
	},
}

// Helper function to create test context with timeout
func testContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// runAdapterTest is a helper to run a test for a specific adapter
func runAdapterTest(t *testing.T, cfg adapterConfig, testFunc func(t *testing.T, adapter llmadapters.Adapter, model string)) {
	// Fail if required environment variable is not set
	if os.Getenv(cfg.envVar) == "" {
		// print environment variables for debugging
		t.Logf("Environment variables: %v", os.Environ())
		t.Fatalf("Missing key: %s", cfg.envVar)
	}

	adapter, err := cfg.createAdapter()
	if err != nil {
		t.Errorf("Failed to create adapter: %v", err)
	}
	require.NoError(t, err)
	require.NotNil(t, adapter)

	model := testModels[cfg.name]
	testFunc(t, adapter, model)
}

// TestAdapterGenerate tests basic generation for all adapters
func TestAdapterGenerate(t *testing.T) {
	testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
		ctx, cancel := testContext()
		defer cancel()

		config := llmadapters.Config{
			Model:       model,
			Temperature: 0.0,
			MaxTokens:   50,
		}

		response, err := adapter.Generate(ctx, "What is 2+2? Reply with just the number.", config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)
		assert.Contains(t, response.Content, "4")
		assert.Greater(t, response.Usage.TotalTokens, 0)
		assert.NotEmpty(t, response.Model)
		assert.NotEmpty(t, response.FinishReason)
	}

	for _, cfg := range adapters {
		t.Run(cfg.name, func(t *testing.T) {
			runAdapterTest(t, cfg, testFunc)
		})
	}
}

// TestAdapterSystemPrompt tests system prompt functionality
func TestAdapterSystemPrompt(t *testing.T) {
	testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
		ctx, cancel := testContext()
		defer cancel()

		config := llmadapters.Config{
			Model:        model,
			Temperature:  0.0,
			MaxTokens:    100,
			SystemPrompt: "You are a helpful assistant. Always respond in exactly 3 words.",
		}

		response, err := adapter.Generate(ctx, "Tell me about the sky", config)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Content)

		// Count words (rough check - should be approximately 3)
		words := strings.Fields(strings.TrimSpace(response.Content))
		assert.LessOrEqual(t, len(words), 5, "Response should be very brief (around 3 words)")
	}

	for _, cfg := range adapters {
		t.Run(cfg.name, func(t *testing.T) {
			runAdapterTest(t, cfg, testFunc)
		})
	}
}

// TestAdapterJSONResponse tests JSON response format
func TestAdapterJSONResponse(t *testing.T) {
	testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
		ctx, cancel := testContext()
		defer cancel()

		config := llmadapters.Config{
			Model:          model,
			Temperature:    0.0,
			MaxTokens:      100,
			ResponseFormat: "json",
			SystemPrompt:   "Always respond with valid JSON containing an 'answer' field with the numeric result.",
		}

		response, err := adapter.Generate(ctx, "What is 10 plus 5?", config)
		require.NoError(t, err)

		// Verify it's valid JSON
		var result map[string]interface{}
		err = json.Unmarshal([]byte(response.Content), &result)
		assert.NoError(t, err, "Response should be valid JSON")
		assert.NotNil(t, result["answer"], "JSON should contain 'answer' field")
	}

	for _, cfg := range adapters {
		t.Run(cfg.name, func(t *testing.T) {
			runAdapterTest(t, cfg, testFunc)
		})
	}
}

// TestAdapterStructuredOutput tests structured output (mainly for Gemini)
func TestAdapterStructuredOutput(t *testing.T) {
	testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
		// All adapters now support structured output via different mechanisms
		// OpenAI and Anthropic use native structured output, Gemini uses response_schema

		ctx, cancel := testContext()
		defer cancel()

		schema := json.RawMessage(`{
			"type": "object",
			"properties": {
				"name": {"type": "string"},
				"age": {"type": "integer"},
				"occupation": {"type": "string"}
			},
			"required": ["name", "age", "occupation"],
			"additionalProperties": false
		}`)

		config := llmadapters.Config{
			Model:          model,
			Temperature:    0.0,
			MaxTokens:      100,
			ResponseFormat: "json",
			ResponseSchema: schema,
		}

		response, err := adapter.Generate(ctx,
			"Generate a person with name John Doe, age 30, and occupation Software Engineer.",
			config)
		require.NoError(t, err)

		// Log the response for debugging
		t.Logf("Structured output response for %s: %s", model, response.Content)

		// Verify structured output
		var result map[string]interface{}
		err = json.Unmarshal([]byte(response.Content), &result)
		require.NoError(t, err)
		assert.Equal(t, "John Doe", result["name"])
		assert.Equal(t, float64(30), result["age"])
		assert.Equal(t, "Software Engineer", result["occupation"])
	}

	for _, cfg := range adapters {
		t.Run(cfg.name, func(t *testing.T) {
			runAdapterTest(t, cfg, testFunc)
		})
	}
}

// TestAdapterToolCalling tests tool/function calling
func TestAdapterToolCalling(t *testing.T) {
	testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
		ctx, cancel := testContext()
		defer cancel()

		tools := []llmadapters.Tool{
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

		config := llmadapters.Config{
			Model:       model,
			Temperature: 0.0,
			MaxTokens:   100,
		}

		response, err := adapter.GenerateWithTools(ctx,
			"What's the weather in San Francisco?",
			tools,
			config)
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
			assert.Contains(t, strings.ToLower(args["location"].(string)), "san francisco")
		} else {
			// Some models might explain instead of calling the tool
			assert.NotEmpty(t, response.Content)
			assert.Contains(t, strings.ToLower(response.Content), "weather")
		}
	}

	for _, cfg := range adapters {
		t.Run(cfg.name, func(t *testing.T) {
			runAdapterTest(t, cfg, testFunc)
		})
	}
}

// TestAdapterStreaming tests streaming functionality
func TestAdapterStreaming(t *testing.T) {
	testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
		ctx, cancel := testContext()
		defer cancel()

		config := llmadapters.Config{
			Model:       model,
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

		assert.Greater(t, tokenCount, 0, "Should receive at least one token")
		content := fullContent.String()
		assert.NotEmpty(t, content)

		// Should contain numbers 1-5
		numbersFound := 0
		for i := 1; i <= 5; i++ {
			if strings.Contains(content, fmt.Sprintf("%d", i)) {
				numbersFound++
			}
		}
		assert.GreaterOrEqual(t, numbersFound, 3, "Should contain at least 3 of the numbers 1-5")
	}

	for _, cfg := range adapters {
		t.Run(cfg.name, func(t *testing.T) {
			runAdapterTest(t, cfg, testFunc)
		})
	}
}

// TestAdapterErrorHandling tests error scenarios
func TestAdapterErrorHandling(t *testing.T) {
	t.Run("InvalidModel", func(t *testing.T) {
		testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
			ctx, cancel := testContext()
			defer cancel()

			config := llmadapters.Config{
				Model:     "invalid-model-xyz-123",
				MaxTokens: 50,
			}

			_, err := adapter.Generate(ctx, "Hello", config)
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "model")
		}

		for _, cfg := range adapters {
			t.Run(cfg.name, func(t *testing.T) {
				runAdapterTest(t, cfg, testFunc)
			})
		}
	})

	t.Run("ContextTimeout", func(t *testing.T) {
		testFunc := func(t *testing.T, adapter llmadapters.Adapter, model string) {
			// Create a context that times out immediately
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			// Wait for context to expire
			time.Sleep(2 * time.Millisecond)

			config := llmadapters.Config{
				Model:     model,
				MaxTokens: 50,
			}

			_, err := adapter.Generate(ctx, "Hello", config)
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "context")
		}

		// Only test with one adapter to avoid rate limiting
		if os.Getenv("OPENAI_API_KEY") != "" {
			adapter, err := llmadapters.NewOpenAIAdapter("")
			require.NoError(t, err)
			testFunc(t, adapter, testModels["openai"])
		} else if os.Getenv("GEMINI_API_KEY") != "" {
			adapter, err := llmadapters.NewGeminiAdapter("")
			require.NoError(t, err)
			testFunc(t, adapter, testModels["gemini"])
		}
	})
}

// TestCrossAdapterConsistency tests that all adapters give similar results
func TestCrossAdapterConsistency(t *testing.T) {
	ctx, cancel := testContext()
	defer cancel()

	prompt := "What is the capital of France? Reply with just the city name."
	responses := make(map[string]string)

	for _, cfg := range adapters {
		t.Run(cfg.name, func(t *testing.T) {
			if os.Getenv(cfg.envVar) == "" {
				t.Errorf("Missing Api Key: %s", cfg.envVar)
				return
			}

			adapter, err := cfg.createAdapter()
			if err != nil {
				t.Errorf("Failed to create adapter: %v", err)
				return
			}

			config := llmadapters.Config{
				Model:       testModels[cfg.name],
				Temperature: 0.0,
				MaxTokens:   20,
			}

			response, err := adapter.Generate(ctx, prompt, config)
			if err != nil {
				t.Logf("Adapter %s failed: %v", cfg.name, err)
				return
			}

			responses[cfg.name] = strings.TrimSpace(response.Content)
		})
	}

	// All adapters should mention Paris
	for name, response := range responses {
		assert.Contains(t, strings.ToLower(response), "paris",
			"Adapter %s response doesn't contain 'paris': %s", name, response)
	}
}

// TestRegistryIntegration tests the registry with available adapters
func TestRegistryIntegration(t *testing.T) {
	registry := llmadapters.NewRegistry()
	availableAdapters := 0

	// Register all available adapters
	for _, cfg := range adapters {
		if os.Getenv(cfg.envVar) != "" {
			adapter, err := cfg.createAdapter()
			if err == nil && adapter != nil {
				err = registry.Register(cfg.name, adapter)
				require.NoError(t, err)
				availableAdapters++
			}
		}
	}

	if availableAdapters == 0 {
		t.Fatal("No API keys configured for integration tests")
	}

	// Verify all registered adapters are accessible
	registeredAdapters := registry.List()
	assert.Equal(t, availableAdapters, len(registeredAdapters))

	// Test each adapter through the registry
	ctx, cancel := testContext()
	defer cancel()

	for _, name := range registeredAdapters {
		t.Run(name, func(t *testing.T) {
			adapter, err := registry.Get(name)
			require.NoError(t, err)
			require.NotNil(t, adapter)

			config := llmadapters.Config{
				Model:       testModels[name],
				Temperature: 0.0,
				MaxTokens:   50,
			}

			response, err := adapter.Generate(ctx, "What is 1+1?", config)
			require.NoError(t, err)
			assert.NotEmpty(t, response.Content)
			assert.Contains(t, response.Content, "2")
		})
	}
}
