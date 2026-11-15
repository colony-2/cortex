package cortex

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/colony2/server/internal/webdist"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

func TestRetiredGoalMutationRequests(t *testing.T) {
	for _, mode := range []string{"sqlite", "sqlite-remote"} {
		for _, spa := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/api-only", true: "/spa"}[spa], func(t *testing.T) {
				ctx := testjobdb.Context(context.Background())
				runtime := retirementRuntime(t, mode)
				cfg := Config{WorkingDir: writeC2JConfig(t)}
				spaIndex := []byte("<!doctype html><html><body>SPA fallback</body></html>")
				if spa {
					cfg.StaticFS = fstest.MapFS{"index.html": &fstest.MapFile{Data: spaIndex}}
					// The ordinary Go-only suite also runs before web-embed.
					// When built, exercise the actual embedded production index.
					if built, err := fs.ReadFile(webdist.FS(), "index.html"); err == nil {
						cfg.StaticFS = webdist.FS()
						spaIndex = built
						t.Log("SPA index: embedded production build")
					} else {
						t.Log("SPA index: fixture; same production fallback handler")
					}
				}
				s := retirementServer(t, runtime, cfg)
				_, err := s.cells.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "platform", Repo: "github.com/acme/platform", RequestID: "cell"})
				if err != nil {
					t.Fatal(err)
				}
				a := submitPrimaryFixture(t, s, "tenant", "A", "platform", "", nil)
				b := submitPrimaryFixture(t, s, "tenant", "B", "platform", "", nil)
				api := httptest.NewServer(testjobdb.Handler(s))
				defer api.Close()
				if spa {
					response, err := http.Get(api.URL + "/project/tenant/goals/" + a.GoalID)
					if err != nil {
						t.Fatal(err)
					}
					data, _ := io.ReadAll(response.Body)
					response.Body.Close()
					if response.StatusCode != 200 || string(data) != string(spaIndex) {
						t.Fatal("SPA was not enabled")
					}
				}
				snapshot := func() string {
					rows, err := runtime.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"tenant"}, Stores: []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}, IncludeHidden: true, PageSize: 100})
					if err != nil {
						t.Fatal(err)
					}
					out := map[string]any{}
					for _, row := range rows.Jobs {
						info, err := runtime.GetJob(ctx, row.JobKey)
						if err != nil {
							t.Fatal(err)
						}
						out[row.JobKey.JobId] = []any{info.GoalID, info.OtherGoalIDs}
					}
					data, _ := json.Marshal(out)
					return string(data)
				}
				// The published retired URL plus common former forwarding shapes. These
				// requests never invoke the native attachment API, even in fixture setup.
				for _, request := range []struct{ method, path string }{
					{"POST", "/goals/" + a.GoalID + "/candidates"}, {"DELETE", "/goals/" + a.GoalID + "/candidates/B"},
					{"PUT", "/jobs/B/goals/" + a.GoalID}, {"POST", "/jobs/B/goals"}, {"PATCH", "/jobs/B/goals"},
				} {
					before := snapshot()
					req, _ := http.NewRequest(request.method, api.URL+"/api/projects/tenant"+request.path, strings.NewReader(`{"job_id":"B","goal_id":"`+a.GoalID+`"}`))
					req.Header.Set("Content-Type", "application/json")
					response, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					data, _ := io.ReadAll(response.Body)
					response.Body.Close()
					if response.StatusCode < 400 || strings.Contains(string(data), "SPA fallback") {
						t.Fatalf("retired %s %s: %d %s", request.method, request.path, response.StatusCode, data)
					}
					if after := snapshot(); after != before {
						t.Fatalf("membership changed: %s => %s", before, after)
					}
				}
				if a.GoalID == b.GoalID {
					t.Fatal("independent goals collapsed")
				}
			})
		}
	}
}

