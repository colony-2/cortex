package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

var (
	activitiesNamespace string
	activitiesRunID     string
	activitiesShowAll   bool
)

var workflowActivitiesCmd = &cobra.Command{
	Use:   "activities [workflow-id]",
	Short: "List activities for a workflow execution",
	Long: `Display all activities that have been executed or are currently
executing as part of a workflow.

This shows:
- Activity name and type
- Start and completion times
- Duration
- Status (completed, failed, running)
- Retry attempts
- Input/output summary

Examples:
  # List activities for a workflow
  ono workflow activities my-workflow-123
  
  # Include failed attempts
  ono workflow activities my-workflow-123 --all
  
  # Specific run
  ono workflow activities my-workflow-123 --run-id abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowActivities,
}

func init() {
	workflowActivitiesCmd.Flags().StringVarP(&activitiesNamespace, "namespace", "n", "default", "Temporal namespace")
	workflowActivitiesCmd.Flags().StringVar(&activitiesRunID, "run-id", "", "Specific run ID (optional)")
	workflowActivitiesCmd.Flags().BoolVar(&activitiesShowAll, "all", false, "Show all attempts including failures")
	addServerFlags(workflowActivitiesCmd)
}

type ActivityInfo struct {
	Name          string
	ActivityID    string
	EventID       int64
	ScheduledTime time.Time
	StartedTime   *time.Time
	CompletedTime *time.Time
	Status        string
	Attempts      int32
	Duration      time.Duration
	FailureReason string
	RetryState    string
}

func runWorkflowActivities(cmd *cobra.Command, args []string) error {
	workflowID := args[0]

	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", serverHost, serverPort),
		Namespace: activitiesNamespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	ctx := context.Background()

	// Get workflow history and extract activity information
	activities, err := extractActivities(ctx, c, workflowID, activitiesRunID)
	if err != nil {
		return fmt.Errorf("failed to extract activities: %w", err)
	}

	if len(activities) == 0 {
		fmt.Println("No activities found for this workflow")
		return nil
	}

	printActivities(activities)
	return nil
}

func extractActivities(ctx context.Context, c client.Client, workflowID, runID string) ([]ActivityInfo, error) {
	iter := c.GetWorkflowHistory(ctx, workflowID, runID, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)

	activityMap := make(map[int64]*ActivityInfo)
	var activities []ActivityInfo

	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			return nil, err
		}

		switch event.EventType {
		case enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
			attrs := event.GetActivityTaskScheduledEventAttributes()
			info := &ActivityInfo{
				Name:          attrs.ActivityType.Name,
				ActivityID:    attrs.ActivityId,
				EventID:       event.EventId,
				ScheduledTime: event.EventTime.AsTime(),
				Status:        "Scheduled",
				Attempts:      0,
			}
			activityMap[event.EventId] = info

		case enums.EVENT_TYPE_ACTIVITY_TASK_STARTED:
			attrs := event.GetActivityTaskStartedEventAttributes()
			if info, ok := activityMap[attrs.ScheduledEventId]; ok {
				startTime := event.EventTime.AsTime()
				info.StartedTime = &startTime
				info.Status = "Running"
				info.Attempts = attrs.Attempt
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
			attrs := event.GetActivityTaskCompletedEventAttributes()
			if info, ok := activityMap[attrs.ScheduledEventId]; ok {
				completedTime := event.EventTime.AsTime()
				info.CompletedTime = &completedTime
				info.Status = "Completed"

				if info.StartedTime != nil {
					info.Duration = completedTime.Sub(*info.StartedTime)
				} else {
					info.Duration = completedTime.Sub(info.ScheduledTime)
				}
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_FAILED:
			attrs := event.GetActivityTaskFailedEventAttributes()
			if info, ok := activityMap[attrs.ScheduledEventId]; ok {
				if activitiesShowAll || info.Status != "Completed" {
					failedInfo := *info
					failedInfo.Status = "Failed"
					if attrs.Failure != nil {
						failedInfo.FailureReason = attrs.Failure.Message
					}
					failedTime := event.EventTime.AsTime()
					failedInfo.CompletedTime = &failedTime

					if info.StartedTime != nil {
						failedInfo.Duration = failedTime.Sub(*info.StartedTime)
					}

					if attrs.RetryState == enums.RETRY_STATE_IN_PROGRESS {
						failedInfo.RetryState = "Retrying"
					} else if attrs.RetryState == enums.RETRY_STATE_NON_RETRYABLE_FAILURE {
						failedInfo.RetryState = "Non-retryable"
					}

					activities = append(activities, failedInfo)
				}
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT:
			attrs := event.GetActivityTaskTimedOutEventAttributes()
			if info, ok := activityMap[attrs.ScheduledEventId]; ok {
				info.Status = "Timed Out"
				info.FailureReason = "Activity timed out"
				timeoutTime := event.EventTime.AsTime()
				info.CompletedTime = &timeoutTime

				if info.StartedTime != nil {
					info.Duration = timeoutTime.Sub(*info.StartedTime)
				}
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_CANCEL_REQUESTED:
			attrs := event.GetActivityTaskCancelRequestedEventAttributes()
			if info, ok := activityMap[attrs.ScheduledEventId]; ok {
				info.Status = "Cancel Requested"
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_CANCELED:
			attrs := event.GetActivityTaskCanceledEventAttributes()
			if info, ok := activityMap[attrs.ScheduledEventId]; ok {
				info.Status = "Canceled"
				cancelTime := event.EventTime.AsTime()
				info.CompletedTime = &cancelTime
			}
		}
	}

	// Add completed activities to the list
	for _, info := range activityMap {
		if info.Status == "Completed" || info.Status == "Running" ||
			info.Status == "Timed Out" || info.Status == "Canceled" ||
			(info.Status == "Failed" && !activitiesShowAll) {
			activities = append(activities, *info)
		}
	}

	return activities, nil
}

func printActivities(activities []ActivityInfo) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Println("=== Workflow Activities ===")
	fmt.Printf("Total Activities: %d\n\n", len(activities))

	// Print header
	fmt.Fprintln(w, "ACTIVITY\tID\tSTATUS\tATTEMPTS\tDURATION\tTIME")
	fmt.Fprintln(w, strings.Repeat("-", 100))

	for _, activity := range activities {
		// Format time
		timeStr := activity.ScheduledTime.Format("15:04:05")
		if activity.CompletedTime != nil {
			timeStr = fmt.Sprintf("%s - %s",
				activity.ScheduledTime.Format("15:04:05"),
				activity.CompletedTime.Format("15:04:05"))
		}

		// Format duration
		durationStr := "-"
		if activity.Duration > 0 {
			durationStr = formatDuration(activity.Duration)
		}

		// Format status with color codes (would need color library for actual colors)
		statusStr := activity.Status
		if activity.Status == "Failed" && activity.RetryState != "" {
			statusStr = fmt.Sprintf("%s (%s)", activity.Status, activity.RetryState)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			activity.Name,
			activity.ActivityID,
			statusStr,
			activity.Attempts,
			durationStr,
			timeStr,
		)

		// Show failure reason if any
		if activity.FailureReason != "" {
			fmt.Fprintf(w, "  └─ Error: %s\n", activity.FailureReason)
		}
	}

	// Summary
	fmt.Println("\n=== Summary ===")
	completed := 0
	failed := 0
	running := 0
	other := 0

	for _, activity := range activities {
		switch activity.Status {
		case "Completed":
			completed++
		case "Failed":
			failed++
		case "Running":
			running++
		default:
			other++
		}
	}

	fmt.Printf("Completed: %d, Failed: %d, Running: %d", completed, failed, running)
	if other > 0 {
		fmt.Printf(", Other: %d", other)
	}
	fmt.Println()
}
