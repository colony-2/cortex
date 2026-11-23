package cortex

import (
	"github.com/colony-2/c2j/pkg/execution"
	"github.com/colony-2/c2j/pkg/recipejob"
)

// Retained requirements are presentation data, separate from current scheduling
// demand. Inspect only the metadata and payload already returned by JobDB.
type jobWithExecutionRequirements struct {
	recipejob.RecipeJob
	CancelledBy       string          `json:"cancelled_by,omitempty"`
	LaunchID          string          `json:"launch_id,omitempty"`
	RecordedExecution *execution.View `json:"recorded_execution,omitempty"`
}

func withExecutionRequirements(job recipejob.RecipeJob) jobWithExecutionRequirements {
	result := jobWithExecutionRequirements{RecipeJob: job}
	if job.CompletionStatus == "cancelled" || job.Status == "CANCELLED" {
		result.CancelledBy, result.CompletionDetail = cancellationMessage(job.CompletionDetail)
	}
	if job.Execution.Status == "in_flight" || job.Execution.Status == "not_waiting" {
		view := execution.Inspect(job.Metadata, job.ClientPayload)
		result.RecordedExecution = &view
	}
	return result
}

func withListExecutionRequirements(page recipejob.ListRecipeJobsResponse) any {
	jobs := make([]jobWithExecutionRequirements, len(page.Jobs))
	for i, job := range page.Jobs {
		jobs[i] = withExecutionRequirements(job)
	}
	return struct {
		Jobs          []jobWithExecutionRequirements `json:"jobs"`
		NextPageToken string                         `json:"next_page_token,omitempty"`
	}{jobs, page.NextPageToken}
}
