package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/workflow/pkg/workflow"
)

type fakeWorkflowService struct {
	listReq       *workflow.ListWorkflowsRequest
	getReq        *workflow.GetWorkflowRequest
	startReq      *workflow.StartWorkflowRequest
	artifactReq   *workflow.GetWorkflowArtifactRequest
	engineArtReq  *workflow.GetArtifactByOrdinalRequest
	outcomeReq    *workflow.GetWorkflowOutcomeRequest
	listResp      []workflow.WorkflowSummary
	getResp       *workflow.WorkflowDetail
	startResp     *workflow.WorkflowSummary
	artifactResp  *workflow.ArtifactData
	engineArtResp *workflow.ArtifactData
	outcomeResp   *workflow.WorkflowOutcome
	listErr       error
	getErr        error
	startErr      error
	artifactErr   error
	engineArtErr  error
	outcomeErr    error
}

func (f *fakeWorkflowService) ListWorkflows(ctx context.Context, req workflow.ListWorkflowsRequest) ([]workflow.WorkflowSummary, error) {
	f.listReq = &req
	return f.listResp, f.listErr
}

func (f *fakeWorkflowService) GetWorkflow(ctx context.Context, req workflow.GetWorkflowRequest) (*workflow.WorkflowDetail, error) {
	f.getReq = &req
	return f.getResp, f.getErr
}

func (f *fakeWorkflowService) StartWorkflow(ctx context.Context, req workflow.StartWorkflowRequest) (*workflow.WorkflowSummary, error) {
	f.startReq = &req
	return f.startResp, f.startErr
}

func (f *fakeWorkflowService) GetWorkflowArtifact(ctx context.Context, req workflow.GetWorkflowArtifactRequest) (*workflow.ArtifactData, error) {
	f.artifactReq = &req
	return f.artifactResp, f.artifactErr
}

func (f *fakeWorkflowService) GetArtifactByOrdinal(ctx context.Context, req workflow.GetArtifactByOrdinalRequest) (*workflow.ArtifactData, error) {
	f.engineArtReq = &req
	return f.engineArtResp, f.engineArtErr
}

func (f *fakeWorkflowService) GetWorkflowOutcome(ctx context.Context, req workflow.GetWorkflowOutcomeRequest) (*workflow.WorkflowOutcome, error) {
	f.outcomeReq = &req
	return f.outcomeResp, f.outcomeErr
}

func TestHandleListWorkflows_OK(t *testing.T) {
	projectID := "proj_123"
	now := time.Now().UTC()

	fake := &fakeWorkflowService{
		listResp: []workflow.WorkflowSummary{
			{
				WorkflowID: "wf_1",
				RunID:      "wf_1",
				Status:     workflow.WorkflowStatusRunning,
				RecipeName: "recipe",
				CreatedAt:  now,
				Actor:      workflow.Actor{Type: workflow.ActorTypeUser, User: &workflow.ActorUser{Email: "user@example.com"}},
			},
		},
	}

	h := &Handlers{workflows: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/workflows?status=running&ticket_id=ticket_1&cell_id=cell_1&limit=10&offset=2", nil)
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
	var body []openapi.WorkflowSummary
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected 1 workflow summary, got %d", len(body))
	}
	if fake.listReq == nil || fake.listReq.ProjectID != projectID {
		t.Fatalf("expected list request captured with project %q", projectID)
	}
	if fake.listReq.Limit != 10 || fake.listReq.Offset != 2 {
		t.Fatalf("expected limit/offset 10/2, got %d/%d", fake.listReq.Limit, fake.listReq.Offset)
	}
}

