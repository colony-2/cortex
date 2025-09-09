package llm

import (
	"context"
	"encoding/json"
	"fmt"

	f2 "github.com/divisive-ai/vibethis/server/core/pkg/file"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"

	"time"

	llmadapters "github.com/divisive-ai/vibethis/server/llm/adapters"
)

type LLMInferenceConfig = LLMInferenceInput

// LLMInferenceInput defines enhanced input (backward compatible)
type LLMInferenceInput struct {
	Provider      string            `yaml:"default_provider" json:"default_provider,omitempty"`
	Model         string            `yaml:"default_model" json:"default_model,omitempty"`
	APIKeys       map[string]string `yaml:"api_keys" json:"api_keys,omitempty"`
	Temperature   float64           `json:"temperature,omitempty"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	TopP          float64           `json:"top_p,omitempty"`
	StopSequences []string          `json:"stop_sequences,omitempty"`

	EnableSandbox   bool     `yaml:"enable_sandbox" json:"enable_sandbox,omitempty"`
	AllowedPaths    []string `yaml:"allowed_paths" json:"allowed_paths,omitempty"`
	RestrictedPaths []string `yaml:"restricted_paths" json:"restricted_paths,omitempty"`

	// File handling settings
	DefaultFileHandling string `yaml:"default_file_handling" json:"default_file_handling,omitempty"`
	MaxFileContextSize  int    `yaml:"max_file_context_size" json:"max_file_context_size,omitempty"`

	Prompt         string          `json:"prompt,omitempty"`
	SystemPrompt   string          `json:"system_prompt,omitempty"`
	ResponseSchema json.RawMessage `json:"response_schema,omitempty"`

	// Enhanced fields (new)
	Files               []f2.File              `json:"files,omitempty"`
	FileHandling        string                 `json:"file_handling,omitempty"` // native, text_fallback, hybrid
	Tools               []ToolDefinition       `json:"tools,omitempty"`
	ExecuteTools        bool                   `json:"execute_tools,omitempty"`
	DefaultWorkingDir   string                 `yaml:"default_working_dir" json:"default_working_dir,omitempty"`
	ToolWorkingDir      string                 `json:"tool_working_dir,omitempty"`
	EnableToolExecution bool                   `yaml:"enable_tool_execution" json:"enable_tool_execution,omitempty"`
	MaxToolRounds       int                    `yaml:"max_tool_rounds" json:"max_tool_rounds,omitempty"`
	ContinueOnToolError bool                   `json:"continue_on_tool_error,omitempty"`
	ToolTimeout         string                 `json:"tool_timeout,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

// ToolDefinition defines a tool for LLM
type ToolDefinition struct {
	Name        string          `json:"name" validate:"required"`
	Description string          `json:"description" validate:"required"`
	Parameters  json.RawMessage `json:"parameters" validate:"required"`
}

// LLMInferenceOutput defines enhanced output (backward compatible)
type LLMInferenceOutput struct {
	// Standard fields (existing)
	Response     json.RawMessage `json:"response"`
	Model        string          `json:"model"`
	FinishReason string          `json:"finish_reason"`
	Usage        Usage           `json:"usage"`

	// Tool execution fields (new)
	ToolCalls           []ToolCallInfo `json:"tool_calls,omitempty"`
	ToolResults         []ToolResult   `json:"tool_results,omitempty"`
	ToolExecutionErrors []string       `json:"tool_execution_errors,omitempty"`
	ToolRoundsUsed      int            `json:"tool_rounds_used,omitempty"`

	// File operation tracking (new)
	FilesWritten []string `json:"files_written,omitempty"`
	FilesRead    []string `json:"files_read,omitempty"`
	FilesDeleted []string `json:"files_deleted,omitempty"`

	// Enhanced metadata
	ExecutionTime    int64                  `json:"execution_time_ms,omitempty"`
	ProviderMetadata map[string]interface{} `json:"provider_metadata,omitempty"`

	// Backward compatibility fields
	Telemetry LLMTelemetry `json:"telemetry,omitempty"`
}

// Usage tracks token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolCallInfo describes a tool call
type ToolCallInfo struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	Timestamp int64                  `json:"timestamp"`
}

// ToolResult describes the result of a tool execution
type ToolResult struct {
	ToolCallID string      `json:"tool_call_id"`
	ToolName   string      `json:"tool_name"`
	Success    bool        `json:"success"`
	Result     interface{} `json:"result,omitempty"`
	Error      string      `json:"error,omitempty"`
	Duration   int64       `json:"duration_ms"`
}

// EnhancedLLMInferenceActivity implements RegisterableOp
type EnhancedLLMInferenceActivity struct {
	registry     llmadapters.Registry
	toolExecutor *FileToolExecutor
	sandbox      *SecuritySandbox
	config       *LLMInferenceConfig
}

