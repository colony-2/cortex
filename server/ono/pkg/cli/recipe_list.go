package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	recipecore "github.com/vibethis/server/recipe-core"
	recipeworker "github.com/vibethis/server/recipe-worker"
	"vibethis/ono/pkg/cli/format"
)

// NewRecipeListCommand creates the recipe list command
func NewRecipeListCommand() *cobra.Command {
	var (
		format string
		status string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available recipes",
		Long: `Lists all discovered recipes from the recipe configuration directory.

The output includes recipe name, version, status, worker status, and last update time.`,
		Example: `  # List all recipes in table format
  ono recipe list

  # List recipes in JSON format
  ono recipe list --format json

  # List only recipes with running workers
  ono recipe list --status current`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecipeList(cmd, format, status)
		},
	}

	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")
	cmd.Flags().StringVar(&status, "status", "all", "Filter by worker status: current, removed, all")

	return cmd
}

func runRecipeList(cmd *cobra.Command, format, status string) error {
	// Get recipe directory from config
	recipesDir := getRecipesDir()

	// Create logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create recipe registry (without worker manager for listing)
	registry, err := recipeworker.NewRegistry(logger, recipesDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create recipe registry: %w", err)
	}

	// Start registry to discover recipes
	if err := registry.Start(); err != nil {
		return fmt.Errorf("failed to start recipe registry: %w", err)
	}
	defer registry.Stop()

	// Allow time for discovery
	time.Sleep(100 * time.Millisecond)

	// Build filter
	var filter *recipecore.RecipeFilter
	switch status {
	case "current":
		running := recipecore.WorkerStatusRunning
		filter = &recipecore.RecipeFilter{
			Status:         &running,
			IncludeRemoved: false,
		}
	case "removed":
		stopped := recipecore.WorkerStatusStopped
		filter = &recipecore.RecipeFilter{
			Status:         &stopped,
			IncludeRemoved: true,
		}
	case "all":
		filter = &recipecore.RecipeFilter{
			IncludeRemoved: true,
		}
	default:
		return fmt.Errorf("invalid status filter: %s", status)
	}

	// List recipes
	recipes, err := registry.ListRecipes(filter)
	if err != nil {
		return fmt.Errorf("failed to list recipes: %w", err)
	}

	// Output results
	switch format {
	case "table":
		return outputRecipesTable(cmd, recipes)
	case "json":
		return outputRecipesJSON(cmd, recipes)
	default:
		return fmt.Errorf("invalid output format: %s", format)
	}
}

func outputRecipesTable(cmd *cobra.Command, recipes []*recipecore.Recipe) error {
	// Use color output if terminal supports it
	useColor := color.NoColor == false
	formatter := format.NewRecipeFormatter(useColor)

	output := formatter.FormatRecipeList(recipes)
	fmt.Fprint(cmd.OutOrStdout(), output)

	return nil
}

func outputRecipesJSON(cmd *cobra.Command, recipes []*recipecore.Recipe) error {
	// Create simplified output for JSON
	type recipeOutput struct {
		Name         string    `json:"name"`
		Version      string    `json:"version"`
		Description  string    `json:"description"`
		Status       string    `json:"status"`
		WorkerStatus string    `json:"worker_status"`
		LastModified time.Time `json:"last_modified"`
	}

	output := make([]recipeOutput, len(recipes))
	for i, r := range recipes {
		status := "active"
		if r.WorkerStatus == recipecore.WorkerStatusStopped {
			status = "removed"
		}

		output[i] = recipeOutput{
			Name:         r.Name,
			Version:      r.Version,
			Description:  r.Description,
			Status:       status,
			WorkerStatus: string(r.WorkerStatus),
			LastModified: r.LastModified,
		}
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// getRecipesDir returns the configured recipes directory
func getRecipesDir() string {
	// Check environment variable first
	if dir := os.Getenv("ONO_RECIPE_DIR"); dir != "" {
		return dir
	}

	// Default to ~/.ono/recipes
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".ono/recipes"
	}

	return filepath.Join(homeDir, ".ono", "recipes")
}
