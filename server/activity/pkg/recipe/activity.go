package recipe

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RecipeActivity handles recipe-to-recipe invocation
type RecipeActivity struct {
	// Recipe path (e.g., "data-processing/transform")
	Recipe string `json:"recipe"`
	
	// Execution configuration
	Timeout      time.Duration          `json:"timeout,omitempty"`
	RetryPolicy  *RetryPolicy           `json:"retry_policy,omitempty"`
	Version      string                 `json:"version,omitempty"`
	
	// Runtime inputs
	Inputs       map[string]interface{} `json:"inputs,omitempty"`
	
	// Context variables
	Context      *RecipeContext         `json:"context,omitempty"`
}

// RetryPolicy defines retry behavior for recipe execution
type RetryPolicy struct {
	MaximumAttempts    int32         `json:"maximum_attempts"`
	InitialInterval    time.Duration `json:"initial_interval"`
	BackoffCoefficient float64       `json:"backoff_coefficient"`
	MaximumInterval    time.Duration `json:"maximum_interval,omitempty"`
}

// RecipeContext contains system-provided context variables
type RecipeContext struct {
	Recipe     RecipeInfo     `json:"recipe"`
	Environment EnvironmentInfo `json:"environment"`
	Execution   ExecutionInfo   `json:"execution"`
	Auth        AuthInfo        `json:"auth"`
}

// RecipeInfo contains information about the current recipe
type RecipeInfo struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	ExecutionID       string `json:"execution_id"`
	ParentExecutionID string `json:"parent_execution_id,omitempty"`
}

// EnvironmentInfo contains environment context
type EnvironmentInfo struct {
	Name    string `json:"name"`    // production, staging, development
	Region  string `json:"region"`
	Cluster string `json:"cluster"`
}

// ExecutionInfo contains execution context
type ExecutionInfo struct {
	Host       string        `json:"host"`
	Namespace  string        `json:"namespace"`
	TaskQueue  string        `json:"task_queue"`
	StartedAt  time.Time     `json:"started_at"`
	Timeout    time.Duration `json:"timeout"`
}

// AuthInfo contains authentication context
type AuthInfo struct {
	Identity string `json:"identity"`
	Token    string `json:"token,omitempty"` // Securely managed
}

// RecipeActivityOutput represents the output from a recipe execution
type RecipeActivityOutput struct {
	ExecutionID       string                 `json:"execution_id"`
	Result            map[string]interface{} `json:"result"`
	Status            string                 `json:"status"`
	ExecutionMetadata ExecutionMetadata      `json:"execution_metadata"`
}

// ExecutionMetadata contains metadata about the recipe execution
type ExecutionMetadata struct {
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	DurationMs   int64     `json:"duration_ms"`
	AttemptCount int       `json:"attempt_count"`
}

// TemporalClientProvider provides access to Temporal client
type TemporalClientProvider interface {
	GetClient(ctx context.Context) (client.Client, error)
}

// DefaultClientProvider uses a singleton Temporal client
type DefaultClientProvider struct {
	client client.Client
}

// NewDefaultClientProvider creates a new default client provider
func NewDefaultClientProvider(c client.Client) *DefaultClientProvider {
	return &DefaultClientProvider{client: c}
}

// GetClient returns the Temporal client
func (p *DefaultClientProvider) GetClient(ctx context.Context) (client.Client, error) {
	if p.client == nil {
		return nil, fmt.Errorf("temporal client not initialized")
	}
	return p.client, nil
}

// globalClientProvider holds the client provider
var globalClientProvider TemporalClientProvider

// SetClientProvider sets the global client provider
func SetClientProvider(provider TemporalClientProvider) {
	globalClientProvider = provider
}

// GetClientProvider returns the global client provider
func GetClientProvider() TemporalClientProvider {
	return globalClientProvider
}

