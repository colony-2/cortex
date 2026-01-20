package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/api/internal/handlers"
	"github.com/colony-2/colony2/server/api/internal/opssetup"
	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type jobIDGenerator struct {
	count int
	max   int
}

func (g *jobIDGenerator) Generate(tenantId string) (swf.JobKey, error) {
	g.count++
	if g.count > g.max {
		return swf.JobKey{}, fmt.Errorf("too many jobs")
	}
	return swf.JobKey{TenantId: tenantId, JobId: fmt.Sprintf("job-%d", g.count)}, nil
}

type sseTestEnv struct {
	projID     string
	job        workflowctl.StartJob
	eng        swf.SWFEngine
	testRecipe *recipe.Recipe
	srv        *httptest.Server
	cleanup    func()
}

func setupSSETestEnv(t *testing.T) sseTestEnv {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	projectStore, err := project.NewStore(db)
	require.NoError(t, err)
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	require.NoError(t, err)

	cellStore, err := cell.NewStore(db)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	require.NoError(t, err)

	ctx := context.Background()
	testProject, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "test-project",
		GitRepoPath: t.TempDir(),
	})
	require.NoError(t, err)
	projID := string(testProject.ID)

	g := jobIDGenerator{max: 10}
	eng := toy.NewToyEngine([]swf.WorkSet{}, toy.WithJobIDGenerator(g.Generate))

	wf := workflow.SWFWorkflowControl{
		Engine: eng,
	}

	depContainer := opssetup.NewDependencyContainer().
		WithWorkflowControl(&wf).
		WithSSEManager(input.NewSimpleSSEManager()).
		Build()

	webExtensionRoutes, cleanup, err := opssetup.SetupOps(depContainer)
	require.NoError(t, err)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	extensionRoutes := make([]handlers.ExtensionRoute, len(webExtensionRoutes))
	for i, ext := range webExtensionRoutes {
		extensionRoutes[i] = handlers.ExtensionRoute{
			Method:  ext.Method,
			Path:    ext.Path,
			Handler: ext.Handler,
		}
	}

	workSet, err := compiler.NewRecipeWorker(depContainer, registry)
	require.NoError(t, err)
	require.NoError(t, eng.RegisterWorkers(workSet))

	h := handlers.New(nil, nil, projectSvc, cellSvc, nil, nil, nil, cellStore)
	router := h.SetupRoutesWithExtensions(nil, extensionRoutes)
	srv := httptest.NewServer(router)

	recipeYaml := `
---
id: test-input-recipe
op: input
inputs:
  form:
    question: "What is your name?"
`
	testRecipe, err := recipe.LoadRecipeFromString([]byte(recipeYaml))
	require.NoError(t, err)

	jobCtx, gitCtx := compiler.GenerateTestContext()
	job := workflowctl.StartJob{
		TenantId:   projID,
		RecipeName: testRecipe.GetMetadata().ID,
		Inputs:     map[string]interface{}{},
		JobContext: jobCtx,
		GitRef:     gitCtx.ParentRef,
	}

	cleanupFunc := func() {
		for _, c := range cleanup {
			c()
		}
		srv.Close()
	}

	return sseTestEnv{
		projID:     projID,
		job:        job,
		eng:        eng,
		testRecipe: testRecipe,
		srv:        srv,
		cleanup:    cleanupFunc,
	}
}

// TestUserInputsSSEIntegration tests the SSE stream endpoint with full server stack
func TestUserInputsSSEIntegration(t *testing.T) {
	env := setupSSETestEnv(t)
	defer env.cleanup()

	// Test SSE stream endpoint
	t.Run("SSEStreamWithEvents", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/projects/%s/user-inputs/stream", env.srv.URL, env.projID)
		client := env.srv.Client()

		// Create a context with timeout for the stream.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		require.NoError(t, err)

		respCh := make(chan *http.Response, 1)
		reqErrs := make(chan error, 1)

		go func() {
			resp, err := client.Do(req)
			if err != nil {
				reqErrs <- err
				return
			}
			respCh <- resp
		}()

		// Submit job after the stream request is in flight.
		go func() {
			_, _ = starter.StartRecipeJob(context.Background(), env.job, env.eng, *env.testRecipe)
		}()

		var resp *http.Response
		select {
		case err := <-reqErrs:
			require.NoError(t, err)
		case resp = <-respCh:
		}
		require.NotNil(t, resp)
		defer resp.Body.Close()

		// Verify SSE headers
		require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
		require.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
		require.Equal(t, "keep-alive", resp.Header.Get("Connection"))

		events := make(chan SSEEvent, 1)
		readErrs := make(chan error, 1)

		go func() {
			scanner := bufio.NewScanner(resp.Body)
			var currentEvent SSEEvent
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					if currentEvent.Type == "input_pending" {
						events <- currentEvent
						return
					}
					currentEvent = SSEEvent{}
					continue
				}
				if strings.HasPrefix(line, "event: ") {
					currentEvent.Type = strings.TrimPrefix(line, "event: ")
				} else if strings.HasPrefix(line, "data: ") {
					currentEvent.Data = strings.TrimPrefix(line, "data: ")
				}
			}
			if err := scanner.Err(); err != nil {
				readErrs <- err
				return
			}
			readErrs <- io.EOF
		}()

		select {
		case event := <-events:
			t.Logf("Event: type=%s, data=%s", event.Type, event.Data)
			var pendingData map[string]interface{}
			err := json.Unmarshal([]byte(event.Data), &pendingData)
			require.NoError(t, err)
			cancel()
		case err := <-readErrs:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for input_pending event")
		}
	})
}

