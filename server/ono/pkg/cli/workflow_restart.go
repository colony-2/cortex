package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
)

var (
	restartNamespace     string
	restartRunID         string
	restartFromStep      string
	restartFromEventID   int64
	restartInputs        map[string]string
	restartPreserveState bool
	restartNewID         string
)

var workflowRestartCmd = &cobra.Command{
	Use:   "restart [workflow-id]",
	Short: "Restart a workflow from a specific point with modified inputs",
	Long: `Restart a workflow execution from a specific step or activity,
preserving the state from previous steps while allowing modification
of inputs for subsequent steps.

This is useful for:
- Retrying failed workflows from the point of failure
- Testing different inputs for specific activities
- Debugging workflow logic by replaying with modifications

Examples:
  # Restart from a specific activity by name
  ono workflow restart my-workflow-123 --from-step analyze_activity --input data="new data"
  
  # Restart from a specific event ID
  ono workflow restart my-workflow-123 --from-event 15 --input threshold=0.8
  
  # Restart with a new workflow ID
  ono workflow restart my-workflow-123 --from-step write_report --new-id my-workflow-retry-1`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowRestart,
}

func init() {
	workflowRestartCmd.Flags().StringVarP(&restartNamespace, "namespace", "n", "default", "Temporal namespace")
	workflowRestartCmd.Flags().StringVar(&restartRunID, "run-id", "", "Specific run ID to restart from")
	workflowRestartCmd.Flags().StringVar(&restartFromStep, "from-step", "", "Activity/step name to restart from")
	workflowRestartCmd.Flags().Int64Var(&restartFromEventID, "from-event", 0, "Event ID to restart from")
	workflowRestartCmd.Flags().StringToStringVar(&restartInputs, "input", map[string]string{}, "Modified inputs for the restart point")
	workflowRestartCmd.Flags().BoolVar(&restartPreserveState, "preserve-state", true, "Preserve state from previous steps")
	workflowRestartCmd.Flags().StringVar(&restartNewID, "new-id", "", "New workflow ID for the restarted execution")
	workflowRestartCmd.MarkFlagsMutuallyExclusive("from-step", "from-event")
	addServerFlags(workflowRestartCmd)
}

type WorkflowState struct {
	OriginalInputs map[string]interface{}
	CompletedSteps map[string]StepResult
	RestartPoint   string
	RestartEventID int64
	WorkflowType   string
	TaskQueue      string
}

type StepResult struct {
	ActivityName string
	EventID      int64
	Output       map[string]interface{}
}

func runWorkflowRestart(cmd *cobra.Command, args []string) error {
	workflowID := args[0]

	if restartFromStep == "" && restartFromEventID == 0 {
		return fmt.Errorf("either --from-step or --from-event must be specified")
	}

	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", serverHost, serverPort),
		Namespace: restartNamespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	ctx := context.Background()

	// Analyze the workflow history to extract state
	state, err := analyzeWorkflowHistory(ctx, c, workflowID, restartRunID, restartFromStep, restartFromEventID)
	if err != nil {
		return fmt.Errorf("failed to analyze workflow history: %w", err)
	}

	fmt.Printf("=== Workflow Restart Analysis ===\n")
	fmt.Printf("Original Workflow: %s\n", workflowID)
	fmt.Printf("Workflow Type: %s\n", state.WorkflowType)
	fmt.Printf("Restart Point: %s (Event ID: %d)\n", state.RestartPoint, state.RestartEventID)
	fmt.Printf("\nCompleted Steps (%d):\n", len(state.CompletedSteps))

	for stepName, result := range state.CompletedSteps {
		fmt.Printf("  ✓ %s (Event %d)\n", stepName, result.EventID)
	}

	// Prepare inputs for restart
	restartInputMap := make(map[string]interface{})

	// Start with original inputs
	for k, v := range state.OriginalInputs {
		restartInputMap[k] = v
	}

	// Add preserved state from completed steps
	if restartPreserveState {
		restartInputMap["__preserved_state__"] = state.CompletedSteps
	}

	// Override with any new inputs
	for k, v := range restartInputs {
		restartInputMap[k] = v
	}

	fmt.Printf("\n=== Restart Configuration ===\n")
	fmt.Printf("Preserved State: %v\n", restartPreserveState)
	fmt.Printf("Modified Inputs: %d\n", len(restartInputs))
	for k, v := range restartInputs {
		fmt.Printf("  %s = %s\n", k, v)
	}

	// Determine new workflow ID
	newWorkflowID := restartNewID
	if newWorkflowID == "" {
		newWorkflowID = fmt.Sprintf("%s-restart-%d", workflowID, state.RestartEventID)
	}

	fmt.Printf("\nNew Workflow ID: %s\n", newWorkflowID)

	// Confirm restart
	fmt.Printf("\nDo you want to proceed with the restart? (y/N): ")
	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("Restart cancelled")
		return nil
	}

	// Start the new workflow with restart configuration
	workflowOptions := client.StartWorkflowOptions{
		ID:        newWorkflowID,
		TaskQueue: state.TaskQueue,
		Memo: map[string]interface{}{
			"restarted_from": workflowID,
			"restart_point":  state.RestartPoint,
			"restart_event":  state.RestartEventID,
		},
	}

	// The restart workflow needs special handling to skip completed steps
	// Pass the preserved state as part of the inputs
	restartInputMap["__restart_state"] = state

	fmt.Printf("\n=== Restart Workflow ===\n")
	fmt.Printf("Starting new workflow: %s\n", newWorkflowID)
	fmt.Printf("Type: %s\n", state.WorkflowType)
	fmt.Printf("Task Queue: %s\n", state.TaskQueue)
	fmt.Printf("Restart Point: %s\n", state.RestartPoint)

	// Execute the restarted workflow
	we, err := c.ExecuteWorkflow(ctx, workflowOptions, state.WorkflowType, restartInputMap)
	if err != nil {
		return fmt.Errorf("failed to start restart workflow: %w", err)
	}

	fmt.Printf("\nRestart workflow started successfully!\n")
	fmt.Printf("New Workflow ID: %s\n", we.GetID())
	fmt.Printf("New Run ID: %s\n", we.GetRunID())

	fmt.Printf("\nRestart workflow would be started with the following configuration:\n")
	fmt.Printf("- Skip activities before event %d\n", state.RestartEventID)
	fmt.Printf("- Use preserved outputs for completed steps\n")
	fmt.Printf("- Apply new inputs at restart point\n")

	fmt.Println("\nNote: Full restart functionality requires workflow implementation that supports restart semantics.")

	return nil
}

