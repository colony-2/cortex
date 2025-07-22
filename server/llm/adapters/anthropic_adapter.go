package llmadapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// AnthropicAdapter implements the Adapter interface for Anthropic Claude API
type AnthropicAdapter struct {
	apiKey      string
	httpClient  *http.Client
	rateLimiter *rate.Limiter
	baseURL     string
}

// NewAnthropicAdapter creates a new Anthropic adapter
func NewAnthropicAdapter(apiKey string) (*AnthropicAdapter, error) {
	if apiKey == "" {
		// Try to get from environment variable
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, ErrAPIKeyMissing
		}
	}

	// Create rate limiter: 50 requests per minute
	rateLimiter := rate.NewLimiter(rate.Every(time.Minute/50), 50)

	return &AnthropicAdapter{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimiter: rateLimiter,
		baseURL:     "https://api.anthropic.com/v1",
	}, nil
}

// anthropicRequest represents the request structure for Anthropic API
type anthropicRequest struct {
	Model         string                   `json:"model"`
	Messages      []anthropicMessage       `json:"messages"`
	System        string                   `json:"system,omitempty"`
	MaxTokens     int                      `json:"max_tokens"`
	Temperature   float64                  `json:"temperature,omitempty"`
	TopP          float64                  `json:"top_p,omitempty"`
	StopSequences []string                 `json:"stop_sequences,omitempty"`
	Stream        bool                     `json:"stream,omitempty"`
	Tools         []anthropicTool          `json:"tools,omitempty"`
	Metadata      map[string]interface{}   `json:"metadata,omitempty"`
}

// anthropicMessage represents a message in the Anthropic format
type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content anthropicContent       `json:"content"`
}

// anthropicContent can be either a string or an array of content blocks
type anthropicContent interface{}

// anthropicTextContent represents a text content block
type anthropicTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicToolUseContent represents a tool use content block
type anthropicToolUseContent struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// anthropicTool represents a tool in Anthropic format
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicResponse represents the response from Anthropic API
type anthropicResponse struct {
	ID           string                    `json:"id"`
	Type         string                    `json:"type"`
	Role         string                    `json:"role"`
	Content      []anthropicContentBlock   `json:"content"`
	Model        string                    `json:"model"`
	StopReason   string                    `json:"stop_reason"`
	StopSequence string                    `json:"stop_sequence"`
	Usage        anthropicUsage            `json:"usage"`
}

// anthropicContentBlock represents a content block in the response
type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// anthropicUsage represents token usage information
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicStreamEvent represents a streaming event
type anthropicStreamEvent struct {
	Type         string                  `json:"type"`
	Message      *anthropicResponse      `json:"message,omitempty"`
	ContentBlock *anthropicContentBlock  `json:"content_block,omitempty"`
	Delta        *anthropicDelta         `json:"delta,omitempty"`
	Index        int                     `json:"index,omitempty"`
	Error        map[string]interface{}  `json:"error,omitempty"`
}

// anthropicDelta represents a delta update in streaming
type anthropicDelta struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Generate creates a completion for the given prompt
func (a *AnthropicAdapter) Generate(ctx context.Context, prompt string, config Config) (Response, error) {
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

	// Build request
	req := anthropicRequest{
		Model: a.mapModel(config.Model),
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   config.MaxTokens,
		Temperature: config.Temperature,
	}

	if config.SystemPrompt != "" {
		req.System = config.SystemPrompt
	}

	if config.TopP > 0 {
		req.TopP = config.TopP
	}

	if len(config.StopSequences) > 0 {
		req.StopSequences = config.StopSequences
	}

	if config.Metadata != nil {
		req.Metadata = config.Metadata
	}

	// Make API call
	resp, err := a.makeRequest(ctx, req)
	if err != nil {
		return Response{}, err
	}

	// Extract content
	content := ""
	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
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
		Model:        resp.Model,
		Metadata:     config.Metadata,
	}, nil
}

// GenerateWithTools creates a completion with tool/function calling support
func (a *AnthropicAdapter) GenerateWithTools(ctx context.Context, prompt string, tools []Tool, config Config) (Response, error) {
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

	// Convert tools to Anthropic format
	anthropicTools := make([]anthropicTool, len(tools))
	for i, tool := range tools {
		anthropicTools[i] = anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		}
	}

	// Build request
	req := anthropicRequest{
		Model: a.mapModel(config.Model),
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   config.MaxTokens,
		Temperature: config.Temperature,
		Tools:       anthropicTools,
	}

	if config.SystemPrompt != "" {
		req.System = config.SystemPrompt
	}

	if config.TopP > 0 {
		req.TopP = config.TopP
	}

	if len(config.StopSequences) > 0 {
		req.StopSequences = config.StopSequences
	}

	// Make API call
	resp, err := a.makeRequest(ctx, req)
	if err != nil {
		return Response{}, err
	}

	// Extract content and tool calls
	content := ""
	var toolCalls []ToolCall
	
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			toolCalls = append(toolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return Response{
		Content:   content,
		ToolCalls: toolCalls,
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
		FinishReason: resp.StopReason,
		Model:        resp.Model,
	}, nil
}

