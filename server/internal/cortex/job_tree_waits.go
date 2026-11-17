package cortex

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// This read port deliberately exposes no goal or job mutations.
type treeWaitReader interface {
	ListDependencyWaits(context.Context, jobdb.ListDependencyWaitsRequest) (jobdb.ListDependencyWaitsResponse, error)
}
type treeWaitSnapshot struct {
	State         string          `json:"state"`
	ObservedAt    time.Time       `json:"observed_at"`
	Status        jobdb.JobStatus `json:"status,omitempty"`
	LegacyWaitFor []string        `json:"legacy_wait_for,omitempty"`
}
type treeWaitsResponse struct {
	Consumer struct {
		TenantID string `json:"tenant_id"`
		JobID    string `json:"job_id"`
	} `json:"consumer"`
	Waits         []jobdb.DependencyWaitInfo `json:"waits"`
	NextPageToken string                     `json:"next_page_token,omitempty"`
	ObservedAt    time.Time                  `json:"observed_at"`
	Issues        []string                   `json:"issues"`
	Snapshot      *treeWaitSnapshot          `json:"consumer_snapshot,omitempty"`
}

func (s *Server) handleTreeWaits(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tenant, id := s.tenantID(r), mux.Vars(r)["jobId"]
	key := jobdb.JobKey{TenantId: tenant, JobId: id}
	query := treeQuery(tenant)
	query.JobKeys, query.PageSize = []jobdb.JobKey{key}, 1
	exact, err := s.engine.ListJobs(ctx, query)
	if err != nil {
		treeError(w, err)
		return
	}
	found := false
	for _, row := range exact.Jobs {
		if row.JobKey == key {
			found = true
		}
	}
	if !found {
		treeError(w, jobdb.ErrJobNotFound)
		return
	}
	reader, ok := s.engine.(treeWaitReader)
	if !ok {
		writeHTTPError(w, http.StatusNotImplemented, errors.New("Pending dependency information unavailable"))
		return
	}
	token := r.URL.Query().Get("pageToken")
	page, err := reader.ListDependencyWaits(ctx, jobdb.ListDependencyWaitsRequest{TenantId: tenant, ConsumerJobID: id, States: []jobdb.DependencyState{jobdb.DependencyPending}, PageSize: 20, PageToken: token})
	if err != nil {
		treeError(w, err)
		return
	}
	out := treeWaitsResponse{Waits: []jobdb.DependencyWaitInfo{}, Issues: []string{}, NextPageToken: page.NextPageToken, ObservedAt: time.Now().UTC()}
	out.Consumer.TenantID, out.Consumer.JobID = tenant, id
	issue := func(code string) {
		for _, old := range out.Issues {
			if old == code {
				return
			}
		}
		out.Issues = append(out.Issues, code)
	}
	if page.Waits == nil {
		issue("missing_wait_records")
	}
	// Copy only fields used by this view; never expose failure payloads or foreign identities.
	for index, wait := range page.Waits {
		if index >= 20 {
			issue("wait_page_record_limit")
			break
		}
		if wait.JobKey != key {
			issue("mismatched_consumer")
			continue
		}
		if strings.TrimSpace(wait.WaitID) == "" || wait.State != jobdb.DependencyPending || wait.Dependencies == nil {
			issue("invalid_wait_record")
			continue
		}
		projected := jobdb.DependencyWaitInfo{JobKey: key, WaitID: wait.WaitID, State: wait.State, Deadline: wait.Deadline, Dependencies: []jobdb.DependencyResolution{}}
		for _, d := range wait.Dependencies {
			projected.Dependencies = append(projected.Dependencies, jobdb.DependencyResolution{Target: d.Target, Condition: d.Condition, State: d.State, BlockedReason: d.BlockedReason})
			if (strings.TrimSpace(d.Target.JobID) == "") == (strings.TrimSpace(d.Target.GoalID) == "") || strings.TrimSpace(string(d.Condition)) == "" {
				issue("invalid_dependency")
			}
			switch d.State {
			case jobdb.DependencyPending, jobdb.DependencySatisfied, jobdb.DependencyFailed, jobdb.DependencyTimedOut, jobdb.DependencyCancelled:
			default:
				issue("unknown_dependency_state")
			}
		}
		out.Waits = append(out.Waits, projected)
	}
	if token == "" {
		snapshot := &treeWaitSnapshot{State: "unavailable", ObservedAt: time.Now().UTC()}
		if info, err := s.engine.GetJob(ctx, key); err == nil {
			snapshot.State, snapshot.Status, snapshot.LegacyWaitFor = "available", info.Status, info.WaitFor
		}
		out.Snapshot = snapshot
	}
	writeJSON(w, http.StatusOK, out)
}
