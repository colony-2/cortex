package ono

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
)

var (
	historyRunID       string
	historyEventID     int64
	historyEventFilter string
	historyFormat      string
	historyLimit       int
)

var workflowHistoryCmd = &cobra.Command{
	Use:   "history [workflow-id]",
	Short: "Show the event history of a workflow execution",
	Long: `Display the event history of a workflow execution.
	
This shows all events that have occurred during the workflow execution,
including activity starts/completions, timers, signals, and state transitions.

Examples:
  # Show full history
  ono workflow history my-workflow-123
  
  # Show only activity events
  ono workflow history my-workflow-123 --filter activity
  
  # Show events after a specific event ID
  ono workflow history my-workflow-123 --after-event 10
  
  # Limit number of events
  ono workflow history my-workflow-123 --limit 20`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowHistory,
}

func init() {
	workflowHistoryCmd.Flags().StringVarP(&historyNamespace, "namespace", "n", "default", "Temporal namespace")
	workflowHistoryCmd.Flags().StringVar(&historyRunID, "run-id", "", "Specific run ID (optional, uses latest if not specified)")
	workflowHistoryCmd.Flags().Int64Var(&historyEventID, "after-event", 0, "Show events after this event ID")
	workflowHistoryCmd.Flags().StringVar(&historyEventFilter, "filter", "", "Filter events (activity, timer, signal, workflow)")
	workflowHistoryCmd.Flags().StringVarP(&historyFormat, "format", "o", "text", "Output format (text, json)")
	workflowHistoryCmd.Flags().IntVar(&historyLimit, "limit", 100, "Maximum number of events to show")
	addServerFlags(workflowHistoryCmd)
}

var historyNamespace string