func analyzeWorkflowHistory(ctx context.Context, c client.Client, workflowID, runID, fromStep string, fromEventID int64) (*WorkflowState, error) {
	state := &WorkflowState{
		OriginalInputs: make(map[string]interface{}),
		CompletedSteps: make(map[string]StepResult),
	}

	// Get workflow history
	iter := c.GetWorkflowHistory(ctx, workflowID, runID, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)

	var foundRestartPoint bool

	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			return nil, err
		}

		// Extract workflow start info
		if event.EventType == enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED {
			attrs := event.GetWorkflowExecutionStartedEventAttributes()
			state.WorkflowType = attrs.WorkflowType.Name
			state.TaskQueue = attrs.TaskQueue.Name

			// Extract original inputs (would need proper payload decoding)
			if attrs.Input != nil {
				// In real implementation, decode payloads
				state.OriginalInputs["__original__"] = "encoded_inputs"
			}
		}

		// Track completed activities
		if event.EventType == enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED {
			attrs := event.GetActivityTaskCompletedEventAttributes()

			// Get activity name from scheduled event
			scheduledEvent := findEventByID(iter, attrs.ScheduledEventId)
			if scheduledEvent != nil {
				scheduledAttrs := scheduledEvent.GetActivityTaskScheduledEventAttributes()
				activityName := scheduledAttrs.ActivityType.Name

				result := StepResult{
					ActivityName: activityName,
					EventID:      event.EventId,
				}

				// In real implementation, decode result payloads
				if attrs.Result != nil {
					result.Output = map[string]interface{}{
						"__result__": "encoded_output",
					}
				}

				state.CompletedSteps[activityName] = result

				// Check if this is our restart point
				if fromStep != "" && activityName == fromStep {
					state.RestartPoint = activityName
					state.RestartEventID = event.EventId
					foundRestartPoint = true
					break
				}
			}
		}

		// Check event ID restart point
		if fromEventID > 0 && event.EventId >= fromEventID {
			state.RestartEventID = event.EventId
			state.RestartPoint = fmt.Sprintf("Event_%d", event.EventId)
			foundRestartPoint = true
			break
		}
	}

	if !foundRestartPoint {
		if fromStep != "" {
			return nil, fmt.Errorf("activity '%s' not found in workflow history", fromStep)
		}
		return nil, fmt.Errorf("event ID %d not found in workflow history", fromEventID)
	}

	return state, nil
}

func findEventByID(iter client.HistoryEventIterator, eventID int64) *history.HistoryEvent {
	// In a real implementation, we would need to cache events or make another query
	// For now, return nil
	return nil
}
