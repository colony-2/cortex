package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	workerops "github.com/colony-2/c2j/pkg/worker/ops"
	c2workflow "github.com/colony-2/c2j/pkg/worker/workflow"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func nativeReviewV2(t *testing.T, amendments ...func(*input.InputForm)) (*Server, context.Context, input.InputForm) {
	t.Helper()
	ctx := testjobdb.Context(context.Background())
	rt, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: filepath.Join(t.TempDir(), "review.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close(ctx)) })
	_, err = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "test", Repo: "example.test/test"})
	require.NoError(t, err)
	data, err := os.ReadFile("../../../web/app/tests/fixtures/recipes/browser-review-v2.yaml")
	require.NoError(t, err)
	var fixture struct {
		Sequence []struct {
			Inputs struct {
				Form input.InputForm `json:"form"`
			} `json:"inputs"`
		} `json:"sequence"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	form := fixture.Sequence[1].Inputs.Form
	form.RequestID = "native-request"
	request := form.Request.(map[string]any)
	request["review_id"] = "thread"
	request["documents"] = map[string]any{}
	delete(request, "document_metadata")
	delete(request, "overview_document_id")
	for _, amend := range amendments {
		amend(&form)
	}
	worker := workflow.ExecutionWorker{JobType: "recipe", Execute: func(c workflow.ExecutionContext, _ jobdb.JobData) (jobdb.JobData, error) {
		raw, e := json.Marshal(form)
		if e != nil {
			return nil, e
		}
		var value map[string]any
		if e = json.Unmarshal(raw, &value); e != nil {
			return nil, e
		}
		_, e = c.DoTask(workflow.TaskCall{Site: "review", TaskType: "input:collect_user_input", ContractVersion: 1, Command: raw}, func(context.Context) (jobdb.TaskData, error) {
			return workerops.NewInvocationTaskData(workerops.ActivityInvocationRequest{Input: value})
		})
		return jobdb.NewTaskDataOrPanic(nil), e
	}}
	handle, err := rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", JobID: "job", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(map[string]any{}), Cell: "test", CellRepo: "example.test/test"}})
	require.NoError(t, err)
	run, err := workflow.GetJobForRun(ctx, rt, workflow.GetJobForRunRequest{JobKey: handle.JobKey, JobWorker: worker, WorkerID: "fixture", LeaseDuration: time.Minute})
	require.NoError(t, err)
	_, err = run.Run(nil)
	require.NoError(t, err)
	engine, err := workflow.NewEngineBuilder().WithRuntime(rt).BuildEngine()
	require.NoError(t, err)
	ctl := &c2workflow.SWFWorkflowControl{Engine: engine}
	runtime, err := input.NewRuntime(ctl, nil)
	require.NoError(t, err)
	history, err := input.NewHistoryService(input.HistoryConfig{Reader: ctl})
	require.NoError(t, err)
	return &Server{inputs: runtime, engine: engine, history: history}, ctx, form
}
func postReviewV2(t *testing.T, s *Server, ctx context.Context, envelope any, files []string, cookie bool, extra func(*multipart.Writer)) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	data, err := json.Marshal(envelope)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("submission", string(data)))
	for _, text := range files {
		part, err := writer.CreateFormFile("attachments", "notes.md")
		require.NoError(t, err)
		_, err = io.WriteString(part, text)
		require.NoError(t, err)
	}
	if extra != nil {
		extra(writer)
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest("POST", "/tenant/job/respond", &body).WithContext(ctx)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if cookie {
		req.Header.Set("Cookie", "colony2:user:email=reviewer%40example.com")
	}
	req = mux.SetURLVars(req, map[string]string{"projectId": "tenant", "jobId": "job"})
	w := httptest.NewRecorder()
	s.handleReviewResponse(w, req)
	return w
}
func reviewEnvelope(response map[string]any) map[string]any {
	return map[string]any{"request_id": "native-request", "submission_id": "submission", "response": response}
}
func reviewResponse(direction, feedback string, settings map[string]any) map[string]any {
	return map[string]any{"direction": direction, "feedback": feedback, "attachments": []any{}, "settings": settings}
}
func TestReviewV2NativeSubmission(t *testing.T) {
	s, ctx, _ := nativeReviewV2(t)
	for _, tc := range []struct {
		name   string
		edit   func(map[string]any)
		files  []string
		cookie bool
		status int
		extra  func(*multipart.Writer)
	}{
		{name: "identity", cookie: false, status: 401},
		{name: "stale", cookie: true, status: 409, edit: func(e map[string]any) { e["request_id"] = "old" }},
		{name: "blank", cookie: true, status: 400},
		{name: "missing feedback", cookie: true, status: 400, edit: func(e map[string]any) {
			r := e["response"].(map[string]any)
			r["direction"] = "approve"
			delete(r, "feedback")
		}},
		{name: "null feedback", cookie: true, status: 400, edit: func(e map[string]any) {
			r := e["response"].(map[string]any)
			r["direction"] = "approve"
			r["feedback"] = nil
		}},
		{name: "settings do not satisfy feedback", cookie: true, status: 400, edit: func(e map[string]any) {
			e["response"].(map[string]any)["settings"] = map[string]any{"citations": false}
		}},
		{name: "unknown direction", cookie: true, status: 400, edit: func(e map[string]any) { e["response"].(map[string]any)["direction"] = "unknown" }},
		{name: "unknown setting", cookie: true, status: 400, edit: func(e map[string]any) {
			r := e["response"].(map[string]any)
			r["feedback"] = "question"
			r["settings"] = map[string]any{"unknown": true}
		}},
		{name: "inapplicable setting", cookie: true, status: 400, edit: func(e map[string]any) {
			r := e["response"].(map[string]any)
			r["feedback"] = "question"
			r["settings"] = map[string]any{"examples": 0}
		}},
		{name: "nonblank model-like override", cookie: true, status: 400, edit: func(e map[string]any) {
			r := e["response"].(map[string]any)
			r["feedback"] = "question"
			r["settings"] = map[string]any{"context_hint": "\u0085\ufeff"}
		}},
		{name: "browser refs", cookie: true, status: 400, edit: func(e map[string]any) {
			e["response"].(map[string]any)["attachments"] = []any{map[string]any{"kind": "stored"}}
		}},
		{name: "caller actor", cookie: true, status: 400, edit: func(e map[string]any) { e["actor"] = map[string]any{"id": "fake"} }},
		{name: "caller receipt", cookie: true, status: 400, edit: func(e map[string]any) { e["receipt"] = map[string]any{"actor": "fake"} }},
		{name: "replacement conversation", cookie: true, status: 400, edit: func(e map[string]any) { e["response"].(map[string]any)["conversation"] = []any{} }},
		{name: "duplicate submission parts", cookie: true, status: 400, extra: func(w *multipart.Writer) { require.NoError(t, w.WriteField("submission", "{}")) }},
		{name: "unexpected file part", cookie: true, status: 400, extra: func(w *multipart.Writer) { _, err := w.CreateFormFile("other", "notes.md"); require.NoError(t, err) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := reviewEnvelope(reviewResponse("question", " \u0085\ufeff", map[string]any{}))
			if tc.edit != nil {
				tc.edit(e)
			}
			w := postReviewV2(t, s, ctx, e, tc.files, tc.cookie, tc.extra)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			_, err := s.inputs.GetForm(ctx, "tenant", "job")
			require.NoError(t, err, "invalid response must leave input pending")
		})
	}
	response := reviewResponse("question", "", map[string]any{"response_style": "brief", "context_hint": nil, "citations": false})
	w := postReviewV2(t, s, ctx, reviewEnvelope(response), []string{"first bytes", "second bytes"}, true, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var out input.Output
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, input.Actor{ID: "reviewer@example.com", Kind: "human"}, out.Receipt.Actor)
	require.Equal(t, "submission", out.Receipt.SubmissionID)
	accepted := out.Response.(map[string]any)
	require.Equal(t, map[string]any{"response_style": "brief", "context_hint": nil, "citations": false}, accepted["settings"])
	attachments := accepted["attachments"].([]any)
	require.Len(t, attachments, 2)
	names := map[string]bool{}
	for i, value := range attachments {
		ref := value.(map[string]any)
		keyData, _ := json.Marshal(ref["stored"].(map[string]any)["key"])
		var key jobdb.ArtifactKey
		require.NoError(t, json.Unmarshal(keyData, &key))
		require.False(t, names[key.Name])
		names[key.Name] = true
		require.True(t, strings.HasSuffix(key.Name, "/notes.md"))
		a, err := s.engine.GetArtifact("tenant", key)
		require.NoError(t, err)
		r, err := a.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		require.Equal(t, []string{"first bytes", "second bytes"}[i], string(data))
	}
	// Retained native response manifests expose the contributed notes during the gap.
	page, err := s.history.ListJobInputStory(ctx, input.ListJobInputStoryRequest{TenantID: "tenant", JobID: "job"})
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	detail, err := s.history.GetJobInputStoryEntry(ctx, input.GetJobInputStoryEntryRequest{TenantID: "tenant", JobID: "job", EntryID: page.Entries[0].EntryID})
	require.NoError(t, err)
	require.True(t, detail.AcceptedResponse.Present)
	require.Len(t, detail.ResponseManifest, 2)
}
func TestReviewV2EnvelopeLimits(t *testing.T) {
	uploadDir := t.TempDir()
	t.Setenv("TMPDIR", uploadDir)
	s, ctx, _ := nativeReviewV2(t)
	for _, size := range []int{32 << 20, (32 << 20) + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse("question", "x", map[string]any{})), []string{strings.Repeat("x", size)}, true, nil)
			require.Equal(t, 400, w.Code, w.Body.String())
			_, err := s.inputs.GetForm(ctx, "tenant", "job")
			require.NoError(t, err)
			entries, err := os.ReadDir(uploadDir)
			require.NoError(t, err)
			for _, entry := range entries {
				require.False(t, strings.HasPrefix(entry.Name(), "multipart-"), "temporary upload leaked")
			}
		})
	}
	w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse("question", "x", map[string]any{})), []string{strings.Repeat("x", (32<<20)-(64<<10))}, true, nil)
	require.Equal(t, 200, w.Code, w.Body.String()[:min(100, w.Body.Len())])
}

func TestReviewV2DirectionEncodings(t *testing.T) {
	_, _, form := nativeReviewV2(t)
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"unknown mode", func(_ map[string]any, d map[string]any) { d["feedback_mode"] = "sometimes" }},
		{"missing mode", func(_ map[string]any, d map[string]any) { delete(d, "feedback_mode") }},
		{"missing uploads", func(_ map[string]any, d map[string]any) { delete(d, "allow_attachments") }},
		{"null uploads", func(_ map[string]any, d map[string]any) { d["allow_attachments"] = nil }},
		{"forbidden uploads", func(_ map[string]any, d map[string]any) { d["allow_attachments"] = true }},
		{"old flag even null", func(_ map[string]any, d map[string]any) { d["feedback_required"] = nil }},
		{"global even null", func(r map[string]any, _ map[string]any) { r["feedback"].(map[string]any)["allow_attachments"] = nil }},
		{"global even false", func(r map[string]any, _ map[string]any) { r["feedback"].(map[string]any)["allow_attachments"] = false }},
		{"mixed directions", func(r map[string]any, _ map[string]any) {
			r["directions"].([]any)[1] = map[string]any{"id": "question", "label": "Question", "feedback_required": true, "continues_review": true}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(form.Request)
			require.NoError(t, err)
			var request map[string]any
			require.NoError(t, json.Unmarshal(data, &request))
			tc.edit(request, request["directions"].([]any)[0].(map[string]any))
			copy := form
			copy.Request = request
			_, err = reviewV2Form(copy)
			require.Error(t, err)
		})
	}
}

func TestReviewV2CleanApproval(t *testing.T) {
	s, ctx, _ := nativeReviewV2(t)
	// All rejected attempts leave the same native request actionable. Empty files
	// count as multipart contributions even though response.attachments is empty.
	for _, tc := range []struct {
		text  string
		files []string
	}{
		{text: "notes"}, {text: " "}, {text: "\u0085\ufeff"}, {files: []string{""}}, {files: []string{"new bytes"}},
	} {
		w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse("approve", tc.text, map[string]any{"response_style": "brief"})), tc.files, true, nil)
		require.Equal(t, 400, w.Code, w.Body.String())
		_, err := s.inputs.GetForm(ctx, "tenant", "job")
		require.NoError(t, err)
	}
	w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse("approve", "", map[string]any{"response_style": "brief"})), nil, true, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var output input.Output
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &output))
	require.Equal(t, "", output.Response.(map[string]any)["feedback"])
	require.Equal(t, map[string]any{"response_style": "brief"}, output.Response.(map[string]any)["settings"])
}

func TestReviewV2RequiredDirectionUploads(t *testing.T) {
	for _, direction := range []string{"revise", "annotate"} {
		t.Run(direction, func(t *testing.T) {
			s, ctx, _ := nativeReviewV2(t)
			for _, text := range []string{"", " \u0085\ufeff"} {
				w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse(direction, text, map[string]any{"response_style": "brief"})), nil, true, nil)
				require.Equal(t, 400, w.Code, w.Body.String())
				_, err := s.inputs.GetForm(ctx, "tenant", "job")
				require.NoError(t, err)
			}
			w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse(direction, "", map[string]any{})), []string{"document revision notes"}, true, nil)
			require.Equal(t, 200, w.Code, w.Body.String())
			var output input.Output
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &output))
			require.Len(t, output.Response.(map[string]any)["attachments"], 1)
		})
	}
}

func TestReviewV2NativeSchemasWithoutBrowserOrAdapter(t *testing.T) {
	s, ctx, _ := nativeReviewV2(t)
	for _, response := range []map[string]any{
		reviewResponse("approve", " ", map[string]any{}),
		reviewResponse("revise", "\u0085\ufeff", map[string]any{"response_style": "brief"}),
		reviewResponse("annotate", "", map[string]any{}),
	} {
		_, err := s.inputs.SubmitStructuredResponse(ctx, "tenant", "job", input.StructuredSubmission{RequestID: "native-request", SubmissionID: "direct", Response: response}, input.Actor{ID: "test", Kind: "human"})
		require.Error(t, err)
		_, err = s.inputs.GetForm(ctx, "tenant", "job")
		require.NoError(t, err)
	}
	response := reviewResponse("approve", "", map[string]any{})
	response["attachments"] = []any{jobdb.NewArtifactFromBytes("direct.md", []byte("notes"))}
	_, err := s.inputs.SubmitStructuredResponse(ctx, "tenant", "job", input.StructuredSubmission{RequestID: "native-request", SubmissionID: "direct", Response: response}, input.Actor{ID: "test", Kind: "human"})
	require.Error(t, err)
	_, err = s.inputs.GetForm(ctx, "tenant", "job")
	require.NoError(t, err)
}

func TestReviewV2FrozenApprovalWithNotes(t *testing.T) {
	s, ctx, form := nativeReviewV2(t, func(form *input.InputForm) {
		r := form.Request.(map[string]any)
		r["feedback"].(map[string]any)["allow_attachments"] = true
		for _, raw := range r["directions"].([]any) {
			d := raw.(map[string]any)
			d["feedback_required"] = d["feedback_mode"] == "required"
			delete(d, "feedback_mode")
			delete(d, "allow_attachments")
		}
		branch := form.ResponseSchema["oneOf"].([]any)[0].(map[string]any)
		props := branch["properties"].(map[string]any)
		delete(props["feedback"].(map[string]any), "maxLength")
		delete(props["attachments"].(map[string]any), "maxItems")
	})
	parsed, err := reviewV2Form(form)
	require.NoError(t, err)
	require.Equal(t, "optional", parsed.Directions[0].FeedbackMode)
	w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse("approve", " exact old notes ", map[string]any{})), []string{"old note bytes"}, true, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var output input.Output
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &output))
	require.Equal(t, " exact old notes ", output.Response.(map[string]any)["feedback"])
}

func TestReviewV2OptionalTextWithoutUploads(t *testing.T) {
	s, ctx, _ := nativeReviewV2(t, func(form *input.InputForm) {
		r := form.Request.(map[string]any)
		d := r["directions"].([]any)[1].(map[string]any)
		d["feedback_mode"] = "optional"
		d["allow_attachments"] = false
		branch := form.ResponseSchema["oneOf"].([]any)[1].(map[string]any)
		delete(branch, "anyOf")
		branch["properties"].(map[string]any)["attachments"].(map[string]any)["maxItems"] = 0
	})
	w := postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse("question", "optional notes", map[string]any{})), []string{""}, true, nil)
	require.Equal(t, 400, w.Code, w.Body.String())
	_, err := s.inputs.GetForm(ctx, "tenant", "job")
	require.NoError(t, err)
	w = postReviewV2(t, s, ctx, reviewEnvelope(reviewResponse("question", " \u0085\ufeff", map[string]any{})), nil, true, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var output input.Output
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &output))
	require.Equal(t, " \u0085\ufeff", output.Response.(map[string]any)["feedback"])
}
