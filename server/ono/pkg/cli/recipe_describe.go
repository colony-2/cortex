package cli

import (
	"encoding/json"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"vibethis/ono/internal/recipe"
	formatpkg "vibethis/ono/pkg/cli/format"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
)

// NewRecipeDescribeCommand creates the recipe describe command
func NewRecipeDescribeCommand() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "describe <recipe-name>",
		Short: "Show recipe structure and configuration",
		Long: `Shows detailed information about a recipe including its metadata,
input parameters, expected outputs, and workflow structure.`,
		Example: `  # Describe a recipe in text format
  ono recipe describe my-recipe

  # Output in JSON format
  ono recipe describe my-recipe --format json

  # Output in YAML format
  ono recipe describe my-recipe --format yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecipeDescribe(cmd, args[0], format)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, json, yaml")

	return cmd
}

func runRecipeDescribe(cmd *cobra.Command, recipeName, format string) error {
	// Get recipe directory from config
	recipesDir := getRecipesDir()

	// Create logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create recipe registry
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

	// Output based on format
	switch format {
	case "text":
		return outputRecipeText(cmd, r)
	case "json":
		return outputRecipeJSON(cmd, r)
	case "yaml":
		return outputRecipeYAML(cmd, r)
	default:
		return fmt.Errorf("invalid output format: %s", format)
	}
}

func outputRecipeText(cmd *cobra.Command, r *recipe.Recipe) error {
	// Use color output if terminal supports it
	useColor := color.NoColor == false
	formatter := formatpkg.NewRecipeFormatter(useColor)

	output := formatter.FormatRecipeDetail(r)
	fmt.Fprint(cmd.OutOrStdout(), output)

	return nil
}

func outputRecipeJSON(cmd *cobra.Command, r *recipe.Recipe) error {
	// Create a simplified structure for JSON output
	type recipeOutput struct {
		Name         string                       `json:"name"`
		Version      string                       `json:"version"`
		Description  string                       `json:"description"`
		Status       string                       `json:"status"`
		WorkerStatus string                       `json:"worker_status"`
		LastModified string                       `json:"last_modified"`
		Workflow     *yamlpkg.WorkflowDefinition  `json:"workflow,omitempty"`
		Activities   []yamlpkg.ActivityDefinition `json:"activities,omitempty"`
	}

	output := recipeOutput{
		Name:         r.Name,
		Version:      r.Version,
		Description:  r.Description,
		Status:       "active",
		WorkerStatus: string(r.WorkerStatus),
		LastModified: r.LastModified.Format("2006-01-02 15:04:05"),
		Workflow:     r.Workflow,
		Activities:   r.Activities,
	}

	if r.WorkerStatus == recipe.WorkerStatusStopped {
		output.Status = "removed"
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputRecipeYAML(cmd *cobra.Command, r *recipe.Recipe) error {
	// Create a structure that matches the YAML format
	output := map[string]interface{}{
		"recipe": map[string]interface{}{
			"name":          r.Name,
			"version":       r.Version,
			"description":   r.Description,
			"status":        r.WorkerStatus,
			"last_modified": r.LastModified.Format("2006-01-02 15:04:05"),
		},
	}

	if r.Workflow != nil {
		output["workflow"] = r.Workflow
	}

	if len(r.Activities) > 0 {
		output["activities"] = r.Activities
	}

	encoder := yaml.NewEncoder(cmd.OutOrStdout())
	defer encoder.Close()
	return encoder.Encode(output)
}
