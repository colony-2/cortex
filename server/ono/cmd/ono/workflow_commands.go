package ono

import (
	"github.com/spf13/cobra"
)

var (
	serverHost string
	serverPort int
)

var WorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage and execute workflows",
	Long: `Commands for managing Temporal workflows:
  - Run workflows from YAML definitions
  - List workflow executions
  - Inspect workflow details
  - View workflow history
  - Restart workflows from specific points`,
}

// Add default server connection flags to a command
func addServerFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&serverHost, "host", "127.0.0.1", "Temporal server host")
	cmd.Flags().IntVarP(&serverPort, "port", "p", 7233, "Temporal server port")
}

func init() {
	// Add workflow execution commands
	WorkflowCmd.AddCommand(workflowRunCmd)
	WorkflowCmd.AddCommand(workflowTestConnectionCmd)
	WorkflowCmd.AddCommand(workflowListCmd)
	WorkflowCmd.AddCommand(workflowDescribeCmd)
	WorkflowCmd.AddCommand(workflowHistoryCmd)
	WorkflowCmd.AddCommand(workflowRestartCmd)
	WorkflowCmd.AddCommand(workflowActivitiesCmd)
	
	// Set flags for the run command
	workflowRunCmd.Flags().StringVarP(&workflowFile, "file", "f", "", "Path to workflow YAML file (required)")
	workflowRunCmd.Flags().StringVarP(&workflowID, "id", "i", "", "Workflow ID (optional, auto-generated if not provided)")
	workflowRunCmd.Flags().StringVarP(&runNamespace, "namespace", "n", "default", "Temporal namespace")
	workflowRunCmd.Flags().StringToStringVarP(&workflowInputs, "input", "", map[string]string{}, "Workflow inputs as key=value pairs")
	workflowRunCmd.Flags().StringVar(&taskQueue, "task-queue", "ono-task-queue", "Task queue name")
	addServerFlags(workflowRunCmd)
	workflowRunCmd.MarkFlagRequired("file")
	
	// Set flags for test-connection command
	workflowTestConnectionCmd.Flags().StringVarP(&testConnectionNamespace, "namespace", "n", "", "Temporal namespace (optional)")
	addServerFlags(workflowTestConnectionCmd)
	
	// Set flags for list command
	addServerFlags(workflowListCmd)
}