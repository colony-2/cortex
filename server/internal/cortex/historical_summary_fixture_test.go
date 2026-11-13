package cortex

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/colony-2/jobdb/pkg/jobdb"
)

// Stored read fixture for translated historical summaries, not a runtime
// submission path or a migration test. Unimplemented detail operations panic.
type historicalSummaryEngine struct {
	compactListEngine
	rows []jobdb.JobSummary
}

func (e *historicalSummaryEngine) ListJobs(_ context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	e.requests = append(e.requests, req)
	rows := []jobdb.JobSummary{}
	for _, j := range e.rows {
		if len(req.TenantIds) > 0 && !slices.Contains(req.TenantIds, j.JobKey.TenantId) {
			continue
		}
		if len(req.CellNames) > 0 && !slices.Contains(req.CellNames, j.Cell) {
			continue
		}
		if len(req.Statuses) > 0 && !slices.Contains(req.Statuses, j.Status) {
			continue
		}
		store := jobdb.JobStoreActive
		if j.ArchivedAt != nil {
			store = jobdb.JobStoreArchived
		}
		if len(req.Stores) > 0 && !slices.Contains(req.Stores, store) {
			continue
		}
		if len(req.GoalIDs) > 0 && !slices.Contains(req.GoalIDs, j.GoalID) {
			continue
		}
		rows = append(rows, j)
	}
	offset := 0
	if req.PageToken != "" {
		var err error
		offset, err = strconv.Atoi(strings.TrimPrefix(req.PageToken, "stored:"))
		if err != nil || offset < 0 || offset > len(rows) {
			return jobdb.ListJobsResponse{}, fmt.Errorf("bad stored cursor")
		}
	}
	end := min(len(rows), offset+req.PageSize)
	page := jobdb.ListJobsResponse{Jobs: rows[offset:end]}
	if end < len(rows) {
		page.NextPageToken = fmt.Sprintf("stored:%d", end)
	}
	return page, nil
}
