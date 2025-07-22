package llmadapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenAIAdapter(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		envKey    string
		wantErr   bool
	}{
		{
			name:    "with api key",
			apiKey:  "test-api-key",
			wantErr: false,
		},
		{
			name:    "with env var",
			apiKey:  "",
			envKey:  "env-api-key",
			wantErr: false,
		},
		{
			name:    "no api key",
			apiKey:  "",
			envKey:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env var if needed
			if tt.envKey != "" {
				os.Setenv("OPENAI_API_KEY", tt.envKey)
				defer os.Unsetenv("OPENAI_API_KEY")
			}

			adapter, err := NewOpenAIAdapter(tt.apiKey)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrAPIKeyMissing, err)
				assert.Nil(t, adapter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter)
				assert.NotNil(t, adapter.client)
				assert.NotNil(t, adapter.rateLimiter)
			}
		})
	}
}

func TestOpenAIAdapter_Generate(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		// Parse request body
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Send response
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Test response",
					},
					FinishReason: openai.FinishReasonStop,
				},
			},
			Usage: openai.Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create adapter with custom client
	clientConfig := openai.DefaultConfig("test-key")
	clientConfig.BaseURL = server.URL + "/v1"
	adapter := &OpenAIAdapter{
		client:      openai.NewClientWithConfig(clientConfig),
		rateLimiter: nil, // Disable rate limiting for tests
	}

	// Test generate
	config := Config{
		Model:        "gpt-4",
		Temperature:  0.7,
		MaxTokens:    100,
		SystemPrompt: "You are a helpful assistant",
	}

	resp, err := adapter.Generate(context.Background(), "Test prompt", config)
	
	assert.NoError(t, err)
	assert.Equal(t, "Test response", resp.Content)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
	assert.Equal(t, "stop", resp.FinishReason)
}

func TestOpenAIAdapter_GenerateWithTools(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify tools were included
		assert.Len(t, req.Tools, 2)
		assert.Equal(t, "get_weather", req.Tools[0].Function.Name)

		// Send response with tool call
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "I'll check the weather for you.",
						ToolCalls: []openai.ToolCall{
							{
								ID:   "call_abc123",
								Type: openai.ToolTypeFunction,
								Function: openai.FunctionCall{
									Name:      "get_weather",
									Arguments: `{"location": "New York"}`,
								},
							},
						},
					},
					FinishReason: openai.FinishReasonToolCalls,
				},
			},
			Usage: openai.Usage{
				PromptTokens:     20,
				CompletionTokens: 10,
				TotalTokens:      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create adapter with custom client
	clientConfig := openai.DefaultConfig("test-key")
	clientConfig.BaseURL = server.URL + "/v1"
	adapter := &OpenAIAdapter{
		client: openai.NewClientWithConfig(clientConfig),
		rateLimiter: nil,
	}

	// Define tools
	tools := []Tool{
		{
			Name:        "get_weather",
			Description: "Get the weather for a location",
			Parameters:  json.RawMessage(`{"type": "object", "properties": {"location": {"type": "string"}}}`),
		},
		{
			Name:        "search_web",
			Description: "Search the web",
			Parameters:  json.RawMessage(`{"type": "object", "properties": {"query": {"type": "string"}}}`),
		},
	}

	config := Config{
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   100,
	}

	resp, err := adapter.GenerateWithTools(context.Background(), "What's the weather in New York?", tools, config)
	
	assert.NoError(t, err)
	assert.Equal(t, "I'll check the weather for you.", resp.Content)
	assert.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "call_abc123", resp.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", resp.ToolCalls[0].Name)
	
	// Verify tool arguments
	var args map[string]string
	err = json.Unmarshal(resp.ToolCalls[0].Arguments, &args)
	assert.NoError(t, err)
	assert.Equal(t, "New York", args["location"])
}

