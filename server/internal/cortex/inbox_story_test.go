package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	coreops "github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/c2j/pkg/starter"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/c2j/pkg/task"
	"github.com/colony-2/c2j/pkg/worker/compiler"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	wf "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

const inboxTestOp = "cortex_inbox_fixture"

type inboxHTTPTask struct{}

func (inboxHTTPTask) Name() string { return inboxTestOp + ":" + inboxTestOp }
func (inboxHTTPTask) Run(wf.TaskContext, jobdb.TaskData) (jobdb.TaskData, error) {
	envelope, err := task.NewOutputEnvelope(task.OutputKindActivityInvocationOutput, map[string]any{"output": map[string]any{"kept": "output"}, "git": map[string]any{}, "nextTaskType": ""})
	if err != nil {
		return nil, err
	}
	return jobdb.NewTaskData(envelope)
}

// HTTP uses the public c2j reader over native retained SQLite history. The only
// execution is fixture creation; inspection never invokes the registered op.
func TestInboxNativeHTTP(t *testing.T) {
	coreops.Register(coreops.NewActivityMappedOpV2[struct{}, map[string]any](coreops.OpMetadata{Type: inboxTestOp, AcceptsArtifacts: true}, func(coreops.OpDependencies, context.Context, struct{}) (map[string]any, error) {
		panic("inspection executed operation")
	}))
	ctx := testjobdb.Context(context.Background())
	rt, err := sqlite.NewFromConfig(ctx, sqlite.Config{DBPath: filepath.Join(t.TempDir(), "inbox.db")})
	require.NoError(t, err)
	defer rt.Close(ctx)
	_, err = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "test", Repo: "example.test/test"})
	require.NoError(t, err)
	original := jobdb.NewArtifactFromBytes("original.md", []byte("# Original stored bytes\n"))
	_, err = rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", JobID: "producer", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(workflowctl.StartJob{TenantId: "tenant", RecipeName: "source", Cell: "test", CellRepo: "example.test/test"}, original), Cell: "test", CellRepo: "example.test/test"}})
	require.NoError(t, err)
	source, err := original.ArtifactKey()
	require.NoError(t, err)
	worker := compiler.NewRecipeJobWorker(compiler.RecipeJobWorkerOptions{})
	engine, err := wf.NewEngineBuilder().WithRuntime(rt).WithWorkerTenantId("tenant").PlusWorkers(worker, inboxHTTPTask{}).BuildEngine()
	require.NoError(t, err)
	recipe := fmt.Sprintf(`id: inbox
sequence:
 - id: read
   op: %s
   artifacts:
     docs/alias.md: {kind: stored, stored: {key: {jobId: producer, taskOrdinal: %d, name: original.md, sizeBytes: 24}}}
     docs/value.json: {kind: value, format: json, value: {large: 9007199254740993, n: null}}
     docs/text.txt: {kind: value, format: text, value: 'literal text'}
`, inboxTestOp, source.TaskOrdinal)
	start, err := jobdb.NewTaskData(workflowctl.StartJob{TenantId: "tenant", RecipeName: "inbox", Cell: "test", CellRepo: "example.test/test"}, jobdb.NewArtifactFromBytes("inbox"+starter.RecipeArtifactSuffix, []byte(recipe)))
	require.NoError(t, err)
	handle, err := engine.SubmitJob(ctx, jobdb.SubmitJob{TenantId: "tenant", JobID: "receiver", JobType: "recipe", Data: start, Cell: "test", CellRepo: "example.test/test"})
	require.NoError(t, err)
	run, err := wf.GetJobForRun(ctx, rt, wf.GetJobForRunRequest{JobKey: handle.JobKey, JobWorker: worker, TaskWorkers: []wf.TaskWorker{compiler.NewCellContextTaskWorker(), inboxHTTPTask{}}, WorkerID: "fixture", LeaseDuration: time.Minute})
	require.NoError(t, err)
	_, err = run.Run(nil)
	require.NoError(t, err)
	svc, err := story.New(story.ServiceConfig{Engine: engine})
	require.NoError(t, err)
	s := &Server{story: svc, engine: engine}
	router := mux.NewRouter()
	router.HandleFunc("/{projectId}/{jobId}/story", s.requireArchivedJob(s.handleJobStory))
	router.HandleFunc("/{projectId}/{jobId}/value", s.requireArchivedJob(s.handleInboxValue))
	router.HandleFunc("/{projectId}/{jobId}/tasks/{taskOrdinal}/artifacts/{artifactName:.+}", s.requireArchivedJob(s.handleArtifactByOrdinal))
	read := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
		return w
	}
	response := read("/tenant/receiver/story")
	require.Equal(t, 200, response.Code, response.Body.String())
	var st story.JobRunStory
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &st))
	var selected *story.JobRunStoryNode
	var visit func(*story.JobRunStoryNode)
	visit = func(n *story.JobRunStoryNode) {
		if n == nil {
			return
		}
		if n.Inbox != nil && len(n.Inbox.Entries) > 0 {
			selected = n
		}
		for _, ch := range n.Children {
			visit(ch)
		}
	}
	visit(st.Root)
	require.NotNil(t, selected, response.Body.String())
	require.Equal(t, "available", selected.Inbox.Status)
	require.Len(t, selected.Inbox.Entries, 3)
	require.Contains(t, response.Body.String(), `"kept":"output"`)
	require.NotContains(t, response.Body.String(), `"value":{"large"`)
	require.Equal(t, source, *selected.Inbox.Entries[0].SourceKey)
	content := read(fmt.Sprintf("/tenant/producer/tasks/%d/artifacts/original.md", source.TaskOrdinal))
	require.Equal(t, 200, content.Code, content.Body.String())
	require.Equal(t, "# Original stored bytes\n", content.Body.String())
	selectors := url.Values{"job_attempt": {fmt.Sprint(st.Root.JobAttempt)}, "task_ordinal": {fmt.Sprint(*selected.TaskOrdinal)}, "task_attempt": {fmt.Sprint(selected.Attempt)}, "invocation_id": {selected.InvocationID}, "binding": {"docs/value.json"}}
	get := func(q url.Values) *httptest.ResponseRecorder { return read("/tenant/receiver/value?" + q.Encode()) }
	value := get(selectors)
	require.Equal(t, 200, value.Code, value.Body.String())
	ref := artifacts.Ref{Kind: artifacts.RefKindValue, Format: "json", Value: json.RawMessage(`{"large":9007199254740993,"n":null}`)}
	execution, err := ref.ValueBytes()
	require.NoError(t, err)
	require.Equal(t, string(execution), value.Body.String())
	require.Equal(t, "application/json", value.Header().Get("Content-Type"))
	_, params, err := mime.ParseMediaType(value.Header().Get("Content-Disposition"))
	require.NoError(t, err)
	require.Equal(t, "value.json", params["filename"])
	selectors.Set("binding", "docs/text.txt")
	value = get(selectors)
	require.Equal(t, 200, value.Code)
	require.Equal(t, "literal text", value.Body.String())
	for _, tc := range []struct {
		key, value string
		status     int
	}{{"binding", "unknown", 404}, {"task_attempt", "99", 404}, {"job_attempt", "0", 400}, {"task_ordinal", "-1", 400}, {"invocation_id", "", 400}, {"value", "private payload", 400}, {"url", "https://private.test", 400}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			q := url.Values{}
			for k, v := range selectors {
				q[k] = append([]string(nil), v...)
			}
			q.Set(tc.key, tc.value)
			w := get(q)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "private payload")
		})
	}
	duplicate := selectors.Encode() + "&binding=other"
	require.Equal(t, 400, read("/tenant/receiver/value?"+duplicate).Code)
	require.Equal(t, 404, read("/other/receiver/value?"+selectors.Encode()).Code)
	require.Equal(t, 404, read("/tenant/missing/value?"+selectors.Encode()).Code)
	require.Equal(t, 404, read(fmt.Sprintf("/other/producer/tasks/%d/artifacts/original.md", source.TaskOrdinal)).Code)
}

