package cortex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	runtimecore "github.com/colony-2/jobdb/pkg/jobdb/runtime/core"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	_ "modernc.org/sqlite"
)

// Native submission always writes chapter zero. Seed a retained nonzero start
// in SQLite to exercise the read compatibility contract, not submission behavior.
// Only fixture storage is changed: chapter reads, artifact bytes, the workflow
// engine, the story adapter, job guards and both HTTP handlers remain real.
func TestInputArtifactsNonzeroStartNativeHTTP(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	runtime, err := sqlite.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	engine, err := jobworkflow.NewEngineBuilder().WithRuntime(runtime).WithWorkerTenantId("retained").BuildEngine()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(Config{WorkingDir: writeC2JConfig(t)}, jobdbschema.WorkflowEngine{Engine: engine, Registry: runtime})
	if err != nil {
		t.Fatal(err)
	}
	s.chapters = runtime
	api := httptest.NewServer(testjobdb.Handler(s))
	defer api.Close()
	base := api.URL + "/api/projects/retained"
	postJSON[jobdb.Cell](t, base+"/cells", map[string]string{"name": "self", "repo": "github.com/acme/self"})
	contents := map[string]string{"docs/brief 雪 ?#%.md": "# Native retained input\n", "nested/raw.bin": "\x00\xff\x01", "empty": ""}
	var artifacts []jobdb.Artifact
	for name, data := range contents {
		artifacts = append(artifacts, jobdb.NewArtifactFromBytes(name, []byte(data)))
	}
	ctx := testjobdb.Context(context.Background())
	key := jobdb.JobKey{TenantId: "retained", JobId: "nonzero"}
	_, err = s.engine.SubmitJob(ctx, jobdb.SubmitJob{TenantId: key.TenantId, JobID: key.JobId, JobType: "recipe", Cell: "self", CellRepo: "github.com/acme/self", Data: jobdb.NewTaskDataOrPanic(map[string]any{"tenantId": "retained", "cell": "self", "recipe": "test", "context": map[string]any{}}, artifacts...)})
	if err != nil {
		t.Fatal(err)
	}
	start, err := runtime.GetChapter(ctx, jobdb.ChapterRef{JobKey: key, Ordinal: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := start.Body.(jobdb.JobStartChapter); !ok {
		t.Fatalf("expected native start, got %T", start.Body)
	}
	const recordedOrdinal int64 = 1
	start.Ordinal = recordedOrdinal
	encoded, err := runtimecore.EncodeChapter(runtimecore.EncodeChapterRequest{Chapter: start})
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := runtimecore.EncodeChapter(runtimecore.EncodeChapterRequest{Chapter: jobdb.Chapter{Ordinal: 0, Body: jobdb.ExecutionChapter{Record: json.RawMessage(fmt.Sprintf(`{"version":%d,"kind":"header","versions":{"execution_pattern_version":%d,"jobdb_version":"test"}}`, jobdb.ExecutionPatternVersion, jobdb.ExecutionPatternVersion))}}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Keep native artifact descriptors and stored bytes, moving their owner along
	// with the single start chapter. EncodeChapter preserves its typed start body.
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO jobdb_chapter_chapters (anthology_id, story_id, ordinal, body, created_at_ns) SELECT anthology_id, story_id, ?, ?, created_at_ns FROM jobdb_chapter_chapters WHERE anthology_id=? AND story_id=? AND ordinal=0`, []any{recordedOrdinal, encoded.Payload, key.TenantId, key.JobId}},
		{`UPDATE jobdb_chapter_artifacts SET ordinal=? WHERE anthology_id=? AND story_id=? AND ordinal=0`, []any{recordedOrdinal, key.TenantId, key.JobId}},
		{`UPDATE jobdb_chapter_chapters SET body=? WHERE anthology_id=? AND story_id=? AND ordinal=0`, []any{prefix.Payload, key.TenantId, key.JobId}},
		{`UPDATE jobdb_chapter_stories SET latest_ordinal=?, chapter_count=2 WHERE anthology_id=? AND story_id=?`, []any{recordedOrdinal, key.TenantId, key.JobId}},
	} {
		result, err := tx.ExecContext(ctx, statement.query, statement.args...)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows == 0 {
			t.Fatalf("fixture did not update native storage: rows=%d error=%v", rows, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	chapters, err := runtime.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: key})
	if err != nil || len(chapters) != 2 || chapters[1].Ordinal != recordedOrdinal {
		t.Fatalf("persisted chapters: %#v %v", chapters, err)
	}
	if _, ok := chapters[1].Body.(jobdb.JobStartChapter); !ok {
		t.Fatalf("persisted chapter is %T", chapters[1].Body)
	}
	inventory := getJSON[struct {
		Artifacts []inputArtifact `json:"artifacts"`
	}](t, base+"/jobs/"+key.JobId+"/input-artifacts")
	if len(inventory.Artifacts) != len(contents) {
		t.Fatalf("inventory: %#v", inventory)
	}
	seen := map[string]bool{}
	for _, artifact := range inventory.Artifacts {
		expected, ok := contents[artifact.Name]
		if !ok || seen[artifact.Name] || artifact.JobID != key.JobId || artifact.TaskOrdinal != recordedOrdinal || artifact.SizeBytes != int64(len(expected)) {
			t.Fatalf("wrong persisted identity: %#v", artifact)
		}
		seen[artifact.Name] = true
		// Use the inventory identity, through the actual story/engine artifact adapter.
		response, err := http.Get(fmt.Sprintf("%s/jobs/%s/tasks/%d/artifacts/%s", base, url.PathEscape(artifact.JobID), artifact.TaskOrdinal, url.PathEscape(artifact.Name)))
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || string(data) != expected {
			t.Fatalf("%s: status=%d bytes=%x error=%v", artifact.Name, response.StatusCode, data, err)
		}
		t.Logf("native start ordinal=%d name=%q bytes=%d: exact HTTP content", artifact.TaskOrdinal, artifact.Name, len(data))
	}
}