func GetEnhancedOp() ops.RegisterableOp {
	e := &EnhancedLLMInferenceActivity{}
	return ops.NewActivityMappedOp(
		ops.OpMetadata{
			Type:           "llm_inference2",
			Description:    "Executes LLM inference with various providers (OpenAI, Anthropic, Gemini)",
			Version:        "1.0.0",
			DefaultTimeout: 5 * time.Minute,
		},
		e.Execute)
}

// GetMetadata returns activity metadata
func (a *EnhancedLLMInferenceActivity) GetMetadata() ops.OpMetadata {
	return ops.OpMetadata{
		Type:        "llm_inference", // Same type for backward compatibility
		Description: "Enhanced LLM inference with file and tool support",
		Version:     "2.0.0",
	}
}

// Execute runs the enhanced LLM inference activity
func (a *EnhancedLLMInferenceActivity) Execute(
	ctx context.Context,
	input LLMInferenceInput,
) (LLMInferenceOutput, error) {

	config := LLMInferenceConfig{}
	startTime := time.Now()

	// Store config for use in other methods
	conf := LLMInferenceConfig(input)
	a.config = &conf

	// Validate input
	if err := a.validateInput(input, config); err != nil {
		return LLMInferenceOutput{}, fmt.Errorf("validation failed: %w", err)
	}

	// Initialize components
	if err := a.initializeComponents(input, config); err != nil {
		return LLMInferenceOutput{}, fmt.Errorf("initialization failed: %w", err)
	}

	// Get adapter from registry
	adapter, err := a.registry.Get(input.Provider)
	if err != nil {
		return LLMInferenceOutput{}, fmt.Errorf("failed to get adapter: %w", err)
	}

	// Execute based on capabilities
	var output LLMInferenceOutput

	if len(input.Files) > 0 && len(input.Tools) > 0 {
		output, err = a.executeWithFilesAndTools(ctx, adapter, input)
	} else if len(input.Files) > 0 {
		output, err = a.executeWithFiles(ctx, adapter, input)
	} else if len(input.Tools) > 0 {
		output, err = a.executeWithTools(ctx, adapter, input)
	} else {
		output, err = a.executeBasic(ctx, adapter, input)
	}

	if err != nil {
		return output, err
	}

	output.ExecutionTime = time.Since(startTime).Milliseconds()

	// Add backward compatibility telemetry
	output.Telemetry = LLMTelemetry{
		PromptTokens:     output.Usage.PromptTokens,
		CompletionTokens: output.Usage.CompletionTokens,
		TotalTokens:      output.Usage.TotalTokens,
	}

	return output, nil
}

// validateInput validates the input parameters
func (a *EnhancedLLMInferenceActivity) validateInput(input LLMInferenceInput, config LLMInferenceConfig) error {
	// Validate provider
	if input.Provider == "" {
		return fmt.Errorf("provider is required")
	}

	// Validate model
	if input.Model == "" {
		return fmt.Errorf("model is required")
	}

	// Validate prompt or files
	if input.Prompt == "" && len(input.Files) == 0 {
		return fmt.Errorf("prompt or files required")
	}

	// Validate tool configuration
	if input.ExecuteTools {
		if input.ToolWorkingDir == "" {
			input.ToolWorkingDir = "."
		}

		if !config.EnableToolExecution {
			return fmt.Errorf("tool execution is disabled in configuration")
		}

		for _, tool := range input.Tools {
			if err := a.validateToolDefinition(tool); err != nil {
				return fmt.Errorf("invalid tool %s: %w", tool.Name, err)
			}
		}
	}

	// Validate file handling
	if len(input.Files) > 0 && config.MaxFileContextSize > 0 {
		totalSize := 0
		for _, file := range input.Files {
			totalSize += len(file.Content)
		}

		if totalSize > config.MaxFileContextSize {
			return fmt.Errorf("total file size %d exceeds limit %d", totalSize, config.MaxFileContextSize)
		}
	}

	return nil
}

// validateToolDefinition validates a tool definition
func (a *EnhancedLLMInferenceActivity) validateToolDefinition(tool ToolDefinition) error {
	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if tool.Description == "" {
		return fmt.Errorf("tool description is required")
	}
	if len(tool.Parameters) == 0 {
		return fmt.Errorf("tool parameters are required")
	}

	// Validate JSON schema
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		return fmt.Errorf("invalid tool parameters schema: %w", err)
	}

	return nil
}

// initializeComponents initializes tool executor and sandbox
func (a *EnhancedLLMInferenceActivity) initializeComponents(input LLMInferenceInput, config LLMInferenceConfig) error {
	// Initialize sandbox if enabled
	if config.EnableSandbox {
		a.sandbox = NewSecuritySandbox(SandboxConfig{
			AllowedPaths:    config.AllowedPaths,
			RestrictedPaths: config.RestrictedPaths,
		})
	}

	// Initialize tool executor if tools are provided
	if input.ExecuteTools && len(input.Tools) > 0 {
		a.toolExecutor = NewFileToolExecutor(input.ToolWorkingDir, a.sandbox)
	}

	return nil
}
