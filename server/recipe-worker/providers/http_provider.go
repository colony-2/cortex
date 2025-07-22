package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPConfig defines the configuration for HTTP activities
type HTTPConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Timeout int               `json:"timeout,omitempty"` // seconds
}

// HTTPInput defines the input for HTTP activities
type HTTPInput struct {
	Body        interface{}       `json:"body,omitempty"`
	QueryParams map[string]string `json:"queryParams,omitempty"`
}

// HTTPOutput defines the output for HTTP activities
type HTTPOutput struct {
	StatusCode int                    `json:"statusCode"`
	Headers    map[string][]string    `json:"headers"`
	Body       interface{}            `json:"body"`
}

// HTTPProvider implements the HTTP activity type
type HTTPProvider struct{}

func (p *HTTPProvider) GetType() string {
	return "http"
}

func (p *HTTPProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("expected 2 arguments (config, input), got %d", len(args))
	}
	
	// Convert config from map to HTTPConfig
	configMap, ok := args[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected map[string]interface{}")
	}
	
	configBytes, err := json.Marshal(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	
	var config HTTPConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Convert input from map to HTTPInput
	inputMap, ok := args[1].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid input type: expected map[string]interface{}")
	}
	
	inputBytes, err := json.Marshal(inputMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}
	
	var input HTTPInput
	if err := json.Unmarshal(inputBytes, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}
	
	// Execute the HTTP request
	return p.executeHTTP(ctx, config, input)
}

func (p *HTTPProvider) executeHTTP(ctx context.Context, config HTTPConfig, input HTTPInput) (*HTTPOutput, error) {
	// Create HTTP client with timeout
	timeout := 30 * time.Second
	if config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}
	
	client := &http.Client{
		Timeout: timeout,
	}
	
	// Prepare request body
	var body io.Reader
	if input.Body != nil {
		jsonBody, err := json.Marshal(input.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewReader(jsonBody)
	}
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}
	
	// Set query parameters
	if len(input.QueryParams) > 0 {
		q := req.URL.Query()
		for key, value := range input.QueryParams {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}
	
	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Try to parse response as JSON, fallback to string
	var responseBody interface{}
	if err := json.Unmarshal(respBody, &responseBody); err != nil {
		responseBody = string(respBody)
	}
	
	return &HTTPOutput{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       responseBody,
	}, nil
}

// NewHTTPProvider creates a new HTTP activity provider
func NewHTTPProvider() *HTTPProvider {
	return &HTTPProvider{}
}