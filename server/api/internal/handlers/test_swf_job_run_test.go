package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
)

type fakeJobRunGetter struct {
	gotReq *swf.GetJobRunRequest
	resp   swf.GetJobRunResponse
	err    error
}

func (f *fakeJobRunGetter) GetJobRun(ctx context.Context, req swf.GetJobRunRequest) (swf.GetJobRunResponse, error) {
	f.gotReq = &req
	return f.resp, f.err
}

func TestHandleTestGetJobRun_OK(t *testing.T) {
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	projectID := "proj_123"
	jobID := "job_456"

	fake := &fakeJobRunGetter{
		resp: swf.GetJobRunResponse{
			Job: swf.JobRunSummary{
				JobKey:    swf.JobKey{TenantId: projectID, JobId: jobID},
				JobType:   "recipe",
				Status:    swf.JobStatusCompleted,
				CreatedAt: now,
				Metadata:  json.RawMessage(`{"x":1}`),
			},
			Start: swf.JobStart{
				Ordinal:   0,
				WorkerID:  "worker_1",
				CreatedAt: now,
			},
		},
	}

	h := &Handlers{swfEngine: fake}
	srv := httptest.NewServer(h.SetupRoutes(nil))
	defer srv.Close()

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		srv.URL+"/api/_test/projects/"+projectID+"/jobs/"+jobID+"/engine/get-job-run?includeInputs=true&include_outputs=1&includeArtifacts=0&include_attempt_inputs=false",
		nil,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got swf.GetJobRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Job.JobKey.TenantId != projectID || got.Job.JobKey.JobId != jobID {
		t.Fatalf("unexpected job key: %#v", got.Job.JobKey)
	}
	if fake.gotReq == nil {
		t.Fatalf("expected GetJobRun to be called")
	}
	if fake.gotReq.JobKey.TenantId != projectID || fake.gotReq.JobKey.JobId != jobID {
		t.Fatalf("unexpected request job key: %#v", fake.gotReq.JobKey)
	}
	if !fake.gotReq.IncludeInputs || !fake.gotReq.IncludeOutputs || fake.gotReq.IncludeArtifacts || fake.gotReq.IncludeAttemptInputs {
		t.Fatalf("unexpected include flags: %#v", *fake.gotReq)
	}
}

func TestHandleTestGetJobRun_NotFound(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_456"

	fake := &fakeJobRunGetter{err: swf.ErrJobNotFound}
	h := &Handlers{swfEngine: fake}
	srv := httptest.NewServer(h.SetupRoutes(nil))
	defer srv.Close()

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		srv.URL+"/api/_test/projects/"+projectID+"/jobs/"+jobID+"/engine/get-job-run",
		nil,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