func TestGoalReadsWaitsAndFailureSurviveRetirement(t *testing.T) {
	for _, mode := range []string{"sqlite", "sqlite-remote"} {
		t.Run(mode, func(t *testing.T) {
			ctx := testjobdb.Context(context.Background())
			runtime := retirementRuntime(t, mode)
			s := retirementServer(t, runtime, Config{WorkingDir: writeC2JConfig(t)})
			_, err := s.cells.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "platform", Repo: "github.com/acme/platform", RequestID: "cell"})
			if err != nil {
				t.Fatal(err)
			}
			target := submitPrimaryFixture(t, s, "tenant", "target", "platform", "", nil)
			consumer := submitPrimaryFixture(t, s, "tenant", "consumer", "platform", "", nil)
			lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: consumer.JobID}, WorkerID: "wait", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
			if err != nil || lease == nil {
				t.Fatalf("%v %v", lease, err)
			}
			defer lease.StopKeepAlive()
			if err := lease.Reschedule(ctx, jobdb.RescheduleExecutionRequest{NextRoute: jobdb.Route{JobType: "recipe"}, DependencyWait: &jobdb.DependencyWait{WaitID: "goal-wait", Dependencies: []jobdb.Dependency{{Target: jobdb.DependencyTarget{GoalID: target.GoalID}, Condition: jobdb.JobPrereqSuccess}}}}); err != nil {
				t.Fatal(err)
			}
			ctl := s.engine.(interface {
				GetGoal(context.Context, jobdb.GoalKey) (jobdb.GoalInfo, error)
				FailGoal(context.Context, jobdb.FailGoalRequest) (jobdb.GoalInfo, error)
				GetDependencyWait(context.Context, jobdb.DependencyWaitKey) (jobdb.DependencyWaitInfo, error)
			})
			key := jobdb.GoalKey{TenantId: "tenant", GoalID: target.GoalID}
			goal, err := ctl.GetGoal(ctx, key)
			if err != nil || goal.State != jobdb.GoalOpen {
				t.Fatalf("%+v %v", goal, err)
			}
			failed, err := ctl.FailGoal(ctx, jobdb.FailGoalRequest{GoalKey: key, Message: "No further attempts are planned"})
			if err != nil || failed.Failure == nil || failed.Failure.Message != "No further attempts are planned" {
				t.Fatalf("%+v %v", failed, err)
			}
			wait, err := ctl.GetDependencyWait(ctx, jobdb.DependencyWaitKey{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: consumer.JobID}, WaitID: "goal-wait"})
			if err != nil || wait.State != jobdb.DependencyFailed || wait.Err() == nil {
				t.Fatalf("%+v %v", wait, err)
			}
			info, err := runtime.GetJob(ctx, jobdb.JobKey{TenantId: "tenant", JobId: target.JobID})
			if err != nil || info.Status != jobdb.JobStatusReady {
				t.Fatalf("failure canceled candidate: %+v %v", info, err)
			}
		})
	}
}

// A read-only retained-history translation supplies a settled witness whose
// producer has another primary. Native artifact storage/transport stay real.
type retainedWaitEngine struct {
	jobworkflow.Engine
	wait *jobdb.DependencyWaitInfo
}

func (e retainedWaitEngine) GetJob(ctx context.Context, key jobdb.JobKey) (jobdb.JobInfo, error) {
	info, err := e.Engine.GetJob(ctx, key)
	if key.JobId == "consumer" && err == nil {
		info.BlockingWait = e.wait
	}
	return info, err
}
func TestRetainedWaitKeepsExactProducer(t *testing.T) {
	ctx := testjobdb.Context(context.Background())
	runtime := retirementRuntime(t, "sqlite-remote")
	s := retirementServer(t, runtime, Config{WorkingDir: writeC2JConfig(t)})
	_, err := s.cells.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "platform", Repo: "github.com/acme/platform", RequestID: "cell"})
	if err != nil {
		t.Fatal(err)
	}
	producer := submitPrimaryFixture(t, s, "tenant", "B", "platform", "", nil)
	consumer := submitPrimaryFixture(t, s, "tenant", "consumer", "platform", "", map[string]any{"documents": map[string]any{"result.md": artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "B", TaskOrdinal: 1, Name: "original.md"})}})
	lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "B"}, WorkerID: "producer", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
	if err != nil || lease == nil {
		t.Fatalf("%v %v", lease, err)
	}
	defer lease.StopKeepAlive()
	if err := lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: "success", Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"result":"B"}`)}}}}, ArtifactUploads: []jobdb.ArtifactUpload{{Name: "original.md", Size: 16, Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("original B bytes")), nil }}}}); err != nil {
		t.Fatal(err)
	}
	wait := retirementWait()
	s.engine = retainedWaitEngine{Engine: s.engine, wait: wait}
	api := httptest.NewServer(testjobdb.Handler(s))
	defer api.Close()
	detail := getJSON[recipejob.RecipeJob](t, api.URL+"/api/projects/tenant/jobs/consumer")
	if !reflect.DeepEqual(detail.BlockingWait, wait) || detail.GoalID != consumer.GoalID || producer.GoalID == "G" {
		t.Fatalf("wrong retained identity: %+v", detail)
	}
	docs := getJSON[struct {
		Artifacts []inputDocument `json:"artifacts"`
	}](t, api.URL+"/api/projects/tenant/jobs/consumer/input-artifacts")
	if len(docs.Artifacts) != 1 || docs.Artifacts[0].StoredKey == nil || docs.Artifacts[0].StoredKey.JobId != "B" {
		t.Fatalf("%+v", docs)
	}
	response, err := http.Get(api.URL + "/api/projects/tenant/jobs/B/tasks/1/artifacts/original.md")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || string(data) != "original B bytes" {
		t.Fatalf("%d %s", response.StatusCode, data)
	}
}
