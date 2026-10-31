package cortex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
)

// Exercise persisted context across SQLite, HTTP JobDB, c2j's projection, and
// Cortex's real detail endpoint rather than injecting browser-shaped responses.
func TestJobContextDetail(t *testing.T) {
	ctx := context.Background()
	runtime, err := sqliteruntime.StartEmbeddedRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown()
	backend := httptest.NewServer(remote.NewServer(runtime.Runtime))
	defer backend.Close()
	srv, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	base := api.URL + "/api/projects/context/jobs"
	const prompt = "  Build <a useful tool> 雪\n\tKeep the whole prompt.  "
	const detail = "  diagnostic <script>literal</script> 雪\n\tsecond line  "
	for _, outcome := range []string{"failed_app", "failed_system", "failed_timeout"} {
		t.Run(outcome, func(t *testing.T) {
			postJSON[recipejob.RecipeJob](t, base, map[string]any{"job_id": outcome, "cell": "alpha", "recipe": "smoke", "inputs": map[string]any{"prompt": prompt}})
			active := getJSON[recipejob.RecipeJob](t, base+"/"+outcome)
			if active.Prompt != prompt || active.Store != jobdb.JobStoreActive || active.CompletionStatus != "" || active.CompletionDetail != "" {
				t.Fatalf("active context: %+v", active)
			}
			lease, err := srv.engine.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "context", JobId: outcome}, WorkerID: "context-test", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			if lease == nil {
				t.Fatal("expected lease")
			}
			defer lease.StopKeepAlive()
			err = lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: outcome, Detail: detail, Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now().UTC(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{}`)}}}}})
			if err != nil {
				t.Fatal(err)
			}
			completed := getJSON[recipejob.RecipeJob](t, base+"/"+outcome)
			if completed.Prompt != prompt || string(completed.CompletionStatus) != outcome || completed.CompletionDetail != detail || completed.Store != jobdb.JobStoreArchived {
				t.Fatalf("archived context: %+v", completed)
			}
			response, err := http.Get(api.URL + "/api/projects/other/jobs/" + outcome)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("other tenant: %d", response.StatusCode)
			}
		})
	}
	postJSON[recipejob.RecipeJob](t, base, map[string]any{"job_id": "empty", "cell": "alpha", "recipe": "smoke"})
	empty := getJSON[recipejob.RecipeJob](t, base+"/empty")
	if empty.Prompt != "" || empty.CompletionStatus != "" || empty.CompletionDetail != "" {
		t.Fatalf("invented context: %+v", empty)
	}
}