// Controlled retained descriptors exercise transport failures which a successful
// recipe cannot naturally produce (unsupported formats and damaged history).
type inboxReadEngine struct {
	wf.Engine
	run     jobdb.GetJobRunResponse
	err     error
	request jobdb.GetJobRunRequest
}

func (e *inboxReadEngine) GetJobRun(_ context.Context, r jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	e.request = r
	return e.run, e.err
}
func TestInboxValueTransportBoundaries(t *testing.T) {
	invocation := `/scope:"$job"#1/attempt:2/task:"read"#1/attempt:3`
	engine := &inboxReadEngine{run: jobdb.GetJobRunResponse{Job: jobdb.JobRunSummary{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "receiver"}}}}
	svc, err := story.New(story.ServiceConfig{Engine: engine})
	require.NoError(t, err)
	server := &Server{story: svc}
	query := url.Values{"job_attempt": {"2"}, "invocation_id": {invocation}, "task_ordinal": {"7"}, "task_attempt": {"3"}, "binding": {"file"}}
	read := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/?"+query.Encode(), nil)
		r = mux.SetURLVars(r, map[string]string{"projectId": "tenant", "jobId": "receiver"})
		server.handleInboxValue(w, r)
		return w
	}
	for _, tc := range []struct {
		name, raw string
		status    int
		want      string
	}{
		{"text literal", `{"context":{},"input":null,"artifacts":{"file":{"kind":"value","format":"text","value":"${{ expression }}\n$(false)"}}}`, 200, "${{ expression }}\n$(false)"},
		{"json beyond int64", `{"context":{},"input":null,"artifacts":{"file":{"kind":"value","format":"json","value":{"large":9007199254740993123456789,"n":null}}}}`, 200, "{\"large\":9007199254740993123456789,\"n\":null}\n"},
		{"unsupported format", `{"context":{},"input":null,"artifacts":{"file":{"kind":"value","format":"yaml","value":"private payload"}}}`, 422, "format_unsupported"},
		{"external", `{"context":{},"input":null,"artifacts":{"file":{"kind":"external","url":"https://user:password@private.test"}}}`, 422, "source_unsupported"},
		{"unknown binding", `{"context":{},"input":null,"artifacts":{}}`, 404, "binding_unknown"},
		{"malformed history", `{"context":`, 500, "history_malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine.run.Execution = []jobdb.ExecutionHistoryEntry{{Ordinal: 7, Record: jobdb.ExecutionRecord{Kind: "result", ID: invocation, Input: &jobdb.ExecutionSnapshot{Data: []byte(tc.raw)}}}}
			w := read()
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.want)
			require.NotContains(t, w.Body.String(), "private payload")
			require.NotContains(t, w.Body.String(), "password")
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Equal(t, jobdb.JobKey{TenantId: "tenant", JobId: "receiver"}, engine.request.JobKey)
		})
	}
	engine.run.Execution = nil
	w := read()
	require.Equal(t, 404, w.Code)
	require.Contains(t, w.Body.String(), "history_missing")
	engine.err = jobdb.ErrLeasePermissionDenied
	w = read()
	require.Equal(t, 403, w.Code)
	require.Contains(t, w.Body.String(), "history_denied")
}

type inboxFilenameService struct{ story.Service }

func (inboxFilenameService) GetInboxValue(context.Context, story.GetInboxValueRequest) (*story.InboxValue, error) {
	return &story.InboxValue{Data: []byte("bytes"), Filename: "../../folder\\unsafe\r\nname.json", Format: "json"}, nil
}
func TestInboxDownloadFilename(t *testing.T) {
	s := &Server{story: inboxFilenameService{}}
	r := httptest.NewRequest("GET", "/?job_attempt=1&task_attempt=1&task_ordinal=1&invocation_id=task&binding=file", nil)
	w := httptest.NewRecorder()
	s.handleInboxValue(w, r)
	require.Equal(t, 200, w.Code)
	disposition, p, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
	require.NoError(t, err)
	require.Equal(t, "attachment", disposition)
	require.Equal(t, "unsafename.json", p["filename"])
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}
