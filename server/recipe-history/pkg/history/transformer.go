package history

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

// Transformer handles the transformation between Recipe/Job abstractions
// and Temporal's workflow/activity concepts
type Transformer struct {
	getRecipe GetRecipeFunc
}

// GetRecipeFunc is a function type for retrieving recipes by name
type GetRecipeFunc func(name string) (*recipe.Recipe, error)

// NewTransformer creates a new transformer instance
func NewTransformer(getRecipe GetRecipeFunc) *Transformer {
	return &Transformer{
		getRecipe: getRecipe,
	}
}

// WorkflowExecutionToJob transforms a Temporal workflow execution into a Job
func (t *Transformer) WorkflowExecutionToJob(
	execution *workflow.WorkflowExecutionInfo,
	recipeName string,
) (*recipe.Job, error) {
	// Extract job ID from workflow ID (format: recipe-name-timestamp)
	jobID := execution.Execution.WorkflowId

	// Parse start time
	startTime := execution.StartTime.AsTime()

	// Map Temporal status to JobStatus
	status := t.mapWorkflowStatusToJobStatus(execution.Status)

	// Create job instance
	job := &recipe.Job{
		ID:           jobID,
		RecipeName:   recipeName,
		Status:       status,
		StartTime:    startTime,
		UpdateTime:   startTime, // Default to start time
		WorkflowType: execution.Type.Name,
		RunID:        execution.Execution.RunId,
		ExecutionInfo: &recipe.WorkflowExecutionInfo{
			WorkflowID: execution.Execution.WorkflowId,
			RunID:      execution.Execution.RunId,
		},
	}
	
	// Update time if available
	if execution.CloseTime != nil && !execution.CloseTime.AsTime().IsZero() {
		job.UpdateTime = execution.CloseTime.AsTime()
	}

	// Add error message if failed
	if status == recipe.JobStatusFailed && execution.GetHistoryLength() > 0 {
		job.Error = t.extractErrorFromExecution(execution)
	}

	// Calculate duration and end time if completed
	if execution.CloseTime != nil && !execution.CloseTime.AsTime().IsZero() {
		endTime := execution.CloseTime.AsTime()
		job.EndTime = &endTime
		duration := endTime.Sub(startTime)
		job.Duration = &duration
	}

	return job, nil
}

