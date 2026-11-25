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
	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/story"
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
	// Force the second frozen form into JobDB's artifact store.
	forms[1].Fields[0].Question = strings.Repeat("Explain the decision. ", 6500)
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
	var reader *countingHistoryReader
	setup := func() {
		engine, e := workflow.NewEngineBuilder().WithRuntime(rt).BuildEngine()
		require.NoError(t, e)
		ctl := &c2workflow.SWFWorkflowControl{Engine: engine}
		reader = &countingHistoryReader{HistoryReader: ctl}
		history, e := input.NewHistoryService(input.HistoryConfig{Reader: conversationHistoryReader{HistoryReader: reader}})
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
	var previousCursor *threadCursor
	checkThread := func(expected *input.UserInputDetails, count int) {
		beforeArtifacts := reader.artifactReads
		engine := &threadReadCounts{Engine: server.engine, JobRunReader: rt}
		history, err := input.NewHistoryService(input.HistoryConfig{Reader: conversationHistoryReader{HistoryReader: reader, runtime: engine}})
		require.NoError(t, err)
		pages := &singleEntryThreadHistory{HistoryService: history, HistorySnapshotService: history.(input.HistorySnapshotService)}
		storyReader, err := story.New(story.ServiceConfig{Engine: snapshotStoryEngine{jobdbschema.WorkflowEngine{Engine: engine, Registry: rt}}})
		require.NoError(t, err)
		lookup := &countedChapterLookup{chapterReader: rt, chapterLookup: rt}
		threadServer := &Server{engine: engine, history: pages, story: storyReader, chapters: lookup}
		path := "/conversation?thread=true"
		if previousCursor != nil {
			threadServer.threadCursors.remember(previousCursor)
			path += "&since_snapshot=" + url.QueryEscape(previousCursor.token)
		}
		w := httptest.NewRecorder()
		threadServer.handleThreadConversation(w, withHistoryVars(httptest.NewRequest("GET", path, nil).WithContext(ctx), "tenant", "job"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var result struct {
			ThreadCursor   string `json:"thread_cursor"`
			ReusedEntries  int    `json:"reused_entries"`
			Entries        []conversationEntry
			NextPageToken  string              `json:"next_page_token"`
			CurrentInput   json.RawMessage     `json:"current_input"`
			Documents      conversationSection `json:"final_documents"`
			InputArtifacts json.RawMessage     `json:"input_artifacts"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		require.NotEmpty(t, result.ThreadCursor)
		reused := 0
		if expected == nil {
			reused = 1
		}
		require.Equal(t, reused, result.ReusedEntries)
		require.Len(t, result.Entries, count-reused)
		if previousCursor != nil {
			require.Equal(t, "answered", result.Entries[0].State)
			require.Equal(t, []int64{previousCursor.nextOrdinal}, lookup.ordinals)
		}
		previousCursor = threadServer.threadCursors.find(handle.JobKey, result.ThreadCursor)
		require.NotNil(t, previousCursor)
		lookup.ordinals = nil
		poll := httptest.NewRecorder()
		path = "/conversation?thread=true&since_snapshot=" + url.QueryEscape(result.ThreadCursor)
		threadServer.handleThreadConversation(poll, withHistoryVars(httptest.NewRequest("GET", path, nil).WithContext(ctx), "tenant", "job"))
		require.Equal(t, http.StatusNoContent, poll.Code, poll.Body.String())
		require.Empty(t, poll.Body.Bytes())
		require.Equal(t, []int64{previousCursor.nextOrdinal}, lookup.ordinals)
		require.Equal(t, count, pages.reads, "continuations are served by the same retained snapshot")
		require.Empty(t, result.NextPageToken)
		require.NotEmpty(t, result.InputArtifacts)
		require.Equal(t, "available", result.Documents.Availability)
		expectedJSON, err := json.Marshal(expected)
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), string(result.CurrentInput))
		require.Equal(t, 1, engine.scans)
		require.Equal(t, 1, engine.lists)
		require.Equal(t, 1, engine.details)
		require.Equal(t, count-1, reader.artifactReads-beforeArtifacts, "only the second frozen form needs an artifact read")
	}
	run()
	current, err := runtime.GetDetails(ctx, "tenant", "job")
	require.NoError(t, err)
	checkThread(current, 1)
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
	current, err = runtime.GetDetails(ctx, "tenant", "job")
	require.NoError(t, err)
	checkThread(current, 2)
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
	var reviewPage struct {
		input.JobInputStoryPage
		Entries []conversationEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "?page_size=1&include_review_attachments=true", 200)), &reviewPage))
	paged = reviewPage.JobInputStoryPage
	require.NotEmpty(t, paged.NextPageToken)
	require.Equal(t, 1, reader.runReads, "the thread and attachment manifests share one JobDB run read")
	require.Len(t, reviewPage.Entries, 1)
	require.Equal(t, firstID, reviewPage.Entries[0].EntryID)
	manifests := reviewPage.Entries[0].ReviewAttachments
	require.NotNil(t, manifests)
	require.Len(t, manifests.Request, 1)
	require.NotEmpty(t, manifests.Response)
	beforeReads, err := server.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: handle.JobKey})
	require.NoError(t, err)
	var later input.JobInputStoryEntryDetail
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "/entry?entry_id="+url.QueryEscape(second.Entries[1].EntryID)+"&snapshot_token="+url.QueryEscape(paged.SnapshotToken), 200)), &later))
	require.Equal(t, "review-two", later.RequestID)
	require.Equal(t, "pending", later.State)
	require.Equal(t, "read_only", later.Actionability.Observation)
	require.Equal(t, 1, reader.runReads, "cached detail must skip the live actionability read")
	var detail input.JobInputStoryEntryDetail
	require.NoError(t, json.Unmarshal([]byte(read("tenant", "/entry?entry_id="+url.QueryEscape(firstID), 200)), &detail))
	require.Equal(t, "review-one", detail.RequestID)
	require.True(t, detail.AcceptedResponse.Present)
	require.False(t, detail.Actionability.Actionable)
	require.Len(t, detail.RequestManifest, 1)
	memberQuery := "/artifact?entry_id=" + url.QueryEscape(firstID) + "&artifact_id=" + url.QueryEscape(detail.RequestManifest[0].ArtifactID)
	require.Equal(t, 2, reader.runReads, "a standalone detail captures one snapshot")
	require.Equal(t, "first original document", read("tenant", memberQuery+"&snapshot_token="+url.QueryEscape(paged.SnapshotToken), 200))
	require.Equal(t, 2, reader.runReads, "opening a cached attachment needs only its bytes")
	require.Equal(t, "first original document", read("tenant", memberQuery, 200))
	require.Equal(t, 3, reader.runReads, "a standalone attachment reuses the snapshot obtained for the membership lookup")
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
	checkThread(nil, 2)
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

type countingHistoryReader struct {
	input.HistoryReader
	runReads, artifactReads int
}

func (r *countingHistoryReader) GetArtifactLazy(ctx context.Context, tenant string, key jobdb.ArtifactKey) jobdb.Artifact {
	r.artifactReads++
	return r.HistoryReader.GetArtifactLazy(ctx, tenant, key)
}

func (r *countingHistoryReader) GetJobRun(ctx context.Context, req jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	r.runReads++
	return r.HistoryReader.GetJobRun(ctx, req)
}

func withHistoryVars(r *http.Request, tenant, job string) *http.Request {
	return mux.SetURLVars(r, map[string]string{"projectId": tenant, "jobId": job})
}

type countedChapterLookup struct {
	chapterReader
	chapterLookup
	ordinals []int64
}

func (r *countedChapterLookup) GetChapter(ctx context.Context, ref jobdb.ChapterRef) (jobdb.Chapter, error) {
	r.ordinals = append(r.ordinals, ref.Ordinal)
	return r.chapterLookup.GetChapter(ctx, ref)
}

type threadReadCounts struct {
	workflow.Engine
	jobdb.JobRunReader
	scans, lists, details int
}

func (r *threadReadCounts) GetJobRun(ctx context.Context, req jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	return jobdb.GetJobRun(ctx, r, req)
}
func (r *threadReadCounts) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	r.lists++
	return r.JobRunReader.ListJobs(ctx, req)
}
func (r *threadReadCounts) GetJob(ctx context.Context, key jobdb.JobKey) (jobdb.JobInfo, error) {
	r.details++
	return r.JobRunReader.GetJob(ctx, key)
}
func (r *threadReadCounts) ListChapters(ctx context.Context, req jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
	r.scans++
	if req.StartOrdinal != 0 || req.EndOrdinal != nil {
		panic("expected one unbounded chapter scan")
	}
	return r.JobRunReader.ListChapters(ctx, req)
}

// Exercise the continuation path with a two-message SQLite fixture.
type singleEntryThreadHistory struct {
	input.HistoryService
	input.HistorySnapshotService
	reads int
}

func (h *singleEntryThreadHistory) ListJobInputStory(ctx context.Context, req input.ListJobInputStoryRequest) (*input.JobInputStoryPage, error) {
	h.reads++
	req.PageSize = 1
	return h.HistoryService.ListJobInputStory(ctx, req)
}
