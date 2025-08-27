package recipe

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"

	"math/rand"
	"os"
	"strings"
	"time"
)

// RecipeConfig defines the configuration for recipe activities
type RecipeConfig struct {
	Recipe      string             `json:"recipe"`       // Required: Recipe path (e.g., "data-processing/transform")
	Version     string             `json:"version"`      // Optional: Recipe version (e.g., "v2.1.0")
	Timeout     string             `json:"timeout"`      // Optional: Execution timeout (e.g., "30m")
	RetryPolicy *RetryPolicyConfig `json:"retry_policy"` // Optional: Retry configuration
}

// RetryPolicyConfig defines retry configuration in JSON format
type RetryPolicyConfig struct {
	MaximumAttempts    int32   `json:"maximum_attempts"`
	InitialInterval    string  `json:"initial_interval"` // Duration string (e.g., "5s")
	BackoffCoefficient float64 `json:"backoff_coefficient"`
	MaximumInterval    string  `json:"maximum_interval"` // Duration string (e.g., "1m")
}

// RecipeInput defines the input for recipe activities
type RecipeInput map[string]interface{}

// RecipeOutput defines the output from recipe activities
type RecipeOutput struct {
	ExecutionID   string                 `json:"execution_id"`       // ID of the recipe execution
	Result        map[string]interface{} `json:"result"`             // Outputs from the executed recipe
	Status        string                 `json:"status"`             // Final status of the recipe
	RecipeOutputs map[string]interface{} `json:"recipe_outputs"`     // Direct outputs from the recipe
	Metadata      RecipeMetadata         `json:"execution_metadata"` // Execution metadata
}

// RecipeMetadata contains execution metadata
type RecipeMetadata struct {
	StartTime    string `json:"start_time"`    // RFC3339 formatted time
	EndTime      string `json:"end_time"`      // RFC3339 formatted time
	DurationMs   int64  `json:"duration_ms"`   // Duration in milliseconds
	AttemptCount int    `json:"attempt_count"` // Number of attempts
}

// RecipeActivityWrapper implements the RegisterableOp interface
type RecipeActivityWrapper struct {
	isWorkflowContext bool // Indicates if running in workflow context
}

// Ensure we implement the interface
var _ types.RegisterableOp[RecipeConfig, RecipeInput, RecipeOutput] = (*RecipeActivityWrapper)(nil)

// NewRecipeActivity creates a new recipe activity that implements RegisterableOp
func NewRecipeActivity() types.RegisterableOp[RecipeConfig, RecipeInput, RecipeOutput] {
	return &RecipeActivityWrapper{isWorkflowContext: false}
}

// NewRecipeWorkflowActivity creates a recipe activity for use within workflows
func NewRecipeWorkflowActivity() types.RegisterableOp[RecipeConfig, RecipeInput, RecipeOutput] {
	return &RecipeActivityWrapper{isWorkflowContext: true}
}

// GetMetadata returns activity metadata for registration
func (a *RecipeActivityWrapper) GetMetadata() types.OpMetadata {
	return types.OpMetadata{
		Type:           "recipe",
		Name:           "Recipe Invocation",
		Description:    "Invokes another recipe as a child workflow with automatic context propagation",
		Version:        "1.0.0",
		DefaultTimeout: 35 * time.Minute, // Default timeout, can be overridden
		RetryPolicy: &yaml.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    60 * time.Second,
			NonRetryableErrorTypes: []string{
				"InvalidRecipeError",
				"RecipeNotFoundError",
			},
		},
	}
}

