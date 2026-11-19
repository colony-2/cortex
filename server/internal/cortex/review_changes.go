package cortex

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/colony-2/c2j/pkg/jobinspect"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

const changesPreviewBytes = 2 << 20

// Keep Changes composition separate from the read-only lease history capability.
// The shared reader currently takes WorkflowRuntime; handlers receive only its
// read API, scoped to the requested tenant with job authorization on every read.
func (s *Server) configureChanges(runtime jobdb.WorkflowRuntime) {
	s.changes = func(tenant string) (jobinspect.ChangesAPI, error) {
		service := &jobinspect.Service{Runtime: runtime, Tenant: tenant, Authorize: func(ctx context.Context, job string) error {
			page, err := s.engine.ListJobs(ctx, jobdb.ListJobsRequest{
				TenantIds: []string{tenant}, JobKeys: []jobdb.JobKey{{TenantId: tenant, JobId: job}}, IncludeHidden: true,
				Stores: []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}, PageSize: 1,
			})
			if err != nil {
				return err
			}
			if len(page.Jobs) != 1 || page.Jobs[0].JobKey != (jobdb.JobKey{TenantId: tenant, JobId: job}) {
				return jobdb.ErrCellForbidden
			}
			return nil
		}}
		return jobinspect.NewChangesService(service, s.cfg.Changes)
	}
}

func (s *Server) changesReader(r *http.Request) (jobinspect.ChangesAPI, error) {
	if s.changes == nil {
		return nil, &jobinspect.ChangesError{Version: 1, Code: "backend", Detail: "Changes reader is unavailable on this server"}
	}
	return s.changes(s.tenantID(r))
}

// The browser chooses a comparison, never a repository, workspace or artifact.
func changesQuery(r *http.Request, diff bool) (jobinspect.DiffRequest, string, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return jobinspect.DiffRequest{}, "", err
	}
	allowed := map[string]bool{"cursor": !diff, "limit": !diff, "selection": true}
	if diff {
		for _, key := range []string{"all", "commit", "from", "through", "format"} {
			allowed[key] = true
		}
	}
	for key, values := range q {
		if !allowed[key] || len(values) != 1 {
			return jobinspect.DiffRequest{}, "", fmt.Errorf("invalid changes parameter")
		}
	}
	request := jobinspect.DiffRequest{ChangesRequest: jobinspect.ChangesRequest{JobID: mux.Vars(r)["jobId"], Selection: q.Get("selection"), Cursor: q.Get("cursor"), Limit: 50}, Commit: q.Get("commit"), From: q.Get("from"), Through: q.Get("through")}
	if q.Has("limit") {
		request.Limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || request.Limit < 1 || request.Limit > 500 {
			return request, "", fmt.Errorf("invalid page size")
		}
	}
	format := q.Get("format")
	if !diff {
		return request, format, nil
	}
	request.Limit = 0
	if q.Has("all") && q.Get("all") != "true" {
		return request, format, fmt.Errorf("all must be true")
	}
	request.All = q.Get("all") == "true"
	if request.Selection == "" || (format != "" && format != "patch") {
		return request, format, fmt.Errorf("selection and valid output format are required")
	}
	return request, format, nil
}

func writeChangesError(w http.ResponseWriter, err error) {
	diagnostic := jobinspect.ChangesDiagnostic(err)
	status := http.StatusUnprocessableEntity
	switch diagnostic.Code {
	case "invalid_request", "invalid_range", "invalid_cursor":
		status = http.StatusBadRequest
	case "stale_selection":
		status = http.StatusConflict
	case "job_denied", "repository_denied", "repository_authentication":
		status = http.StatusForbidden
	case "backend", "transport":
		status = http.StatusBadGateway
	case "deadline":
		status = http.StatusGatewayTimeout
	}
	writeJSON(w, status, struct {
		Error      string                   `json:"error"`
		Diagnostic *jobinspect.ChangesError `json:"diagnostic"`
	}{diagnostic.Error(), diagnostic})
}

func changesHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	changesHeaders(w)
	request, _, err := changesQuery(r, false)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	reader, err := s.changesReader(r)
	if err != nil {
		writeChangesError(w, err)
		return
	}
	history, err := reader.History(r.Context(), request.ChangesRequest)
	if err == nil {
		err = jobinspect.ValidateChangesHistory(history)
	}
	if err != nil {
		writeChangesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleChangesDiff(w http.ResponseWriter, r *http.Request) {
	changesHeaders(w)
	request, format, err := changesQuery(r, true)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	reader, err := s.changesReader(r)
	if err != nil {
		writeChangesError(w, err)
		return
	}
	metadata, body, err := reader.OpenDiff(r.Context(), request)
	if err != nil {
		writeChangesError(w, err)
		return
	}
	defer body.Close()
	if err = jobinspect.ValidateChangesDiff(metadata); err != nil {
		writeChangesError(w, err)
		return
	}
	limit := s.cfg.Changes.Limits.PatchBytes
	if limit == 0 {
		limit = jobinspect.DefaultChangesLimits().PatchBytes
	}
	// Verify complete bytes before writing headers, including for downloads.
	patch, err := io.ReadAll(io.LimitReader(body, limit+1))
	closeErr := body.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil && (int64(len(patch)) > limit || int64(len(patch)) != metadata.Bytes) {
		err = &jobinspect.ChangesError{Version: 1, Code: "protocol", Detail: "incomplete patch bytes"}
	}
	if err != nil {
		writeChangesError(w, err)
		return
	}
	if format == "patch" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="changes.patch"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(patch)))
		_, _ = w.Write(patch)
		return
	}
	complete := len(patch) <= changesPreviewBytes
	if !complete {
		patch = patch[:changesPreviewBytes]
	}
	writeJSON(w, http.StatusOK, struct {
		Metadata        *jobinspect.ChangesDiff `json:"metadata"`
		Patch           string                  `json:"patch"`
		PreviewComplete bool                    `json:"preview_complete"`
	}{metadata, string(patch), complete})
}
