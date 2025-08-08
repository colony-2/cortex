package llm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/divisive-ai/vibethis/server/ops/pkg/types"
)

// LLMConfig defines the configuration for LLM activities - ALL fields MUST have json tags
type LLMConfig struct {
	Provider string `json:"provider"` // Required: openai, anthropic, gemini
	Model    string `json:"model"`    // Required: model name
}

// LLMInput defines the input for LLM activities - ALL fields MUST have json tags
type LLMInput struct {
	Prompt         string          `json:"prompt"`                    // Required: the prompt to send
	SystemPrompt   string          `json:"system_prompt"`             // Optional: system prompt
	Temperature    float64         `json:"temperature"`               // Optional: temperature (0-2)
	MaxTokens      int             `json:"max_tokens"`                // Optional: max tokens to generate
	TopP           float64         `json:"top_p"`                     // Optional: nucleus sampling
	StopSequences  []string        `json:"stop_sequences"`            // Optional: stop sequences
	ResponseSchema json.RawMessage `json:"response_schema,omitempty"` // Optional: JSON schema for structured output
}

// LLMOutput defines the output from LLM activities - ALL fields MUST have json tags
type LLMOutput struct {
	Response     json.RawMessage        `json:"response"`      // The LLM response
	Model        string                 `json:"model"`         // The model used
	FinishReason string                 `json:"finish_reason"` // Why generation stopped
	Usage        map[string]interface{} `json:"usage"`         // Token usage statistics
}

// LLMActivityWrapper implements the RegisterableActivity interface
type LLMActivityWrapper struct{}

// Ensure we implement the interface
var _ types.RegisterableActivity[LLMConfig, LLMInput, LLMOutput] = (*LLMActivityWrapper)(nil)

// NewLLMActivity creates a new LLM activity that implements RegisterableActivity
func NewLLMActivity() types.RegisterableActivity[LLMConfig, LLMInput, LLMOutput] {
	return &LLMActivityWrapper{}
}

// GetMetadata returns activity metadata for registration
func (a *LLMActivityWrapper) GetMetadata() types.ActivityMetadata {
	return types.ActivityMetadata{
		Type:           "llm_inference",
		Name:           "LLM Inference",
		Description:    "Executes LLM inference with various providers (OpenAI, Anthropic, Gemini)",
		Version:        "1.0.0",
		DefaultTimeout: 5 * time.Minute,
		RetryPolicy: &types.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			NonRetryableErrorTypes: []string{
				"InvalidRequestError",
				"AuthenticationError",
			},
		},
	}
}

// Execute runs the activity with provided configuration and inputs
func (a *LLMActivityWrapper) Execute(ctx context.Context, config LLMConfig, input LLMInput) (LLMOutput, error) {
	// Ensure registry is initialized
	if globalRegistry == nil {
		if err := InitializeRegistry(); err != nil {
			return LLMOutput{}, err
		}
	}

	// Build the activity input
	activityInput := LLMActivity{
		Prompt:         input.Prompt,
		SystemPrompt:   input.SystemPrompt,
		ModelName:      config.Model,
		AdapterName:    config.Provider,
		Temperature:    input.Temperature,
		MaxTokens:      input.MaxTokens,
		TopP:           input.TopP,
		StopSequences:  input.StopSequences,
		ResponseSchema: input.ResponseSchema,
	}

	// Execute the LLM task
	output, err := ExecuteLLMTask(ctx, activityInput)
	if err != nil {
		return LLMOutput{}, err
	}

	// Convert telemetry to usage map
	usage := map[string]interface{}{
		"prompt_tokens":      output.Telemetry.PromptTokens,
		"completion_tokens":  output.Telemetry.CompletionTokens,
		"total_tokens":       output.Telemetry.TotalTokens,
		"estimated_cost_usd": output.Telemetry.EstimatedCostUSD,
	}

	return LLMOutput{
		Response:     output.Response,
		Model:        output.Model,
		FinishReason: output.FinishReason,
		Usage:        usage,
	}, nil
}