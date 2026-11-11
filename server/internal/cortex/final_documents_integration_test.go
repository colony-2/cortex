package cortex

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestFinalDocumentsSQLiteCortexForwarding(t *testing.T) {
	ctx := testjobdb.Context(context.Background())
	rt, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: filepath.Join(t.TempDir(), "documents.db")})
	require.NoError(t, err)
	defer rt.Close(ctx)
	_, err = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "docs", Repo: "example.test/docs"})
	require.NoError(t, err)
	engine, err := workflow.NewEngineBuilder().WithRuntime(rt).BuildEngine()
	require.NoError(t, err)
	service, err := story.New(story.ServiceConfig{Engine: engine})
	require.NoError(t, err)
	server := &Server{engine: engine, story: service}
	var producer *jobdb.ArtifactKey
	call := func(job, tenant, suffix string, bytes bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", "/"+suffix, nil).WithContext(ctx)
		request = mux.SetURLVars(request, map[string]string{"projectId": tenant, "jobId": job})
		response := httptest.NewRecorder()
		if bytes {
			server.handleFinalDocument(response, request)
		} else {
			server.handleFinalDocuments(response, request)
		}
		return response
	}
	for _, job := range []string{"grandchild", "child", "parent"} {
		alias := job + ".md"
		worker := workflow.ExecutionWorker{JobType: "documents", Execute: func(workflow.ExecutionContext, jobdb.JobData) (jobdb.JobData, error) {
			ref := artifacts.Ref{Kind: artifacts.RefKindAttached, Name: "storage.md"}
			if producer != nil {
				ref = artifacts.NewStoredRef(*producer)
			}
			return jobdb.NewTaskDataOrPanic(map[string]any{"artifact_refs": map[string]artifacts.Ref{alias: ref}, "files": []string{"intermediate.md"}}, jobdb.NewArtifactFromBytes("storage.md", []byte("original final bytes")), jobdb.NewArtifactFromBytes("intermediate.md", []byte("not declared"))), nil
		}}
		handle, err := rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", JobID: job, JobType: "documents", Cell: "docs", CellRepo: "example.test/docs", Data: jobdb.NewTaskDataOrPanic(nil)}})
		require.NoError(t, err)
		require.Equal(t, 202, call(job, "tenant", "", false).Code)
		runner, err := workflow.GetJobForRun(ctx, rt, workflow.GetJobForRunRequest{JobKey: handle.JobKey, JobWorker: worker, WorkerID: "fixture", LeaseDuration: time.Minute})
		require.NoError(t, err)
		_, err = runner.Run(nil)
		require.NoError(t, err)
		response := call(job, "tenant", "", false)
		require.Equal(t, 200, response.Code, response.Body.String())
		var manifest struct {
			story.FinalDocuments
			ManifestID string `json:"manifest_id"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &manifest))
		require.Len(t, manifest.Documents, 1)
		document := manifest.Documents[0]
		require.Equal(t, alias, document.Alias)
		require.Equal(t, "grandchild", document.Producer.JobId)
		require.Equal(t, "storage.md", document.Producer.Name)
		producer = document.Producer
		query := "?manifest_id=" + url.QueryEscape(manifest.ManifestID) + "&alias=" + url.QueryEscape(alias)
		bytes := call(job, "tenant", query, true)
		require.Equal(t, 200, bytes.Code, bytes.Body.String())
		require.Equal(t, "original final bytes", bytes.Body.String())
		require.Equal(t, 404, call(job, "foreign", query, true).Code)
		require.Equal(t, 404, call(job, "tenant", "?manifest_id="+manifest.ManifestID+"&alias=intermediate.md", true).Code)
		// Reusing a valid identity after permissions change must not grant access.
		server.engine = &producerAccessEngine{err: jobdb.ErrCellForbidden}
		denied := call(job, "tenant", query, true)
		require.Equal(t, 403, denied.Code)
		require.NotContains(t, denied.Body.String(), "original final bytes")
		server.engine = engine
	}
}

type finalRetainedEngine struct {
	workflow.Engine
	run jobdb.GetJobRunResponse
}

func (e *finalRetainedEngine) GetJobRun(context.Context, jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	return e.run, nil
}

func TestFinalDocumentsSelectedAttempt(t *testing.T) {
	engine := &finalRetainedEngine{}
	service, err := story.New(story.ServiceConfig{Engine: engine})
	require.NoError(t, err)
	server := &Server{story: service}
	engine.run.Attempts = []jobdb.JobAttempt{
		{Ordinal: 1, Output: &jobdb.TaskIO{Data: json.RawMessage(`{"artifact_refs":{"old.md":{"kind":"value","value":"previous"}}}`)}},
		{Ordinal: 2, Output: &jobdb.TaskIO{Data: json.RawMessage(`{"artifact_refs":{"final.md":{"kind":"value","value":"selected"}},"files":["intermediate.md"]}`)}},
	}
	request := mux.SetURLVars(httptest.NewRequest("GET", "/", nil), map[string]string{"projectId": "tenant", "jobId": "job"})
	engine.run.Job.Status = jobdb.JobStatusActive
	response := httptest.NewRecorder()
	server.handleFinalDocuments(response, request)
	require.Equal(t, 202, response.Code)
	for _, completion := range []string{"success", "failed_app", "cancelled"} {
		engine.run.Job.Status = jobdb.JobStatusCompleted
		if completion == "cancelled" {
			engine.run.Job.Status = jobdb.JobStatusCancelled
		}
		engine.run.Job.CompletionStatus = completion
		response = httptest.NewRecorder()
		server.handleFinalDocuments(response, request)
		require.Equal(t, 200, response.Code)
		require.Contains(t, response.Body.String(), `"alias":"final.md"`)
		require.Contains(t, response.Body.String(), `"attempt_ordinal":2`)
		require.NotContains(t, response.Body.String(), "old.md")
		require.NotContains(t, response.Body.String(), "intermediate.md")
	}
}