func runWorkflowHistory(cmd *cobra.Command, args []string) error {
	workflowID := args[0]

	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", serverHost, serverPort),
		Namespace: historyNamespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	ctx := context.Background()

	// Get workflow history
	iter := c.GetWorkflowHistory(ctx, workflowID, historyRunID, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	
	var events []*history.HistoryEvent
	eventCount := 0
	
	for iter.HasNext() && eventCount < historyLimit {
		event, err := iter.Next()
		if err != nil {
			return fmt.Errorf("failed to get history event: %w", err)
		}
		
		// Skip events before the specified event ID
		if historyEventID > 0 && event.EventId <= historyEventID {
			continue
		}
		
		// Apply filter if specified
		if historyEventFilter != "" && !matchesFilter(event, historyEventFilter) {
			continue
		}
		
		events = append(events, event)
		eventCount++
	}

	if historyFormat == "json" {
		return printHistoryJSON(events)
	}

	return printHistoryText(events)
}

func matchesFilter(event *history.HistoryEvent, filter string) bool {
	filter = strings.ToLower(filter)
	
	switch filter {
	case "activity":
		switch event.EventType {
		case enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
			enums.EVENT_TYPE_ACTIVITY_TASK_STARTED,
			enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED,
			enums.EVENT_TYPE_ACTIVITY_TASK_FAILED,
			enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT,
			enums.EVENT_TYPE_ACTIVITY_TASK_CANCELED,
			enums.EVENT_TYPE_ACTIVITY_TASK_CANCEL_REQUESTED:
			return true
		}
		return false
	case "timer":
		switch event.EventType {
		case enums.EVENT_TYPE_TIMER_STARTED,
			enums.EVENT_TYPE_TIMER_FIRED,
			enums.EVENT_TYPE_TIMER_CANCELED:
			return true
		}
		return false
	case "signal":
		switch event.EventType {
		case enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
			enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED,
			enums.EVENT_TYPE_EXTERNAL_WORKFLOW_EXECUTION_SIGNALED:
			return true
		}
		return false
	case "workflow":
		switch event.EventType {
		case enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED,
			enums.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED,
			enums.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT,
			enums.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED,
			enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED,
			enums.EVENT_TYPE_WORKFLOW_EXECUTION_CONTINUED_AS_NEW:
			return true
		}
		return false
	default:
		return true
	}
}

func printHistoryText(events []*history.HistoryEvent) error {
	if len(events) == 0 {
		fmt.Println("No events found")
		return nil
	}

	fmt.Println("=== Workflow History ===")
	fmt.Printf("Total events shown: %d\n\n", len(events))

	for _, event := range events {
		printEvent(event)
		fmt.Println()
	}

	return nil
}

func printEvent(event *history.HistoryEvent) {
	eventTime := event.EventTime.AsTime()
	fmt.Printf("[%d] %s - %s\n", 
		event.EventId, 
		eventTime.Format("15:04:05.000"),
		getEventTypeName(event.EventType))
	
	// Print event-specific details
	switch event.EventType {
	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
		attrs := event.GetWorkflowExecutionStartedEventAttributes()
		fmt.Printf("  Workflow Type: %s\n", attrs.WorkflowType.Name)
		fmt.Printf("  Task Queue: %s\n", attrs.TaskQueue.Name)
		if attrs.Input != nil && len(attrs.Input.Payloads) > 0 {
			fmt.Printf("  Inputs: %d payload(s)\n", len(attrs.Input.Payloads))
		}
		
	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:
		attrs := event.GetWorkflowExecutionCompletedEventAttributes()
		if attrs.Result != nil && len(attrs.Result.Payloads) > 0 {
			fmt.Printf("  Results: %d payload(s)\n", len(attrs.Result.Payloads))
		}
		
	case enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
		attrs := event.GetActivityTaskScheduledEventAttributes()
		fmt.Printf("  Activity Type: %s\n", attrs.ActivityType.Name)
		fmt.Printf("  Activity ID: %s\n", attrs.ActivityId)
		
	case enums.EVENT_TYPE_ACTIVITY_TASK_STARTED:
		attrs := event.GetActivityTaskStartedEventAttributes()
		fmt.Printf("  Scheduled Event ID: %d\n", attrs.ScheduledEventId)
		fmt.Printf("  Attempt: %d\n", attrs.Attempt)
		
	case enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
		attrs := event.GetActivityTaskCompletedEventAttributes()
		fmt.Printf("  Scheduled Event ID: %d\n", attrs.ScheduledEventId)
		fmt.Printf("  Started Event ID: %d\n", attrs.StartedEventId)
		if attrs.Result != nil && len(attrs.Result.Payloads) > 0 {
			fmt.Printf("  Results: %d payload(s)\n", len(attrs.Result.Payloads))
		}
		
	case enums.EVENT_TYPE_ACTIVITY_TASK_FAILED:
		attrs := event.GetActivityTaskFailedEventAttributes()
		fmt.Printf("  Scheduled Event ID: %d\n", attrs.ScheduledEventId)
		fmt.Printf("  Started Event ID: %d\n", attrs.StartedEventId)
		if attrs.Failure != nil {
			fmt.Printf("  Error: %s\n", attrs.Failure.Message)
		}
		
	case enums.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED:
		attrs := event.GetWorkflowTaskScheduledEventAttributes()
		fmt.Printf("  Task Queue: %s\n", attrs.TaskQueue.Name)
		
	case enums.EVENT_TYPE_WORKFLOW_TASK_STARTED:
		attrs := event.GetWorkflowTaskStartedEventAttributes()
		fmt.Printf("  Scheduled Event ID: %d\n", attrs.ScheduledEventId)
		
	case enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED:
		attrs := event.GetWorkflowTaskCompletedEventAttributes()
		fmt.Printf("  Scheduled Event ID: %d\n", attrs.ScheduledEventId)
		fmt.Printf("  Started Event ID: %d\n", attrs.StartedEventId)
	}
}

func getEventTypeName(eventType enums.EventType) string {
	// Use a map for known event types to ensure correct formatting
	eventNames := map[enums.EventType]string{
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:                  "Workflow Execution Started",
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:                "Workflow Execution Completed",
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:                   "Workflow Execution Failed",
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:                "Workflow Execution Timed Out",
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:                 "Workflow Execution Canceled",
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED:               "Workflow Execution Terminated",
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_CONTINUED_AS_NEW:         "Workflow Execution Continued As New",
		enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED:                 "Workflow Execution Signaled",
		enums.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED:                     "Workflow Task Scheduled",
		enums.EVENT_TYPE_WORKFLOW_TASK_STARTED:                       "Workflow Task Started",
		enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED:                     "Workflow Task Completed",
		enums.EVENT_TYPE_WORKFLOW_TASK_TIMED_OUT:                     "Workflow Task Timed Out",
		enums.EVENT_TYPE_WORKFLOW_TASK_FAILED:                        "Workflow Task Failed",
		enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:                     "Activity Task Scheduled",
		enums.EVENT_TYPE_ACTIVITY_TASK_STARTED:                       "Activity Task Started",
		enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:                     "Activity Task Completed",
		enums.EVENT_TYPE_ACTIVITY_TASK_FAILED:                        "Activity Task Failed",
		enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT:                     "Activity Task Timed Out",
		enums.EVENT_TYPE_ACTIVITY_TASK_CANCELED:                      "Activity Task Canceled",
		enums.EVENT_TYPE_ACTIVITY_TASK_CANCEL_REQUESTED:              "Activity Task Cancel Requested",
		enums.EVENT_TYPE_TIMER_STARTED:                               "Timer Started",
		enums.EVENT_TYPE_TIMER_FIRED:                                 "Timer Fired",
		enums.EVENT_TYPE_TIMER_CANCELED:                              "Timer Canceled",
		enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED: "Signal External Workflow Execution Initiated",
		enums.EVENT_TYPE_EXTERNAL_WORKFLOW_EXECUTION_SIGNALED:        "External Workflow Execution Signaled",
		enums.EVENT_TYPE_MARKER_RECORDED:                             "Marker Recorded",
		enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED:     "Start Child Workflow Execution Initiated",
		enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_STARTED:            "Child Workflow Execution Started",
		enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_COMPLETED:          "Child Workflow Execution Completed",
		enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_FAILED:             "Child Workflow Execution Failed",
		enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_CANCELED:           "Child Workflow Execution Canceled",
		enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TIMED_OUT:          "Child Workflow Execution Timed Out",
		enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TERMINATED:         "Child Workflow Execution Terminated",
	}
	
	if name, ok := eventNames[eventType]; ok {
		return name
	}
	
	// Fallback: format the string representation
	name := eventType.String()
	// Split on capital letters to handle CamelCase
	var result []string
	var current []rune
	
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' && len(current) > 0 {
			result = append(result, string(current))
			current = []rune{r}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		result = append(result, string(current))
	}
	
	return strings.Join(result, " ")
}

func printHistoryJSON(events []*history.HistoryEvent) error {
	output, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}
	fmt.Println(string(output))
	return nil
}