package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestGoalAttachmentNativePersistence(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/attachments"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "platform", "repo": "github.com/acme/platform"})
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("submission", `{"cell":"platform","job_id":"files","inputs":{"prompt":"Build with files","type":"build"}}`)
	for _, p := range []struct{ name, data string }{{"雪.bin", "\x00\xff\x01"}, {"empty", ""}, {"brief ?#.md", "# Hello"}} {
		part, e := w.CreateFormFile("files", p.name)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = io.WriteString(part, p.data)
	}
	_ = w.Close()
	req, _ := http.NewRequest("POST", base+"/jobs", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 201 {
		t.Fatalf("%d %s", resp.StatusCode, data)
	}
	var submitted map[string]any
	if err := json.Unmarshal(data, &submitted); err != nil {
		t.Fatal(err)
	}
	if submitted["goal_id"] == "" || submitted["job_id"] != "files" {
		t.Fatal(submitted)
	}
	inventory := getJSON[struct {
		Artifacts []inputArtifact `json:"artifacts"`
	}](t, base+"/jobs/files/input-artifacts")
	if len(inventory.Artifacts) != 3 {
		t.Fatalf("inventory: %#v", inventory)
	}
	for _, suffix := range []string{"/api/projects/other/jobs/files/input-artifacts", "/api/projects/attachments/jobs/missing/input-artifacts", "/api/projects/other/jobs/files/tasks/0/artifacts/empty", "/api/projects/attachments/jobs/files/tasks/0/artifacts/missing"} {
		response, e := http.Get(api.URL + suffix)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		want := 404
		if strings.HasSuffix(suffix, "/artifacts/missing") {
			want = 500
		} // Preserve the existing story adapter missing-byte mapping.
		if response.StatusCode != want {
			t.Fatalf("guard %s: %d", suffix, response.StatusCode)
		}
	}
	for _, p := range []struct{ name, data string }{{"雪.bin", "\x00\xff\x01"}, {"empty", ""}, {"brief ?#.md", "# Hello"}} {
		response, e := http.Get(base + "/jobs/files/tasks/0/artifacts/" + url.PathEscape(p.name))
		if e != nil {
			t.Fatal(e)
		}
		downloaded, e := io.ReadAll(response.Body)
		response.Body.Close()
		if e != nil || response.StatusCode != 200 || string(downloaded) != p.data {
			t.Fatalf("HTTP input bytes: %d %x %v", response.StatusCode, downloaded, e)
		}
		art, e := srv.engine.GetArtifact("attachments", jobdb.ArtifactKey{JobId: "files", TaskOrdinal: 0, Name: p.name})
		if e != nil {
			t.Fatal(e)
		}
		actual, e := art.Bytes(context.Background())
		if e != nil || string(actual) != p.data {
			t.Fatalf("persisted %q %x %v", p.name, actual, e)
		}
	}
	// Nonzero execution attachments retain the same native HTTP retrieval path.
	// Start ordinal selection and malformed legacy histories are tested separately.
	ctx := testjobdb.Context(context.Background())
	lease, e := srv.engine.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "attachments", JobId: "files"}, WorkerID: "input-test", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
	if e != nil || lease == nil {
		t.Fatalf("lease: %v", e)
	}
	defer lease.StopKeepAlive()
	artifact := jobdb.NewArtifactFromBytes("docs/nested 雪 ?#.txt", []byte("recorded ordinal"))
	runtime := srv.engine.(jobdbschema.WorkflowEngine).Registry.(jobdb.WorkflowRuntime)
	token := ""
	if l, ok := lease.(interface{ LeaseToken() string }); ok {
		token = l.LeaseToken()
	}
	e = runtime.PutChapter(ctx, jobdb.PutChapterRequest{LeaseID: lease.LeaseID(), LeaseToken: token, Ref: jobdb.ChapterRef{JobKey: jobdb.JobKey{TenantId: "attachments", JobId: "files"}, Ordinal: 1}, Chapter: jobdb.Chapter{Ordinal: 1, Body: jobdb.ExecutionChapter{Record: json.RawMessage(fmt.Sprintf(`{"version":%d,"kind":"header","versions":{"execution_pattern_version":%d,"jobdb_version":"test"}}`, jobdb.ExecutionPatternVersion, jobdb.ExecutionPatternVersion))}}, ArtifactUploads: []jobdb.ArtifactUpload{{Name: artifact.Name(), Size: artifact.Size(), Open: artifact.Open}}})
	if e != nil {
		t.Fatal(e)
	}
	response, e := http.Get(base + "/jobs/files/tasks/1/artifacts/" + url.PathEscape(artifact.Name()))
	if e != nil {
		t.Fatal(e)
	}
	downloaded, e := io.ReadAll(response.Body)
	response.Body.Close()
	if e != nil || response.StatusCode != 200 || string(downloaded) != "recorded ordinal" {
		t.Fatalf("recorded ordinal bytes: %d %q %v", response.StatusCode, downloaded, e)
	}
}