func TestHandleStartWorkflow_OK(t *testing.T) {
	projectID := "proj_123"
	now := time.Now().UTC()
	fake := &fakeWorkflowService{
		startResp: &workflow.WorkflowSummary{
			WorkflowID:  "wf_1",
			RunID:       "wf_1",
			Status:      workflow.WorkflowStatusRunning,
			RecipeName:  "workflows/ci/build",
			CreatedAt:   now,
			SubmittedAt: &now,
			Actor:       workflow.Actor{Type: workflow.ActorTypeUser},
		},
	}

	h := &Handlers{workflows: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	body := map[string]interface{}{
		"recipe_name": "workflows/ci/build",
		"cell_id":     "cell_1",
		"inputs": map[string]interface{}{
			"branch": "main",
		},
	}
	raw, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/projects/"+projectID+"/workflows", bytes.NewBuffer(raw))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	rawResp, _ := io.ReadAll(resp.Body)
	var out openapi.WorkflowSummary
	if err := json.Unmarshal(rawResp, &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, string(rawResp))
	}
	if fake.startReq == nil || fake.startReq.ProjectID != projectID || fake.startReq.RecipeName != "workflows/ci/build" {
		t.Fatalf("expected start req captured, got %#v", fake.startReq)
	}
	if out.WorkflowId != "wf_1" {
		t.Fatalf("expected workflow id wf_1, got %s", out.WorkflowId)
	}
}

func TestHandleGetWorkflow_Errors(t *testing.T) {
	projectID := "proj_123"
	workflowID := "wf_1"

	fake := &fakeWorkflowService{
		getErr: workflow.ErrWorkflowNotInProject,
	}

	h := &Handlers{workflows: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/workflows/"+workflowID, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	fake.getErr = workflow.ErrNotFound
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHandleGetWorkflow_IncludeRawJobData(t *testing.T) {
	projectID := "proj_123"
	workflowID := "wf_1"
	now := time.Now().UTC()

	fake := &fakeWorkflowService{
		getResp: &workflow.WorkflowDetail{
			WorkflowID: workflowID,
			RunID:      workflowID,
			Status:     workflow.WorkflowStatusCompleted,
			RecipeName: "recipe",
			CreatedAt:  now,
			Actor:      workflow.Actor{Type: workflow.ActorTypeUser, User: &workflow.ActorUser{Email: "user@example.com"}},
			Chapters:   []workflow.ChapterDetail{},
		},
	}

	h := &Handlers{workflows: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/workflows/"+workflowID+"?includeRawJobData=true", nil)
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
	var body openapi.WorkflowDetail
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fake.getReq == nil || !fake.getReq.IncludeRawJobData {
		t.Fatalf("expected includeRawJobData=true, got %#v", fake.getReq)
	}
}

func TestHandleGetWorkflowOutcome_OK(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_1"
	now := time.Now().UTC()

	fake := &fakeWorkflowService{
		outcomeResp: &workflow.WorkflowOutcome{
			JobID:          jobID,
			Status:         workflow.WorkflowStatusCompleted,
			AttemptOrdinal: ptrInt64(3),
			Output:         map[string]interface{}{"foo": "bar"},
			Artifacts: []workflow.ArtifactReference{{
				ArtifactID:   "a1",
				ArtifactType: "text/plain",
				Name:         "log.txt",
				SizeBytes:    ptrInt64(12),
				CreatedAt:    now,
			}},
		},
	}

	h := &Handlers{workflows: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/jobs/"+jobID+"/outcome", nil)
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
	if fake.outcomeReq == nil || fake.outcomeReq.ProjectID != projectID || fake.outcomeReq.JobID != jobID {
		t.Fatalf("expected outcome request captured, got %#v", fake.outcomeReq)
	}
	var body openapi.WorkflowOutcome
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.JobId != jobID || body.AttemptOrdinal == nil || *body.AttemptOrdinal != 3 {
		t.Fatalf("unexpected outcome body: %#v", body)
	}
}

func TestHandleGetJobArtifact_OK(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_1"
	content := []byte("hello")

	fake := &fakeWorkflowService{
		engineArtResp: &workflow.ArtifactData{
			Content:   content,
			Filename:  "log.txt",
			SizeBytes: int64(len(content)),
		},
	}

	h := &Handlers{workflows: fake}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/projects/"+projectID+"/jobs/"+jobID+"/tasks/5/artifacts/log.txt", nil)
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
	if fake.engineArtReq == nil || fake.engineArtReq.TaskOrdinal != 5 || fake.engineArtReq.ArtifactName != "log.txt" {
		t.Fatalf("unexpected request capture: %#v", fake.engineArtReq)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}