// WorkflowExecutionsToJobs transforms multiple workflow executions to jobs
func (t *Transformer) WorkflowExecutionsToJobs(
	executions []*workflow.WorkflowExecutionInfo,
	recipeName string,
) ([]*recipe.Job, error) {
	jobs := make([]*recipe.Job, 0, len(executions))

	for _, exec := range executions {
		job, err := t.WorkflowExecutionToJob(exec, recipeName)
		if err != nil {
			return nil, fmt.Errorf("failed to transform execution %s: %w",
				exec.Execution.WorkflowId, err)
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// DescribeWorkflowToJob transforms a workflow description to a detailed job
func (t *Transformer) DescribeWorkflowToJob(
	desc *workflowservice.DescribeWorkflowExecutionResponse,
	recipeName string,
) (*recipe.Job, error) {
	info := desc.WorkflowExecutionInfo

	// Create basic job from execution info
	job, err := t.WorkflowExecutionToJob(info, recipeName)
	if err != nil {
		return nil, err
	}

	// Add pending activities
	if len(desc.PendingActivities) > 0 {
		job.Activities = make([]*recipe.ActivityExecution, 0, len(desc.PendingActivities))
		for _, pa := range desc.PendingActivities {
			activity := t.pendingActivityToExecution(pa)
			job.Activities = append(job.Activities, activity)
		}
	}

	// Parse workflow inputs if available
	if info.GetSearchAttributes() != nil {
		if inputs, ok := info.SearchAttributes.IndexedFields["Inputs"]; ok {
			job.Inputs = t.parseInputs(inputs)
		}
	}

	// Add workflow type info
	job.WorkflowType = info.GetType().GetName()

	return job, nil
}

// HistoryToActivityExecutions transforms workflow history into activity executions
func (t *Transformer) HistoryToActivityExecutions(
	history *history.History,
	recipeName string,
) ([]*recipe.ActivityExecution, error) {
	// Get recipe to help with activity name mapping
	rec, _ := t.getRecipe(recipeName)
	
	activities := make([]*recipe.ActivityExecution, 0)
	activityMap := make(map[int64]*recipe.ActivityExecution) // eventID -> activity

	for _, event := range history.Events {
		switch event.GetEventType() {
		case enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
			scheduled := event.GetActivityTaskScheduledEventAttributes()
			activity := &recipe.ActivityExecution{
				Name:       t.extractActivityName(scheduled.ActivityType.Name, rec),
				Status:     "scheduled",
				StartTime:  event.EventTime.AsTime(),
				ActivityID: fmt.Sprintf("%d", event.EventId),
			}
			activityMap[event.EventId] = activity
			activities = append(activities, activity)

		case enums.EVENT_TYPE_ACTIVITY_TASK_STARTED:
			started := event.GetActivityTaskStartedEventAttributes()
			if activity, ok := activityMap[started.ScheduledEventId]; ok {
				activity.Status = "running"
				activity.StartTime = event.EventTime.AsTime()
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
			completed := event.GetActivityTaskCompletedEventAttributes()
			if activity, ok := activityMap[completed.ScheduledEventId]; ok {
				activity.Status = "completed"
				activity.EndTime = event.EventTime.AsTime()
				duration := activity.EndTime.Sub(activity.StartTime)
				activity.Duration = &duration

				// Parse result
				if completed.Result != nil && len(completed.Result.Payloads) > 0 {
					activity.Result = t.parsePayload(completed.Result.Payloads[0])
				}
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_FAILED:
			failed := event.GetActivityTaskFailedEventAttributes()
			if activity, ok := activityMap[failed.ScheduledEventId]; ok {
				activity.Status = "failed"
				activity.EndTime = event.EventTime.AsTime()
				duration := activity.EndTime.Sub(activity.StartTime)
				activity.Duration = &duration
				activity.Error = failed.GetFailure().GetMessage()
			}

		case enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT:
			timedOut := event.GetActivityTaskTimedOutEventAttributes()
			if activity, ok := activityMap[timedOut.ScheduledEventId]; ok {
				activity.Status = "timed_out"
				activity.EndTime = event.EventTime.AsTime()
				activity.Error = "Activity timed out"
			}
		}
	}

	return activities, nil
}

// mapWorkflowStatusToJobStatus maps Temporal workflow status to JobStatus
func (t *Transformer) mapWorkflowStatusToJobStatus(status enums.WorkflowExecutionStatus) recipe.JobStatus {
	switch status {
	case enums.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return recipe.JobStatusRunning
	case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return recipe.JobStatusCompleted
	case enums.WORKFLOW_EXECUTION_STATUS_FAILED:
		return recipe.JobStatusFailed
	case enums.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return recipe.JobStatusCanceled
	case enums.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return recipe.JobStatusTerminated
	case enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return recipe.JobStatusRunning
	case enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return recipe.JobStatusFailed
	default:
		return recipe.JobStatusUnknown
	}
}

// extractActivityName extracts user-friendly activity name
func (t *Transformer) extractActivityName(temporalName string, recipe *recipe.Recipe) string {
	// If we have recipe metadata, try to map to original activity name
	if recipe != nil && recipe.Recipe != nil {
		// Check steps for matching activity names
		for _, step := range recipe.Recipe.Steps {
			// Check if Temporal name contains the step uses or ID
			if strings.Contains(strings.ToLower(temporalName), strings.ToLower(step.Uses)) ||
			   strings.Contains(strings.ToLower(temporalName), strings.ToLower(step.ID)) {
				return step.Uses
			}
		}
		
		// Check shared activities
		for name, sharedActivity := range recipe.Recipe.Shared {
			// Check if Temporal name contains the shared activity name or uses
			if strings.Contains(strings.ToLower(temporalName), strings.ToLower(name)) ||
			   strings.Contains(strings.ToLower(temporalName), strings.ToLower(sharedActivity.Uses)) {
				return name
			}
		}
	}

	// Otherwise, clean up the Temporal name
	// Remove common prefixes/suffixes
	name := temporalName
	name = strings.TrimPrefix(name, "Activity")
	name = strings.TrimSuffix(name, "Activity")

	// Convert from camelCase to kebab-case
	return toKebabCase(name)
}

// pendingActivityToExecution converts pending activity to execution
func (t *Transformer) pendingActivityToExecution(pa *workflow.PendingActivityInfo) *recipe.ActivityExecution {
	return &recipe.ActivityExecution{
		Name:       pa.ActivityType.Name,
		ActivityID: pa.ActivityId,
		Status:     t.mapPendingActivityState(pa.State),
		StartTime:  pa.ScheduledTime.AsTime(),
		Attempt:    int(pa.Attempt),
	}
}

// mapPendingActivityState maps pending activity state to status string
func (t *Transformer) mapPendingActivityState(state enums.PendingActivityState) string {
	switch state {
	case enums.PENDING_ACTIVITY_STATE_SCHEDULED:
		return "scheduled"
	case enums.PENDING_ACTIVITY_STATE_STARTED:
		return "running"
	case enums.PENDING_ACTIVITY_STATE_CANCEL_REQUESTED:
		return "canceling"
	default:
		return "unknown"
	}
}

// extractErrorFromExecution extracts error message from workflow execution
func (t *Transformer) extractErrorFromExecution(exec *workflow.WorkflowExecutionInfo) string {
	// This is a simplified version - in practice, you'd parse the history
	// to find the actual error event
	if exec.Status == enums.WORKFLOW_EXECUTION_STATUS_FAILED {
		return "Workflow execution failed"
	}
	if exec.Status == enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT {
		return "Workflow execution timed out"
	}
	return ""
}

// parseInputs parses workflow inputs from search attributes
func (t *Transformer) parseInputs(payload *common.Payload) map[string]interface{} {
	return t.parsePayload(payload)
}

// parsePayload parses a Temporal payload into a map
func (t *Transformer) parsePayload(payload *common.Payload) map[string]interface{} {
	if payload == nil || len(payload.Data) == 0 {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(payload.Data, &result); err != nil {
		// If not JSON, return as string
		return map[string]interface{}{
			"data": string(payload.Data),
		}
	}

	return result
}

// toKebabCase converts a string to kebab-case
func toKebabCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '-')
		}
		result = append(result, []rune(strings.ToLower(string(r)))...)
	}
	return string(result)
}