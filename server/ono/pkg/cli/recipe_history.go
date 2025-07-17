package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/divisive-ai/vibethis/server/ono/pkg/cli/format"
	recipe "github.com/vibethis/server/recipe-core/pkg/recipe"
	history "github.com/vibethis/server/recipe-history/pkg/history"
	worker "github.com/vibethis/server/recipe-worker/pkg/worker"
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
	registry, err := worker.NewRegistry(logger, recipesDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create recipe registry: %w", err)
	}

	// Start registry to discover recipes
	if err := registry.Start(); err != nil {
		return fmt.Errorf("failed to start recipe registry: %w", err)
	}
	defer registry.Stop()

	// Verify the recipe exists
	_, err = registry.GetRecipe(recipeName)
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
		Namespace: namespace,
		PageSize:  int32(limit),
		Query:     query,
	}

	resp, err := c.WorkflowService().ListWorkflowExecutions(ctx, listRequest)
	if err != nil {
		return fmt.Errorf("failed to list workflow executions: %w", err)
	}

	workflows := resp.Executions

	// Create transformer to convert workflow executions to jobs
	// Create GetRecipeFunc that uses the registry
	getRecipe := func(name string) (*recipe.Recipe, error) {
		return registry.GetRecipe(name)
	}
	transformer := history.NewTransformer(getRecipe)

	// Convert workflow executions to jobs
	jobs, err := transformer.WorkflowExecutionsToJobs(workflows, recipeName)
	if err != nil {
		return fmt.Errorf("failed to transform workflow executions: %w", err)
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
	// Use color output if terminal supports it
	useColor := color.NoColor == false
	formatter := format.NewRecipeFormatter(useColor)

	// Get recipe name from first job (all jobs are for same recipe)
	recipeName := ""
	if len(jobs) > 0 {
		recipeName = jobs[0].RecipeName
	}

	output := formatter.FormatJobList(jobs, recipeName)
	fmt.Fprint(cmd.OutOrStdout(), output)

	return nil
}

func outputJobsJSON(cmd *cobra.Command, jobs []*recipe.Job) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(jobs)
}
