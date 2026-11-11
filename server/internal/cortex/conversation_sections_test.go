package cortex

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/workflow"
	"github.com/stretchr/testify/require"
)

func TestConversationTerminalSections(t *testing.T) {
	ctx := testjobdb.Context(context.Background())
	rt, err := sqliteruntime.StartEmbeddedRuntime(ctx)
	require.NoError(t, err)
	defer rt.Shutdown()
	backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(rt.Runtime)))
	defer backend.Close()
	server, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	require.NoError(t, err)
	api := httptest.NewServer(server)
	defer api.Close()
	postJSON[jobdb.Cell](t, api.URL+"/api/projects/tenant/cells", map[string]string{"name": "alpha", "repo": "example.test/alpha"})
	for _, outcome := range []string{"success", "failed_app", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			base := api.URL + "/api/projects/tenant/jobs"
			postJSON[recipejob.RecipeJob](t, base, map[string]any{"job_id": outcome, "cell": "alpha", "recipe": "smoke"})
			var pending struct {
				Activity  conversationSection
				Documents conversationSection `json:"final_documents"`
			}
			pending = getJSON[struct {
				Activity  conversationSection
				Documents conversationSection `json:"final_documents"`
			}](t, base+"/"+outcome+"/conversation")
			require.Equal(t, "available", pending.Activity.Availability)
			require.Equal(t, "pending", pending.Documents.Value.(map[string]any)["state"])
			lease, err := server.engine.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: outcome}, WorkerID: "sections", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
			require.NoError(t, err)
			require.NotNil(t, lease)
			defer lease.StopKeepAlive()
			refs := map[string]artifacts.Ref{}
			for _, producer := range []string{outcome, "child", "grandchild"} {
				refs[producer+"/alias.md"] = artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: producer, TaskOrdinal: 4, Name: "original.md"})
			}
			raw, err := json.Marshal(map[string]any{"artifact_refs": refs})
			require.NoError(t, err)
			require.NoError(t, lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: outcome, Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now().UTC(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: raw}}}}}))
			var page struct {
				Activity struct {
					Availability string
					Value        recipejob.RecipeJob
				}
				Documents struct {
					Availability string
					Value        struct {
						*story.FinalDocuments
						ManifestID string `json:"manifest_id"`
					}
				} `json:"final_documents"`
			}
			page = getJSON[struct {
				Activity struct {
					Availability string
					Value        recipejob.RecipeJob
				}
				Documents struct {
					Availability string
					Value        struct {
						*story.FinalDocuments
						ManifestID string `json:"manifest_id"`
					}
				} `json:"final_documents"`
			}](t, base+"/"+outcome+"/conversation")
			require.Equal(t, "available", page.Activity.Availability)
			require.Equal(t, outcome, page.Activity.Value.CompletionStatus)
			require.Equal(t, "available", page.Documents.Availability)
			require.NotEmpty(t, page.Documents.Value.ManifestID)
			require.Equal(t, int64(1), *page.Documents.Value.AttemptOrdinal)
			require.Len(t, page.Documents.Value.Documents, 3)
			for _, doc := range page.Documents.Value.Documents {
				require.Equal(t, "stored", doc.Resolution)
				require.Equal(t, doc.Producer.JobId+"/alias.md", doc.Alias)
				require.Equal(t, "original.md", doc.Producer.Name)
			}
			// A section reader failure must preserve a readable history response.
			original := server.story
			server.story = &finalStoryStub{err: story.ErrFinalDocumentsUnsupported}
			failed := getJSON[map[string]any](t, base+"/"+outcome+"/conversation")
			require.Equal(t, "unavailable", failed["final_documents"].(map[string]any)["availability"])
			require.Equal(t, "available", failed["activity"].(map[string]any)["availability"])
			require.Contains(t, failed, "entries")
			server.story = &finalStoryStub{err: story.ErrOutcomePending}
			pendingOutcome := getJSON[map[string]any](t, base+"/"+outcome+"/conversation")
			require.Equal(t, "available", pendingOutcome["final_documents"].(map[string]any)["availability"])
			require.Equal(t, "pending", pendingOutcome["final_documents"].(map[string]any)["value"].(map[string]any)["state"])
			// CV-014: public projection selects the latest attempt, excluding earlier
			// declarations and intermediate task output even with plausible filenames.
			oldEngine := server.engine
			prior := &priorAttemptEngine{Engine: oldEngine}
			server.engine = prior
			projected, e := story.New(story.ServiceConfig{Engine: prior})
			require.NoError(t, e)
			server.story = projected
			selected := getJSON[map[string]any](t, base+"/"+outcome+"/conversation")
			manifest := selected["final_documents"].(map[string]any)["value"].(map[string]any)
			require.Len(t, manifest["documents"], 3)
			require.NotContains(t, selected["final_documents"], "previous.md")
			encoded, e := json.Marshal(manifest)
			require.NoError(t, e)
			require.NotContains(t, string(encoded), "previous.md")
			require.NotContains(t, string(encoded), "intermediate.md")
			server.engine = oldEngine
			server.story = original
		})
	}
}

type priorAttemptEngine struct{ workflow.Engine }

func (e *priorAttemptEngine) GetJobRun(ctx context.Context, r jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	run, err := e.Engine.GetJobRun(ctx, r)
	if err != nil {
		return run, err
	}
	previous := jobdb.JobAttempt{Ordinal: 0, Output: &jobdb.TaskIO{Data: json.RawMessage(`{"artifact_refs":{"previous.md":{"kind":"external","url":"https://example.invalid/previous"}}}`)}}
	run.Attempts = append([]jobdb.JobAttempt{previous}, run.Attempts...)
	if len(run.Attempts) > 1 {
		last := run.Attempts[len(run.Attempts)-1]
		last.Tasks = append(last.Tasks, jobdb.TaskRun{TaskType: "intermediate", Attempts: []jobdb.TaskAttempt{{Output: &jobdb.TaskIO{Data: json.RawMessage(`{"artifact_refs":{"intermediate.md":null}}`)}}}})
		run.Attempts[len(run.Attempts)-1] = last
	}
	return run, nil
}

func TestConversationActivityUnavailablePreservesHistory(t *testing.T) {
	w := httptest.NewRecorder()
	historyRouter(&historyStub{}).ServeHTTP(w, httptest.NewRequest("GET", "/tenant/job", nil))
	require.Equal(t, 200, w.Code)
	var page map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Contains(t, page, "entries")
	require.Equal(t, "unavailable", page["activity"].(map[string]any)["availability"])
	require.Equal(t, "unavailable", page["final_documents"].(map[string]any)["availability"])
}
