package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/workflow/pkg/workflow"
)

type fakeWorkflowService struct {
	listReq  *workflow.ListWorkflowsRequest
	getReq   *workflow.GetWorkflowRequest
	listResp []workflow.WorkflowSummary
	getResp  *workflow.WorkflowDetail
	listErr  error
	getErr   error
}

func (f *fakeWorkflowService) ListWorkflows(ctx context.Context, req workflow.ListWorkflowsRequest) ([]workflow.WorkflowSummary, error) {
	f.listReq = &req
	return f.listResp, f.listErr
}

func (f *fakeWorkflowService) GetWorkflow(ctx context.Context, req workflow.GetWorkflowRequest) (*workflow.WorkflowDetail, error) {
	f.getReq = &req
	return f.getResp, f.getErr
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
