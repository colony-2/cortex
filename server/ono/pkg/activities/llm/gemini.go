package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// GeminiProvider implements LLM provider for Google Gemini
type GeminiProvider struct {
	apiKey string
	model  string
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	return &GeminiProvider{
		apiKey: apiKey,
		model:  model,
	}
}

// ExecutePrompt executes a prompt using Gemini REST API
func (p *GeminiProvider) ExecutePrompt(ctx context.Context, prompt string, config map[string]interface{}) (map[string]interface{}, error) {
	// Build API URL
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", p.model, p.apiKey)

	// Build request body
	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": prompt,
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{},
	}

	// Configure generation settings
	if temp, ok := config["temperature"].(float64); ok {
		requestBody["generationConfig"].(map[string]interface{})["temperature"] = temp
	}

	// Check if we need structured output (JSON response)
	if _, ok := config["structured_output"].(map[string]interface{}); ok {
		requestBody["generationConfig"].(map[string]interface{})["responseMimeType"] = "application/json"
	}

	// Marshal request
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response content")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text

	// If we expect JSON, try to parse it
	if responseMimeType, ok := requestBody["generationConfig"].(map[string]interface{})["responseMimeType"]; ok && responseMimeType == "application/json" {
		var jsonResult map[string]interface{}
		if err := json.Unmarshal([]byte(responseText), &jsonResult); err != nil {
			return nil, fmt.Errorf("failed to parse JSON response: %w", err)
		}
		return jsonResult, nil
	}

	// Return as string result
	return map[string]interface{}{
		"result": responseText,
	}, nil
}

// LoadGeminiAPIKey loads the Gemini API key from file
func LoadGeminiAPIKey() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	keyPath := fmt.Sprintf("%s/gemini.key", homeDir)
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read API key from %s: %w", keyPath, err)
	}

	// Trim any whitespace
	apiKey := string(data)
	if len(apiKey) > 0 && apiKey[len(apiKey)-1] == '\n' {
		apiKey = apiKey[:len(apiKey)-1]
	}

	return apiKey, nil
}