package llmadapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAnthropicAdapter(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		envKey  string
		wantErr bool
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
				os.Setenv("ANTHROPIC_API_KEY", tt.envKey)
				defer os.Unsetenv("ANTHROPIC_API_KEY")
			}

			adapter, err := NewAnthropicAdapter(tt.apiKey)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrAPIKeyMissing, err)
				assert.Nil(t, adapter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter)
				assert.NotNil(t, adapter.httpClient)
				assert.NotNil(t, adapter.rateLimiter)
				assert.Equal(t, "https://api.anthropic.com/v1", adapter.baseURL)
			}
		})
	}
}

func TestAnthropicAdapter_Generate(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		// Parse request body
		var req anthropicRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify request
		assert.Equal(t, "claude-3-opus-20240229", req.Model)
		assert.Equal(t, "You are a helpful assistant", req.System)
		assert.Len(t, req.Messages, 1)
		assert.Equal(t, "user", req.Messages[0].Role)

		// Send response
		resp := anthropicResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{
					Type: "text",
					Text: "Test response from Claude",
				},
			},
			Model:      req.Model,
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create adapter with test server URL
	adapter := &AnthropicAdapter{
		apiKey:      "test-key",
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rateLimiter: nil, // Disable rate limiting for tests
		baseURL:     server.URL + "/v1",
	}

	// Test generate
	config := Config{
		Model:        "claude-3-opus",
		Temperature:  0.7,
		MaxTokens:    100,
		SystemPrompt: "You are a helpful assistant",
	}

	resp, err := adapter.Generate(context.Background(), "Test prompt", config)
	
	assert.NoError(t, err)
	assert.Equal(t, "Test response from Claude", resp.Content)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
	assert.Equal(t, "end_turn", resp.FinishReason)
}

func TestAnthropicAdapter_GenerateWithTools(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req anthropicRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify tools were included
		assert.Len(t, req.Tools, 2)
		assert.Equal(t, "get_weather", req.Tools[0].Name)

		// Send response with tool use
		resp := anthropicResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{
					Type: "text",
					Text: "I'll check the weather for you.",
				},
				{
					Type:  "tool_use",
					ID:    "toolu_123",
					Name:  "get_weather",
					Input: json.RawMessage(`{"location": "New York"}`),
				},
			},
			Model:      req.Model,
			StopReason: "tool_use",
			Usage: anthropicUsage{
				InputTokens:  20,
				OutputTokens: 10,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create adapter
	adapter := &AnthropicAdapter{
		apiKey:      "test-key",
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rateLimiter: nil,
		baseURL:     server.URL + "/v1",
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
		Model:       "claude-3-opus",
		Temperature: 0.7,
		MaxTokens:   100,
	}

	resp, err := adapter.GenerateWithTools(context.Background(), "What's the weather in New York?", tools, config)
	
	assert.NoError(t, err)
	assert.Equal(t, "I'll check the weather for you.", resp.Content)
	assert.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "toolu_123", resp.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", resp.ToolCalls[0].Name)
	
	// Verify tool arguments
	var args map[string]string
	err = json.Unmarshal(resp.ToolCalls[0].Arguments, &args)
	assert.NoError(t, err)
	assert.Equal(t, "New York", args["location"])
}

func TestAnthropicAdapter_StreamGenerate(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req anthropicRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.True(t, req.Stream)

		// Send streaming response
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		writer := bufio.NewWriter(w)
		
		// Send message start event
		startEvent := anthropicStreamEvent{
			Type: "message_start",
			Message: &anthropicResponse{
				ID:    "msg_123",
				Type:  "message",
				Role:  "assistant",
				Model: req.Model,
			},
		}
		data, _ := json.Marshal(startEvent)
		writer.WriteString(fmt.Sprintf("event: message_start\ndata: %s\n\n", data))
		writer.Flush()

		// Send content chunks
		chunks := []string{"Hello", " ", "streaming", " ", "world", "!"}
		for i, chunk := range chunks {
			deltaEvent := anthropicStreamEvent{
				Type:  "content_block_delta",
				Index: 0,
				Delta: &anthropicDelta{
					Type: "text_delta",
					Text: chunk,
				},
			}
			data, _ := json.Marshal(deltaEvent)
			writer.WriteString(fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", data))
			writer.Flush()
			
			if i < len(chunks)-1 {
				time.Sleep(10 * time.Millisecond)
			}
		}

		// Send message stop event
		stopEvent := anthropicStreamEvent{
			Type: "message_stop",
		}
		data, _ = json.Marshal(stopEvent)
		writer.WriteString(fmt.Sprintf("event: message_stop\ndata: %s\n\n", data))
		writer.Flush()
	}))
	defer server.Close()

	// Create adapter
	adapter := &AnthropicAdapter{
		apiKey:      "test-key",
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rateLimiter: nil,
		baseURL:     server.URL + "/v1",
	}

	config := Config{
		Model:       "claude-3-opus",
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
	expected := "Hello streaming world!"
	result := strings.Join(tokens, "")
	assert.Equal(t, expected, result)
}