// ExecuteRecipeActivity executes a child recipe as a Temporal workflow
func ExecuteRecipeActivity(ctx context.Context, input RecipeActivity) (*RecipeActivityOutput, error) {
	startTime := time.Now()
	
	// Get Temporal client
	if globalClientProvider == nil {
		return nil, fmt.Errorf("temporal client provider not initialized")
	}
	
	temporalClient, err := globalClientProvider.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get temporal client: %w", err)
	}
	
	// Build workflow options
	workflowOptions := client.StartWorkflowOptions{
		TaskQueue: input.Context.Execution.TaskQueue,
	}
	
	// Set workflow ID based on recipe name and execution context
	if input.Context != nil && input.Context.Recipe.ExecutionID != "" {
		workflowOptions.ID = fmt.Sprintf("%s-child-%s-%d", 
			input.Context.Recipe.ExecutionID, 
			input.Recipe, 
			time.Now().UnixNano())
	}
	
	// Set timeout if specified
	if input.Timeout > 0 {
		workflowOptions.WorkflowExecutionTimeout = input.Timeout
	}
	
	// Set retry policy if specified
	if input.RetryPolicy != nil {
		workflowOptions.RetryPolicy = &temporal.RetryPolicy{
			MaximumAttempts:    input.RetryPolicy.MaximumAttempts,
			InitialInterval:    input.RetryPolicy.InitialInterval,
			BackoffCoefficient: input.RetryPolicy.BackoffCoefficient,
			MaximumInterval:    input.RetryPolicy.MaximumInterval,
		}
	}
	
	// Build workflow inputs with context
	workflowInputs := map[string]interface{}{
		"inputs": input.Inputs,
	}
	
	// Add context if available
	if input.Context != nil {
		// Update parent execution ID for child
		childContext := *input.Context
		childContext.Recipe.ParentExecutionID = input.Context.Recipe.ExecutionID
		childContext.Recipe.Name = input.Recipe
		childContext.Recipe.Version = input.Version
		
		workflowInputs["context"] = childContext
	}
	
	// Start the child workflow
	workflowRun, err := temporalClient.ExecuteWorkflow(
		ctx,
		workflowOptions,
		input.Recipe, // Workflow type is the recipe name
		workflowInputs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start child recipe workflow: %w", err)
	}
	
	// Wait for the workflow to complete
	var result map[string]interface{}
	err = workflowRun.Get(ctx, &result)
	
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	
	// Build output
	output := &RecipeActivityOutput{
		ExecutionID: workflowRun.GetID(),
		Result:      result,
		Status:      "completed",
		ExecutionMetadata: ExecutionMetadata{
			StartTime:    startTime,
			EndTime:      endTime,
			DurationMs:   duration.Milliseconds(),
			AttemptCount: 1, // TODO: Get actual attempt count from workflow
		},
	}
	
	if err != nil {
		output.Status = "failed"
		return output, fmt.Errorf("child recipe workflow failed: %w", err)
	}
	
	return output, nil
}

// ExecuteChildRecipeWorkflow executes a child recipe from within a workflow
// This is used when the recipe activity is called from within another workflow
func ExecuteChildRecipeWorkflow(ctx workflow.Context, input RecipeActivity) (*RecipeActivityOutput, error) {
	startTime := workflow.Now(ctx)
	
	// Build child workflow options
	childOptions := workflow.ChildWorkflowOptions{
		TaskQueue: input.Context.Execution.TaskQueue,
	}
	
	// Set workflow ID
	if input.Context != nil && input.Context.Recipe.ExecutionID != "" {
		childOptions.WorkflowID = fmt.Sprintf("%s-child-%s-%d",
			input.Context.Recipe.ExecutionID,
			input.Recipe,
			workflow.Now(ctx).UnixNano())
	}
	
	// Set timeout if specified
	if input.Timeout > 0 {
		childOptions.WorkflowExecutionTimeout = input.Timeout
	}
	
	// Set retry policy if specified
	if input.RetryPolicy != nil {
		childOptions.RetryPolicy = &temporal.RetryPolicy{
			MaximumAttempts:    input.RetryPolicy.MaximumAttempts,
			InitialInterval:    input.RetryPolicy.InitialInterval,
			BackoffCoefficient: input.RetryPolicy.BackoffCoefficient,
			MaximumInterval:    input.RetryPolicy.MaximumInterval,
		}
	}
	
	// Build workflow inputs with context
	workflowInputs := map[string]interface{}{
		"inputs": input.Inputs,
	}
	
	// Add context if available
	if input.Context != nil {
		// Update parent execution ID for child
		childContext := *input.Context
		childContext.Recipe.ParentExecutionID = input.Context.Recipe.ExecutionID
		childContext.Recipe.Name = input.Recipe
		childContext.Recipe.Version = input.Version
		childContext.Recipe.ExecutionID = workflow.GetInfo(ctx).WorkflowExecution.ID
		
		workflowInputs["context"] = childContext
	}
	
	// Execute child workflow
	ctx = workflow.WithChildOptions(ctx, childOptions)
	
	var result map[string]interface{}
	future := workflow.ExecuteChildWorkflow(ctx, input.Recipe, workflowInputs)
	
	err := future.Get(ctx, &result)
	
	endTime := workflow.Now(ctx)
	duration := endTime.Sub(startTime)
	
	// Get workflow execution info
	var executionInfo workflow.Execution
	_ = future.GetChildWorkflowExecution().Get(ctx, &executionInfo)
	
	// Build output
	output := &RecipeActivityOutput{
		ExecutionID: executionInfo.ID,
		Result:      result,
		Status:      "completed",
		ExecutionMetadata: ExecutionMetadata{
			StartTime:    startTime,
			EndTime:      endTime,
			DurationMs:   duration.Milliseconds(),
			AttemptCount: 1, // TODO: Get actual attempt count
		},
	}
	
	if err != nil {
		output.Status = "failed"
		return output, fmt.Errorf("child recipe workflow failed: %w", err)
	}
	
	return output, nil
}