package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

func TestChildrenRuntimeIntegration(t *testing.T) {
	for _, mode := range []string{"toy", "remote", "sqlite-remote"} {
		t.Run(mode, func(t *testing.T) {
			var runtime jobdb.WorkflowRuntime = toy.New()
			if mode == "sqlite-remote" {
				embedded, err := sqliteruntime.StartEmbeddedRuntime(testjobdb.Context(context.Background()))
				if err != nil {
					t.Fatal(err)
				}
				defer embedded.Shutdown()
				runtime = embedded.Runtime
			}
			if mode != "toy" {
				backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(runtime)))
				defer backend.Close()
				var err error
				runtime, err = remote.New(backend.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx := testjobdb.Context(context.Background())
			submit := func(tenant, id string) jobdb.JobHandle {
				t.Helper()
				h, err := celltest.Submit(runtime, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: id, Cell: "platform", CellRepo: "github.com/acme/platform", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(1)}})
				if err != nil {
					t.Fatal(err)
				}
				return h
			}
			leaseFor := func(key jobdb.JobKey) jobdb.ExecutionLease {
				t.Helper()
				lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: key, WorkerID: "relationships", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
				if err != nil || lease == nil {
					t.Fatalf("lease %v %v", lease, err)
				}
				return lease
			}
			_ = submit("a", "same")
			_ = submit("b", "same")
			unrelated := submit("a", "unrelated-same-goal")
			_ = submit("a", "dependency-only")
			parentA := leaseFor(jobdb.JobKey{TenantId: "a", JobId: "same"})
			defer parentA.StopKeepAlive()
			parentB := leaseFor(jobdb.JobKey{TenantId: "b", JobId: "same"})
			defer parentB.StopKeepAlive()
			completePrimaryFixture(t, runtime, "a", unrelated.JobKey.JobId, "failed_app")
			child := func(parent jobdb.ExecutionLease, id, cell string) jobdb.JobHandle {
				t.Helper()
				h, err := parent.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "b", JobID: id, Cell: cell, CellRepo: "github.com/acme/" + cell, JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(1)}})
				if err != nil {
					t.Fatal(err)
				}
				return h
			}
			// A caller-supplied child tenant cannot override the parent's tenant.
			active := child(parentA, "active", "platform")
			if active.JobKey.TenantId != "a" {
				t.Fatalf("tenant escape: %+v", active)
			}
			assertHistoricalSameGoalPeer(t, ctx, runtime, active, unrelated)

			hidden := child(parentA, "hidden", "platform")
			completed := child(parentA, "completed", "platform")
			if _, err := parentA.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "a", Name: "other", Repo: "github.com/acme/other", RequestID: "create-other"}); err != nil {
				t.Fatal(err)
			}
			failed := child(parentA, "failed", "other")
			_ = child(parentB, "active", "platform")
			grandParent := leaseFor(active.JobKey)
			defer grandParent.StopKeepAlive()
			_ = child(grandParent, "grandchild", "platform")
			for _, item := range []struct {
				h      jobdb.JobHandle
				status string
			}{{completed, "success"}, {failed, "failed_app"}, {hidden, "success"}} {
				lease := leaseFor(item.h.JobKey)
				if err := lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: item.status, Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"ok":true}`)}}}}}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := runtime.(jobdb.JobDeletionRuntime).DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: "a", JobIDs: []string{"hidden"}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Hour)
			if err := parentA.Reschedule(ctx, jobdb.RescheduleExecutionRequest{NextRoute: jobdb.Route{JobType: "recipe"}, DependencyWait: &jobdb.DependencyWait{WaitID: "dependencies-only", Deadline: &deadline, Dependencies: []jobdb.Dependency{{Target: jobdb.DependencyTarget{JobID: "dependency-only"}, Condition: jobdb.JobPrereqComplete}}}}); err != nil {
				t.Fatal(err)
			}
			var found = map[string]bool{}
			token := ""
			for {
				w := httptest.NewRecorder()
				request := mux.SetURLVars(httptest.NewRequest("GET", fmt.Sprintf("/children?pageSize=2&pageToken=%s", url.QueryEscape(token)), nil).WithContext(ctx), map[string]string{"projectId": "a", "jobId": "same"})
				(&Server{engine: childrenRuntimeEngine{runtime: runtime, translatedGoal: active.GoalID}}).handleJobChildren(w, request)
				if w.Code != 200 {
					t.Fatalf("%d %s", w.Code, w.Body)
				}
				var page struct {
					Jobs []struct {
						ID     string `json:"job_id"`
						Tenant string `json:"tenant_id"`
					}
					Token string `json:"next_page_token"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				for _, j := range page.Jobs {
					if j.Tenant != "a" {
						t.Fatal(j)
					}
					found[j.ID] = true
				}
				token = page.Token
				if token == "" {
					break
				}
			}
			if len(found) != 4 || !found["active"] || !found["completed"] || !found["failed"] || !found["hidden"] {
				t.Fatalf("unexpected children: %v", found)
			}
		})
	}
}

type childrenRuntimeEngine struct {
	jobworkflow.Engine
	runtime        jobdb.WorkflowRuntime
	translatedGoal string
}

func (e childrenRuntimeEngine) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	page, err := e.runtime.ListJobs(ctx, req)
	// Bounded stored-summary translation: no database membership mutation. Parent
	// filtering still runs in the real runtime, so this unrelated root stays out.
	for i := range page.Jobs {
		if page.Jobs[i].JobKey.JobId == "unrelated-same-goal" && e.translatedGoal != "" {
			page.Jobs[i].GoalID = e.translatedGoal
		}
	}
	return page, err
}

// Confirm the bounded translated read fixture really presents an unrelated
// same-goal root before proving structural child/tree reads exclude it.
func assertHistoricalSameGoalPeer(t *testing.T, ctx context.Context, runtime jobdb.WorkflowRuntime, active, unrelated jobdb.JobHandle) {
	t.Helper()
	page, err := (childrenRuntimeEngine{runtime: runtime, translatedGoal: active.GoalID}).ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"a"}, JobKeys: []jobdb.JobKey{active.JobKey, unrelated.JobKey}, Stores: []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}, PageSize: 2, IncludeHidden: true})
	if err != nil || len(page.Jobs) != 2 {
		t.Fatalf("translated peer fixture: %+v %v", page, err)
	}
	for _, row := range page.Jobs {
		if row.GoalID != active.GoalID {
			t.Fatal("translated membership missing")
		}
		if row.JobKey == unrelated.JobKey && row.ParentJobID != "" {
			t.Fatal("unrelated peer gained parentage")
		}
	}
}
