package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	
	"vibethis/ono/internal/recipe"
	yamlpkg "vibethis/ono/pkg/yaml"
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
	out := cmd.OutOrStdout()

	// Recipe metadata
	fmt.Fprintf(out, "Recipe: %s\n", r.Name)
	fmt.Fprintf(out, "Version: %s\n", r.Version)
	if r.Description != "" {
		fmt.Fprintf(out, "Description: %s\n", r.Description)
	}
	fmt.Fprintf(out, "Status: %s\n", r.WorkerStatus)
	fmt.Fprintf(out, "Last Modified: %s\n", r.LastModified.Format("2006-01-02 15:04:05"))
	fmt.Fprintln(out)

	// If recipe is removed, we might not have full details
	if r.WorkerStatus == recipe.WorkerStatusStopped && r.Workflow == nil {
		fmt.Fprintln(out, "Note: This recipe has been removed. Full details are no longer available.")
		return nil
	}

	// Workflow information
	if r.Workflow != nil {
		fmt.Fprintln(out, "Workflow:")
		fmt.Fprintf(out, "  Name: %s\n", r.Workflow.Name)
		if r.Workflow.Description != "" {
			fmt.Fprintf(out, "  Description: %s\n", r.Workflow.Description)
		}
		
		// Inputs
		if len(r.Workflow.Inputs) > 0 {
			fmt.Fprintln(out, "\n  Inputs:")
			for _, input := range r.Workflow.Inputs {
				fmt.Fprintf(out, "    - %s (%s)", input.Name, input.Type)
				if input.Required {
					fmt.Fprint(out, " [required]")
				}
				if input.Default != nil {
					fmt.Fprintf(out, " default=%v", input.Default)
				}
				if input.Description != "" {
					fmt.Fprintf(out, " - %s", input.Description)
				}
				fmt.Fprintln(out)
			}
		}
		
		// Outputs
		if len(r.Workflow.Outputs) > 0 {
			fmt.Fprintln(out, "\n  Outputs:")
			for _, output := range r.Workflow.Outputs {
				fmt.Fprintf(out, "    - %s (%s)", output.Name, output.Type)
				if output.Description != "" {
					fmt.Fprintf(out, " - %s", output.Description)
				}
				fmt.Fprintln(out)
			}
		}
		
		// Workflow steps
		if r.Workflow.Workflow.Steps != nil && len(r.Workflow.Workflow.Steps) > 0 {
			fmt.Fprintln(out, "\n  Steps:")
			outputSteps(out, r.Workflow.Workflow.Steps, "    ")
		}
	}

	// Activities
	if len(r.Activities) > 0 {
		fmt.Fprintln(out, "\nActivities:")
		for _, activity := range r.Activities {
			fmt.Fprintf(out, "  - %s", activity.Name)
			if activity.Description != "" {
				fmt.Fprintf(out, " - %s", activity.Description)
			}
			fmt.Fprintln(out)
		}
	}

	return nil
}

func outputSteps(out io.Writer, steps []yamlpkg.Step, indent string) {
	for i, step := range steps {
		if step.ID != "" {
			fmt.Fprintf(out, "%s%d. %s", indent, i+1, step.ID)
		} else {
			fmt.Fprintf(out, "%s%d. Step %d", indent, i+1, i+1)
		}
		
		if step.Activity != "" {
			fmt.Fprintf(out, " (activity: %s)", step.Activity)
		}
		fmt.Fprintln(out)
		
		// Handle parallel steps
		if len(step.Parallel) > 0 {
			fmt.Fprintf(out, "%s   Parallel steps:\n", indent)
			outputSteps(out, step.Parallel, indent+"     ")
		}
	}
}

func outputRecipeJSON(cmd *cobra.Command, r *recipe.Recipe) error {
	// Create a simplified structure for JSON output
	type recipeOutput struct {
		Name         string                      `json:"name"`
		Version      string                      `json:"version"`
		Description  string                      `json:"description"`
		Status       string                      `json:"status"`
		WorkerStatus string                      `json:"worker_status"`
		LastModified string                      `json:"last_modified"`
		Workflow     *yamlpkg.WorkflowDefinition `json:"workflow,omitempty"`
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