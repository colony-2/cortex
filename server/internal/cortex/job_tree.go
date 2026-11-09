package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type treeNode struct {
	TenantID          string          `json:"tenant_id"`
	JobID             string          `json:"job_id"`
	ParentJobID       string          `json:"parent_job_id,omitempty"`
	JobType           string          `json:"job_type"`
	Status            jobdb.JobStatus `json:"status"`
	CompletionStatus  string          `json:"completion_status,omitempty"`
	Cell              string          `json:"cell,omitempty"`
	GoalID            string          `json:"goal_id,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	RecipeParentJobID string          `json:"recipe_parent_job_id,omitempty"`
}

func projectTreeNode(s jobdb.JobSummary) treeNode {
	n := treeNode{TenantID: s.JobKey.TenantId, JobID: s.JobKey.JobId, ParentJobID: s.ParentJobID, JobType: s.JobType, Status: s.Status, CompletionStatus: s.CompletionStatus, Cell: s.Cell, GoalID: s.GoalID, CreatedAt: s.CreatedAt}
	// Attribution is context only. Never use metadata to select a tenant or edge.
	if s.JobType == "recipe" {
		var meta struct {
			ParentJobID    string `json:"parent_job_id"`
			ParentTenantID string `json:"parent_tenant_id"`
		}
		if json.Unmarshal(s.Metadata, &meta) == nil && (meta.ParentTenantID == "" || meta.ParentTenantID == n.TenantID) {
			n.RecipeParentJobID = meta.ParentJobID
		}
	}
	return n
}
func treeError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, jobdb.ErrLeasePermissionDenied) {
		status = http.StatusForbidden
	}
	if errors.Is(err, jobdb.ErrJobNotFound) {
		status = http.StatusNotFound
	}
	if s, _, ok := jobdb.CellErrorStatus(err); ok {
		status = s
	}
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	writeHTTPError(w, status, errors.New("Execution tree records unavailable"))
}
func treeQuery(tenant string) jobdb.ListJobsRequest {
	return jobdb.ListJobsRequest{TenantIds: []string{tenant}, IncludeHidden: true, Stores: []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}}
}
func (s *Server) handleTreeNode(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tenant, id := s.tenantID(r), mux.Vars(r)["jobId"]
	query := treeQuery(tenant)
	query.JobKeys = []jobdb.JobKey{{TenantId: tenant, JobId: id}}
	query.PageSize = 1
	page, err := s.engine.ListJobs(ctx, query)
	if err != nil {
		treeError(w, err)
		return
	}
	for _, summary := range page.Jobs {
		if summary.JobKey.TenantId == tenant && summary.JobKey.JobId == id {
			writeJSON(w, 200, struct {
				Node       treeNode  `json:"node"`
				ObservedAt time.Time `json:"observed_at"`
			}{projectTreeNode(summary), time.Now().UTC()})
			return
		}
	}
	treeError(w, jobdb.ErrJobNotFound)
}
func (s *Server) handleTreeChildren(w http.ResponseWriter, r *http.Request) {
	size := 100
	if values, ok := r.URL.Query()["pageSize"]; ok {
		var err error
		if len(values) != 1 {
			writeHTTPError(w, 400, errors.New("pageSize must be a positive integer"))
			return
		}
		size, err = strconv.Atoi(values[0])
		if err != nil || size <= 0 {
			writeHTTPError(w, 400, errors.New("pageSize must be a positive integer"))
			return
		}
	}
	size = min(size, 100, jobdb.MaxListJobsPageSize)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tenant := s.tenantID(r)
	query := treeQuery(tenant)
	query.ParentJobIDs = []string{mux.Vars(r)["jobId"]}
	query.PageSize = size
	query.PageToken = r.URL.Query().Get("pageToken")
	page, err := s.engine.ListJobs(ctx, query)
	if err != nil {
		treeError(w, err)
		return
	}
	nodes := []treeNode{}
	for _, summary := range page.Jobs {
		// Foreign identities are never disclosed, even if a backend violates scope.
		if summary.JobKey.TenantId == tenant {
			nodes = append(nodes, projectTreeNode(summary))
		}
	}
	writeJSON(w, 200, struct {
		Nodes         []treeNode `json:"nodes"`
		NextPageToken string     `json:"next_page_token,omitempty"`
		ObservedAt    time.Time  `json:"observed_at"`
	}{nodes, page.NextPageToken, time.Now().UTC()})
}
