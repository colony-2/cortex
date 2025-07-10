package ono

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var (
	listNamespace      string
	listWorkflowType   string
	listStatus         string
	listLimit          int
	listArchived       bool
)

var workflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workflow executions",
	Long: `List workflow executions with optional filters.
	
Examples:
  # List all running workflows
  ono workflow list
  
  # List workflows of a specific type
  ono workflow list --type research-workflow
  
  # List completed workflows
  ono workflow list --status completed
  
  # List archived workflows
  ono workflow list --archived`,
	RunE: runWorkflowList,
}

func init() {
	workflowListCmd.Flags().StringVarP(&listNamespace, "namespace", "n", "default", "Temporal namespace")
	workflowListCmd.Flags().StringVarP(&listWorkflowType, "type", "t", "", "Filter by workflow type")
	workflowListCmd.Flags().StringVarP(&listStatus, "status", "s", "", "Filter by status (running, completed, failed, canceled, terminated, timeout)")
	workflowListCmd.Flags().IntVarP(&listLimit, "limit", "l", 20, "Maximum number of workflows to list")
	workflowListCmd.Flags().BoolVar(&listArchived, "archived", false, "List archived workflows")
}

func runWorkflowList(cmd *cobra.Command, args []string) error {
	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", serverHost, serverPort),
		Namespace: listNamespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	query := buildListQuery()
	
	var request interface{}
	if listArchived {
		request = &workflowservice.ListArchivedWorkflowExecutionsRequest{
			Namespace:     listNamespace,
			PageSize:      int32(listLimit),
			Query:         query,
		}
	} else {
		request = &workflowservice.ListWorkflowExecutionsRequest{
			Namespace:     listNamespace,
			PageSize:      int32(listLimit),
			Query:         query,
		}
	}

	ctx := context.Background()
	var executions []*workflowpb.WorkflowExecutionInfo
	
	if listArchived {
		resp, err := c.WorkflowService().ListArchivedWorkflowExecutions(ctx, request.(*workflowservice.ListArchivedWorkflowExecutionsRequest))
		if err != nil {
			return fmt.Errorf("failed to list archived workflows: %w", err)
		}
		executions = resp.Executions
	} else {
		resp, err := c.WorkflowService().ListWorkflowExecutions(ctx, request.(*workflowservice.ListWorkflowExecutionsRequest))
		if err != nil {
			return fmt.Errorf("failed to list workflows: %w", err)
		}
		executions = resp.Executions
	}

	if len(executions) == 0 {
		fmt.Println("No workflows found")
		return nil
	}

	printWorkflowList(executions)
	return nil
}

func buildListQuery() string {
	var conditions []string
	
	if listWorkflowType != "" {
		conditions = append(conditions, fmt.Sprintf("WorkflowType = '%s'", listWorkflowType))
	}
	
	if listStatus != "" {
		statusValue := getExecutionStatus(listStatus)
		if statusValue != enums.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED {
			conditions = append(conditions, fmt.Sprintf("ExecutionStatus = %d", statusValue))
		}
	}
	
	if len(conditions) == 0 {
		return ""
	}
	
	return strings.Join(conditions, " AND ")
}

func getExecutionStatus(status string) enums.WorkflowExecutionStatus {
	switch strings.ToLower(status) {
	case "running":
		return enums.WORKFLOW_EXECUTION_STATUS_RUNNING
	case "completed":
		return enums.WORKFLOW_EXECUTION_STATUS_COMPLETED
	case "failed":
		return enums.WORKFLOW_EXECUTION_STATUS_FAILED
	case "canceled":
		return enums.WORKFLOW_EXECUTION_STATUS_CANCELED
	case "terminated":
		return enums.WORKFLOW_EXECUTION_STATUS_TERMINATED
	case "continuedasnew":
		return enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW
	case "timedout", "timeout":
		return enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT
	default:
		return enums.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED
	}
}

func printWorkflowList(executions []*workflowpb.WorkflowExecutionInfo) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Print header
	fmt.Fprintln(w, "WORKFLOW ID\tRUN ID\tTYPE\tSTATUS\tSTART TIME\tEXECUTION TIME")
	fmt.Fprintln(w, strings.Repeat("-", 120))

	for _, exec := range executions {
		workflowID := exec.Execution.WorkflowId
		runID := exec.Execution.RunId[:8] + "..."
		workflowType := exec.Type.Name
		status := getStatusString(exec.Status)
		startTime := exec.StartTime.AsTime().Format("2006-01-02 15:04:05")
		
		var executionTime string
		if exec.CloseTime != nil {
			duration := exec.CloseTime.AsTime().Sub(exec.StartTime.AsTime())
			executionTime = formatDuration(duration)
		} else {
			duration := time.Since(exec.StartTime.AsTime())
			executionTime = formatDuration(duration) + " (running)"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			workflowID,
			runID,
			workflowType,
			status,
			startTime,
			executionTime,
		)
	}
}

func getStatusString(status enums.WorkflowExecutionStatus) string {
	switch status {
	case enums.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return "Running"
	case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return "Completed"
	case enums.WORKFLOW_EXECUTION_STATUS_FAILED:
		return "Failed"
	case enums.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return "Canceled"
	case enums.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return "Terminated"
	case enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return "ContinuedAsNew"
	case enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return "TimedOut"
	default:
		return "Unknown"
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	} else {
		return fmt.Sprintf("%.1fd", d.Hours()/24)
	}
}