func TestAnthropicAdapter_ValidateConfig(t *testing.T) {
	adapter := &AnthropicAdapter{}

	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Model:       "claude-3-opus",
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
				Model:       "claude-3-opus",
				Temperature: 1.5,
				MaxTokens:   100,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 1",
		},
		{
			name: "invalid top_p",
			config: Config{
				Model:     "claude-3-opus",
				MaxTokens: 100,
				TopP:      1.5,
			},
			wantErr: true,
			errMsg:  "top_p must be between 0 and 1",
		},
		{
			name: "zero max tokens gets default",
			config: Config{
				Model:     "claude-3-opus",
				MaxTokens: 0,
			},
			wantErr: false,
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

func TestAnthropicAdapter_MapModel(t *testing.T) {
	adapter := &AnthropicAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		{"claude-3-opus", "claude-3-opus-20240229"},
		{"claude-3-sonnet", "claude-3-sonnet-20240229"},
		{"claude-3-haiku", "claude-3-haiku-20240307"},
		{"claude-2.1", "claude-2.1"},
		{"claude-2", "claude-2.0"},
		{"claude-instant", "claude-instant-1.2"},
		{"custom-model", "custom-model"}, // Unknown models pass through
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := adapter.mapModel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnthropicAdapter_HandleHTTPError(t *testing.T) {
	adapter := &AnthropicAdapter{}

	tests := []struct {
		name       string
		statusCode int
		body       string
		expected   error
	}{
		{
			name:       "rate limit error",
			statusCode: 429,
			body:       `{"error": {"type": "rate_limit_error", "message": "Too many requests"}}`,
			expected:   ErrRateLimitExceeded,
		},
		{
			name:       "invalid request error",
			statusCode: 400,
			body:       `{"error": {"type": "invalid_request_error", "message": "Invalid parameter"}}`,
			expected:   ErrInvalidConfig,
		},
		{
			name:       "model not found error",
			statusCode: 404,
			body:       `{"error": {"type": "not_found_error", "message": "Model not found"}}`,
			expected:   ErrModelNotSupported,
		},
		{
			name:       "generic error",
			statusCode: 500,
			body:       `{"error": {"type": "server_error", "message": "Internal error"}}`,
			expected:   nil, // Will be a generic error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(bytes.NewBufferString(tt.body)),
			}

			err := adapter.handleHTTPError(resp)
			assert.Error(t, err)

			if tt.expected != nil {
				if errors.Is(err, ErrInvalidConfig) {
					assert.Contains(t, err.Error(), "invalid configuration")
				} else {
					assert.Equal(t, tt.expected, err)
				}
			} else {
				assert.Contains(t, err.Error(), "anthropic api error")
			}
		})
	}
}

func TestAnthropicAdapter_ErrorHandling(t *testing.T) {
	// Test server that returns various errors
	tests := []struct {
		name         string
		serverFunc   http.HandlerFunc
		expectedErr  string
		checkSpecific error
	}{
		{
			name: "network error",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				// Simulate network error by closing connection
				hj, _ := w.(http.Hijacker)
				conn, _, _ := hj.Hijack()
				conn.Close()
			},
			expectedErr: "failed to make request",
		},
		{
			name: "invalid json response",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("invalid json"))
			},
			expectedErr: "failed to decode response",
		},
		{
			name: "empty response",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
			},
			expectedErr: "", // Should succeed but with empty content
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverFunc)
			defer server.Close()

			adapter := &AnthropicAdapter{
				apiKey:      "test-key",
				httpClient:  &http.Client{Timeout: 1 * time.Second},
				rateLimiter: nil,
				baseURL:     server.URL + "/v1",
			}

			config := Config{
				Model:     "claude-3-opus",
				MaxTokens: 100,
			}

			_, err := adapter.Generate(context.Background(), "Test", config)
			
			if tt.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else if tt.checkSpecific != nil {
				assert.Equal(t, tt.checkSpecific, err)
			}
		})
	}
}