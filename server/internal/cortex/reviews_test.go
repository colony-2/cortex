package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
}

func (e *reviewArtifactEngine) GetArtifact(tenant string, key jobdb.ArtifactKey) (jobdb.Artifact, error) {
	e.tenant, e.key = tenant, key
	return jobdb.NewArtifactFromBytes(key.Name, []byte("# Original\n")), nil
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
	t.Run("summaries classify forms and skip completed inputs", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/tenant-a/pending", nil))
		var items []pendingInputSummary
		if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || len(items) != 2 || items[0].Kind != "" || items[1].Kind != "review" || items[1].DocumentCount != 1 || items[1].RequestID != "request-2" {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	})
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
		data, err := attachment.Bytes(context.Background())
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
