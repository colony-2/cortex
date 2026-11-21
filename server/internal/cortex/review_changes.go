package cortex

import (
	"context"
	"errors"
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

func (s *Server) configureChanges(runtime jobdb.WorkflowRuntime) {
	s.workspaceDiff = func(tenant string) (jobinspect.WorkspaceDiffAPI, error) {
		return &jobinspect.Service{Runtime: runtime, Tenant: tenant, Authorize: func(ctx context.Context, job string) error {
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
		}}, nil
	}
}

func workspaceDiffQuery(r *http.Request) (jobinspect.WorkspaceDiffRequest, error) {
	request := jobinspect.WorkspaceDiffRequest{JobID: mux.Vars(r)["jobId"]}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return request, jobinspect.WorkspaceDiffDiagnostic("invalid_request", "Invalid query encoding", err)
	}
	for key, values := range q {
		if len(values) != 1 {
			return request, jobinspect.WorkspaceDiffDiagnostic("invalid_request", "Duplicate selector", nil)
		}
		value := values[0]
		switch key {
		case "task":
			request.Task = &value
		case "scope":
			request.Scope = &value
		case "workspace":
			request.Workspace = &value
		case "boundary":
			request.Boundary = &value
		case "attempt":
			n, err := strconv.Atoi(value)
			if err != nil {
				return request, jobinspect.WorkspaceDiffDiagnostic("invalid_request", "Attempt must be a positive integer", err)
			}
			request.Attempt = &n
		default:
			return request, jobinspect.WorkspaceDiffDiagnostic("invalid_request", "Use task/scope IDs from job log/story, with optional attempt, workspace and boundary selectors", nil)
		}
	}
	return request, request.Validate()
}

func writeWorkspaceDiffError(w http.ResponseWriter, err error) {
	var diagnostic *jobinspect.WorkspaceDiffError
	if !errors.As(err, &diagnostic) {
		diagnostic = jobinspect.WorkspaceDiffDiagnostic("backend", "Could not read saved workspace diff", err)
	}
	status := http.StatusBadGateway
	switch diagnostic.Code {
	case "invalid_request", "point_not_found", "workspace_ambiguous", "workspace_not_found":
		status = http.StatusBadRequest
	case "job_denied", "source_denied", "access_denied", "denied":
		status = http.StatusForbidden
	case "unsupported":
		status = http.StatusNotImplemented
	case "deadline":
		status = http.StatusGatewayTimeout
	}
	if errors.Is(err, jobdb.ErrCellForbidden) {
		status = http.StatusForbidden
	}
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	// Do not disclose foreign producer information on access failures.
	if status == http.StatusForbidden {
		diagnostic = jobinspect.WorkspaceDiffDiagnostic("job_denied", "Access to the saved comparison is denied", nil)
	}
	writeJSON(w, status, struct {
		Error      string                         `json:"error"`
		Diagnostic *jobinspect.WorkspaceDiffError `json:"diagnostic"`
	}{diagnostic.Detail, diagnostic})
}

func changesHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (s *Server) handleRetiredChanges(w http.ResponseWriter, r *http.Request) {
	changesHeaders(w)
	writeJSON(w, http.StatusGone, map[string]string{"error": "Changes history and commit ranges have been retired. Use workspace-diff with optional task/scope, attempt, workspace and boundary selectors."})
}

type workspaceDiffPreview struct {
	Metadata        *jobinspect.WorkspaceDiff `json:"metadata"`
	Patch           string                    `json:"patch"`
	PreviewComplete bool                      `json:"preview_complete"`
	DownloadURL     string                    `json:"download_url,omitempty"`
}

func (s *Server) handleWorkspaceDiff(w http.ResponseWriter, r *http.Request) {
	changesHeaders(w)
	request, err := workspaceDiffQuery(r)
	if err != nil {
		writeWorkspaceDiffError(w, err)
		return
	}
	if s.workspaceDiff == nil {
		writeWorkspaceDiffError(w, jobinspect.WorkspaceDiffDiagnostic("unsupported", "Upgrade Cortex to enable saved workspace diff reads", nil))
		return
	}
	reader, err := s.workspaceDiff(s.tenantID(r))
	if err != nil {
		writeWorkspaceDiffError(w, err)
		return
	}
	metadata, body, err := reader.OpenWorkspaceDiff(r.Context(), request)
	defer func() {
		if body != nil {
			_ = body.Close()
		}
	}()
	if err == nil {
		err = jobinspect.ValidateWorkspaceDiff(request, metadata)
	}
	if err != nil {
		writeWorkspaceDiffError(w, err)
		return
	}
	result := workspaceDiffPreview{Metadata: metadata, PreviewComplete: true}
	if metadata.Status != "unavailable" {
		if body == nil {
			writeWorkspaceDiffError(w, jobinspect.WorkspaceDiffDiagnostic("protocol", "Saved comparison stream is missing", nil))
			return
		}
		// A bounded text preview deliberately does not certify full-file integrity.
		patch, readErr := io.ReadAll(io.LimitReader(body, changesPreviewBytes+1))
		closeErr := body.Close()
		body = nil
		if err = errors.Join(readErr, closeErr); err != nil {
			writeWorkspaceDiffError(w, err)
			return
		}
		result.PreviewComplete = len(patch) <= changesPreviewBytes
		if result.PreviewComplete && metadata.Bytes != nil && int64(len(patch)) != *metadata.Bytes {
			writeWorkspaceDiffError(w, jobinspect.WorkspaceDiffDiagnostic("protocol", "Incomplete saved comparison bytes", nil))
			return
		}
		if !result.PreviewComplete {
			patch = patch[:changesPreviewBytes]
		}
		result.Patch = string(patch)
		if metadata.Source != nil {
			key := metadata.Source.Key
			result.DownloadURL = fmt.Sprintf("/api/projects/%s/jobs/%s/tasks/%d/artifacts/%s", url.PathEscape(s.tenantID(r)), url.PathEscape(key.JobId), key.TaskOrdinal, url.PathEscape(key.Name))
		}
	}
	writeJSON(w, http.StatusOK, result)
}
