package cortex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestDocumentInventoryBindings(t *testing.T) {
	key := jobdb.ArtifactKey{JobId: "producer", TaskOrdinal: 7, Name: "original.md", SizeBytes: 5}
	stored := artifacts.NewStoredRef(key)
	stored.Name = "display.md"
	data := jobdb.NewTaskDataOrPanic(workflowctl.StartJob{Inputs: map[string]interface{}{"documents": map[string]any{
		"alias": stored, "other": stored, "otherProducer": artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "other-producer", TaskOrdinal: 7, Name: "original.md"}), "external": artifacts.NewExternalRef("remote.md", "https://invalid.example", false),
		"inline": map[string]any{"kind": "value", "format": "text", "name": "inline.md", "value": "hello"}, "bad": false, "attached": map[string]any{"kind": "attached", "name": "missing.md"}, "unknown": map[string]any{"kind": "future"},
	}}})
	rows, err := documentInventory("job", []jobdb.Chapter{{Ordinal: 17, Body: jobdb.JobStartChapter{Input: jobdb.ApplicationInputBytes{Data: data.GetDataOrPanic()}}, Artifacts: []jobdb.StoredArtifact{{Name: "direct.md", Size: 9}}}})
	if err != nil || len(rows) != 9 {
		t.Fatalf("%#v %v", rows, err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.ID] {
			t.Fatal("duplicate identity")
		}
		seen[r.ID] = true
		switch r.Binding {
		case "alias", "other":
			if r.StoredKey == nil || r.StoredKey.Name != "original.md" || r.Name != "display.md" || r.ContentBinding == nil {
				t.Fatalf("%#v", r)
			}
		case "otherProducer":
			if r.StoredKey == nil || r.StoredKey.JobId != "other-producer" || r.StoredKey.Name != "original.md" {
				t.Fatalf("%#v", r)
			}
		case "external", "bad", "attached", "unknown":
			if r.Reason == "" || r.ContentBinding != nil {
				t.Fatalf("%#v", r)
			}
		}
	}
	bytes, _ := json.Marshal(rows)
	if strings.Contains(string(bytes), "hello") {
		t.Fatal("inline payload leaked")
	}
	// Merge only identical binding/display and full producer identity.
	ref := artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "job", TaskOrdinal: 17, Name: "direct.md"})
	for _, binding := range []string{"direct.md", "alias"} {
		d := jobdb.NewTaskDataOrPanic(workflowctl.StartJob{Inputs: map[string]interface{}{"documents": map[string]any{binding: ref}}})
		got, e := documentInventory("job", []jobdb.Chapter{{Ordinal: 17, Body: jobdb.JobStartChapter{Input: jobdb.ApplicationInputBytes{Data: d.GetDataOrPanic()}}, Artifacts: []jobdb.StoredArtifact{{Name: "direct.md"}}}})
		if e != nil {
			t.Fatal(e)
		}
		if binding == "direct.md" {
			if len(got) != 1 || len(got[0].Sources) != 2 {
				t.Fatalf("%#v", got)
			}
		} else if len(got) != 2 {
			t.Fatalf("%#v", got)
		}
	}
	for _, documents := range []any{nil, map[string]any{}, "bad map"} {
		d := jobdb.NewTaskDataOrPanic(workflowctl.StartJob{Inputs: map[string]interface{}{"documents": documents}})
		got, e := documentInventory("job", []jobdb.Chapter{{Ordinal: 17, Body: jobdb.JobStartChapter{Input: jobdb.ApplicationInputBytes{Data: d.GetDataOrPanic()}}, Artifacts: []jobdb.StoredArtifact{{Name: "direct.md"}}}})
		if e != nil || len(got) < 1 || got[0].Name != "direct.md" {
			t.Fatalf("%#v %v", got, e)
		}
		if documents == "bad map" && (len(got) != 2 || got[1].Reason == "") {
			t.Fatalf("%#v", got)
		}
	}
}

