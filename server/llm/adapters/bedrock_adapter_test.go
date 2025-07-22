package llmadapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock Bedrock client
type mockBedrockClient struct {
	mock.Mock
}

func (m *mockBedrockClient) InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bedrockruntime.InvokeModelOutput), args.Error(1)
}

func (m *mockBedrockClient) InvokeModelWithResponseStream(ctx context.Context, params *bedrockruntime.InvokeModelWithResponseStreamInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelWithResponseStreamOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bedrockruntime.InvokeModelWithResponseStreamOutput), args.Error(1)
}

func TestNewBedrockAdapter(t *testing.T) {
	tests := []struct {
		name   string
		region string
		envVar string
	}{
		{
			name:   "with explicit region",
			region: "us-west-2",
		},
		{
			name:   "with env var region",
			region: "",
			envVar: "eu-west-1",
		},
		{
			name:   "default region",
			region: "",
			envVar: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				os.Setenv("AWS_REGION", tt.envVar)
				defer os.Unsetenv("AWS_REGION")
			}

			adapter, err := NewBedrockAdapter(tt.region)
			
			// May fail if AWS credentials are not configured
			if err != nil {
				assert.Contains(t, err.Error(), "failed to load AWS config")
			} else {
				assert.NotNil(t, adapter)
				assert.NotNil(t, adapter.client)
				assert.NotNil(t, adapter.rateLimiter)
				
				expectedRegion := tt.region
				if expectedRegion == "" {
					expectedRegion = tt.envVar
					if expectedRegion == "" {
						expectedRegion = "us-east-1"
					}
				}
				assert.Equal(t, expectedRegion, adapter.region)
			}
		})
	}
}

func TestBedrockAdapter_Generate_Claude(t *testing.T) {
	// Create adapter with mock client
	adapter := &BedrockAdapter{
		client:      nil, // Will be replaced with mock
		rateLimiter: nil, // Disable rate limiting for tests
		region:      "us-east-1",
	}

	// Create mock response
	claudeResp := anthropicBedrockResponse{
		ID:   "msg_123",
		Type: "message",
		Role: "assistant",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: "Hello from Claude on Bedrock"},
		},
		Model:      "anthropic.claude-3-opus-20240229-v1:0",
		StopReason: "end_turn",
		Usage: struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	respBody, _ := json.Marshal(claudeResp)

	// Mock client behavior
	mockClient := new(mockBedrockClient)
	mockClient.On("InvokeModel", mock.Anything, mock.MatchedBy(func(input *bedrockruntime.InvokeModelInput) bool {
		// Verify model ID
		return *input.ModelId == "anthropic.claude-3-opus-20240229-v1:0"
	})).Return(&bedrockruntime.InvokeModelOutput{
		Body:        respBody,
		ContentType: aws.String("application/json"),
	}, nil)

	adapter.client = mockClient

	// Test
	config := Config{
		Model:        "claude-3-opus",
		Temperature:  0.7,
		MaxTokens:    100,
		SystemPrompt: "You are helpful",
	}

	resp, err := adapter.Generate(context.Background(), "Hello", config)
	
	assert.NoError(t, err)
	assert.Equal(t, "Hello from Claude on Bedrock", resp.Content)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
	assert.Equal(t, "end_turn", resp.FinishReason)
	
	mockClient.AssertExpectations(t)
}

func TestBedrockAdapter_Generate_Titan(t *testing.T) {
	// Create adapter with mock client
	adapter := &BedrockAdapter{
		client:      nil,
		rateLimiter: nil,
		region:      "us-east-1",
	}

	// Create mock response
	titanResp := titanResponse{
		InputTextTokenCount: 8,
		Results: []struct {
			TokenCount       int    `json:"tokenCount"`
			OutputText       string `json:"outputText"`
			CompletionReason string `json:"completionReason"`
		}{
			{
				TokenCount:       6,
				OutputText:       "Hello from Titan",
				CompletionReason: "FINISH",
			},
		},
	}

	respBody, _ := json.Marshal(titanResp)

	// Mock client behavior
	mockClient := new(mockBedrockClient)
	mockClient.On("InvokeModel", mock.Anything, mock.MatchedBy(func(input *bedrockruntime.InvokeModelInput) bool {
		// Verify model ID and request structure
		if *input.ModelId != "amazon.titan-text-express-v1" {
			return false
		}
		
		// Verify request body
		var req titanRequest
		json.Unmarshal(input.Body, &req)
		return req.TextGenerationConfig.Temperature == 0.7
	})).Return(&bedrockruntime.InvokeModelOutput{
		Body:        respBody,
		ContentType: aws.String("application/json"),
	}, nil)

	adapter.client = mockClient

	// Test
	config := Config{
		Model:       "titan-text-express",
		Temperature: 0.7,
		MaxTokens:   100,
	}

	resp, err := adapter.Generate(context.Background(), "Hello", config)
	
	assert.NoError(t, err)
	assert.Equal(t, "Hello from Titan", resp.Content)
	assert.Equal(t, 8, resp.Usage.PromptTokens)
	assert.Equal(t, 6, resp.Usage.CompletionTokens)
	assert.Equal(t, 14, resp.Usage.TotalTokens)
	assert.Equal(t, "FINISH", resp.FinishReason)
	
	mockClient.AssertExpectations(t)
}

