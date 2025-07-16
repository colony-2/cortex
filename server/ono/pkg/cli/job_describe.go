package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	recipecore "github.com/vibethis/server/recipe-core"
	recipehistory "github.com/vibethis/server/recipe-history"
	recipeworker "github.com/vibethis/server/recipe-worker"
	"vibethis/ono/pkg/cli/format"
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
	registry, err := recipeworker.NewRegistry(logger, recipesDir, nil)
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

	// Get workflow execution details
	ctx := context.Background()
	desc, err := c.DescribeWorkflowExecution(ctx, jobID, "")
	if err != nil {
		return fmt.Errorf("failed to describe workflow execution: %w", err)
	}

	// Create transformer to convert workflow data to job
	// Create GetRecipeFunc that uses the registry
	getRecipe := func(name string) (*recipecore.Recipe, error) {
		return registry.GetRecipe(name)
	}
	transformer := recipehistory.NewTransformer(getRecipe)

	// Transform workflow description to job
	job, err := transformer.DescribeWorkflowToJob(desc, recipeName)
	if err != nil {
		return fmt.Errorf("failed to transform workflow description: %w", err)
	}

	// Get activity executions if verbose
	if verbose && len(desc.PendingActivities) == 0 {
		// Get workflow history to extract completed activities
		iter := c.GetWorkflowHistory(ctx, jobID, "", false, 0)
		var events []*history.HistoryEvent
		for iter.HasNext() {
			event, err := iter.Next()
			if err != nil {
				logger.Warn("Failed to iterate workflow history", zap.Error(err))
				break
			}
			events = append(events, event)
		}

		if len(events) > 0 {
			hist := &history.History{
				Events: events,
			}
			activities, err := transformer.HistoryToActivityExecutions(hist, recipeName)
			if err != nil {
				logger.Warn("Failed to transform activity history", zap.Error(err))
			} else {
				job.Activities = activities
			}
		}
	}

	// Output based on format
	switch format {
	case "text":
		return outputJobText(cmd, job)
	case "json":
		return outputJobJSON(cmd, job)
	default:
		return fmt.Errorf("invalid output format: %s", format)
	}
}

func outputJobText(cmd *cobra.Command, job *recipecore.Job) error {
	// Use color output if terminal supports it
	useColor := color.NoColor == false
	formatter := format.NewRecipeFormatter(useColor)

	output := formatter.FormatJobDetail(job)
	fmt.Fprint(cmd.OutOrStdout(), output)

	return nil
}

func outputJobJSON(cmd *cobra.Command, job *recipecore.Job) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(job)
}