func TestInputDocumentContentHTTP(t *testing.T) {
	s, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(s)
	defer api.Close()
	base := api.URL + "/api/projects/a"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "self", "repo": "github.com/acme/self"})
	submit := func(tenant, job string, data jobdb.TaskData) {
		t.Helper()
		var envelope map[string]any
		if err := json.Unmarshal(data.GetDataOrPanic(), &envelope); err != nil {
			t.Fatal(err)
		}
		envelope["tenantId"] = tenant
		envelope["recipe"] = "test"
		envelope["cell"] = "self"
		envelope["context"] = map[string]any{}
		attachments, e := data.GetArtifacts()
		if e != nil {
			t.Fatal(e)
		}
		data = jobdb.NewTaskDataOrPanic(envelope, attachments...)
		_, err := s.engine.SubmitJob(testjobdb.Context(context.Background()), jobdb.SubmitJob{TenantId: tenant, JobID: job, JobType: "recipe", Cell: "self", CellRepo: "github.com/acme/self", Data: data})
		if err != nil {
			t.Fatal(err)
		}
	}
	submit("a", "producer", jobdb.NewTaskDataOrPanic(map[string]any{}, jobdb.NewArtifactFromBytes("original.md", []byte("original bytes"))))
	stored := artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "producer", TaskOrdinal: 0, Name: "original.md"})
	stored.Name = "renamed.md"
	docs := map[string]any{"alias ?雪/#": stored, "inline": map[string]any{"kind": "value", "name": "inline.json", "format": "json", "value": map[string]any{"a": 1}}, "text": map[string]any{"kind": "value", "name": "inline.md", "format": "text", "value": "# Inline"}, "missing": artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "producer", TaskOrdinal: 0, Name: "missing.md"}), "external": artifacts.NewExternalRef("external.md", "https://invalid.example", false)}
	submit("a", "consumer", jobdb.NewTaskDataOrPanic(workflowctl.StartJob{Inputs: map[string]interface{}{"documents": docs}}, jobdb.NewArtifactFromBytes("direct.md", []byte("direct"))))
	inv := getJSON[struct {
		Artifacts []inputDocument `json:"artifacts"`
	}](t, base+"/jobs/consumer/input-artifacts")
	if len(inv.Artifacts) != 6 {
		t.Fatalf("%#v", inv)
	}
	check := func(tenant, binding string, status int, want string) {
		t.Helper()
		r, e := http.Get(api.URL + "/api/projects/" + tenant + "/jobs/consumer/input-document?binding=" + url.QueryEscape(binding))
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		if r.StatusCode != status || (want != "" && ((status == 200 && string(b) != want) || (status != 200 && !strings.Contains(string(b), want)))) {
			t.Fatalf("%s %d %q", binding, r.StatusCode, b)
		}
		if status == 200 && (r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("X-Content-Type-Options") != "nosniff") {
			t.Fatal(r.Header)
		}
	}
	check("a", "alias ?雪/#", 200, "original bytes")
	check("a", "inline", 200, "{\"a\":1}\n")
	check("a", "text", 200, "# Inline")
	check("a", "missing", 404, "unavailable")
	check("a", "external", 404, "unavailable")
	check("other", "text", 404, "")
	// Same consumer in a second tenant must not resolve the first tenant's producer.
	postJSON[jobdb.Cell](t, api.URL+"/api/projects/b/cells", map[string]string{"name": "self", "repo": "github.com/acme/self"})
	submit("b", "consumer", jobdb.NewTaskDataOrPanic(workflowctl.StartJob{Inputs: map[string]interface{}{"documents": docs}}))
	check("b", "alias ?雪/#", 404, "unavailable")
	check("b", "text", 200, "# Inline")
	documentsOnly := getJSON[struct {
		Artifacts []inputDocument `json:"artifacts"`
	}](t, api.URL+"/api/projects/b/jobs/consumer/input-artifacts")
	if len(documentsOnly.Artifacts) != 5 {
		t.Fatalf("documents-only: %#v", documentsOnly)
	}

}

func TestDocumentInventoryStartSelectionAndInlinePrecision(t *testing.T) {
	payload := []byte(`{"inputs":{"documents":{"inline":{"kind":"value","format":"json","name":"large.json","value":{"integer":9007199254740993}}},"unrelated":{"kind":"value","format":"text","value":"excluded"}}}`)
	chapters := []jobdb.Chapter{
		{Ordinal: 0, Body: jobdb.ExecutionChapter{}, Artifacts: []jobdb.StoredArtifact{{Name: "later.md"}}},
		{Ordinal: 17, Body: &jobdb.JobStartChapter{Input: jobdb.ApplicationInputBytes{Data: payload}}},
		{Ordinal: 18, Body: jobdb.ExecutionChapter{}, Artifacts: []jobdb.StoredArtifact{{Name: "later.md"}}},
	}
	rows, err := documentInventory("job", chapters)
	if err != nil || len(rows) != 1 || rows[0].Binding != "inline" {
		t.Fatalf("%#v %v", rows, err)
	}
	bytes, err := rows[0].ref.ValueBytes()
	if err != nil || string(bytes) != "{\"integer\":9007199254740993}\n" {
		t.Fatalf("%q %v", bytes, err)
	}
	encoded, _ := json.Marshal(rows[0])
	if strings.Contains(string(encoded), "taskOrdinal") || strings.Contains(string(encoded), "jobId") {
		t.Fatal("invented stored locator", string(encoded))
	}
}
