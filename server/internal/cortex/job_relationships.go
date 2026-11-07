package cortex

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// Children are filtered by authoritative submission parentage before pagination.
// Lease-scoped submission fixes the child's tenant to its parent's tenant.
func (s *Server) handleJobChildren(w http.ResponseWriter, r *http.Request) {
	size := 20
	if values, ok := r.URL.Query()["pageSize"]; ok {
		var err error
		if len(values) != 1 {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("pageSize must be a positive integer"))
			return
		}
		size, err = strconv.Atoi(values[0])
		if err != nil || size <= 0 {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("pageSize must be a positive integer"))
			return
		}
	}
	if size > jobdb.MaxListJobsPageSize {
		size = jobdb.MaxListJobsPageSize
	}
	page, err := s.engine.ListJobs(r.Context(), jobdb.ListJobsRequest{
		TenantIds: []string{s.tenantID(r)}, ParentJobIDs: []string{mux.Vars(r)["jobId"]},
		IncludeHidden: true, Stores: []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived},
		PageSize: size, PageToken: r.URL.Query().Get("pageToken"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response := recipejob.ListRecipeJobsResponse{Jobs: []recipejob.RecipeJob{}, NextPageToken: page.NextPageToken}
	for _, summary := range page.Jobs {
		job, ok, err := recipejob.RecipeJobFromSummary(summary)
		if err != nil {
			writeError(w, err)
			return
		}
		if ok {
			response.Jobs = append(response.Jobs, job)
		}
	}
	writeJSON(w, http.StatusOK, response)
}