func TestUserInputsSSECompletion(t *testing.T) {
	env := setupSSETestEnv(t)
	defer env.cleanup()

	url := fmt.Sprintf("%s/api/projects/%s/user-inputs/stream", env.srv.URL, env.projID)
	client := env.srv.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	pendingCh := make(chan SSEEvent, 1)
	completedCh := make(chan SSEEvent, 1)
	readErrs := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(resp.Body)
		var currentEvent SSEEvent
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				switch currentEvent.Type {
				case "input_pending":
					pendingCh <- currentEvent
				case "input_completed":
					completedCh <- currentEvent
					return
				}
				currentEvent = SSEEvent{}
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				currentEvent.Type = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				currentEvent.Data = strings.TrimPrefix(line, "data: ")
			}
		}
		if err := scanner.Err(); err != nil {
			readErrs <- err
			return
		}
		readErrs <- io.EOF
	}()

	go func() {
		_, _ = starter.StartRecipeJob(context.Background(), env.job, env.eng, *env.testRecipe)
	}()

	var jobID string
	select {
	case event := <-pendingCh:
		var pendingData map[string]interface{}
		err := json.Unmarshal([]byte(event.Data), &pendingData)
		require.NoError(t, err)
		id, ok := pendingData["id"].(string)
		require.True(t, ok, "pending id should be a string")
		jobID = id
	case err := <-readErrs:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for input_pending event")
	}

	require.NotEmpty(t, jobID)

	payload := map[string]interface{}{
		"response": "Test User",
		"fields": map[string]interface{}{
			"name": "Test User",
		},
		"user_id": "test-user",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	submitURL := fmt.Sprintf("%s/api/projects/%s/user-inputs/%s/respond", env.srv.URL, env.projID, jobID)
	submitReq, err := http.NewRequestWithContext(ctx, "POST", submitURL, bytes.NewReader(body))
	require.NoError(t, err)
	submitReq.Header.Set("Content-Type", "application/json")

	submitResp, err := client.Do(submitReq)
	require.NoError(t, err)
	defer submitResp.Body.Close()

	require.Equal(t, http.StatusOK, submitResp.StatusCode)

	select {
	case event := <-completedCh:
		var completedData map[string]interface{}
		err := json.Unmarshal([]byte(event.Data), &completedData)
		require.NoError(t, err)
		cancel()
	case err := <-readErrs:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for input_completed event")
	}
}

// SSEEvent represents a parsed server-sent event
type SSEEvent struct {
	Type string
	Data string
}

// parseSSEEvents parses SSE format from response body
func parseSSEEvents(body string) []SSEEvent {
	var events []SSEEvent
	scanner := bufio.NewScanner(strings.NewReader(body))

	var currentEvent SSEEvent
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line indicates end of event
			if currentEvent.Type != "" {
				events = append(events, currentEvent)
				currentEvent = SSEEvent{}
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			currentEvent.Type = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			currentEvent.Data = strings.TrimPrefix(line, "data: ")
		}
	}

	// Add last event if present
	if currentEvent.Type != "" {
		events = append(events, currentEvent)
	}

	return events
}

