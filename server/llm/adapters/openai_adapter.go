package llmadapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/time/rate"
)

// OpenAIAdapter implements the Adapter interface for OpenAI API
type OpenAIAdapter struct {
	client      *openai.Client
	rateLimiter *rate.Limiter
}

// NewOpenAIAdapter creates a new OpenAI adapter
func NewOpenAIAdapter(apiKey string) (*OpenAIAdapter, error) {
	if apiKey == "" {
		// Try to get from environment variable
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, ErrAPIKeyMissing
		}
	}

	client := openai.NewClient(apiKey)
	
	// Create rate limiter: 60 requests per minute
	rateLimiter := rate.NewLimiter(rate.Limit(1), 60)

	return &OpenAIAdapter{
		client:      client,
		rateLimiter: rateLimiter,
	}, nil
}

// Generate creates a completion for the given prompt
func (a *OpenAIAdapter) Generate(ctx context.Context, prompt string, config Config) (Response, error) {
	// Apply rate limiting
	if a.rateLimiter != nil {
		if err := a.rateLimiter.Wait(ctx); err != nil {
			return Response{}, fmt.Errorf("rate limiter error: %w", err)
		}
	}

	// Validate configuration
	if err := a.validateConfig(config); err != nil {
		return Response{}, err
	}

	// Build messages
	messages := []openai.ChatCompletionMessage{}
	
	if config.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: config.SystemPrompt,
		})
	}
	
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})

	// Build request
	req := openai.ChatCompletionRequest{
		Model:       a.mapModel(config.Model),
		Messages:    messages,
		Temperature: float32(config.Temperature),
		MaxTokens:   config.MaxTokens,
	}

	if config.TopP > 0 {
		req.TopP = float32(config.TopP)
	}

	if len(config.StopSequences) > 0 {
		req.Stop = config.StopSequences
	}

	if config.ResponseFormat == "json" {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	// Make API call
	resp, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return Response{}, a.handleError(err)
	}

	if len(resp.Choices) == 0 {
		return Response{}, ErrInvalidResponse
	}

	choice := resp.Choices[0]
	
	return Response{
		Content: choice.Message.Content,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		FinishReason: string(choice.FinishReason),
		Model:        resp.Model,
	}, nil
}

// GenerateWithTools creates a completion with tool/function calling support
func (a *OpenAIAdapter) GenerateWithTools(ctx context.Context, prompt string, tools []Tool, config Config) (Response, error) {
	// Apply rate limiting
	if a.rateLimiter != nil {
		if err := a.rateLimiter.Wait(ctx); err != nil {
			return Response{}, fmt.Errorf("rate limiter error: %w", err)
		}
	}

	// Validate configuration
	if err := a.validateConfig(config); err != nil {
		return Response{}, err
	}

	// Build messages
	messages := []openai.ChatCompletionMessage{}
	
	if config.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: config.SystemPrompt,
		})
	}
	
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})

	// Convert tools to OpenAI format
	openaiTools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		openaiTools[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		}
	}

	// Build request
	req := openai.ChatCompletionRequest{
		Model:       a.mapModel(config.Model),
		Messages:    messages,
		Temperature: float32(config.Temperature),
		MaxTokens:   config.MaxTokens,
		Tools:       openaiTools,
	}

	if config.TopP > 0 {
		req.TopP = float32(config.TopP)
	}

	if len(config.StopSequences) > 0 {
		req.Stop = config.StopSequences
	}

	// Make API call
	resp, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return Response{}, a.handleError(err)
	}

	if len(resp.Choices) == 0 {
		return Response{}, ErrInvalidResponse
	}

	choice := resp.Choices[0]
	
	// Convert tool calls
	var toolCalls []ToolCall
	if len(choice.Message.ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			toolCalls[i] = ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			}
		}
	}
	
	return Response{
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		FinishReason: string(choice.FinishReason),
		Model:        resp.Model,
	}, nil
}

// StreamGenerate creates a streaming completion for the given prompt
func (a *OpenAIAdapter) StreamGenerate(ctx context.Context, prompt string, config Config) (<-chan Token, error) {
	// Apply rate limiting
	if a.rateLimiter != nil {
		if err := a.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}
	}

	// Validate configuration
	if err := a.validateConfig(config); err != nil {
		return nil, err
	}

	// Build messages
	messages := []openai.ChatCompletionMessage{}
	
	if config.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: config.SystemPrompt,
		})
	}
	
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})

	// Build request
	req := openai.ChatCompletionRequest{
		Model:       a.mapModel(config.Model),
		Messages:    messages,
		Temperature: float32(config.Temperature),
		MaxTokens:   config.MaxTokens,
		Stream:      true,
	}

	if config.TopP > 0 {
		req.TopP = float32(config.TopP)
	}

	if len(config.StopSequences) > 0 {
		req.Stop = config.StopSequences
	}

	// Create stream
	stream, err := a.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, a.handleError(err)
	}

	// Create output channel
	tokenChan := make(chan Token)

	// Start goroutine to read from stream
	go func() {
		defer close(tokenChan)
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				select {
				case tokenChan <- Token{Error: err}:
				case <-ctx.Done():
				}
				return
			}

			if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
				select {
				case tokenChan <- Token{Content: response.Choices[0].Delta.Content}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return tokenChan, nil
}

// validateConfig validates the configuration
func (a *OpenAIAdapter) validateConfig(config Config) error {
	if config.Model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidConfig)
	}

	if config.Temperature < 0 || config.Temperature > 2 {
		return fmt.Errorf("%w: temperature must be between 0 and 2", ErrInvalidConfig)
	}

	if config.TopP < 0 || config.TopP > 1 {
		return fmt.Errorf("%w: top_p must be between 0 and 1", ErrInvalidConfig)
	}

	if config.MaxTokens < 0 {
		return fmt.Errorf("%w: max_tokens must be non-negative", ErrInvalidConfig)
	}

	if config.ResponseFormat != "" && config.ResponseFormat != "text" && config.ResponseFormat != "json" {
		return fmt.Errorf("%w: response_format must be 'text' or 'json'", ErrInvalidConfig)
	}

	return nil
}

// mapModel maps generic model names to OpenAI model names
func (a *OpenAIAdapter) mapModel(model string) string {
	// Map common model names to OpenAI model names
	modelMap := map[string]string{
		"gpt-4":           openai.GPT4,
		"gpt-4-turbo":     "gpt-4-turbo-preview",
		"gpt-3.5-turbo":   openai.GPT3Dot5Turbo,
		"gpt-4o":          "gpt-4o",
		"gpt-4o-mini":     "gpt-4o-mini",
	}

	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model
}

// handleError converts OpenAI errors to adapter errors
func (a *OpenAIAdapter) handleError(err error) error {
	if err == nil {
		return nil
	}

	// Check for specific error types
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 429:
			return ErrRateLimitExceeded
		case 400:
			if apiErr.Code == "context_length_exceeded" {
				return ErrContextLengthExceeded
			}
		case 404:
			if apiErr.Code == "model_not_found" {
				return ErrModelNotSupported
			}
		}
	}

	return fmt.Errorf("openai api error: %w", err)
}