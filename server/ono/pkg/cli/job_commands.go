package cli

import (
	"github.com/spf13/cobra"
)

// NewJobCommand creates the job command group
func NewJobCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "Manage job executions",
		Long: `Manage job executions - running instances of recipes.

Jobs represent individual executions of recipes with specific inputs and outputs.
You can view job details, restart failed jobs, or cancel running jobs.`,
	}

	// Add subcommands
	cmd.AddCommand(
		NewJobDescribeCommand(),
		NewJobRestartCommand(),
		NewJobCancelCommand(),
	)

	return cmd
}