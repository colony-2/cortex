package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	
	"vibethis/ono/internal/recipe"
)

// NewRecipeHistoryCommand creates the recipe history command
func NewRecipeHistoryCommand() *cobra.Command {
	var (
		limit      int
		status     string
		format     string
		namespace  string
		serverAddr string
	)

	cmd := &cobra.Command{
		Use:   "history <recipe-name>",
		Short: "Show job execution history for a recipe",
		Long: `Shows the execution history of jobs for a specific recipe, including
their status, start time, duration, and result summary.`,
		Example: `  # Show recent job history
  ono recipe history my-recipe

  # Show only failed jobs
  ono recipe history my-recipe --status failed

  # Show more jobs
  ono recipe history my-recipe --limit 50

  # Output in JSON format
  ono recipe history my-recipe --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecipeHistory(cmd, args[0], limit, status, format, namespace, serverAddr)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Number of jobs to show")
	cmd.Flags().StringVar(&status, "status", "all", "Filter by status: running, completed, failed, all")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Temporal namespace")
	cmd.Flags().StringVar(&serverAddr, "address", "localhost:7233", "Temporal server address")

	return cmd
}

func runRecipeHistory(cmd *cobra.Command, recipeName string, limit int, status, format, namespace, serverAddr string) error {
	// Get recipe directory from config
	recipesDir := getRecipesDir()
	
	// Create logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create recipe registry to verify recipe exists
	registry, err := recipe.NewRegistry(logger, recipesDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create recipe registry: %w", err)
	}

	// Start registry to discover recipes
	if err := registry.Start(); err != nil {
		return fmt.Errorf("failed to start recipe registry: %w", err)
	}
	defer registry.Stop()

	// Verify the recipe exists
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

	// Build query for listing workflows
	query := fmt.Sprintf(`TaskQueue = "%s-%s"`, "ono-recipes", recipeName)
	
	// Add status filter if specified
	switch status {
	case "running":
		query += ` AND ExecutionStatus = "Running"`
	case "completed":
		query += ` AND ExecutionStatus = "Completed"`
	case "failed":
		query += ` AND ExecutionStatus = "Failed"`
	case "all":
		// No additional filter
	default:
		return fmt.Errorf("invalid status filter: %s", status)
	}

	// List workflow executions
	ctx := context.Background()
	
	listRequest := &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: int32(limit),
		Query:    query,
	}

	resp, err := c.WorkflowService().ListWorkflowExecutions(ctx, listRequest)
	if err != nil {
		return fmt.Errorf("failed to list workflow executions: %w", err)
	}

	workflows := resp.Executions

	// Convert to job format
	jobs := make([]*recipe.Job, len(workflows))
	for i, wf := range workflows {
		job := &recipe.Job{
			ID:          wf.Execution.WorkflowId,
			RecipeName:  recipeName,
			RecipeVersion: r.Version,
			StartTime:   wf.StartTime.AsTime(),
			WorkflowID:  wf.Execution.WorkflowId,
			RunID:       wf.Execution.RunId,
		}

		// Map status
		switch wf.Status {
		case 1: // Running
			job.Status = recipe.JobStatusRunning
		case 2: // Completed
			job.Status = recipe.JobStatusCompleted
			if wf.CloseTime != nil {
				endTime := wf.CloseTime.AsTime()
				job.EndTime = &endTime
			}
		case 3: // Failed
			job.Status = recipe.JobStatusFailed
			if wf.CloseTime != nil {
				endTime := wf.CloseTime.AsTime()
				job.EndTime = &endTime
			}
		case 4: // Canceled
			job.Status = recipe.JobStatusCanceled
			if wf.CloseTime != nil {
				endTime := wf.CloseTime.AsTime()
				job.EndTime = &endTime
			}
		case 5: // Terminated
			job.Status = recipe.JobStatusTerminated
			if wf.CloseTime != nil {
				endTime := wf.CloseTime.AsTime()
				job.EndTime = &endTime
			}
		}

		jobs[i] = job
	}

	// Output results
	switch format {
	case "table":
		return outputJobsTable(cmd, jobs)
	case "json":
		return outputJobsJSON(cmd, jobs)
	default:
		return fmt.Errorf("invalid output format: %s", format)
	}
}

func outputJobsTable(cmd *cobra.Command, jobs []*recipe.Job) error {
	if len(jobs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No jobs found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	defer w.Flush()

	// Header
	fmt.Fprintln(w, "JOB ID\tSTATUS\tSTART TIME\tDURATION\tRESULT SUMMARY")

	// Rows
	for _, job := range jobs {
		duration := "Running"
		if job.EndTime != nil {
			duration = job.EndTime.Sub(job.StartTime).Round(time.Second).String()
		}

		resultSummary := ""
		if job.Status == recipe.JobStatusCompleted {
			resultSummary = "Success"
		} else if job.Status == recipe.JobStatusFailed && job.Error != "" {
			resultSummary = job.Error
			if len(resultSummary) > 50 {
				resultSummary = resultSummary[:47] + "..."
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			job.ID,
			job.Status,
			job.StartTime.Format("2006-01-02 15:04:05"),
			duration,
			resultSummary,
		)
	}

	return nil
}

func outputJobsJSON(cmd *cobra.Command, jobs []*recipe.Job) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(jobs)
}