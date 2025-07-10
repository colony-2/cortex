package llm

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"vibethis/ono/pkg/yaml"
)

// Provider interface for LLM providers
type Provider interface {
	ExecutePrompt(ctx context.Context, prompt string, config map[string]interface{}) (map[string]interface{}, error)
}

// Executor handles LLM-based activity execution
type Executor struct {
	providers map[string]Provider
}

// NewExecutor creates a new LLM executor
func NewExecutor() *Executor {
	return &Executor{
		providers: make(map[string]Provider),
	}
}

// RegisterProvider registers an LLM provider
func (e *Executor) RegisterProvider(name string, provider Provider) {
	e.providers[name] = provider
}

// ExecuteAIPromptActivity executes an AI prompt activity
func (e *Executor) ExecuteAIPromptActivity(ctx context.Context, activityDef *yaml.ActivityDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
	config := activityDef.Implementation.Config

	// Get provider name
	providerName := "openai" // default
	if p, ok := config["provider"].(string); ok {
		providerName = p
	}

	// Get model
	model := "gpt-4"
	if m, ok := config["model"].(string); ok {
		model = m
	}

	// Handle Gemini provider
	if providerName == "gemini" || strings.HasPrefix(model, "gemini") {
		providerName = "gemini"
		if !strings.HasPrefix(model, "gemini") {
			model = "gemini-2.0-flash-exp" // default Gemini model
		}
	}

	// Get provider
	provider, ok := e.providers[providerName]
	if !ok {
		// Try to create provider on demand
		switch providerName {
		case "gemini":
			apiKey, err := LoadGeminiAPIKey()
			if err != nil {
				return nil, fmt.Errorf("failed to load Gemini API key: %w", err)
			}
			provider = NewGeminiProvider(apiKey, model)
			e.RegisterProvider("gemini", provider)
		default:
			return nil, fmt.Errorf("unsupported LLM provider: %s", providerName)
		}
	}

	// Process prompt template
	promptTemplate, ok := config["prompt"].(string)
	if !ok {
		return nil, fmt.Errorf("prompt is required for AI prompt activity")
	}

	// Render the prompt with inputs
	prompt, err := renderPrompt(promptTemplate, inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to render prompt: %w", err)
	}

	// Execute the prompt
	result, err := provider.ExecutePrompt(ctx, prompt, config)
	if err != nil {
		return nil, fmt.Errorf("failed to execute prompt: %w", err)
	}

	// Map the result to expected output
	outputs := make(map[string]interface{})
	for _, output := range activityDef.Outputs {
		if output.Name == "final_report" && result["result"] != nil {
			outputs["final_report"] = result["result"]
		} else if result[output.Name] != nil {
			outputs[output.Name] = result[output.Name]
		}
	}

	return outputs, nil
}

func renderPrompt(promptTemplate string, inputs map[string]interface{}) (string, error) {
	tmpl, err := template.New("prompt").Funcs(template.FuncMap{
		"json": func(v interface{}) string {
			// Simple JSON marshaling for the template
			return fmt.Sprintf("%v", v)
		},
	}).Parse(promptTemplate)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, inputs); err != nil {
		return "", err
	}

	return buf.String(), nil
}