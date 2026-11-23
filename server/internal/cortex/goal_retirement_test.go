package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

// Reflect on both method sets, including promoted methods. Native runtimes are
// setup handles; owned ports/compositions must not inherit attachment mutation.
func TestOwnedMethodSetsExcludeGoalAttachment(t *testing.T) {
	for _, value := range []any{
		Server{}, nativeSubmitter{}, archivedJobReader{}, leaseEventsRuntime{}, leaseEventsEngine{}, leaseCountingReader{}, leaseCountingEngine{}, retainedWaitEngine{}, retirementResponseEngine{},
		childrenRuntimeEngine{}, childrenErrorEngine{}, historicalSummaryEngine{}, compactListEngine{},
		producerAccessEngine{}, priorAttemptEngine{},
		inboxReadEngine{}, finalRetainedEngine{}, reviewArtifactEngine{}, inputStreamEngine{},
		failingGoalEngine{}, failingArchiveRuntime{}, failingCancellationEngine{}, treeDeadlineEngine{},
		timingEngine{}, timingRuntime{}, rootErrorEngine{}, publicationRecordReader{}, contextInspectionService{}, replayForwardService{},
		brokenStoryService{}, inboxFilenameService{}, reviewRuntimeStub{}, historyStub{},
		snapshotHistoryStub{}, finalStoryStub{}, finalDocumentsStub{}, recordingSecrets{}, remoteSecretAdmin{},
		jobdbschema.WorkflowEngine{},
	} {
		typ := reflect.TypeOf(value)
		for _, set := range []reflect.Type{typ, reflect.PointerTo(typ)} {
			for i := 0; i < set.NumMethod(); i++ {
				name := set.Method(i).Name
				if name == "AddGoalCandidate" || strings.Contains(name, "AttachGoal") || strings.Contains(name, "DetachGoal") || strings.Contains(name, "RemoveGoalCandidate") {
					t.Errorf("%s exposes retired mutation %s", set, name)
				}
			}
		}
	}
	if reflect.TypeOf(leaseEventsRuntime{}).NumMethod() != 1 {
		t.Fatal("lease reader grew unrelated methods")
	}
}

type retirementResponseEngine struct {
	jobworkflow.Engine
	source          jobdb.JobInfo
	inspectionError error
	submitted       int
	accepted        jobdb.JobHandle
}

func (e *retirementResponseEngine) GetJob(context.Context, jobdb.JobKey) (jobdb.JobInfo, error) {
	return e.source, e.inspectionError
}
func (e *retirementResponseEngine) SubmitJob(ctx context.Context, job jobdb.SubmitJob) (jobdb.JobHandle, error) {
	e.submitted++
	handle, err := e.Engine.SubmitJob(ctx, job)
	e.accepted = handle
	return handle, err
}
func retirementWait() *jobdb.DependencyWaitInfo {
	deadline := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	return &jobdb.DependencyWaitInfo{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "selected"}, WaitID: "retained-wait", State: jobdb.DependencySatisfied, Deadline: &deadline,
		Dependencies: []jobdb.DependencyResolution{{Target: jobdb.DependencyTarget{GoalID: "G"}, Condition: jobdb.JobPrereqSuccess, State: jobdb.DependencySatisfied, SatisfiedByJobID: "B", BlockedReason: "retained reason"}}}
}
func TestGoalRetirementResponseBoundaries(t *testing.T) {
	for _, mode := range []string{"detail", "submission", "unavailable inspection"} {
		t.Run(mode, func(t *testing.T) {
			s, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
			if err != nil {
				t.Fatal(err)
			}
			ctx := testjobdb.Context(context.Background())
			cell, err := s.cells.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "platform", Repo: "github.com/acme/platform", RequestID: "cell"})
			if err != nil {
				t.Fatal(err)
			}
			e := &retirementResponseEngine{Engine: s.engine, source: jobdb.JobInfo{GoalID: "primary", OtherGoalIDs: []string{"obsolete"}, BlockingWait: retirementWait()}}
			s.engine = e
			if mode == "detail" {
				submitPrimaryFixture(t, &Server{engine: e.Engine, cells: s.cells}, "tenant", "selected", "platform", "", nil)
			}
			if mode == "unavailable inspection" {
				e.inspectionError = errors.New("inspection offline")
			}
			before, _ := json.Marshal(e.source)
			w := httptest.NewRecorder()
			req := mux.SetURLVars(httptest.NewRequest("GET", "/", nil).WithContext(ctx), map[string]string{"projectId": "tenant", "jobId": "selected"})
			if mode == "detail" {
				s.handleGetJob(w, req)
			} else {
				req = mux.SetURLVars(httptest.NewRequest("POST", "/", strings.NewReader(`{"cell":"platform","job_id":"selected","inputs":{"prompt":"Test"}}`)).WithContext(ctx), map[string]string{"projectId": "tenant"})
				s.handleSubmitJob(w, req)
			}
			wantStatus := 200
			if mode != "detail" {
				wantStatus = 201
			}
			var got recipejob.RecipeJob
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if w.Code != wantStatus || got.JobID != "selected" || got.TenantID != "tenant" || got.CellID != cell.ID || got.CellName != "platform" || got.RepositorySource != "github.com/acme/platform" || got.GoalID == "" || len(got.OtherGoalIDs) != 0 {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if mode != "unavailable inspection" && (got.GoalID != "primary" || !reflect.DeepEqual(got.BlockingWait, e.source.BlockingWait)) {
				t.Fatalf("lost primary/wait: %s", w.Body)
			}
			if mode == "unavailable inspection" && got.GoalID != e.accepted.GoalID {
				t.Fatal("fallback lost accepted primary identity")
			}
			if mode != "detail" && e.submitted != 1 {
				t.Fatalf("submitted %d times", e.submitted)
			}
			after, _ := json.Marshal(e.source)
			if string(before) != string(after) {
				t.Fatal("source data changed")
			}
		})
	}
}

func TestGoalRetirementConcurrentDetailProjection(t *testing.T) {
	s, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := testjobdb.Context(context.Background())
	_, err = s.cells.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "platform", Repo: "github.com/acme/platform", RequestID: "cell"})
	if err != nil {
		t.Fatal(err)
	}
	submitPrimaryFixture(t, s, "tenant", "selected", "platform", "", nil)
	source := jobdb.JobInfo{GoalID: "primary", OtherGoalIDs: []string{"obsolete"}, BlockingWait: retirementWait()}
	s.engine = &retirementResponseEngine{Engine: s.engine, source: source}
	before, _ := json.Marshal(source)
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			w := httptest.NewRecorder()
			r := mux.SetURLVars(httptest.NewRequest("GET", "/", nil).WithContext(ctx), map[string]string{"projectId": "tenant", "jobId": "selected"})
			s.handleGetJob(w, r)
			var detail recipejob.RecipeJob
			if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil || w.Code != 200 || len(detail.OtherGoalIDs) != 0 || detail.GoalID != "primary" || !reflect.DeepEqual(detail.BlockingWait, source.BlockingWait) {
				t.Errorf("%d %s %v", w.Code, w.Body, err)
			}
		}()
	}
	group.Wait()
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("shared inspection source changed")
	}
}
