package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	
	"vibethis/ono/internal/recipe"
)

// NewJobDescribeCommand creates the job describe command
func NewJobDescribeCommand() *cobra.Command {
	var (
		format     string
		verbose    bool
		namespace  string
		serverAddr string
	)

	cmd := &cobra.Command{
		Use:   "describe <recipe-name> <job-id>",
		Short: "Show detailed information about a specific job execution",
		Long: `Shows detailed information about a job including its metadata, input parameters,
current/final outputs, and activity execution details.`,
		Example: `  # Describe a job
  ono job describe my-recipe job-123

  # Show verbose activity information
  ono job describe my-recipe job-123 --verbose

  # Output in JSON format
  ono job describe my-recipe job-123 --format json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobDescribe(cmd, args[0], args[1], format, verbose, namespace, serverAddr)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, json")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed activity information")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Temporal namespace")
	cmd.Flags().StringVar(&serverAddr, "address", "localhost:7233", "Temporal server address")

	return cmd
}

func runJobDescribe(cmd *cobra.Command, recipeName, jobID string, format string, verbose bool, namespace, serverAddr string) error {
	// Get recipe directory from config
	recipesDir := getRecipesDir()
	
	// Create logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create recipe registry to get recipe details
	registry, err := recipe.NewRegistry(logger, recipesDir, nil)
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

	// Connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort:  serverAddr,
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	// Get workflow execution details
	ctx := context.Background()
	desc, err := c.DescribeWorkflowExecution(ctx, jobID, "")
	if err != nil {
		return fmt.Errorf("failed to describe workflow execution: %w", err)
	}

	// Build job information
	job := &recipe.Job{
		ID:            jobID,
		RecipeName:    recipeName,
		RecipeVersion: r.Version,
		StartTime:     desc.WorkflowExecutionInfo.StartTime.AsTime(),
		WorkflowID:    desc.WorkflowExecutionInfo.Execution.WorkflowId,
		RunID:         desc.WorkflowExecutionInfo.Execution.RunId,
	}

	// Set status
	switch desc.WorkflowExecutionInfo.Status {
	case 1: // Running
		job.Status = recipe.JobStatusRunning
	case 2: // Completed
		job.Status = recipe.JobStatusCompleted
		if desc.WorkflowExecutionInfo.CloseTime != nil {
			endTime := desc.WorkflowExecutionInfo.CloseTime.AsTime()
			job.EndTime = &endTime
		}
	case 3: // Failed
		job.Status = recipe.JobStatusFailed
		if desc.WorkflowExecutionInfo.CloseTime != nil {
			endTime := desc.WorkflowExecutionInfo.CloseTime.AsTime()
			job.EndTime = &endTime
		}
	case 4: // Canceled
		job.Status = recipe.JobStatusCanceled
		if desc.WorkflowExecutionInfo.CloseTime != nil {
			endTime := desc.WorkflowExecutionInfo.CloseTime.AsTime()
			job.EndTime = &endTime
		}
	case 5: // Terminated
		job.Status = recipe.JobStatusTerminated
		if desc.WorkflowExecutionInfo.CloseTime != nil {
			endTime := desc.WorkflowExecutionInfo.CloseTime.AsTime()
			job.EndTime = &endTime
		}
	}

	// Get activity executions if verbose
	var activities []*recipe.ActivityExecution
	if verbose {
		// TODO: Parse workflow history to extract activity executions
		// For now, we'll just show a placeholder
		activities = []*recipe.ActivityExecution{}
	}

	// Output based on format
	switch format {
	case "text":
		return outputJobText(cmd, job, r, activities)
	case "json":
		return outputJobJSON(cmd, job, activities)
	default:
		return fmt.Errorf("invalid output format: %s", format)
	}
}

func outputJobText(cmd *cobra.Command, job *recipe.Job, r *recipe.Recipe, activities []*recipe.ActivityExecution) error {
	out := cmd.OutOrStdout()

	// Job metadata
	fmt.Fprintf(out, "Job ID: %s\n", job.ID)
	fmt.Fprintf(out, "Recipe: %s (v%s)\n", job.RecipeName, job.RecipeVersion)
	fmt.Fprintf(out, "Status: %s\n", job.Status)
	fmt.Fprintf(out, "Start Time: %s\n", job.StartTime.Format("2006-01-02 15:04:05"))
	
	if job.EndTime != nil {
		fmt.Fprintf(out, "End Time: %s\n", job.EndTime.Format("2006-01-02 15:04:05"))
		duration := job.EndTime.Sub(job.StartTime).Round(time.Second)
		fmt.Fprintf(out, "Duration: %s\n", duration)
	}

	fmt.Fprintln(out)

	// Workflow execution details
	fmt.Fprintln(out, "Execution Details:")
	fmt.Fprintf(out, "  Workflow ID: %s\n", job.WorkflowID)
	fmt.Fprintf(out, "  Run ID: %s\n", job.RunID)

	// Input parameters (if available)
	if len(job.Input) > 0 {
		fmt.Fprintln(out, "\nInput Parameters:")
		for k, v := range job.Input {
			fmt.Fprintf(out, "  %s: %v\n", k, v)
		}
	}

	// Output/Result (if completed)
	if job.Status == recipe.JobStatusCompleted && len(job.Output) > 0 {
		fmt.Fprintln(out, "\nOutputs:")
		for k, v := range job.Output {
			fmt.Fprintf(out, "  %s: %v\n", k, v)
		}
	}

	// Error (if failed)
	if job.Status == recipe.JobStatusFailed && job.Error != "" {
		fmt.Fprintf(out, "\nError: %s\n", job.Error)
	}

	// Activity executions (if verbose)
	if len(activities) > 0 {
		fmt.Fprintln(out, "\nActivity Executions:")
		for i, activity := range activities {
			fmt.Fprintf(out, "  %d. %s\n", i+1, activity.Name)
			fmt.Fprintf(out, "     Status: %s\n", activity.Status)
			if activity.StartTime.After(time.Time{}) {
				fmt.Fprintf(out, "     Started: %s\n", activity.StartTime.Format("15:04:05"))
			}
			if activity.EndTime != nil {
				fmt.Fprintf(out, "     Duration: %s\n", activity.Duration)
			}
			if activity.Error != "" {
				fmt.Fprintf(out, "     Error: %s\n", activity.Error)
			}
		}
	}

	return nil
}

func outputJobJSON(cmd *cobra.Command, job *recipe.Job, activities []*recipe.ActivityExecution) error {
	output := map[string]interface{}{
		"job":        job,
		"activities": activities,
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}