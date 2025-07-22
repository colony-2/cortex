package llmadapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"golang.org/x/time/rate"
)

// bedrockClient interface for testing
type bedrockClient interface {
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
	InvokeModelWithResponseStream(ctx context.Context, params *bedrockruntime.InvokeModelWithResponseStreamInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelWithResponseStreamOutput, error)
}

// BedrockAdapter implements the Adapter interface for AWS Bedrock
type BedrockAdapter struct {
	client      bedrockClient
	rateLimiter *rate.Limiter
	region      string
}

// NewBedrockAdapter creates a new AWS Bedrock adapter
func NewBedrockAdapter(region string) (*BedrockAdapter, error) {
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1" // Default region
		}
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create Bedrock Runtime client
	client := bedrockruntime.NewFromConfig(cfg)

	// Create rate limiter: 30 requests per minute (conservative default)
	rateLimiter := rate.NewLimiter(rate.Limit(0.5), 30)

	return &BedrockAdapter{
		client:      client,
		rateLimiter: rateLimiter,
		region:      region,
	}, nil
}

// anthropicBedrockRequest represents Claude request format for Bedrock
type anthropicBedrockRequest struct {
	AnthropicVersion string                    `json:"anthropic_version"`
	MaxTokens        int                       `json:"max_tokens"`
	Messages         []anthropicBedrockMessage `json:"messages"`
	Temperature      float64                   `json:"temperature,omitempty"`
	TopP             float64                   `json:"top_p,omitempty"`
	StopSequences    []string                  `json:"stop_sequences,omitempty"`
	System           string                    `json:"system,omitempty"`
}

type anthropicBedrockMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicBedrockResponse represents Claude response format from Bedrock
type anthropicBedrockResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	ID           string `json:"id"`
	Model        string `json:"model"`
	Role         string `json:"role"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Type         string `json:"type"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// titanRequest represents Titan request format for Bedrock
type titanRequest struct {
	InputText            string                 `json:"inputText"`
	TextGenerationConfig titanGenerationConfig  `json:"textGenerationConfig"`
}

type titanGenerationConfig struct {
	MaxTokenCount     int      `json:"maxTokenCount,omitempty"`
	Temperature       float64  `json:"temperature,omitempty"`
	TopP              float64  `json:"topP,omitempty"`
	StopSequences     []string `json:"stopSequences,omitempty"`
}

// titanResponse represents Titan response format from Bedrock
type titanResponse struct {
	InputTextTokenCount int `json:"inputTextTokenCount"`
	Results             []struct {
		TokenCount       int    `json:"tokenCount"`
		OutputText       string `json:"outputText"`
		CompletionReason string `json:"completionReason"`
	} `json:"results"`
}

// Generate creates a completion for the given prompt
func (a *BedrockAdapter) Generate(ctx context.Context, prompt string, cfg Config) (Response, error) {
	// Apply rate limiting
	if a.rateLimiter != nil {
		if err := a.rateLimiter.Wait(ctx); err != nil {
			return Response{}, fmt.Errorf("rate limiter error: %w", err)
		}
	}

	// Validate configuration
	if err := a.validateConfig(cfg); err != nil {
		return Response{}, err
	}

	// Route to appropriate model handler
	modelID := a.mapModel(cfg.Model)
	
	switch {
	case isClaudeModel(modelID):
		return a.generateClaude(ctx, prompt, cfg, modelID)
	case isTitanModel(modelID):
		return a.generateTitan(ctx, prompt, cfg, modelID)
	default:
		return Response{}, fmt.Errorf("%w: %s", ErrModelNotSupported, cfg.Model)
	}
}