// Execute runs the activity with provided configuration and inputs
func (a *RecipeActivityWrapper) Execute(ctx context.Context, config RecipeConfig, input RecipeInput) (RecipeOutput, error) {
	// Parse recipe name and version
	recipeName, version := parseRecipePath(config.Recipe)
	if version == "" {
		version = config.Version
	}

	// Parse timeout
	var timeout time.Duration
	if config.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(config.Timeout)
		if err != nil {
			return RecipeOutput{}, fmt.Errorf("invalid timeout format: %w", err)
		}
	}

	// Convert retry policy
	var retryPolicy *RetryPolicy
	if config.RetryPolicy != nil {
		retryPolicy = &RetryPolicy{
			MaximumAttempts:    config.RetryPolicy.MaximumAttempts,
			BackoffCoefficient: config.RetryPolicy.BackoffCoefficient,
		}

		if config.RetryPolicy.InitialInterval != "" {
			d, err := time.ParseDuration(config.RetryPolicy.InitialInterval)
			if err != nil {
				return RecipeOutput{}, fmt.Errorf("invalid initial_interval format: %w", err)
			}
			retryPolicy.InitialInterval = d
		}

		if config.RetryPolicy.MaximumInterval != "" {
			d, err := time.ParseDuration(config.RetryPolicy.MaximumInterval)
			if err != nil {
				return RecipeOutput{}, fmt.Errorf("invalid maximum_interval format: %w", err)
			}
			retryPolicy.MaximumInterval = d
		}
	}

	// Build context from current execution context
	recipeContext := buildRecipeContext(ctx, a.isWorkflowContext)

	// Build recipe activity input
	activityInput := RecipeActivity{
		Recipe:      recipeName,
		Version:     version,
		Timeout:     timeout,
		RetryPolicy: retryPolicy,
		Inputs:      input,
		Context:     recipeContext,
	}

	// Execute based on context type
	var output *RecipeActivityOutput
	var err error

	// Since workflow.Context and context.Context are different types,
	// we need to handle this differently. The isWorkflowContext flag
	// indicates the intended usage, but we'll use the regular activity
	// execution for now since we're in an activity context
	output, err = ExecuteRecipeActivity(ctx, activityInput)

	if err != nil {
		return RecipeOutput{}, err
	}

	// Convert output to the expected format
	return RecipeOutput{
		ExecutionID:   output.ExecutionID,
		Result:        output.Result,
		Status:        output.Status,
		RecipeOutputs: output.Result, // Recipe outputs are the direct result
		Metadata: RecipeMetadata{
			StartTime:    output.ExecutionMetadata.StartTime.Format(time.RFC3339),
			EndTime:      output.ExecutionMetadata.EndTime.Format(time.RFC3339),
			DurationMs:   output.ExecutionMetadata.DurationMs,
			AttemptCount: output.ExecutionMetadata.AttemptCount,
		},
	}, nil
}