func logJobSnapshot(t *testing.T, ctl workflowctl.WorkflowControl, tenantID string, label string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	jobs, _, err := ctl.ListJobs(ctx, swf.ListJobsRequest{
		Stores:    []swf.JobStore{swf.JobStoreActive},
		TenantIds: []string{tenantID},
		PageSize:  100,
	})
	if err != nil {
		t.Logf("job_snapshot[%s]: list error: %v", label, err)
		return
	}
	t.Logf("job_snapshot[%s]: count=%d", label, len(jobs))
	for _, job := range jobs {
		waitNext := "<nil>"
		if job.TaskWaitNext != nil {
			waitNext = *job.TaskWaitNext
		}
		t.Logf(
			"job_snapshot[%s]: job=%s status=%s type=%s wait_next=%s wait_in=%v wait_out=%v cancel=%v",
			label,
			job.JobKey.JobId,
			job.Status,
			job.JobType,
			waitNext,
			job.TaskWaitInput,
			job.TaskWaitOutput,
			job.CancelRequested,
		)
	}

	filtered, _, err := ctl.ListJobs(ctx, swf.ListJobsRequest{
		Stores:    []swf.JobStore{swf.JobStoreActive},
		TenantIds: []string{tenantID},
		Statuses:  []swf.JobStatus{swf.JobStatusReady},
		JobTasks: []swf.JobTaskFilter{{
			JobType:  "recipe",
			TaskType: "input:collect_user_input",
		}},
		PageSize: 100,
	})
	if err != nil {
		t.Logf("job_snapshot[%s]: pending filter error: %v", label, err)
		return
	}
	t.Logf("job_snapshot[%s]: pending_input_count=%d", label, len(filtered))
	for _, job := range filtered {
		waitNext := "<nil>"
		if job.TaskWaitNext != nil {
			waitNext = *job.TaskWaitNext
		}
		t.Logf(
			"job_snapshot[%s]: pending job=%s status=%s type=%s wait_next=%s wait_in=%v wait_out=%v",
			label,
			job.JobKey.JobId,
			job.Status,
			job.JobType,
			waitNext,
			job.TaskWaitInput,
			job.TaskWaitOutput,
		)
	}
}

// TestUserInputsSSEFlushingWorks verifies that Flush() actually sends data
func TestUserInputsSSEFlushingWorks(t *testing.T) {
	// Setup minimal server with input routes
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	projectStore, err := project.NewStore(db)
	require.NoError(t, err)
	projectSvc, err := project.NewService(project.ServiceConfig{Store: projectStore})
	require.NoError(t, err)

	cellStore, err := cell.NewStore(db)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projectSvc})
	require.NoError(t, err)

	ctx := context.Background()
	testProject, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "test-project-flush",
		GitRepoPath: t.TempDir(),
	})
	require.NoError(t, err)
	projID := string(testProject.ID)

	// Setup workflow engine
	g := jobIDGenerator{max: 10}
	eng := toy.NewToyEngine([]swf.WorkSet{}, toy.WithJobIDGenerator(g.Generate))
	wf := workflow.SWFWorkflowControl{Engine: eng}

	depContainer := opssetup.NewDependencyContainer().
		WithWorkflowControl(&wf).
		WithSSEManager(input.NewSimpleSSEManager()).
		Build()

	webExtensionRoutes, cleanup, err := opssetup.SetupOps(depContainer)
	require.NoError(t, err)
	defer func() {
		for _, c := range cleanup {
			c()
		}
	}()

	// Convert web.ExtensionRoute to handlers.ExtensionRoute
	extensionRoutes := make([]handlers.ExtensionRoute, len(webExtensionRoutes))
	for i, ext := range webExtensionRoutes {
		extensionRoutes[i] = handlers.ExtensionRoute{
			Method:  ext.Method,
			Path:    ext.Path,
			Handler: ext.Handler,
		}
	}

	// Setup server
	h := handlers.New(nil, nil, projectSvc, cellSvc, nil, nil, nil, cellStore)
	router := h.SetupRoutesWithExtensions(nil, extensionRoutes)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Test that data is flushed immediately
	t.Run("DataIsFlushedImmediately", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/projects/%s/user-inputs/stream", srv.URL, projID)

		// Use the actual HTTP client to get streaming response
		client := srv.Client()
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Verify SSE headers
		require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

		// Read first chunk - should arrive quickly due to flushing
		buf := make([]byte, 1024)
		n, err := resp.Body.Read(buf)

		// We should get data even though stream is still open
		require.Greater(t, n, 0, "Should receive initial data immediately due to flushing")

		firstChunk := string(buf[:n])
		t.Logf("First chunk received (%d bytes): %s", n, firstChunk)

		// Verify we got the connected event
		require.Contains(t, firstChunk, "event: connected", "Should receive connected event in first chunk")
	})
}