// generateClaude handles generation for Claude models
func (a *BedrockAdapter) generateClaude(ctx context.Context, prompt string, cfg Config, modelID string) (Response, error) {
	// Build Claude request
	messages := []anthropicBedrockMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	req := anthropicBedrockRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        cfg.MaxTokens,
		Messages:         messages,
		Temperature:      cfg.Temperature,
	}

	if cfg.SystemPrompt != "" {
		req.System = cfg.SystemPrompt
	}

	if cfg.TopP > 0 {
		req.TopP = cfg.TopP
	}

	if len(cfg.StopSequences) > 0 {
		req.StopSequences = cfg.StopSequences
	}

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Invoke model
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        body,
	}

	output, err := a.client.InvokeModel(ctx, input)
	if err != nil {
		return Response{}, a.handleError(err)
	}

	// Parse response
	var resp anthropicBedrockResponse
	if err := json.Unmarshal(output.Body, &resp); err != nil {
		return Response{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract content
	content := ""
	for _, c := range resp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return Response{
		Content: content,
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
		FinishReason: resp.StopReason,
		Model:        modelID,
	}, nil
}

// generateTitan handles generation for Titan models
func (a *BedrockAdapter) generateTitan(ctx context.Context, prompt string, cfg Config, modelID string) (Response, error) {
	// Build Titan request
	fullPrompt := prompt
	if cfg.SystemPrompt != "" {
		fullPrompt = cfg.SystemPrompt + "\n\n" + prompt
	}

	req := titanRequest{
		InputText: fullPrompt,
		TextGenerationConfig: titanGenerationConfig{
			MaxTokenCount: cfg.MaxTokens,
			Temperature:   cfg.Temperature,
			TopP:          cfg.TopP,
			StopSequences: cfg.StopSequences,
		},
	}

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Invoke model
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        body,
	}

	output, err := a.client.InvokeModel(ctx, input)
	if err != nil {
		return Response{}, a.handleError(err)
	}

	// Parse response
	var resp titanResponse
	if err := json.Unmarshal(output.Body, &resp); err != nil {
		return Response{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(resp.Results) == 0 {
		return Response{}, ErrInvalidResponse
	}

	result := resp.Results[0]

	return Response{
		Content: result.OutputText,
		Usage: Usage{
			PromptTokens:     resp.InputTextTokenCount,
			CompletionTokens: result.TokenCount,
			TotalTokens:      resp.InputTextTokenCount + result.TokenCount,
		},
		FinishReason: result.CompletionReason,
		Model:        modelID,
	}, nil
}

// GenerateWithTools creates a completion with tool/function calling support
func (a *BedrockAdapter) GenerateWithTools(ctx context.Context, prompt string, tools []Tool, cfg Config) (Response, error) {
	// Currently, Bedrock doesn't have native tool calling support for most models
	// We'll fall back to regular generation with instructions about tools
	toolPrompt := prompt + "\n\nAvailable tools:\n"
	for _, tool := range tools {
		toolPrompt += fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description)
	}
	toolPrompt += "\nIf you need to use a tool, respond with a JSON object containing 'tool_name' and 'arguments'."

	return a.Generate(ctx, toolPrompt, cfg)
}

// StreamGenerate creates a streaming completion for the given prompt
func (a *BedrockAdapter) StreamGenerate(ctx context.Context, prompt string, cfg Config) (<-chan Token, error) {
	// Apply rate limiting
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}

	// Validate configuration
	if err := a.validateConfig(cfg); err != nil {
		return nil, err
	}

	// Route to appropriate model handler
	modelID := a.mapModel(cfg.Model)
	
	switch {
	case isClaudeModel(modelID):
		return a.streamClaude(ctx, prompt, cfg, modelID)
	default:
		// Fallback to non-streaming for models that don't support streaming
		return a.fallbackStream(ctx, prompt, cfg)
	}
}

