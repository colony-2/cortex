package cortex

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

func parseDisplayStatuses(values []string) ([]string, error) {
	for _, value := range values {
		switch value {
		case "success", "failed_app", "failed_system", "failed_timeout", "CANCEL_REQUESTED", "OUTCOME_UNAVAILABLE":
			continue
		}
		if strings.HasPrefix(value, "completion:") && len(value) > len("completion:") {
			continue
		}
		if status, ok := knownStatus(value); !ok || string(status) != value || status == jobdb.JobStatusCompleted {
			return nil, fmt.Errorf("unknown display status %q", value)
		}
	}
	return values, nil
}

// Keep this precedence in sync with jobStatusValue in the web app: a previous
// attempt's outcome never overrides a job that is still running or retrying.
func jobDisplayStatus(job recipejob.RecipeJob) string {
	if job.Store == jobdb.JobStoreArchived || job.Status == jobdb.JobStatusCompleted {
		switch job.CompletionStatus {
		case "cancelled":
			return "CANCELLED"
		case "success", "failed_app", "failed_system", "failed_timeout":
			return job.CompletionStatus
		case "":
		default:
			return "completion:" + job.CompletionStatus
		}
	} else if job.CancelRequested {
		return "CANCEL_REQUESTED"
	}
	if job.Status == jobdb.JobStatusCompleted {
		return "OUTCOME_UNAVAILABLE"
	}
	return string(job.Status)
}

// JobDB does not yet filter by completion outcome. Filter compact summaries on
// the server, filling the result page without inspecting individual jobs.
// Bound scans per request; sparse pages retain the cursor so nothing is lost.
const maxDisplayStatusPages = 20

func listJobsByDisplayStatus(ctx context.Context, lister recipejob.Lister, req recipejob.ListRecipeJobsRequest, statuses []string) (recipejob.ListRecipeJobsResponse, error) {
	if len(statuses) == 0 {
		return recipejob.ListRecipeJobs(ctx, lister, req)
	}
	result := recipejob.ListRecipeJobsResponse{Jobs: make([]recipejob.RecipeJob, 0, req.PageSize)}
	pageSize := req.PageSize
	seen := make(map[string]bool)
	for range maxDisplayStatusPages {
		if err := ctx.Err(); err != nil {
			return recipejob.ListRecipeJobsResponse{}, err
		}
		if seen[req.PageToken] {
			return recipejob.ListRecipeJobsResponse{}, fmt.Errorf("job list cursor did not advance")
		}
		seen[req.PageToken] = true
		// Fetch at most the remaining capacity so the backend cursor can be
		// returned unchanged, without discarding matches or buffering job data.
		req.PageSize = pageSize - len(result.Jobs)
		page, err := recipejob.ListRecipeJobs(ctx, lister, req)
		if err != nil {
			return recipejob.ListRecipeJobsResponse{}, err
		}
		for _, job := range page.Jobs {
			if slices.Contains(statuses, jobDisplayStatus(job)) {
				result.Jobs = append(result.Jobs, job)
			}
		}
		result.NextPageToken = page.NextPageToken
		if result.NextPageToken == "" || len(result.Jobs) >= pageSize {
			break
		}
		req.PageToken = result.NextPageToken
	}
	return result, nil
}
