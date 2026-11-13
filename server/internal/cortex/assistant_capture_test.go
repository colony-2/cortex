package cortex

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

type assistantCaptureEngine struct {
	jobworkflow.Engine
	tenant string
	keys   []jobdb.ArtifactKey
	data   map[string][]byte
}

func (e *assistantCaptureEngine) GetArtifact(tenant string, key jobdb.ArtifactKey) (jobdb.Artifact, error) {
	e.tenant = tenant
	e.keys = append(e.keys, key)
	data, ok := e.data[key.Name]
	if !ok {
		return nil, errors.New("expired")
	}
	return jobdb.NewArtifactFromBytes(key.Name, data), nil
}
func TestAssistantCaptureExactReview(t *testing.T) {
	form := input.InputForm{Kind: "review", RequestID: "exact", Documents: map[string]artifacts.Ref{}}
	for id, name := range map[string]string{"a": "child.md", "b": "binary.png", "c": "expired.txt", "d": "extra.txt"} {
		form.Documents[id] = artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "child", TaskOrdinal: 12, Name: name})
	}
	engine := &assistantCaptureEngine{data: map[string][]byte{"child.md": []byte(strings.Repeat("child text ", 200)), "binary.png": {0, 255}, "extra.txt": []byte("last")}}
	runtime := &reviewRuntimeStub{forms: map[string]input.InputForm{"parent": form}}
	s := &Server{inputs: runtime, engine: engine}
	source := AssistantSource{Kind: "review", JobID: "parent", RequestID: "exact"}
	capture, err := s.captureAssistantSource(context.Background(), "tenant", source, assistantCaptureLimits{DocumentBytes: 1600, TotalBytes: 1600, Documents: 32})
	if err != nil {
		t.Fatal(err)
	}
	if capture.Source != source || capture.TenantID != "tenant" || capture.At.IsZero() || len(capture.Documents) != 4 {
		t.Fatalf("capture=%+v", capture)
	}
	want := []string{"truncated", "unsupported", "unavailable", "budget_omitted"}
	for i, d := range capture.Documents {
		if d.Outcome != want[i] || d.Ref.Stored.Key.JobId != "child" || d.Ref.Stored.Key.TaskOrdinal != 12 {
			t.Fatalf("document=%+v", d)
		}
	}
	if len(capture.Documents[0].Text) != 1600 || !strings.Contains(capture.Documents[0].Text[1000:], "child text") || engine.tenant != "tenant" {
		t.Fatal("lost child text/tenant")
	}
	source.RequestID = "replaced"
	reads := len(engine.keys)
	if _, err = s.captureAssistantSource(context.Background(), "tenant", source, assistantCaptureLimits{1600, 1600, 32}); !errors.Is(err, ErrAssistantConflict) || len(engine.keys) != reads {
		t.Fatal("stale review read bytes", err)
	}
}
func TestAssistantCaptureGlobalAndEncoding(t *testing.T) {
	s := &Server{}
	capture, err := s.captureAssistantSource(context.Background(), "tenant", AssistantSource{Kind: "global"}, assistantCaptureLimits{})
	if err != nil || len(capture.Documents) != 0 {
		t.Fatal("global evidence", err)
	}
	engine := &assistantCaptureEngine{data: map[string][]byte{"unicode.txt": []byte("a雪雪"), "bad.json": []byte("not JSON")}}
	form := input.InputForm{Kind: "review", RequestID: "exact", Documents: map[string]artifacts.Ref{}}
	for _, name := range []string{"unicode.txt", "bad.json"} {
		form.Documents[name] = artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "child", TaskOrdinal: 1, Name: name})
	}
	s.inputs = &reviewRuntimeStub{forms: map[string]input.InputForm{"parent": form}}
	s.engine = engine
	capture, err = s.captureAssistantSource(context.Background(), "tenant", AssistantSource{Kind: "review", JobID: "parent", RequestID: "exact"}, assistantCaptureLimits{5, 20, 32})
	if err != nil {
		t.Fatal(err)
	}
	// Sorted bad.json is truncated, which is explicitly not a complete JSON read.
	if capture.Documents[0].Outcome != "truncated" || capture.Documents[1].Text != "a雪" || capture.Documents[1].Outcome != "truncated" {
		t.Fatalf("encoding=%+v", capture.Documents)
	}
}

func TestAssistantCaptureDoesNotFetchExternalReferences(t *testing.T) {
	engine := &assistantCaptureEngine{}
	form := input.InputForm{Kind: "review", RequestID: "exact", Documents: map[string]artifacts.Ref{"url": artifacts.NewExternalRef("link", "https://example.invalid/document.txt", false)}}
	s := &Server{inputs: &reviewRuntimeStub{forms: map[string]input.InputForm{"parent": form}}, engine: engine}
	result, err := s.captureAssistantSource(context.Background(), "tenant", AssistantSource{Kind: "review", JobID: "parent", RequestID: "exact"}, assistantCaptureLimits{100, 100, 32})
	if err != nil || len(engine.keys) != 0 || len(result.Documents) != 1 || result.Documents[0].Outcome != "unavailable" || result.Documents[0].Ref != nil {
		t.Fatalf("external source: %+v %v", result, err)
	}
}

type assistantCaptureStory struct {
	storyService
	request story.GetJobRunStoryRequest
}

func (f *assistantCaptureStory) GetJobRunStory(_ context.Context, in story.GetJobRunStoryRequest) (*story.JobRunStory, error) {
	f.request = in
	return &story.JobRunStory{JobID: in.JobID, Recipe: story.JobRunStoryRecipe{Name: strings.Repeat("story text ", 160)}}, nil
}
func TestAssistantCaptureStory(t *testing.T) {
	reader := &assistantCaptureStory{}
	s := &Server{story: reader}
	capture, err := s.captureAssistantSource(context.Background(), "tenant", AssistantSource{Kind: "story", JobID: "job"}, assistantCaptureLimits{8192, 8192, 32})
	if err != nil {
		t.Fatal(err)
	}
	if reader.request.ProjectID != "tenant" || reader.request.JobID != "job" || len(capture.Documents) != 1 || capture.Documents[0].Outcome != "included" || len(capture.Documents[0].Text) < 1000 || capture.Documents[0].Ref != nil {
		t.Fatalf("story: %+v request=%+v", capture, reader.request)
	}
}
