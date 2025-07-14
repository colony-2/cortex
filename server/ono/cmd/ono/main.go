package main

import (
	"os"
	ono2 "vibethis/ono/pkg/cli"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ono",
	Short: "A CLI tool for orchestrating Temporal workflows with YAML",
}

func init() {
	rootCmd.AddCommand(ono2.StartWithRecipesCmd)
	rootCmd.AddCommand(ono2.WorkflowCmd)
	rootCmd.AddCommand(ono2.NewRecipeCommand())
	rootCmd.AddCommand(ono2.NewJobCommand())
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