func TestGoalAttachmentValidationDoesNotSubmit(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/attachment-invalid"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "platform", "repo": "github.com/acme/platform"})
	retired := postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "retired", "repo": "github.com/acme/retired"})
	if _, e := srv.cells.RetireCell(testjobdb.Context(context.Background()), jobdb.RetireCellRequest{CellKey: jobdb.CellKey{TenantID: "attachment-invalid", CellID: retired.ID}, IfMatch: fmt.Sprintf("%q", retired.Revision)}); e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct {
		name, metadata, filename string
		status                   int
	}{
		{"ATT-027 invalid file", `{"cell":"platform"}`, "../escape", 400},
		{"ATT-032 other tenant cell", `{"cell":"missing"}`, "brief", 404},
		{"ATT-032 retired", `{"cell":"retired"}`, "brief", 409},
		{"ATT-032 repo mismatch", `{"cell":"platform","repo":"github.com/acme/other"}`, "brief", 409},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			_ = writer.WriteField("submission", tt.metadata)
			part, _ := writer.CreateFormFile("files", tt.filename)
			_, _ = part.Write([]byte("x"))
			_ = writer.Close()
			req, _ := http.NewRequest("POST", base+"/jobs", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			resp, e := http.DefaultClient.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			defer resp.Body.Close()
			data, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tt.status {
				t.Fatalf("status %d want %d: %s", resp.StatusCode, tt.status, data)
			}
		})
	}
	page := getJSON[map[string]any](t, base+"/jobs?status=all")
	if jobs, ok := page["jobs"].([]any); !ok || len(jobs) != 0 {
		t.Fatalf("invalid request created jobs: %#v", page)
	}
}

type failingGoalEngine struct{ jobworkflow.Engine }

func (e failingGoalEngine) SubmitJob(context.Context, jobdb.SubmitJob) (jobdb.JobHandle, error) {
	return jobdb.JobHandle{}, fmt.Errorf("fixture native submission unavailable")
}

func TestGoalAttachmentNativeFailure(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/attachment-failure"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "platform", "repo": "github.com/acme/platform"})
	srv.engine = failingGoalEngine{srv.engine}
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("submission", `{"cell":"platform"}`)
	part, _ := w.CreateFormFile("files", "big")
	_, _ = part.Write(bytes.Repeat([]byte("x"), 5<<20))
	_ = w.Close()
	req, _ := http.NewRequest("POST", base+"/jobs", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 400 || !bytes.Contains(data, []byte("fixture native submission unavailable")) {
		t.Fatalf("%d %s", resp.StatusCode, data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("temporary files leaked on native failure")
	}
}

func TestGoalAttachmentFilenameValidationDoesNotSubmit(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/filename-invalid"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "platform", "repo": "github.com/acme/platform"})
	for _, tc := range []struct {
		name, raw string
		names     []string
	}{
		{"ATT-039 malformed", "0", []string{`[`}},
		{"ATT-039 null", "0", []string{`null`}},
		{"ATT-039 non-string", "0", []string{`[1]`}},
		{"ATT-039 trailing", "0", []string{`["x"] {}`}},
		{"ATT-039 mismatch", "0", []string{`[]`}},
		{"ATT-039 repeated", "0", []string{`["x"]`, `["x"]`}},
		{"ATT-040 raw path", "../escape", []string{`["safe"]`}},
		{"ATT-041 recovered path", "0", []string{`["../escape"]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			w := multipart.NewWriter(&body)
			_ = w.WriteField("submission", `{"cell":"platform","inputs":{"prompt":"hello","type":"build"}}`)
			for _, names := range tc.names {
				_ = w.WriteField("filenames", names)
			}
			part, e := w.CreateFormFile("files", tc.raw)
			if e != nil {
				t.Fatal(e)
			}
			_, _ = part.Write([]byte("x"))
			_ = w.Close()
			req, _ := http.NewRequest("POST", base+"/jobs", &body)
			req.Header.Set("Content-Type", w.FormDataContentType())
			doJSON[errorResponse](t, req, http.StatusBadRequest)
		})
	}
	page := getJSON[map[string]any](t, base+"/jobs?status=all")
	if jobs, ok := page["jobs"].([]any); !ok || len(jobs) != 0 {
		t.Fatalf("invalid filename requests created jobs: %#v", page)
	}
}
