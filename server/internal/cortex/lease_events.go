package cortex

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/colony-2/c2j/pkg/jobinspect"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// Only pagination is browser-selectable. Scope comes from the configured runtime
// and the existing detail route; this does not authenticate a browser principal.
func (s *Server) handleJobLeaseEvents(w http.ResponseWriter, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, jobdb.ErrInvalidLeaseEventRequest)
		return
	}
	options := jobinspect.LeaseEventOptions{IncludeHidden: true, PageSize: 100}
	for key, values := range query {
		if len(values) != 1 {
			writeError(w, jobdb.ErrInvalidLeaseEventRequest)
			return
		}
		switch key {
		case "pageSize":
			size, err := strconv.Atoi(values[0])
			if err != nil || size < 1 || size > 200 {
				writeError(w, jobdb.ErrInvalidLeaseEventRequest)
				return
			}
			options.PageSize = size
		case "pageToken":
			options.PageToken = values[0]
		default:
			writeError(w, jobdb.ErrInvalidLeaseEventRequest)
			return
		}
	}
	s.requireArchivedJob(func(w http.ResponseWriter, r *http.Request) {
		if s.leaseEvents == nil {
			writeJSON(w, http.StatusNotImplemented, errorResponse{Error: "Lease history is unavailable on this runtime", Code: "unsupported_lease_events"})
			return
		}
		service := jobinspect.Service{Runtime: s.leaseEvents, Tenant: mux.Vars(r)["projectId"]}
		page, err := service.ListJobLeaseEvents(r.Context(), mux.Vars(r)["jobId"], options)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	})(w, r)
}
