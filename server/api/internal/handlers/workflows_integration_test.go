package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/workflow/pkg/workflow"
	"github.com/colony-2/strata-go/pkg/client"
	"github.com/colony-2/swf-go/pkg/swf"
)

type fakeSWFEngine struct {
	jobs []swf.JobSummary
}

func (f *fakeSWFEngine) ListJobs(ctx context.Context, req swf.ListJobsRequest) (swf.ListJobsResponse, error) {
	out := make([]swf.JobSummary, 0, len(f.jobs))
	for _, job := range f.jobs {
		if len(req.TenantIds) > 0 {
			match := false
			for _, tenant := range req.TenantIds {
				if job.JobKey.TenantId == tenant {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		if len(req.JobKeys) > 0 {
			match := false
			for _, key := range req.JobKeys {
				if key == job.JobKey {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		// Filter by status if provided
		// If Statuses is nil or empty, return all jobs (mimics real PGWF behavior)
		if len(req.Statuses) > 0 {
			match := false
			for _, status := range req.Statuses {
				if job.Status == status {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, job)
	}
	return swf.ListJobsResponse{Jobs: out}, nil
}

func (f *fakeSWFEngine) RegisterWorkers(*swf.WorkSet) error { return nil }
func (f *fakeSWFEngine) Run(context.Context)                {}
func (f *fakeSWFEngine) StartJob(context.Context, swf.StartJob) (swf.JobKey, error) {
	return swf.JobKey{}, fmt.Errorf("not implemented")
}
func (f *fakeSWFEngine) RestartJob(context.Context, swf.RestartJob) (swf.JobKey, error) {
	return swf.JobKey{}, fmt.Errorf("not implemented")
}
func (f *fakeSWFEngine) CancelJob(context.Context, swf.CancelJob) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeSWFEngine) CheckJobStatus(context.Context, swf.JobKey) (swf.JobStatus, error) {
	return "", fmt.Errorf("not implemented")
}
func (f *fakeSWFEngine) GetJobResult(context.Context, swf.JobKey) (swf.TaskData, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeSWFEngine) FindTasksWaitingForCapability(context.Context, string, string, []string) ([]swf.TaskHandle, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeSWFEngine) GetWaitingTask(context.Context, swf.JobKey) (swf.TaskHandle, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestWorkflowIntegration_ListAndGet(t *testing.T) {
	projectID := "proj_123"
	workflowID := "wf_1"
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	startJob := workflowctl.StartJob{
		TenantId:   projectID,
		RecipeName: "recipe",
		JobContext: contextual.JobContext{
			Actor: contextual.ActorContext{
				TicketID:   "ticket_1",
				ActorEmail: "user@example.com",
			},
			Workflow: contextual.WorkflowContext{
				CellName: "cell-a",
			},
		},
		GitRef: "main",
	}
	startPayload := map[string]interface{}{}
	startBytes, _ := json.Marshal(startJob)
	_ = json.Unmarshal(startBytes, &startPayload)

	chapters := map[int64]map[string]interface{}{
		0: {
			"meta": map[string]interface{}{
				"ordinal":    0,
				"task_type":  "workflow",
				"created_at": now.Format(time.RFC3339),
			},
			"payload_kind": "App",
			"payload":      startPayload,
		},
		1: {
			"meta": map[string]interface{}{
				"ordinal":    1,
				"task_type":  "step",
				"created_at": now.Format(time.RFC3339),
			},
			"payload_kind": "App",
			"payload": map[string]interface{}{
				"result": "ok",
			},
		},
	}

	strataSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) < 3 || parts[0] != "anthologies" || parts[2] != "stories" {
			http.NotFound(w, r)
			return
		}

		if len(parts) == 3 && r.Method == http.MethodGet {
			payload := map[string]interface{}{
				"stories": []map[string]interface{}{
					{
						"anthologyId":   parts[1],
						"storyId":       workflowID,
						"finalized":     false,
						"latestOrdinal": 1,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}

		if len(parts) < 5 || parts[4] != "chapters" {
			http.NotFound(w, r)
			return
		}

		if len(parts) == 5 && r.Method == http.MethodGet {
			payload := map[string]interface{}{
				"chapters": []int{0, 1},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}

		if len(parts) == 6 && r.Method == http.MethodGet {
			ord, err := strconv.ParseInt(parts[5], 10, 64)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			chapter, ok := chapters[ord]
			if !ok {
				http.NotFound(w, r)
				return
			}
			payload := map[string]interface{}{
				"artifacts":   []map[string]interface{}{},
				"chapter_json": chapter,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}

		http.NotFound(w, r)
	}))
	defer strataSrv.Close()

	strataClient, err := client.New(client.Config{BaseURL: strataSrv.URL, APIKey: "test"})
	if err != nil {
		t.Fatalf("strata client: %v", err)
	}

	engine := &fakeSWFEngine{
		jobs: []swf.JobSummary{
			{
				JobKey:    swf.JobKey{TenantId: projectID, JobId: workflowID},
				Status:    swf.JobStatusActive,
				CreatedAt: now,
				Payload:   json.RawMessage(`{"raw":"data"}`),
			},
		},
	}

	workflowSvc, err := workflow.New(workflow.ServiceConfig{
		Engine: engine,
		Strata: strataClient,
	})
	if err != nil {
		t.Fatalf("workflow service: %v", err)
	}

	h := &Handlers{workflows: workflowSvc}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	apiClient, err := openapi.NewClientWithResponses(srv.URL, openapi.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("api client: %v", err)
	}

	ctx := context.Background()
	listResp, err := apiClient.GetApiProjectsWorkflowsWithResponse(ctx, projectID, nil)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if listResp.JSON200 == nil || len(*listResp.JSON200) != 1 {
		t.Fatalf("expected 1 workflow, got %#v", listResp.JSON200)
	}
	if (*listResp.JSON200)[0].WorkflowId != workflowID {
		t.Fatalf("expected workflow id %q, got %q", workflowID, (*listResp.JSON200)[0].WorkflowId)
	}

	includeRaw := true
	detailResp, err := apiClient.GetApiProjectsWorkflows1WithResponse(ctx, projectID, workflowID, &openapi.GetApiProjectsWorkflows1Params{
		IncludeRawJobData: &includeRaw,
	})
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if detailResp.JSON200 == nil {
		t.Fatalf("expected detail response, got %#v", detailResp)
	}
	if len(detailResp.JSON200.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(detailResp.JSON200.Chapters))
	}
	if detailResp.JSON200.RawJobData == nil {
		t.Fatalf("expected raw job data to be present")
	}
}

func TestWorkflowIntegration_ListReturnsAllStatusesByDefault(t *testing.T) {
	projectID := "proj_123"
	runningWorkflowID := "wf_running"
	completedWorkflowID := "wf_completed"
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	createStartJobPayload := func(workflowID string) map[string]interface{} {
		startJob := workflowctl.StartJob{
			TenantId:   projectID,
			RecipeName: "recipe",
			JobContext: contextual.JobContext{
				Actor: contextual.ActorContext{
					TicketID:   "ticket_1",
					ActorEmail: "user@example.com",
				},
				Workflow: contextual.WorkflowContext{
					CellName: "cell-a",
				},
			},
			GitRef: "main",
		}
		startPayload := map[string]interface{}{}
		startBytes, _ := json.Marshal(startJob)
		_ = json.Unmarshal(startBytes, &startPayload)
		return startPayload
	}

	// Mock Strata server that returns chapters for both workflows
	strataSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")

		if len(parts) < 3 || parts[0] != "anthologies" || parts[2] != "stories" {
			http.NotFound(w, r)
			return
		}

		// List stories endpoint
		if len(parts) == 3 && r.Method == http.MethodGet {
			payload := map[string]interface{}{
				"stories": []map[string]interface{}{
					{
						"anthologyId":   parts[1],
						"storyId":       runningWorkflowID,
						"finalized":     false,
						"latestOrdinal": 0,
					},
					{
						"anthologyId":   parts[1],
						"storyId":       completedWorkflowID,
						"finalized":     true,
						"latestOrdinal": 0,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}

		// List chapters endpoint
		if len(parts) == 5 && parts[4] == "chapters" && r.Method == http.MethodGet {
			payload := map[string]interface{}{
				"chapters": []int{0},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}

		// Get specific chapter endpoint
		if len(parts) == 6 && parts[4] == "chapters" && r.Method == http.MethodGet {
			workflowID := parts[3]
			startPayload := createStartJobPayload(workflowID)
			chapter := map[string]interface{}{
				"meta": map[string]interface{}{
					"ordinal":    0,
					"task_type":  "workflow",
					"created_at": now.Format(time.RFC3339),
				},
				"payload_kind": "App",
				"payload":      startPayload,
			}
			payload := map[string]interface{}{
				"artifacts":    []map[string]interface{}{},
				"chapter_json": chapter,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}

		http.NotFound(w, r)
	}))
	defer strataSrv.Close()

	strataClient, err := client.New(client.Config{BaseURL: strataSrv.URL, APIKey: "test"})
	if err != nil {
		t.Fatalf("strata client: %v", err)
	}

	// Create fake engine with both active and completed workflows
	engine := &fakeSWFEngine{
		jobs: []swf.JobSummary{
			{
				JobKey:    swf.JobKey{TenantId: projectID, JobId: runningWorkflowID},
				Status:    swf.JobStatusActive,
				CreatedAt: now,
				Payload:   json.RawMessage(`{"raw":"data"}`),
			},
			{
				JobKey:     swf.JobKey{TenantId: projectID, JobId: completedWorkflowID},
				Status:     swf.JobStatusCompleted,
				CreatedAt:  now.Add(-1 * time.Hour),
				ArchivedAt: &now,
				Payload:    json.RawMessage(`{"raw":"data"}`),
			},
		},
	}

	workflowSvc, err := workflow.New(workflow.ServiceConfig{
		Engine: engine,
		Strata: strataClient,
	})
	if err != nil {
		t.Fatalf("workflow service: %v", err)
	}

	h := &Handlers{workflows: workflowSvc}
	router := h.SetupRoutes(nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	apiClient, err := openapi.NewClientWithResponses(srv.URL, openapi.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("api client: %v", err)
	}

	// List workflows WITHOUT status filter - should return both active and completed
	ctx := context.Background()
	listResp, err := apiClient.GetApiProjectsWorkflowsWithResponse(ctx, projectID, nil)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}

	if listResp.JSON200 == nil {
		t.Fatalf("expected response body, got nil")
	}

	workflows := *listResp.JSON200
	if len(workflows) != 2 {
		t.Fatalf("expected 2 workflows (active + completed), got %d", len(workflows))
	}

	// Verify we have one running and one completed workflow
	statusCount := make(map[openapi.WorkflowStatus]int)
	workflowIDs := make(map[string]bool)
	for _, wf := range workflows {
		statusCount[wf.Status]++
		workflowIDs[wf.WorkflowId] = true
	}

	if statusCount["running"] != 1 {
		t.Errorf("expected 1 running workflow, got %d. All statuses: %+v", statusCount["running"], statusCount)
	}
	if statusCount["completed"] != 1 {
		t.Errorf("expected 1 completed workflow, got %d. All statuses: %+v", statusCount["completed"], statusCount)
	}

	if !workflowIDs[runningWorkflowID] {
		t.Errorf("expected to find running workflow %q in results", runningWorkflowID)
	}
	if !workflowIDs[completedWorkflowID] {
		t.Errorf("expected to find completed workflow %q in results", completedWorkflowID)
	}
}
