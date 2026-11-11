package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	workerops "github.com/colony-2/c2j/pkg/worker/ops"
	c2workflow "github.com/colony-2/c2j/pkg/worker/workflow"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/workflow"
	"github.com/stretchr/testify/require"
)

// Reads go through Cortex HTTP and the public history reader over file-backed
// SQLite. A controlled worker only creates the two retained review occurrences.
func TestConversationSQLiteReopening(t *testing.T) {
	ctx := testjobdb.Context(context.Background())
	path := filepath.Join(t.TempDir(), "history.db")
	rt, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: path})
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	_, err = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "test", Repo: "example.test/test"})
	require.NoError(t, err)
	_, err = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "documents", Repo: "example.test/documents"})
	require.NoError(t, err)
	original := jobdb.NewArtifactFromBytes("original.md", []byte("first original document"))
	_, err = rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", JobID: "producer", JobType: "source", Data: jobdb.NewTaskDataOrPanic(nil, original), Cell: "documents", CellRepo: "example.test/documents"}})
	require.NoError(t, err)
	key, err := original.ArtifactKey()
	require.NoError(t, err)
	forms := []input.InputForm{}
	for _, id := range []string{"review-one", "review-two"} {
		forms = append(forms, input.InputForm{Kind: "review", Title: id, RequestID: id, Documents: map[string]artifacts.Ref{"brief": artifacts.NewStoredRef(key)}, Fields: []input.FormField{{ID: "decision", Type: input.FieldTypeShortAnswer, Question: "Decision", Required: true}, {ID: "attachment", Type: input.FieldTypeFileUpload, Question: "Attachment"}}})
	}
	worker := workflow.ExecutionWorker{JobType: "recipe", Execute: func(c workflow.ExecutionContext, _ jobdb.JobData) (jobdb.JobData, error) {
		for i, form := range forms {
			raw, e := json.Marshal(form)
			if e != nil {
				return nil, e
			}
			var value map[string]any
			if e = json.Unmarshal(raw, &value); e != nil {
				return nil, e
			}
			_, e = c.DoTask(workflow.TaskCall{Site: fmt.Sprint("input-", i), TaskType: "input:collect_user_input", ContractVersion: 1, Command: raw}, func(context.Context) (jobdb.TaskData, error) {
				return workerops.NewInvocationTaskData(workerops.ActivityInvocationRequest{Input: value})
			})
			if e != nil {
				return nil, e
			}
		}
		return jobdb.NewTaskDataOrPanic(nil), nil
	}}
	policy := jobdb.DefaultRunPolicy()
	policy.Retry.MaximumAttempts = 1
	handle, err := rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", JobID: "job", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(map[string]any{}), RunPolicy: policy, Cell: "test", CellRepo: "example.test/test"}})
	require.NoError(t, err)
	run := func() {
		r, e := workflow.GetJobForRun(ctx, rt, workflow.GetJobForRunRequest{JobKey: handle.JobKey, JobWorker: worker, WorkerID: "fixture", LeaseDuration: time.Minute})
		require.NoError(t, e)
		_, e = r.Run(nil)
		require.NoError(t, e)
	}
	var server *Server
	var runtime *input.Runtime
	setup := func() {
		engine, e := workflow.NewEngineBuilder().WithRuntime(rt).BuildEngine()
		require.NoError(t, e)
		ctl := &c2workflow.SWFWorkflowControl{Engine: engine}
		history, e := input.NewHistoryService(input.HistoryConfig{Reader: ctl})
		require.NoError(t, e)
		runtime, e = input.NewRuntime(ctl, nil)
		require.NoError(t, e)
		server = &Server{engine: engine, history: history, inputs: runtime}
	}
	setup()
	readContext := ctx
	read := func(tenant, suffix string, status int) string {
		r := httptest.NewRequest("GET", "/"+tenant+"/job"+suffix, nil).WithContext(readContext)
		// The same mux variables and guard as registered production routes.
		r = withHistoryVars(r, tenant, "job")
		w := httptest.NewRecorder()
		switch {
		case strings.HasPrefix(suffix, "/artifact"):
			server.requireArchivedJob(server.handleConversationArtifact)(w, r)
		case strings.HasPrefix(suffix, "/entry"):
			server.requireArchivedJob(server.handleConversationEntry)(w, r)
		default:
			server.requireArchivedJob(server.handleConversation)(w, r)
		}
		require.Equal(t, status, w.Code, w.Body.String())
		return w.Body.String()
	}
	run()
	var first input.JobInputStoryPage
	body := read("tenant", "", 200)
	require.NoError(t, json.Unmarshal([]byte(body), &first))
	// CV-027: actual retained-history HTTP response also composes current sections.
	var aggregate struct {
		Activity struct {
			Availability string
			Value        struct {
				JobID string `json:"job_id"`
			}
		}
		Documents struct {
			Availability string
			Value        struct{ State string }
		} `json:"final_documents"`
		Merges []input.HistoryMergeEntry
	}
	require.NoError(t, json.Unmarshal([]byte(body), &aggregate))
	require.Equal(t, "available", aggregate.Activity.Availability)
	require.Equal(t, "job", aggregate.Activity.Value.JobID)
	require.Equal(t, "available", aggregate.Documents.Availability)
	require.Equal(t, "pending", aggregate.Documents.Value.State)
	require.Len(t, first.Entries, 1)
	firstID := first.Entries[0].EntryID
	_, err = runtime.SubmitFormResponse(ctx, "tenant", "job", input.FormSubmission{RequestID: forms[0].RequestID, SubmissionID: "accept-one", Fields: map[string]any{"decision": "approve", "attachment": jobdb.NewArtifactFromBytes("answer.md", []byte("accepted response attachment"))}}, input.Actor{ID: "reviewer", Kind: "human"})
	require.NoError(t, err)
	run()
	var second input.JobInputStoryPage
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "", 200)), &second))
	require.Len(t, second.Entries, 2)
	require.Equal(t, "answered", second.Entries[0].State)
	require.NotEqual(t, firstID, second.Entries[1].EntryID)
	require.NoError(t, rt.Close(ctx))
	rt, err = sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: path})
	require.NoError(t, err)
	setup()
	var paged input.JobInputStoryPage
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "?page_size=1", 200)), &paged))
	require.NotEmpty(t, paged.NextPageToken)
	beforeReads, err := server.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: handle.JobKey})
	require.NoError(t, err)
	var later input.JobInputStoryEntryDetail
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "/entry?entry_id="+url.QueryEscape(second.Entries[1].EntryID), 200)), &later))
	require.Equal(t, "review-two", later.RequestID)
	require.Equal(t, "pending", later.State)
	var detail input.JobInputStoryEntryDetail
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "/entry?entry_id="+url.QueryEscape(firstID), 200)), &detail))
	require.Equal(t, "review-one", detail.RequestID)
	require.True(t, detail.AcceptedResponse.Present)
	require.False(t, detail.Actionability.Actionable)
	require.Len(t, detail.RequestManifest, 1)
	memberQuery := "/artifact?entry_id=" + url.QueryEscape(firstID) + "&artifact_id=" + url.QueryEscape(detail.RequestManifest[0].ArtifactID)
	require.Equal(t, "first original document", read("tenant", memberQuery, 200))
	require.NotEmpty(t, detail.ResponseManifest)
	attachmentQuery := "/artifact?entry_id=" + url.QueryEscape(firstID) + "&artifact_id=" + url.QueryEscape(detail.ResponseManifest[0].ArtifactID)
	require.Equal(t, "accepted response attachment", read("tenant", attachmentQuery, 200))
	// A cached snapshot never grants access to the original producer. This
	// context can inspect the review job but cannot inspect its document cell.
	readContext = jobdb.WithCellAuthority(context.Background(), jobdb.CellAuthority{Principal: "review-reader", TenantID: "tenant", Permissions: map[string]bool{"jobs.list": true}, Cells: map[string]bool{handle.CellID: true}})
	read("tenant", "/entry?entry_id="+url.QueryEscape(firstID)+"&snapshot_token="+url.QueryEscape(detail.SnapshotToken), 200)
	read("tenant", memberQuery+"&snapshot_token="+url.QueryEscape(detail.SnapshotToken), 404)
	// Read authority is sufficient to reopen retained bytes, but cannot acquire
	// execution work. The reads above and below must not advance the job.
	readContext = jobdb.WithCellAuthority(context.Background(), jobdb.CellAuthority{Principal: "history-reader", TenantID: "tenant", Permissions: map[string]bool{"jobs.list": true}})
	require.Equal(t, "first original document", read("tenant", memberQuery, 200))
	_, err = rt.PollWork(readContext, jobdb.PollWorkRequest{TenantId: "tenant", WorkerID: "forbidden-reader", Routes: []jobdb.Route{{JobType: "recipe"}}, Limit: 1})
	require.ErrorIs(t, err, jobdb.ErrCellForbidden)
	readContext = ctx
	read("foreign", memberQuery, 404)
	read("tenant", "/artifact?entry_id="+url.QueryEscape(firstID)+"&artifact_id=arbitrary", 404)
	afterReads, err := server.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: handle.JobKey})
	require.NoError(t, err)
	require.Equal(t, len(beforeReads.Execution), len(afterReads.Execution), "historical reads must not advance execution")
	_, err = runtime.SubmitFormResponse(ctx, "tenant", "job", input.FormSubmission{RequestID: forms[0].RequestID, SubmissionID: "stale", Fields: map[string]any{"decision": "approve"}}, input.Actor{ID: "reviewer", Kind: "human"})
	require.Error(t, err)
	_, err = runtime.SubmitFormResponse(ctx, "tenant", "job", input.FormSubmission{RequestID: forms[1].RequestID, SubmissionID: "accept-two", Fields: map[string]any{"decision": "approve"}}, input.Actor{ID: "reviewer", Kind: "human"})
	require.NoError(t, err)
	run()
	require.Equal(t, "first original document", read("tenant", memberQuery, 200))
	var continued input.JobInputStoryPage
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "?page_size=1&page_token="+url.QueryEscape(paged.NextPageToken), 200)), &continued))
	require.Len(t, continued.Entries, 1)
	require.Equal(t, second.Entries[1].EntryID, continued.Entries[0].EntryID)
	require.Equal(t, "pending", continued.Entries[0].State)
	require.False(t, continued.Entries[0].AcceptedResponse.Present)
	archived, err := rt.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"tenant"}, JobKeys: []jobdb.JobKey{handle.JobKey}, Stores: []jobdb.JobStore{jobdb.JobStoreArchived}})
	require.NoError(t, err)
	require.Len(t, archived.Jobs, 1)
	_, err = rt.DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: "tenant", JobIDs: []string{"job"}})
	require.NoError(t, err)
	visible, err := rt.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"tenant"}, JobKeys: []jobdb.JobKey{handle.JobKey}, Stores: []jobdb.JobStore{jobdb.JobStoreArchived}})
	require.NoError(t, err)
	require.Empty(t, visible.Jobs)
	require.Equal(t, "first original document", read("tenant", memberQuery, 200))
	require.Equal(t, "accepted response attachment", read("tenant", attachmentQuery, 200))
	t.Log("CT-029 bounded retained SQLite restart/archive reopening passed; protected lifetime remains upstream-dependent")
}

func withHistoryVars(r *http.Request, tenant, job string) *http.Request {
	return mux.SetURLVars(r, map[string]string{"projectId": tenant, "jobId": job})
}
