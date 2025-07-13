package cli

import (
	"github.com/spf13/cobra"
)

// NewRecipeCommand creates the recipe command group
func NewRecipeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Manage recipes",
		Long: `Manage Ono recipes - reusable workflow definitions that can be executed as jobs.

Recipes are discovered automatically from your recipe directory and each recipe
has its own dedicated worker process to handle executions.`,
	}

	// Add subcommands
	cmd.AddCommand(
		NewRecipeListCommand(),
		NewRecipeDescribeCommand(),
		NewRecipeRunCommand(),
		NewRecipeHistoryCommand(),
	)

	return cmd
}