// parseRecipePath parses a recipe path to extract name and optional version
// Format: "recipe-name" or "recipe-name@version"
func parseRecipePath(path string) (name string, version string) {
	parts := strings.Split(path, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return path, ""
}

// buildRecipeContext builds the recipe context from the current execution context
func buildRecipeContext(ctx context.Context, isWorkflow bool) *RecipeContext {
	recipeContext := &RecipeContext{
		Recipe: RecipeInfo{
			Name:        "unknown",
			Version:     "1.0.0",
			ExecutionID: generateExecutionID(),
		},
		Environment: EnvironmentInfo{
			Name:    getEnvOrDefault("ENVIRONMENT", "development"),
			Region:  getEnvOrDefault("REGION", "local"),
			Cluster: getEnvOrDefault("CLUSTER", "default"),
		},
		Execution: ExecutionInfo{
			Host:      getEnvOrDefault("HOSTNAME", "localhost"),
			Namespace: getEnvOrDefault("TEMPORAL_NAMESPACE", "default"),
			TaskQueue: getEnvOrDefault("TEMPORAL_TASK_QUEUE", "recipe-queue"),
			StartedAt: time.Now(),
			Timeout:   30 * time.Minute, // Default timeout
		},
		Auth: AuthInfo{
			Identity: getEnvOrDefault("SERVICE_IDENTITY", "recipe-worker"),
		},
	}

	// If in workflow context, we would extract workflow-specific information
	// However, since this is an activity context, we skip this for now
	// The workflow context handling would be done at the workflow level

	// Try to extract context from input if available
	if ctxValue := ctx.Value("recipe_context"); ctxValue != nil {
		if rc, ok := ctxValue.(*RecipeContext); ok {
			// Merge with existing context
			if rc.Recipe.ExecutionID != "" {
				recipeContext.Recipe.ParentExecutionID = rc.Recipe.ExecutionID
			}
			if rc.Environment.Name != "" {
				recipeContext.Environment = rc.Environment
			}
			if rc.Execution.Namespace != "" {
				recipeContext.Execution.Namespace = rc.Execution.Namespace
				recipeContext.Execution.TaskQueue = rc.Execution.TaskQueue
			}
			if rc.Auth.Identity != "" {
				recipeContext.Auth = rc.Auth
			}
		}
	}

	return recipeContext
}

// generateExecutionID generates a unique execution ID
func generateExecutionID() string {
	return fmt.Sprintf("recipe-%d-%d-%d", time.Now().Unix(), time.Now().Nanosecond(), rand.Int63n(1000000))
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := getEnv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnv is a wrapper to allow testing
var getEnv = func(key string) string {
	return os.Getenv(key)
}

// Schema definitions for the recipe activity

// GetConfigSchema returns the JSON schema for recipe configuration
func GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"recipe": map[string]interface{}{
				"type":        "string",
				"description": "Recipe path (e.g., 'data-processing/transform' or 'recipe@v1.0.0')",
			},
			"version": map[string]interface{}{
				"type":        "string",
				"description": "Recipe version (optional if specified in recipe path)",
			},
			"timeout": map[string]interface{}{
				"type":        "string",
				"description": "Execution timeout (e.g., '30m', '1h')",
			},
			"retry_policy": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"maximum_attempts": map[string]interface{}{
						"type":        "integer",
						"minimum":     1,
						"description": "Maximum number of retry attempts",
					},
					"initial_interval": map[string]interface{}{
						"type":        "string",
						"description": "Initial retry interval (e.g., '5s')",
					},
					"backoff_coefficient": map[string]interface{}{
						"type":        "number",
						"minimum":     1.0,
						"description": "Backoff coefficient for exponential retry",
					},
					"maximum_interval": map[string]interface{}{
						"type":        "string",
						"description": "Maximum retry interval (e.g., '1m')",
					},
				},
			},
		},
		"required": []string{"recipe"},
	}
}

// GetInputSchema returns the JSON schema for recipe inputs
func GetInputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Input parameters for the recipe",
		"additionalProperties": true,
	}
}

// GetOutputSchema returns the JSON schema for recipe outputs
func GetOutputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"execution_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the recipe execution",
			},
			"result": map[string]interface{}{
				"type":        "object",
				"description": "Full result from the recipe execution",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Final status of the recipe execution",
				"enum":        []string{"completed", "failed", "cancelled"},
			},
			"recipe_outputs": map[string]interface{}{
				"type":        "object",
				"description": "Direct outputs from the recipe",
			},
			"execution_metadata": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"start_time": map[string]interface{}{
						"type":        "string",
						"format":      "date-time",
						"description": "Start time of execution",
					},
					"end_time": map[string]interface{}{
						"type":        "string",
						"format":      "date-time",
						"description": "End time of execution",
					},
					"duration_ms": map[string]interface{}{
						"type":        "integer",
						"description": "Duration in milliseconds",
					},
					"attempt_count": map[string]interface{}{
						"type":        "integer",
						"description": "Number of attempts made",
					},
				},
			},
		},
	}
}

// MarshalJSON implements json.Marshaler for RecipeContext
func (rc *RecipeContext) MarshalJSON() ([]byte, error) {
	type Alias RecipeContext
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(rc),
	})
}

// UnmarshalJSON implements json.Unmarshaler for RecipeContext
func (rc *RecipeContext) UnmarshalJSON(data []byte) error {
	type Alias RecipeContext
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(rc),
	}
	return json.Unmarshal(data, aux)
}
