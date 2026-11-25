package cortex

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http/httptest"
	"testing"

	"github.com/colony-2/c2j/pkg/story/live"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestRecordedTraceCompactAttemptsAndBoundaries(t *testing.T) {
	run := jobdb.GetJobRunResponse{Job: jobdb.JobRunSummary{JobType: "recipe"}}
	record := func(chapter int64, r jobdb.ExecutionRecord) {
		run.Execution = append(run.Execution, jobdb.ExecutionHistoryEntry{Ordinal: chapter, Record: r})
	}
	const scope = "root/build"
	const taskID = scope + "/attempt:1/task:build"
	const first = taskID + "/attempt:1"
	record(2, jobdb.ExecutionRecord{Kind: "scope", ID: scope, Descriptor: json.RawMessage(`{"restart":true,"title":"Build"}`)})
	record(2, jobdb.ExecutionRecord{Kind: "task", ID: taskID, Descriptor: json.RawMessage(`{"call":{"task_type":"build"}}`)})
	record(2, jobdb.ExecutionRecord{Kind: "attempt", ID: first, Attempt: 1})
	record(2, jobdb.ExecutionRecord{Kind: "result", ID: first, ErrorKind: "AppError", Error: json.RawMessage(`{"message":"build failed","code":"failed_build"}`),
		Input:  &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"input":{"n":9007199254740993},"context":{"invoke_seq":12,"node_path":"build/test"}}`)},
		Output: &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"v":2,"kind":"activity_output","payload":{"output":{"n":9007199254740993}}}`), Artifacts: []jobdb.ExecutionArtifact{{Key: &jobdb.ArtifactKey{Name: "log"}}}}})
	record(3, jobdb.ExecutionRecord{Kind: "attempt", ID: taskID + "/attempt:2", Attempt: 2})
	record(4, jobdb.ExecutionRecord{Kind: "wait", ID: taskID + "/attempt:2", Input: &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"retry":true}`)}})
	record(9, jobdb.ExecutionRecord{Kind: "scope", ID: scope, Descriptor: json.RawMessage(`{"restart":true,"title":"Build"}`)})
	record(9, jobdb.ExecutionRecord{Kind: "task", ID: taskID})
	run.Attempts = []jobdb.JobAttempt{{Ordinal: 8, Attempt: 1, Outcome: jobdb.TaskOutcome{Status: "FAILED"}}}

	rows := recordedTasks(run, live.RecordedRestartScopes(run))
	if len(rows) != 3 {
		t.Fatalf("got %d tasks", len(rows))
	}
	a, b, c := rows[0], rows[1], rows[2]
	if a.Title != "build" || a.Status != "failed" || a.Error.Message != "build failed" || a.Error.Code != "failed_build" || string(a.Input) != `{"n":9007199254740993}` || string(a.Output) != string(a.Input) || len(a.Artifacts) != 1 || len(a.Path) != 2 || *a.InvokeSeq != 12 {
		t.Fatalf("first recorded attempt: %+v", a)
	}
	if b.Title != "build" || b.Status != "running" || b.Attempt != 2 || b.Finished != nil || string(b.Input) != `{"retry":true}` || b.Scope != scope || *b.Boundary != 2 {
		t.Fatalf("open retry: %+v", b)
	}
	if c.JobAttempt != 2 || c.Status != "pending" || c.Scope != scope || *c.Boundary != 9 {
		t.Fatalf("repeated visit: %+v", c)
	}
}

func TestRecordedReadsWithoutStoryService(t *testing.T) {
	engine := &inboxReadEngine{run: jobdb.GetJobRunResponse{Job: jobdb.JobRunSummary{JobType: "recipe"}}}
	server := &Server{engine: engine}
	req := mux.SetURLVars(httptest.NewRequest("GET", "/", nil), map[string]string{"projectId": "tenant", "jobId": "job"})
	points := httptest.NewRecorder()
	server.handleRestartPoints(points, req)
	var scopes []struct{ Title string }
	if points.Code != 200 || json.Unmarshal(points.Body.Bytes(), &scopes) != nil || len(scopes) != 1 || scopes[0].Title != "Job submission" {
		t.Fatalf("restart points: %v", points.Body)
	}
	if engine.request.JobKey.JobId != "job" || engine.request.JobKey.TenantId != "tenant" || engine.request.IncludeInputs || engine.request.IncludeOutputs {
		t.Fatalf("points request: %+v", engine.request)
	}
	trace := httptest.NewRecorder()
	server.handleRecordedTrace(trace, req)
	if trace.Code != 200 || !engine.request.IncludeInputs || !engine.request.IncludeOutputs || !engine.request.IncludeArtifacts {
		t.Fatalf("task request: %+v", engine.request)
	}
}
