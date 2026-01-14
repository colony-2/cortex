package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

// TestUserInputsSSEIntegration tests the SSE stream endpoint with full server stack
func TestUserInputsSSEIntegration(t *testing.T) {
	// Setup database
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

	// Create a test project
	ctx := context.Background()
	testProject, err := projectSvc.CreateProject(ctx, project.CreateInput{
		Name:        "test-project",
		GitRepoPath: t.TempDir(),
	})
	require.NoError(t, err)
	projID := string(testProject.ID)

	// Setup workflow engine and input activity
	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	g := jobIDGenerator{max: 10}
	eng := toy.NewToyEngine([]swf.WorkSet{}, toy.WithJobIDGenerator(g.Generate))

	wf := workflow.SWFWorkflowControl{
		Engine: eng,
	}

	// Setup ops with SSE manager
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

	workSet, err := compiler.NewRecipeWorker(depContainer, registry)
	require.NoError(t, err)
	require.NoError(t, eng.RegisterWorkers(workSet))

	// Setup HTTP server with extension routes
	h := handlers.New(nil, nil, projectSvc, cellSvc, nil, nil, nil, cellStore)
	router := h.SetupRoutesWithExtensions(nil, extensionRoutes)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Start a job that requires input
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

	// Start job in background
	go func() {
		_, _ = starter.StartRecipeJob(context.Background(), job, eng, *testRecipe)
	}()

	// Give job time to start
	time.Sleep(300 * time.Millisecond)

	// Test SSE stream endpoint
	t.Run("SSEStreamWithEvents", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/projects/%s/user-inputs/stream", srv.URL, projID)
		req := httptest.NewRequest("GET", url, nil)

		// Create a context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req = req.WithContext(ctx)

		// Record response
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Verify SSE headers
		require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
		require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
		require.Equal(t, "keep-alive", w.Header().Get("Connection"))

		// Parse SSE events
		body := w.Body.String()
		t.Logf("SSE Response Body:\n%s", body)

		require.NotEmpty(t, body, "SSE stream should send events")

		// Verify we got events
		events := parseSSEEvents(body)
		require.NotEmpty(t, events, "Should receive SSE events")

		// Check for expected event types
		var hasConnected, hasPending bool
		for _, event := range events {
			t.Logf("Event: type=%s, data=%s", event.Type, event.Data)
			if event.Type == "connected" {
				hasConnected = true
				// Verify client_id in data
				require.Contains(t, event.Data, "client_id")
			}
			if event.Type == "input_pending" {
				hasPending = true
				// Verify it's valid JSON
				var pendingData map[string]interface{}
				err := json.Unmarshal([]byte(event.Data), &pendingData)
				require.NoError(t, err)
			}
		}

		require.True(t, hasConnected, "Should receive 'connected' event")
		require.True(t, hasPending, "Should receive 'input_pending' event for the pending job")
	})
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
