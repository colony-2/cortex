package cortex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

type detailsRuntime struct {
	reviewRuntimeStub
	err   error
	calls int
}

func (f *detailsRuntime) GetDetails(ctx context.Context, tenant, job string) (*input.UserInputDetails, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.reviewRuntimeStub.GetDetails(ctx, tenant, job)
}

type detailsEngine struct {
	jobworkflow.Engine
	store  jobdb.JobStore
	hidden bool
}

func (e *detailsEngine) ListJobs(_ context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	if len(req.TenantIds) != 1 || req.TenantIds[0] != "tenant" || len(req.JobKeys) != 1 || req.JobKeys[0].JobId != "known" {
		return jobdb.ListJobsResponse{}, nil
	}
	if e.hidden && !req.IncludeHidden {
		return jobdb.ListJobsResponse{}, nil
	}
	var archived *time.Time
	if e.store == jobdb.JobStoreArchived {
		now := time.Now()
		archived = &now
	}
	return jobdb.ListJobsResponse{Jobs: []jobdb.JobSummary{{ArchivedAt: archived, JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "known"}, JobType: "recipe", Status: jobdb.JobStatusCompleted}}}, nil
}
func TestInputDetailsContract(t *testing.T) {
	for _, tc := range []struct {
		name, tenant, job string
		err               error
		store             jobdb.JobStore
		hidden            bool
		status, calls     int
	}{
		{name: "pending", tenant: "tenant", job: "known", status: 200, calls: 1},
		{name: "absence", tenant: "tenant", job: "known", err: input.ErrInputNotPending, status: 204, calls: 1},
		{name: "wrapped absence", tenant: "tenant", job: "known", err: fmt.Errorf("wrapped: %w", input.ErrInputNotPending), status: 204, calls: 1},
		{name: "archived", tenant: "tenant", job: "known", store: jobdb.JobStoreArchived, err: input.ErrInputNotPending, status: 204, calls: 1},
		{name: "hidden", tenant: "tenant", job: "known", hidden: true, err: input.ErrInputNotPending, status: 204, calls: 1},
		{name: "unknown", tenant: "tenant", job: "unknown", status: 404},
		{name: "wrong tenant", tenant: "other", job: "known", status: 404},
		{name: "denied", tenant: "tenant", job: "known", err: jobdb.ErrLeasePermissionDenied, status: 403, calls: 1},
		{name: "storage", tenant: "tenant", job: "known", err: errors.New("storage failed"), status: 500, calls: 1},
		{name: "malformed form", tenant: "tenant", job: "known", err: errors.New("cannot decode form"), status: 500, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			runtime := &detailsRuntime{err: tc.err, reviewRuntimeStub: reviewRuntimeStub{forms: map[string]input.InputForm{"known": {RequestID: "native", Question: "Continue?"}}}}
			server := &Server{engine: &detailsEngine{store: tc.store, hidden: tc.hidden}, inputs: runtime, logger: slog.New(slog.NewTextHandler(&logs, nil))}
			router := mux.NewRouter()
			router.HandleFunc("/projects/{projectId}/user-inputs/{jobId}", server.handleInputDetails)
			request := httptest.NewRequest(http.MethodGet, "/projects/"+tc.tenant+"/user-inputs/"+tc.job, nil).WithContext(testjobdb.Context(context.Background()))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status || runtime.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, runtime.calls, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
			if tc.status == 204 && (response.Body.Len() != 0 || strings.Contains(logs.String(), "ERROR")) {
				t.Fatalf("absence body/log: %s %s", response.Body.String(), logs.String())
			}
			if tc.status == 200 && (!strings.Contains(response.Body.String(), `"request_id":"native"`) || !strings.Contains(response.Body.String(), `"jobId":"known"`) || !strings.Contains(response.Body.String(), `"question":"Continue?"`)) {
				t.Fatal(response.Body.String())
			}
			if tc.calls > 0 && tc.status >= 400 && !strings.Contains(logs.String(), "ERROR") {
				t.Fatal("missing operational diagnostic")
			}
		})
	}
}
