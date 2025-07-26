package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	
	worker "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
)

// NewJobRestartCommand creates the job restart command
func NewJobRestartCommand() *cobra.Command {
	var (
		fromActivity string
		inputJSON    string
		newJobID     string
		namespace    string
		serverAddr   string
	)

	cmd := &cobra.Command{
		Use:   "restart <recipe-name> <job-id>",
		Short: "Restart a failed job from a specific point",
		Long: `Restarts a failed job execution, optionally from a specific activity.

This is useful for retrying jobs that failed due to transient errors or
after fixing the underlying issue that caused the failure.`,
		Example: `  # Restart a failed job from the beginning
  ono job restart my-recipe job-123

  # Restart from a specific activity
  ono job restart my-recipe job-123 --from-activity process-data

  # Restart with different input parameters
  ono job restart my-recipe job-123 --input '{"retryCount": 2}'

  # Restart with a new job ID
  ono job restart my-recipe job-123 --new-job-id job-123-retry`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobRestart(cmd, args[0], args[1], fromActivity, inputJSON, newJobID, namespace, serverAddr)
		},
	}

	cmd.Flags().StringVar(&fromActivity, "from-activity", "", "Restart from specific activity name")
	cmd.Flags().StringVar(&inputJSON, "input", "", "Override input parameters as JSON")
	cmd.Flags().StringVar(&newJobID, "new-job-id", "", "Custom ID for restarted job")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Temporal namespace")
	cmd.Flags().StringVar(&serverAddr, "address", "localhost:7233", "Temporal server address")

	return cmd
}

func runJobRestart(cmd *cobra.Command, recipeName, jobID, fromActivity, inputJSON, newJobID string, namespace, serverAddr string) error {
	// Get recipe directory from config
	recipesDir := getRecipesDir()
	
	// Create logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create recipe registry to get recipe details
	registry, err := worker.NewRegistry(logger, recipesDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create recipe registry: %w", err)
	}

	// Start registry to discover recipes
	if err := registry.Start(); err != nil {
		return fmt.Errorf("failed to start recipe registry: %w", err)
	}
	defer registry.Stop()

	// Get the recipe
	r, err := registry.GetRecipe(recipeName)
	if err != nil {
		return fmt.Errorf("recipe not found: %w", err)
	}

	if r.Workflow == nil {
		return fmt.Errorf("recipe does not have a workflow defined")
	}

	// Connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort:  serverAddr,
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	// Get the original workflow execution details
	ctx := context.Background()
	desc, err := c.DescribeWorkflowExecution(ctx, jobID, "")
	if err != nil {
		return fmt.Errorf("failed to describe original workflow execution: %w", err)
	}

	// Check if the workflow is in a state that can be restarted
	if desc.WorkflowExecutionInfo.Status == 1 { // Running
		return fmt.Errorf("cannot restart a running job")
	}

	// Parse override inputs if provided
	inputs := make(map[string]interface{})
	if inputJSON != "" {
		if err := json.Unmarshal([]byte(inputJSON), &inputs); err != nil {
			return fmt.Errorf("invalid JSON input: %w", err)
		}
	} else {
		// TODO: Extract original inputs from workflow history
		// For now, we'll use empty inputs
	}

	// Generate new job ID if not provided
	if newJobID == "" {
		newJobID = fmt.Sprintf("%s-restart-%s", jobID, uuid.New().String()[:8])
	}

	// Get the task queue for this recipe
	taskQueue := fmt.Sprintf("ono-recipes-%s", recipeName)

	// Start new workflow execution
	workflowOptions := client.StartWorkflowOptions{
		ID:        newJobID,
		TaskQueue: taskQueue,
		// TODO: Configure based on restart point
	}

	we, err := c.ExecuteWorkflow(ctx, workflowOptions, r.Workflow.Name, inputs)
	if err != nil {
		return fmt.Errorf("failed to start restarted workflow: %w", err)
	}

	// Output result
	fmt.Fprintf(cmd.OutOrStdout(), "Job restarted successfully!\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Original Job ID: %s\n", jobID)
	fmt.Fprintf(cmd.OutOrStdout(), "  New Job ID: %s\n", newJobID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Workflow ID: %s\n", we.GetID())
	fmt.Fprintf(cmd.OutOrStdout(), "  Run ID: %s\n", we.GetRunID())
	
	if fromActivity != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Restart Point: Activity '%s'\n", fromActivity)
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: Restart from specific activity is not yet implemented.")
		fmt.Fprintln(cmd.OutOrStdout(), "The job has been restarted from the beginning.")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nUse 'ono job describe %s %s' to monitor the restarted job.\n", recipeName, newJobID)

	return nil
}