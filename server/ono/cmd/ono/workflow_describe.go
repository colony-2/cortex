package ono

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.temporal.io/api/enums/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var (
	describeRunID   string
	describeFormat  string
)

var workflowDescribeCmd = &cobra.Command{
	Use:   "describe [workflow-id]",
	Short: "Show detailed information about a workflow execution",
	Long: `Display detailed information about a workflow execution including:
  - Current status and execution info
  - Input parameters
  - Current state and pending activities
  - Output results (if completed)
  
Examples:
  # Describe a workflow by ID (latest run)
  ono workflow describe my-workflow-123
  
  # Describe a specific run
  ono workflow describe my-workflow-123 --run-id abc123
  
  # Output as JSON
  ono workflow describe my-workflow-123 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowDescribe,
}

func init() {
	workflowDescribeCmd.Flags().StringVarP(&describeNamespace, "namespace", "n", "default", "Temporal namespace")
	workflowDescribeCmd.Flags().StringVar(&describeRunID, "run-id", "", "Specific run ID (optional, uses latest if not specified)")
	workflowDescribeCmd.Flags().StringVarP(&describeFormat, "format", "o", "text", "Output format (text, json)")
	addServerFlags(workflowDescribeCmd)
}

var describeNamespace string

func runWorkflowDescribe(cmd *cobra.Command, args []string) error {
	workflowID := args[0]

	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", serverHost, serverPort),
		Namespace: describeNamespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	ctx := context.Background()

	// Get workflow execution info
	descResp, err := c.WorkflowService().DescribeWorkflowExecution(ctx, &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: describeNamespace,
		Execution: &workflowservice.WorkflowExecution{
			WorkflowId: workflowID,
			RunId:      describeRunID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to describe workflow: %w", err)
	}

	if describeFormat == "json" {
		return printDescribeJSON(descResp)
	}

	return printDescribeText(descResp)
}

func printDescribeText(resp *workflowservice.DescribeWorkflowExecutionResponse) error {
	exec := resp.WorkflowExecutionInfo
	
	fmt.Println("=== Workflow Execution ===")
	fmt.Printf("Workflow ID: %s\n", exec.Execution.WorkflowId)
	fmt.Printf("Run ID: %s\n", exec.Execution.RunId)
	fmt.Printf("Type: %s\n", exec.Type.Name)
	fmt.Printf("Task Queue: %s\n", exec.TaskQueue)
	fmt.Printf("Status: %s\n", getStatusString(exec.Status))
	
	fmt.Println("\n=== Timing ===")
	fmt.Printf("Start Time: %s\n", exec.StartTime.AsTime().Format(time.RFC3339))
	if exec.CloseTime != nil {
		fmt.Printf("Close Time: %s\n", exec.CloseTime.AsTime().Format(time.RFC3339))
		duration := exec.CloseTime.AsTime().Sub(exec.StartTime.AsTime())
		fmt.Printf("Duration: %s\n", duration.String())
	} else {
		duration := time.Since(exec.StartTime.AsTime())
		fmt.Printf("Duration: %s (running)\n", duration.String())
	}
	
	if exec.ExecutionTime != nil {
		fmt.Printf("Execution Time: %s\n", exec.ExecutionTime.AsTime().Format(time.RFC3339))
	}

	fmt.Println("\n=== Execution Details ===")
	fmt.Printf("History Length: %d\n", exec.HistoryLength)
	fmt.Printf("State Transition Count: %d\n", exec.StateTransitionCount)
	
	if exec.Memo != nil && len(exec.Memo.Fields) > 0 {
		fmt.Println("\n=== Memo ===")
		for key, value := range exec.Memo.Fields {
			fmt.Printf("%s: %s\n", key, value.String())
		}
	}

	if exec.SearchAttributes != nil && len(exec.SearchAttributes.IndexedFields) > 0 {
		fmt.Println("\n=== Search Attributes ===")
		for key, value := range exec.SearchAttributes.IndexedFields {
			fmt.Printf("%s: %s\n", key, value.String())
		}
	}

	// Get pending activities
	if len(resp.PendingActivities) > 0 {
		fmt.Println("\n=== Pending Activities ===")
		for _, activity := range resp.PendingActivities {
			fmt.Printf("- %s (ID: %s)\n", activity.ActivityType.Name, activity.ActivityId)
			fmt.Printf("  State: %s\n", activity.State.String())
			fmt.Printf("  Attempt: %d\n", activity.Attempt)
			if activity.LastFailure != nil {
				fmt.Printf("  Last Failure: %s\n", activity.LastFailure.Message)
			}
			fmt.Printf("  Scheduled: %s\n", activity.ScheduledTime.AsTime().Format(time.RFC3339))
			if activity.LastHeartbeatTime != nil {
				fmt.Printf("  Last Heartbeat: %s ago\n", time.Since(activity.LastHeartbeatTime.AsTime()).String())
			}
		}
	}

	// Get pending children
	if len(resp.PendingChildren) > 0 {
		fmt.Println("\n=== Pending Child Workflows ===")
		for _, child := range resp.PendingChildren {
			fmt.Printf("- %s (Run: %s)\n", child.WorkflowId, child.RunId)
			fmt.Printf("  Type: %s\n", child.WorkflowTypeName)
		}
	}

	// Try to get result if completed
	if exec.Status == enums.WORKFLOW_EXECUTION_STATUS_COMPLETED {
		fmt.Println("\n=== Result ===")
		
		// Query for workflow result
		queryResp, err := c.QueryWorkflow(context.Background(), exec.Execution.WorkflowId, exec.Execution.RunId, "__stack_trace")
		if err == nil && queryResp != nil && queryResp.QueryResult != nil {
			var result interface{}
			if err := queryResp.QueryResult.Get(&result); err == nil {
				resultJSON, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(resultJSON))
			}
		} else {
			fmt.Println("Result not available")
		}
	}

	return nil
}

func printDescribeJSON(resp *workflowservice.DescribeWorkflowExecutionResponse) error {
	output, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	fmt.Println(string(output))
	return nil
}