// StreamGenerate creates a streaming completion for the given prompt
func (a *AnthropicAdapter) StreamGenerate(ctx context.Context, prompt string, config Config) (<-chan Token, error) {
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

	// Build request
	req := anthropicRequest{
		Model: a.mapModel(config.Model),
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   config.MaxTokens,
		Temperature: config.Temperature,
		Stream:      true,
	}

	if config.SystemPrompt != "" {
		req.System = config.SystemPrompt
	}

	if config.TopP > 0 {
		req.TopP = config.TopP
	}

	if len(config.StopSequences) > 0 {
		req.StopSequences = config.StopSequences
	}

	// Create output channel
	tokenChan := make(chan Token)

	// Start goroutine to handle streaming
	go func() {
		defer close(tokenChan)
		
		if err := a.makeStreamRequest(ctx, req, tokenChan); err != nil {
			select {
			case tokenChan <- Token{Error: err}:
			case <-ctx.Done():
			}
		}
	}()

	return tokenChan, nil
}

// makeRequest makes a non-streaming request to the Anthropic API
func (a *AnthropicAdapter) makeRequest(ctx context.Context, req anthropicRequest) (*anthropicResponse, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, a.handleHTTPError(resp)
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &anthropicResp, nil
}

// makeStreamRequest makes a streaming request to the Anthropic API
func (a *AnthropicAdapter) makeStreamRequest(ctx context.Context, req anthropicRequest, tokenChan chan<- Token) error {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return a.handleHTTPError(resp)
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventData string
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Skip empty lines
		if line == "" {
			continue
		}
		
		// Parse SSE format
		if strings.HasPrefix(line, "data: ") {
			eventData = strings.TrimPrefix(line, "data: ")
			
			// Parse the JSON data
			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(eventData), &event); err != nil {
				return fmt.Errorf("failed to decode stream event: %w", err)
			}
			
			// Handle different event types
			switch event.Type {
			case "content_block_delta":
				if event.Delta != nil && event.Delta.Type == "text_delta" {
					select {
					case tokenChan <- Token{Content: event.Delta.Text}:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			case "message_stop":
				return nil
			case "error":
				return fmt.Errorf("stream error: %v", event.Error)
			}
		}
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	
	return nil
}

// validateConfig validates the configuration
func (a *AnthropicAdapter) validateConfig(config Config) error {
	if config.Model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidConfig)
	}

	if config.MaxTokens <= 0 {
		config.MaxTokens = 1024 // Default max tokens for Anthropic
	}

	if config.Temperature < 0 || config.Temperature > 1 {
		return fmt.Errorf("%w: temperature must be between 0 and 1", ErrInvalidConfig)
	}

	if config.TopP < 0 || config.TopP > 1 {
		return fmt.Errorf("%w: top_p must be between 0 and 1", ErrInvalidConfig)
	}

	return nil
}

// mapModel maps generic model names to Anthropic model names
func (a *AnthropicAdapter) mapModel(model string) string {
	// Map common model names to Anthropic model names
	modelMap := map[string]string{
		"claude-3-opus":    "claude-3-opus-20240229",
		"claude-3-sonnet":  "claude-3-sonnet-20240229",
		"claude-3-haiku":   "claude-3-haiku-20240307",
		"claude-2.1":       "claude-2.1",
		"claude-2":         "claude-2.0",
		"claude-instant":   "claude-instant-1.2",
	}

	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model
}

// handleHTTPError handles HTTP error responses
func (a *AnthropicAdapter) handleHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	
	var errResp struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	
	if err := json.Unmarshal(body, &errResp); err == nil {
		switch resp.StatusCode {
		case 429:
			return ErrRateLimitExceeded
		case 400:
			if errResp.Error.Type == "invalid_request_error" {
				return fmt.Errorf("%w: %s", ErrInvalidConfig, errResp.Error.Message)
			}
		case 404:
			return ErrModelNotSupported
		}
	}

	return fmt.Errorf("anthropic api error: status %d, body: %s", resp.StatusCode, body)
}