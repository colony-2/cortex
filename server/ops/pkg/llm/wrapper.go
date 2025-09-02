package llm

import (
	"context"
	"encoding/json"

	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// LLMConfig defines the configuration for LLM activities - ALL fields MUST have json tags
type LLMConfig struct {
	Provider string `json:"provider"` // Required: openai, anthropic, gemini
	Model    string `json:"model"`    // Required: model name
}

// LLMInput defines the input for LLM activities - ALL fields MUST have json tags
type LLMInput struct {
	Provider       string          `json:"provider"`                  // Required: openai, anthropic, gemini
	Model          string          `json:"model"`                     // Required: model name
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

func GetOp() ops.RegisterableOp {
	return ops.NewActivityMappedOp(
		ops.OpMetadata{
			Type:           "llm_inference",
			Name:           "llm_inference",
			Description:    "Executes LLM inference with various providers (OpenAI, Anthropic, Gemini)",
			Version:        "1.0.0",
			DefaultTimeout: 5 * time.Minute,
		},
		execute)
}

// Execute runs the activity with provided configuration and inputs
func execute(ctx context.Context, input LLMInput) (LLMOutput, error) {
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
		ModelName:      input.Model,
		AdapterName:    input.Provider,
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
