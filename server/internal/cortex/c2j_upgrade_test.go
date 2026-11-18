package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/cellref"
	"github.com/colony-2/c2j/pkg/grouping"
	"github.com/colony-2/c2j/pkg/jobdbschema"
	coreops "github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/c2j/pkg/recipe"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/starter"
	"github.com/colony-2/c2j/pkg/worker/compiler"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
	wf "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/stretchr/testify/require"
)

type upgradeLeaseEngine struct {
	wf.Engine
	lease jobdb.ExecutionLease
}

func (e upgradeLeaseEngine) StartOrAppendJob(ctx context.Context, request jobdb.StartOrAppendJob) (jobdb.JobHandle, error) {
	return e.lease.StartOrAppendJob(ctx, jobdb.StartOrAppendJobRequest{Job: request})
}

type upgradePausedTask struct{ inboxHTTPTask }

func (upgradePausedTask) Run(tc wf.TaskContext, _ jobdb.TaskData) (jobdb.TaskData, error) {
	return nil, tc.Yield(tc.Context(), wf.YieldRequest{Reschedule: jobdb.RescheduleExecutionRequest{NextRoute: jobdb.Route{JobType: "recipe"}}})
}

// Group creation is fixture setup. Cortex reads through its existing HTTP
// views, public history readers and read-only replay compiler.
func TestC2JUpgradeGroupedHistory(t *testing.T) {
	ctx := testjobdb.Context(context.Background())
	rt, err := sqlite.NewFromConfig(ctx, sqlite.Config{DBPath: filepath.Join(t.TempDir(), "grouped.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close(ctx)) })
	_, err = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "grouped", Name: "test", Repo: "example.test/test"})
	require.NoError(t, err)
	worker := compiler.NewRecipeJobWorker(compiler.RecipeJobWorkerOptions{})
	rawEngine, err := wf.NewEngineBuilder().WithRuntime(rt).WithWorkerTenantId("grouped").PlusWorkers(worker, inboxHTTPTask{}).BuildEngine()
	require.NoError(t, err)
	previousOps := coreops.List()
	t.Cleanup(func() { coreops.Replace(previousOps...) })
	coreops.Register(coreops.NewActivityMappedOpV2[map[string]any, map[string]any](coreops.OpMetadata{Type: inboxTestOp}, func(coreops.OpDependencies, context.Context, map[string]any) (map[string]any, error) {
		panic("inspection executed grouped operation")
	}))
	rec, err := recipe.LoadRecipeFromString([]byte(`id: batch
sequence:
 - id: read
   op: cortex_inbox_fixture
   inputs:
     contributions: '${{ context.grouping.appends }}'
outputs:
 contributions: '${{ context.grouping.appends }}'
 creator: '${{ context.artifacts["start/foo"] }}'
`))
	require.NoError(t, err)
	refs := make([]jobdb.ArtifactKey, 2)
	for i, content := range []string{"first", "second"} {
		id := fmt.Sprintf("caller-%d", i)
		_, err = rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "grouped", JobID: id, JobType: "recipe", Cell: "test", Data: jobdb.NewTaskDataOrPanic(workflowctl.StartJob{TenantId: "grouped", Cell: "test", RecipeName: "source"}, jobdb.NewArtifactFromBytes("same.txt", []byte(content)))}})
		require.NoError(t, err)
		refs[i] = jobdb.ArtifactKey{JobId: id, TaskOrdinal: 0, Name: "same.txt", SizeBytes: int64(len(content))}
	}
	start := workflowctl.StartJob{TenantId: "grouped", Cell: "test", CellRepo: "example.test/test", RecipeName: "batch", Artifacts: []jobdb.Artifact{jobdb.NewArtifactFromBytes("start/foo", []byte("creator"))}}
	appendOne := func(i int) jobdb.JobKey {
		lease, err := rt.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "grouped", JobId: refs[i].JobId}, WorkerID: fmt.Sprintf("caller-%d", i), Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
		require.NoError(t, err)
		require.NotNil(t, lease)
		t.Cleanup(lease.StopKeepAlive)
		caller := jobdbschema.WorkflowEngine{Engine: upgradeLeaseEngine{Engine: rawEngine, lease: lease}, Registry: rt}
		ref, err := starter.StartOrAppendRecipeJob(ctx, start, grouping.Spec{Present: true, Value: map[string]any{
			"key": "shared", "content": map[string]any{"position": i + 1}, "artifacts": []artifacts.Ref{artifacts.NewStoredRef(refs[i])},
		}}, caller, *rec)
		require.NoError(t, err)
		return ref.Key()
	}
	key := appendOne(0)
	require.Equal(t, key, appendOne(1))
	restarted, err := rt.SubmitRestartJob(ctx, jobdb.SubmitRestartJobRequest{Job: jobdb.SubmitRestartJob{Cell: "test", PriorJobKey: key, LastStepToKeep: 0}})
	require.NoError(t, err)
	info, err := rt.GetJob(ctx, restarted.JobKey)
	require.NoError(t, err)
	require.Empty(t, info.AppendKey)

	backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(rt)))
	t.Cleanup(backend.Close)
	s, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	require.NoError(t, err)
	api := httptest.NewServer(s)
	t.Cleanup(api.Close)
	base := api.URL + "/api/projects/grouped/jobs/"
	readInventory := func(key jobdb.JobKey, count int) {
		t.Helper()
		inventory := getJSON[struct {
			Artifacts []inputDocument `json:"artifacts"`
		}](t, base+key.JobId+"/input-artifacts")
		var selected []inputDocument
		for _, row := range inventory.Artifacts {
			if row.Name == "start/foo" || row.Name == "same.txt" {
				selected = append(selected, row)
			}
		}
		require.Len(t, selected, count+1)
		require.Equal(t, "start/foo", selected[0].Name)
		require.Equal(t, key.JobId, selected[0].JobID)
		seen := map[string]bool{}
		for i, row := range selected {
			require.False(t, seen[row.ID], "duplicate inventory identity")
			seen[row.ID] = true
			want := "creator"
			if i > 0 {
				require.Equal(t, refs[i-1], *row.StoredKey)
				want = []string{"first", "second"}[i-1]
			}
			response, err := http.Get(fmt.Sprintf("%s%s/tasks/%d/artifacts/%s", base, row.JobID, *row.TaskOrdinal, url.PathEscape(row.Name)))
			require.NoError(t, err)
			var content bytes.Buffer
			_, err = content.ReadFrom(response.Body)
			response.Body.Close()
			require.NoError(t, err)
			require.Equal(t, 200, response.StatusCode, content.String())
			require.Equal(t, want, content.String())
		}
	}
	for _, item := range []struct {
		key   jobdb.JobKey
		count int
	}{{key, 2}, {restarted.JobKey, 1}} {
		readInventory(item.key, item.count)
		paused, err := wf.GetJobForRun(ctx, rt, wf.GetJobForRunRequest{JobKey: item.key, JobWorker: worker, TaskWorkers: []wf.TaskWorker{compiler.NewCellContextTaskWorker(), upgradePausedTask{}}, WorkerID: "group-paused", LeaseDuration: time.Minute})
		require.NoError(t, err)
		parked, err := paused.Run(nil)
		require.NoError(t, err)
		require.NotEqual(t, wf.JobRunCompleted, parked.Status)
		pendingBefore, err := rt.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: item.key})
		require.NoError(t, err)
		pending := getJSON[map[string]any](t, base+item.key.JobId+"/story")
		require.Equal(t, item.key.JobId, pending["job_id"])
		pendingJSON, err := json.Marshal(pending)
		require.NoError(t, err)
		require.Contains(t, string(pendingJSON), `"position":1`)
		if item.count == 2 {
			require.Contains(t, string(pendingJSON), `"position":2`)
		}
		pendingAfter, err := rt.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: item.key})
		require.NoError(t, err)
		require.Equal(t, pendingBefore, pendingAfter)
		run, err := wf.GetJobForRun(ctx, rt, wf.GetJobForRunRequest{JobKey: item.key, JobWorker: worker, TaskWorkers: []wf.TaskWorker{compiler.NewCellContextTaskWorker(), inboxHTTPTask{}}, WorkerID: "group-fixture", LeaseDuration: time.Minute})
		require.NoError(t, err)
		out, err := run.Run(nil)
		require.NoError(t, err)
		require.Equal(t, wf.JobRunCompleted, out.Status, "%v", out.JobError)
		before, err := rt.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: item.key})
		require.NoError(t, err)
		for n := 0; n < 2; n++ {
			st := getJSON[map[string]any](t, base+item.key.JobId+"/story")
			raw, err := json.Marshal(st)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"position":1`)
			outcome := getJSON[struct {
				Output struct {
					Contributions []grouping.Contribution `json:"contributions"`
				} `json:"output"`
			}](t, base+item.key.JobId+"/outcome")
			require.Len(t, outcome.Output.Contributions, item.count)
			for i, contribution := range outcome.Output.Contributions {
				require.EqualValues(t, i+1, contribution.Content["position"])
				stored, ok := contribution.Artifacts[0].StoredKey()
				require.True(t, ok)
				require.Equal(t, refs[i], stored)
			}
		}
		readInventory(item.key, item.count)
		after, err := rt.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: item.key})
		require.NoError(t, err)
		require.Equal(t, before, after, "inspection changed retained history")
		detail := getJSON[recipejob.RecipeJob](t, base+item.key.JobId)
		require.Empty(t, detail.Parent, "a shared receiver has no exclusive caller")
	}
	for _, suffix := range []string{"/story", "/outcome", "/input-artifacts", "/tasks/0/artifacts/start%2Ffoo"} {
		response, err := http.Get(api.URL + "/api/projects/other/jobs/" + key.JobId + suffix)
		require.NoError(t, err)
		response.Body.Close()
		require.Equal(t, 404, response.StatusCode)
	}
}

func TestC2JUpgradeSchemaDenial(t *testing.T) {
	rt := toy.New()
	var registration, submissions atomic.Int32
	handler := testjobdb.Handler(remote.NewServer(rt))
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/schemas") {
			registration.Add(1)
			http.Error(w, "schema registration denied", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/jobs") {
			submissions.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(backend.Close)
	s, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	require.NoError(t, err)
	api := httptest.NewServer(s)
	t.Cleanup(api.Close)
	postJSON[jobdb.Cell](t, api.URL+"/api/projects/denied/cells", map[string]string{"name": "test", "repo": "example.test/test"})
	response, err := http.Post(api.URL+"/api/projects/denied/jobs", "application/json", strings.NewReader(`{"cell":"test","recipe":"smoke","job_id":"denied"}`))
	require.NoError(t, err)
	response.Body.Close()
	require.NotEqual(t, 201, response.StatusCode)
	require.Positive(t, registration.Load())
	// The first submit may discover a cold schema; no second route may accept it.
	require.LessOrEqual(t, submissions.Load(), int32(1))
	jobs, err := rt.ListJobs(testjobdb.Context(context.Background()), jobdb.ListJobsRequest{TenantIds: []string{"denied"}})
	require.NoError(t, err)
	require.Empty(t, jobs.Jobs)
}

func TestC2JUpgradeSelectedGoalDenial(t *testing.T) {
	for _, mode := range []string{"denied", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			ctx := testjobdb.Context(context.Background())
			rt := toy.New()
			_, err := rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "selection", Name: "test", Repo: "example.test/test"})
			require.NoError(t, err)
			seed, err := rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "selection", JobID: "seed", Cell: "test", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(workflowctl.StartJob{TenantId: "selection", Cell: "test", RecipeName: "smoke"})}})
			require.NoError(t, err)
			var engine wf.Engine
			if mode == "unsupported" {
				handler := primaryFixtureHandler(remote.NewServer(rt))
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/v1/capabilities/primary-goal-selection" {
						writeJSON(w, 200, map[string]bool{"supported": false})
						return
					}
					handler.ServeHTTP(w, r)
				}))
				t.Cleanup(backend.Close)
				s, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
				require.NoError(t, err)
				engine = s.engine
			} else {
				raw, err := wf.NewEngineBuilder().WithRuntime(rt).BuildEngine()
				require.NoError(t, err)
				engine = jobdbschema.WorkflowEngine{Engine: raw, Registry: rt}
				ctx = jobdb.WithPrimaryGoalSelectionAuthorizer(ctx, func(context.Context, jobdb.GoalKey, jobdb.ResolvedCell) error {
					return errors.New("fixture selection denied")
				})
			}
			target := recipejob.ResolvedTarget{CellName: "test", CellRepo: "example.test/test", RepositorySource: "https://example.test/test", TenantID: "selection", DefaultRef: cellref.DefaultRef}
			_, err = recipejob.SubmitRecipeJob(ctx, recipejob.BuildStartJobRequest{TenantID: "selection", JobID: "denied", GoalID: seed.GoalID, Target: target, Recipe: "smoke"}, nativeSubmitter{engine})
			require.Error(t, err)
			jobs, err := rt.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"selection"}})
			require.NoError(t, err)
			require.Len(t, jobs.Jobs, 1, "selection failure created replacement work")
			require.Equal(t, "seed", jobs.Jobs[0].JobKey.JobId)
		})
	}
}
