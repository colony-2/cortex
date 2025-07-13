package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
)

// NewJobCancelCommand creates the job cancel command
func NewJobCancelCommand() *cobra.Command {
	var (
		reason     string
		namespace  string
		serverAddr string
	)

	cmd := &cobra.Command{
		Use:   "cancel <recipe-name> <job-id>",
		Short: "Cancel a running job execution",
		Long: `Cancels a running job execution.

This will send a cancellation request to the workflow, allowing it to
clean up gracefully before terminating.`,
		Example: `  # Cancel a running job
  ono job cancel my-recipe job-123

  # Cancel with a reason
  ono job cancel my-recipe job-123 --reason "No longer needed"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobCancel(cmd, args[0], args[1], reason, namespace, serverAddr)
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Reason for cancellation")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Temporal namespace")
	cmd.Flags().StringVar(&serverAddr, "address", "localhost:7233", "Temporal server address")

	return cmd
}

func runJobCancel(cmd *cobra.Command, recipeName, jobID, reason, namespace, serverAddr string) error {
	// Connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort:  serverAddr,
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	// Cancel the workflow execution
	ctx := context.Background()
	err = c.CancelWorkflow(ctx, jobID, "")
	if err != nil {
		return fmt.Errorf("failed to cancel workflow: %w", err)
	}

	// Output result
	fmt.Fprintf(cmd.OutOrStdout(), "Job cancellation request sent successfully.\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Recipe: %s\n", recipeName)
	fmt.Fprintf(cmd.OutOrStdout(), "  Job ID: %s\n", jobID)
	if reason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Reason: %s\n", reason)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Note: The job may take some time to cancel as it cleans up.")
	fmt.Fprintf(cmd.OutOrStdout(), "Use 'ono job describe %s %s' to check the final status.\n", recipeName, jobID)

	return nil
}