// streamClaude handles streaming for Claude models
func (a *BedrockAdapter) streamClaude(ctx context.Context, prompt string, cfg Config, modelID string) (<-chan Token, error) {
	// Build Claude request
	messages := []anthropicBedrockMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	req := anthropicBedrockRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        cfg.MaxTokens,
		Messages:         messages,
		Temperature:      cfg.Temperature,
	}

	if cfg.SystemPrompt != "" {
		req.System = cfg.SystemPrompt
	}

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create streaming input
	input := &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        body,
	}

	// Create output channel
	tokenChan := make(chan Token)

	// Start goroutine to handle streaming
	go func() {
		defer close(tokenChan)

		output, err := a.client.InvokeModelWithResponseStream(ctx, input)
		if err != nil {
			select {
			case tokenChan <- Token{Error: a.handleError(err)}:
			case <-ctx.Done():
			}
			return
		}

		stream := output.GetStream()
		for event := range stream.Events() {
			switch v := event.(type) {
			case *types.ResponseStreamMemberChunk:
				// Parse chunk
				var chunk struct {
					Type  string `json:"type"`
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
				}
				
				if err := json.Unmarshal(v.Value.Bytes, &chunk); err != nil {
					continue
				}

				if chunk.Type == "content_block_delta" && chunk.Delta.Type == "text_delta" {
					select {
					case tokenChan <- Token{Content: chunk.Delta.Text}:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			select {
			case tokenChan <- Token{Error: err}:
			case <-ctx.Done():
			}
		}
	}()

	return tokenChan, nil
}

// fallbackStream provides non-streaming fallback
func (a *BedrockAdapter) fallbackStream(ctx context.Context, prompt string, cfg Config) (<-chan Token, error) {
	tokenChan := make(chan Token)

	go func() {
		defer close(tokenChan)

		resp, err := a.Generate(ctx, prompt, cfg)
		if err != nil {
			select {
			case tokenChan <- Token{Error: err}:
			case <-ctx.Done():
			}
			return
		}

		// Send the entire response as one token
		select {
		case tokenChan <- Token{Content: resp.Content}:
		case <-ctx.Done():
		}
	}()

	return tokenChan, nil
}

// validateConfig validates the configuration
func (a *BedrockAdapter) validateConfig(config Config) error {
	if config.Model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidConfig)
	}

	if config.MaxTokens <= 0 {
		config.MaxTokens = 512 // Default for Bedrock
	}

	if config.Temperature < 0 || config.Temperature > 1 {
		return fmt.Errorf("%w: temperature must be between 0 and 1", ErrInvalidConfig)
	}

	if config.TopP < 0 || config.TopP > 1 {
		return fmt.Errorf("%w: top_p must be between 0 and 1", ErrInvalidConfig)
	}

	return nil
}

// mapModel maps generic model names to Bedrock model IDs
func (a *BedrockAdapter) mapModel(model string) string {
	// Map common model names to Bedrock model IDs
	modelMap := map[string]string{
		// Claude models
		"claude-3-opus":    "anthropic.claude-3-opus-20240229-v1:0",
		"claude-3-sonnet":  "anthropic.claude-3-sonnet-20240229-v1:0",
		"claude-3-haiku":   "anthropic.claude-3-haiku-20240307-v1:0",
		"claude-2.1":       "anthropic.claude-v2:1",
		"claude-2":         "anthropic.claude-v2",
		"claude-instant":   "anthropic.claude-instant-v1",
		
		// Titan models
		"titan-text-express": "amazon.titan-text-express-v1",
		"titan-text-lite":    "amazon.titan-text-lite-v1",
		"titan-text-premier": "amazon.titan-text-premier-v1:0",
		
		// Other models
		"cohere-command":      "cohere.command-text-v14",
		"cohere-command-light": "cohere.command-light-text-v14",
		"ai21-j2-ultra":       "ai21.j2-ultra-v1",
		"ai21-j2-mid":         "ai21.j2-mid-v1",
	}

	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model
}

// isClaudeModel checks if the model is a Claude model
func isClaudeModel(modelID string) bool {
	return len(modelID) > 10 && modelID[:10] == "anthropic."
}

// isTitanModel checks if the model is a Titan model
func isTitanModel(modelID string) bool {
	return len(modelID) > 7 && modelID[:7] == "amazon."
}

// handleError converts AWS errors to adapter errors
func (a *BedrockAdapter) handleError(err error) error {
	if err == nil {
		return nil
	}

	// Check for specific AWS error types
	errStr := err.Error()
	
	if contains(errStr, "ThrottlingException") {
		return ErrRateLimitExceeded
	}
	
	if contains(errStr, "ResourceNotFoundException") {
		return ErrModelNotSupported
	}
	
	if contains(errStr, "ValidationException") {
		if contains(errStr, "max_tokens") || contains(errStr, "context") {
			return ErrContextLengthExceeded
		}
		return ErrInvalidConfig
	}

	return fmt.Errorf("bedrock api error: %w", err)
}

// contains is a helper function for string matching
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   len(s) > len(substr) && containsHelper(s[1:], substr)
}

func containsHelper(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	if s[:len(substr)] == substr {
		return true
	}
	return containsHelper(s[1:], substr)
}