func TestOpenAIAdapter_StreamGenerate(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.True(t, req.Stream)

		// Send SSE response
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send chunks
		chunks := []string{"Hello", " ", "world", "!"}
		for i, chunk := range chunks {
			data := openai.ChatCompletionStreamResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: chunk,
						},
					},
				},
			}
			
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			w.(http.Flusher).Flush()
			
			if i < len(chunks)-1 {
				time.Sleep(10 * time.Millisecond) // Simulate streaming delay
			}
		}

		// Send done signal
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}))
	defer server.Close()

	// Create adapter
	clientConfig := openai.DefaultConfig("test-key")
	clientConfig.BaseURL = server.URL + "/v1"
	adapter := &OpenAIAdapter{
		client: openai.NewClientWithConfig(clientConfig),
		rateLimiter: nil,
	}

	config := Config{
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   100,
	}

	ctx := context.Background()
	tokenChan, err := adapter.StreamGenerate(ctx, "Test prompt", config)
	require.NoError(t, err)

	// Collect tokens
	var tokens []string
	for token := range tokenChan {
		if token.Error != nil {
			t.Fatalf("Unexpected error in stream: %v", token.Error)
		}
		tokens = append(tokens, token.Content)
	}

	// Verify result
	assert.Equal(t, []string{"Hello", " ", "world", "!"}, tokens)
}

func TestOpenAIAdapter_ValidateConfig(t *testing.T) {
	adapter := &OpenAIAdapter{}

	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Model:       "gpt-4",
				Temperature: 0.7,
				MaxTokens:   100,
			},
			wantErr: false,
		},
		{
			name: "missing model",
			config: Config{
				Temperature: 0.7,
				MaxTokens:   100,
			},
			wantErr: true,
			errMsg:  "model is required",
		},
		{
			name: "invalid temperature too high",
			config: Config{
				Model:       "gpt-4",
				Temperature: 2.5,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 2",
		},
		{
			name: "invalid temperature negative",
			config: Config{
				Model:       "gpt-4",
				Temperature: -0.5,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 2",
		},
		{
			name: "invalid top_p",
			config: Config{
				Model: "gpt-4",
				TopP:  1.5,
			},
			wantErr: true,
			errMsg:  "top_p must be between 0 and 1",
		},
		{
			name: "invalid response format",
			config: Config{
				Model:          "gpt-4",
				ResponseFormat: "xml",
			},
			wantErr: true,
			errMsg:  "response_format must be 'text' or 'json'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.validateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOpenAIAdapter_MapModel(t *testing.T) {
	adapter := &OpenAIAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		{"gpt-4", openai.GPT4},
		{"gpt-4-turbo", "gpt-4-turbo-preview"},
		{"gpt-3.5-turbo", openai.GPT3Dot5Turbo},
		{"gpt-4o", "gpt-4o"},
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"custom-model", "custom-model"}, // Unknown models pass through
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := adapter.mapModel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOpenAIAdapter_HandleError(t *testing.T) {
	adapter := &OpenAIAdapter{}

	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name: "rate limit error",
			err: &openai.APIError{
				HTTPStatusCode: 429,
			},
			expected: ErrRateLimitExceeded,
		},
		{
			name: "context length error",
			err: &openai.APIError{
				HTTPStatusCode: 400,
				Code:          "context_length_exceeded",
			},
			expected: ErrContextLengthExceeded,
		},
		{
			name: "model not found error",
			err: &openai.APIError{
				HTTPStatusCode: 404,
				Code:          "model_not_found",
			},
			expected: ErrModelNotSupported,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: errors.New("openai api error: some error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.handleError(tt.err)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else if errors.Is(tt.expected, ErrRateLimitExceeded) ||
				errors.Is(tt.expected, ErrContextLengthExceeded) ||
				errors.Is(tt.expected, ErrModelNotSupported) {
				assert.Equal(t, tt.expected, result)
			} else {
				assert.Contains(t, result.Error(), "openai api error")
			}
		})
	}
}