func TestBedrockAdapter_GenerateWithTools(t *testing.T) {
	// Currently tools are not natively supported, so it should add instructions
	adapter := &BedrockAdapter{
		client:      nil,
		rateLimiter: nil,
		region:      "us-east-1",
	}

	// Mock response
	claudeResp := anthropicBedrockResponse{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: "I understand the tools available"},
		},
		Usage: struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}{
			InputTokens:  20,
			OutputTokens: 10,
		},
	}

	respBody, _ := json.Marshal(claudeResp)

	mockClient := new(mockBedrockClient)
	mockClient.On("InvokeModel", mock.Anything, mock.MatchedBy(func(input *bedrockruntime.InvokeModelInput) bool {
		// Verify that the prompt includes tool information
		var req anthropicBedrockRequest
		json.Unmarshal(input.Body, &req)
		return len(req.Messages) > 0 && 
			   strings.Contains(req.Messages[0].Content, "Available tools")
	})).Return(&bedrockruntime.InvokeModelOutput{
		Body: respBody,
	}, nil)

	adapter.client = mockClient

	tools := []Tool{
		{
			Name:        "get_weather",
			Description: "Get weather info",
			Parameters:  json.RawMessage(`{}`),
		},
	}

	config := Config{
		Model:     "claude-3-opus",
		MaxTokens: 100,
	}

	resp, err := adapter.GenerateWithTools(context.Background(), "What's the weather?", tools, config)
	
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Content)
	
	mockClient.AssertExpectations(t)
}

func TestBedrockAdapter_ValidateConfig(t *testing.T) {
	adapter := &BedrockAdapter{}

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
			},
			wantErr: true,
			errMsg:  "model is required",
		},
		{
			name: "invalid temperature",
			config: Config{
				Model:       "claude-3-opus",
				Temperature: 1.5,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 1",
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

func TestBedrockAdapter_MapModel(t *testing.T) {
	adapter := &BedrockAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		// Claude models
		{"claude-3-opus", "anthropic.claude-3-opus-20240229-v1:0"},
		{"claude-3-sonnet", "anthropic.claude-3-sonnet-20240229-v1:0"},
		{"claude-3-haiku", "anthropic.claude-3-haiku-20240307-v1:0"},
		{"claude-2.1", "anthropic.claude-v2:1"},
		{"claude-2", "anthropic.claude-v2"},
		{"claude-instant", "anthropic.claude-instant-v1"},
		
		// Titan models
		{"titan-text-express", "amazon.titan-text-express-v1"},
		{"titan-text-lite", "amazon.titan-text-lite-v1"},
		{"titan-text-premier", "amazon.titan-text-premier-v1:0"},
		
		// Other models
		{"cohere-command", "cohere.command-text-v14"},
		{"ai21-j2-ultra", "ai21.j2-ultra-v1"},
		
		// Unknown model
		{"custom-model", "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := adapter.mapModel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBedrockAdapter_HandleError(t *testing.T) {
	adapter := &BedrockAdapter{}

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
			name:     "throttling error",
			err:      errors.New("ThrottlingException: Too many requests"),
			expected: ErrRateLimitExceeded,
		},
		{
			name:     "model not found",
			err:      errors.New("ResourceNotFoundException: Model not found"),
			expected: ErrModelNotSupported,
		},
		{
			name:     "validation error - context length",
			err:      errors.New("ValidationException: max_tokens exceeds limit"),
			expected: ErrContextLengthExceeded,
		},
		{
			name:     "validation error - general",
			err:      errors.New("ValidationException: Invalid parameter"),
			expected: ErrInvalidConfig,
		},
		{
			name:     "generic error",
			err:      errors.New("Some other error"),
			expected: errors.New("bedrock api error: Some other error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.handleError(tt.err)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else if errors.Is(tt.expected, ErrRateLimitExceeded) ||
				errors.Is(tt.expected, ErrModelNotSupported) ||
				errors.Is(tt.expected, ErrContextLengthExceeded) ||
				errors.Is(tt.expected, ErrInvalidConfig) {
				assert.Equal(t, tt.expected, result)
			} else {
				assert.Contains(t, result.Error(), "bedrock api error")
			}
		})
	}
}

func TestBedrockAdapter_ModelDetection(t *testing.T) {
	tests := []struct {
		modelID  string
		isClaude bool
		isTitan  bool
	}{
		{"anthropic.claude-3-opus-20240229-v1:0", true, false},
		{"anthropic.claude-v2", true, false},
		{"amazon.titan-text-express-v1", false, true},
		{"amazon.titan-text-lite-v1", false, true},
		{"cohere.command-text-v14", false, false},
		{"ai21.j2-ultra-v1", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.isClaude, isClaudeModel(tt.modelID))
			assert.Equal(t, tt.isTitan, isTitanModel(tt.modelID))
		})
	}
}

func TestBedrockAdapter_UnsupportedModel(t *testing.T) {
	adapter := &BedrockAdapter{
		client:      nil,
		rateLimiter: nil,
		region:      "us-east-1",
	}

	config := Config{
		Model:     "unsupported-model",
		MaxTokens: 100,
	}

	_, err := adapter.Generate(context.Background(), "Test", config)
	
	assert.Error(t, err)
	assert.Equal(t, ErrModelNotSupported, errors.Unwrap(err))
}

// Test AWS error handling
type mockAWSError struct {
	smithy.APIError
	code string
}

func (e mockAWSError) ErrorCode() string {
	return e.code
}

func (e mockAWSError) Error() string {
	return e.code + ": mock error"
}

func TestBedrockAdapter_AWSErrors(t *testing.T) {
	adapter := &BedrockAdapter{
		rateLimiter: nil,
	}

	// Mock client that returns AWS errors
	mockClient := new(mockBedrockClient)
	
	// Test throttling error
	mockClient.On("InvokeModel", mock.Anything, mock.Anything).Return(
		nil, 
		mockAWSError{code: "ThrottlingException"},
	).Once()
	
	adapter.client = mockClient
	
	config := Config{
		Model:     "claude-3-opus",
		MaxTokens: 100,
	}
	
	_, err := adapter.Generate(context.Background(), "Test", config)
	assert.Equal(t, ErrRateLimitExceeded, err)
}