package cortex

import (
	"bytes"
	"context"
	"fmt"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

type reviewRuntimeStub struct {
	forms      map[string]input.InputForm
	submission input.FormSubmission
	actor      input.Actor
	tenant     string
}

func (f *reviewRuntimeStub) ListPendingInputs(_ context.Context, tenant string) ([]input.PendingInput, error) {
	f.tenant = tenant
	return []input.PendingInput{{Id: "ordinary"}, {Id: "review"}, {Id: "completed"}}, nil
}
func (f *reviewRuntimeStub) GetForm(_ context.Context, tenant, job string) (input.InputForm, error) {
	f.tenant = tenant
	form, ok := f.forms[job]
	if !ok {
		return input.InputForm{}, input.ErrInputNotPending
	}
	return form, nil
}
func (f *reviewRuntimeStub) SubmitFormResponse(_ context.Context, tenant, job string, sub input.FormSubmission, actor input.Actor) (input.Output, error) {
	f.tenant, f.submission, f.actor = tenant, sub, actor
	if sub.RequestID != f.forms[job].RequestID {
		return input.Output{}, fmt.Errorf("request_id mismatch")
	}
	return input.Output{Receipt: &input.Receipt{RequestID: sub.RequestID, Actor: actor}}, nil
}

type reviewArtifactEngine struct {
	jobworkflow.Engine
	tenant string
	key    jobdb.ArtifactKey
	data   []byte
}

func (e *reviewArtifactEngine) GetArtifact(tenant string, key jobdb.ArtifactKey) (jobdb.Artifact, error) {
	e.tenant, e.key = tenant, key
	data := e.data
	if data == nil {
		data = []byte("# Original\n")
	}
	return jobdb.NewArtifactFromBytes(key.Name, data), nil
}
func TestReviewHTTPAdapters(t *testing.T) {
	key := jobdb.ArtifactKey{JobId: "child-producer", TaskOrdinal: 12, Name: "design.md"}
	runtime := &reviewRuntimeStub{forms: map[string]input.InputForm{
		"ordinary": {Question: "Proceed?"},
		"review":   {Kind: "review", RequestID: "request-2", Title: "Design review", Documents: map[string]artifacts.Ref{"design/id": artifacts.NewStoredRef(key)}},
	}}
	engine := &reviewArtifactEngine{}
	s := &Server{inputs: runtime, engine: engine}
	router := mux.NewRouter()
	router.HandleFunc("/{projectId}/pending", s.handlePendingInputs)
	router.HandleFunc("/{projectId}/reviews/{jobId}/documents", s.handleReviewDocument)
	router.HandleFunc("/{projectId}/reviews/{jobId}/respond", s.handleReviewResponse)
	t.Run("reads original producer in current tenant", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/tenant-a/reviews/review/documents?request_id=request-2&document_id=design%2Fid", nil))
		if w.Code != 200 || w.Body.String() != "# Original\n" || engine.key != key || engine.tenant != "tenant-a" {
			t.Fatalf("%d %s key=%v tenant=%s", w.Code, w.Body.String(), engine.key, engine.tenant)
		}
	})
	t.Run("rejects stale document requests and unknown documents", func(t *testing.T) {
		for _, tc := range []struct {
			query  string
			status int
		}{{"request_id=old&document_id=design%2Fid", 409}, {"request_id=request-2&document_id=missing", 404}} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/tenant-a/reviews/review/documents?"+tc.query, nil))
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		}
	})
	t.Run("A01 A04 preserves original binary bytes and download headers", func(t *testing.T) {
		original := []byte{137, 80, 78, 71, 0, 255, 13, 10}
		engine.data = original
		defer func() { engine.data = nil }()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/tenant-a/reviews/review/documents?request_id=request-2&document_id=design%2Fid", nil))
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), original) {
			t.Fatalf("binary response: %d %v", w.Code, w.Body.Bytes())
		}
		for name, want := range map[string]string{"Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff"} {
			if w.Header().Get(name) != want {
				t.Fatalf("%s = %q", name, w.Header().Get(name))
			}
		}
		if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
			t.Fatal("missing original attachment")
		}
	})
	t.Run("A02 A03 rejects nonreviews and membership bypasses before artifact lookup", func(t *testing.T) {
		for _, path := range []string{
			"/tenant-a/reviews/ordinary/documents?request_id=request-2&document_id=design%2Fid",
			"/tenant-a/reviews/missing/documents?request_id=request-2&document_id=design%2Fid",
			"/tenant-a/reviews/review/documents?request_id=request-2&document_id=..%2Fdesign.md",
			"/tenant-a/reviews/review/documents?request_id=request-2&document_id=https%3A%2F%2Fevil.test",
			"/tenant-a/reviews/review/documents?request_id=request-2&document_id=child-producer",
		} {
			engine.key = jobdb.ArtifactKey{}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 404 || engine.key.JobId != "" {
				t.Fatalf("membership bypass: %s %d", path, w.Code)
			}
		}
	})
	t.Run("submits bytes with application actor and exact identity", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("submission", `{"request_id":"request-2","submission_id":"answer-1","fields":{"decision":"approve"},"actor":{"id":"forged"}}`)
		part, _ := writer.CreateFormFile("annotation", "annotated.md")
		part.Write([]byte("# Annotation"))
		writer.Close()
		req := httptest.NewRequest("POST", "/tenant-a/reviews/review/respond", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Cookie", "colony2:user:email=reviewer%40example.com")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		attachment, ok := runtime.submission.Fields["annotation"].(jobdb.Artifact)
		if w.Code != 200 || !ok || runtime.actor.ID != "reviewer@example.com" || runtime.actor.Kind != "human" || runtime.tenant != "tenant-a" {
			t.Fatalf("%d %s submission=%+v actor=%+v", w.Code, w.Body.String(), runtime.submission, runtime.actor)
		}
		data, err := attachment.Bytes(testjobdb.Context(context.Background()))
		if err != nil || string(data) != "# Annotation" {
			t.Fatalf("%s %v", data, err)
		}
	})
	t.Run("requires login and propagates rejection", func(t *testing.T) {
		for _, signedIn := range []bool{false, true} {
			req := httptest.NewRequest("POST", "/tenant-a/reviews/review/respond", strings.NewReader(`{"request_id":"old","submission_id":"answer-2"}`))
			if signedIn {
				req.Header.Set("Cookie", "colony2:user:email=reviewer%40example.com")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			want := 401
			if signedIn {
				want = 400
			}
			if w.Code != want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		}
	})
}
