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
	if job.Store == jobdb.JobStoreArchived || job.ArchivedAt != nil || job.Status == jobdb.JobStatusCompleted {
		switch job.CompletionStatus {
		case "cancelled":
			return "CANCELLED"
		case "success", "failed_app", "failed_system", "failed_timeout":
			return job.CompletionStatus
		case "":
			return "OUTCOME_UNAVAILABLE"
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

// JobDB also returns secondary-goal candidates and cannot filter completion outcomes.
// Apply primary membership and display status together to compact summaries on
// the server, filling the result page without inspecting individual jobs.
// Bound scans per request; sparse pages retain the cursor so nothing is lost.
const maxDisplayStatusPages = 20

func listJobsByDisplayStatus(ctx context.Context, lister recipejob.Lister, req recipejob.ListRecipeJobsRequest, statuses []string, rootOnly bool) (recipejob.ListRecipeJobsResponse, error) {
	if len(statuses) == 0 && len(req.GoalIDs) == 0 {
		return listScopedRecipeJobs(ctx, lister, req, rootOnly)
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
		page, err := listScopedRecipeJobs(ctx, lister, req, rootOnly)
		if err != nil {
			return recipejob.ListRecipeJobsResponse{}, err
		}
		for _, job := range page.Jobs {
			if (len(req.GoalIDs) == 0 || slices.Contains(req.GoalIDs, job.GoalID)) && (len(statuses) == 0 || slices.Contains(statuses, jobDisplayStatus(job))) {
				result.Jobs = append(result.Jobs, job)
			}
		}
		result.NextPageToken = page.NextPageToken
		if result.NextPageToken != "" && seen[result.NextPageToken] {
			return recipejob.ListRecipeJobsResponse{}, fmt.Errorf("job list cursor did not advance")
		}
		if result.NextPageToken == "" || len(result.Jobs) >= pageSize {
			break
		}
		req.PageToken = result.NextPageToken
	}
	return result, nil
}

// Apply structural parentage in JobDB before pagination. Converted Parent may
// retain inherited metadata on a direct restart and must not classify roots.
func listScopedRecipeJobs(ctx context.Context, lister recipejob.Lister, req recipejob.ListRecipeJobsRequest, rootOnly bool) (recipejob.ListRecipeJobsResponse, error) {
	if lister == nil {
		return recipejob.ListRecipeJobsResponse{}, fmt.Errorf("job lister is required")
	}
	listReq, err := recipejob.BuildListJobsRequest(req)
	if err != nil {
		return recipejob.ListRecipeJobsResponse{}, err
	}
	listReq.RootOnly = rootOnly
	page, err := recipejob.ListExecutionJobs(ctx, lister, listReq, req.ExecutionFilter)
	if err != nil {
		return recipejob.ListRecipeJobsResponse{}, err
	}
	result := recipejob.ListRecipeJobsResponse{Jobs: make([]recipejob.RecipeJob, 0, len(page.Jobs)), NextPageToken: page.NextPageToken}
	for _, summary := range page.Jobs {
		job, include, err := recipejob.RecipeJobFromSummary(summary)
		if err != nil {
			return recipejob.ListRecipeJobsResponse{}, err
		}
		if include {
			result.Jobs = append(result.Jobs, job)
		}
	}
	return result, nil
}
