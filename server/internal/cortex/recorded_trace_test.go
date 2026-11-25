package cortex

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/story/live"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestRecordedTraceCompactAttemptsAndBoundaries(t *testing.T) {
	run := jobdb.GetJobRunResponse{Job: jobdb.JobRunSummary{JobType: "recipe"}}
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	record := func(chapter int64, r jobdb.ExecutionRecord) {
		r.At = base.Add(time.Duration(len(run.Execution)) * time.Second)
		run.Execution = append(run.Execution, jobdb.ExecutionHistoryEntry{Ordinal: chapter, Record: r})
	}
	const scope = "root/build"
	const taskID = scope + "/attempt:1/task:build"
	const first = taskID + "/attempt:1"
	record(2, jobdb.ExecutionRecord{Kind: "scope", ID: scope, Descriptor: json.RawMessage(`{"restart":true,"title":"Build"}`)})
	record(2, jobdb.ExecutionRecord{Kind: "task", ID: taskID, Descriptor: json.RawMessage(`{"call":{"task_type":"build","command":{"node":"build/test","op":"nix:github:acme/ops#check"}}}`)})
	record(2, jobdb.ExecutionRecord{Kind: "attempt", ID: first, Attempt: 1})
	record(2, jobdb.ExecutionRecord{Kind: "result", ID: first, ErrorKind: "AppError", Error: json.RawMessage(`{"message":"build failed","code":"failed_build"}`),
		Input:  &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"input":{"n":9007199254740993},"context":{"invoke_seq":12,"node_path":"build/test"}}`)},
		Output: &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"v":2,"kind":"activity_output","payload":{"output":{"n":9007199254740993}}}`), Artifacts: []jobdb.ExecutionArtifact{{Key: &jobdb.ArtifactKey{Name: "log"}}}}})
	record(3, jobdb.ExecutionRecord{Kind: "attempt", ID: taskID + "/attempt:2", Attempt: 2})
	record(4, jobdb.ExecutionRecord{Kind: "wait", ID: taskID + "/attempt:2", Input: &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"retry":true}`)}})
	record(9, jobdb.ExecutionRecord{Kind: "scope", ID: scope, Descriptor: json.RawMessage(`{"restart":true,"title":"Build"}`)})
	record(9, jobdb.ExecutionRecord{Kind: "task", ID: taskID})
	run.Attempts = []jobdb.JobAttempt{{Ordinal: 8, Attempt: 1, Outcome: jobdb.TaskOutcome{Status: "FAILED"}}}

	rows := recordedTasks(run, live.RecordedRestartScopes(run), nil)
	if len(rows) != 3 {
		t.Fatalf("got %d tasks", len(rows))
	}
	a, b, c := rows[0], rows[1], rows[2]
	if a.Started == nil || !a.Started.Equal(base.Add(2*time.Second)) || a.Finished == nil || !a.Finished.Equal(base.Add(3*time.Second)) || b.Started == nil || !b.Started.Equal(base.Add(4*time.Second)) || c.Started != nil {
		t.Fatalf("incorrect per-attempt timing: %+v %+v %+v", a, b, c)
	}
	for _, row := range []*recordedTask{a, b} {
		if row.OpType != "nix:github:acme/ops#check" || len(row.Path) != 2 || row.Path[1] != "test" {
			t.Fatalf("missing recorded op identity on attempt %d: %+v", row.Attempt, row)
		}
	}
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

func TestRecordedMetricsFromResultEnvelopes(t *testing.T) {
	const reports = `{"runtime":{"status":"valid","report":{"version":1,"mutations":[{"name":"c2j_task_duration_seconds","kind":"histogram","number_type":"float64","op":"observe","value":1.25}]}},"producer":{"status":"valid","report":{"version":1,"mutations":[{"name":"tokens","kind":"counter","number_type":"int64","op":"add","value":9007199254740993}]}}}`
	for _, kind := range []string{"internal_result", "activity_output"} {
		payload := `{"metrics":"business value"}`
		if kind == "activity_output" {
			payload = `{"output":` + payload + `}`
		}
		raw := json.RawMessage(`{"v":2,"kind":"` + kind + `","payload":` + payload + `,"metrics":` + reports + `}`)
		physical := jobdb.GetJobRunResponse{Attempts: []jobdb.JobAttempt{{Tasks: []jobdb.TaskRun{{Attempts: []jobdb.TaskAttempt{{Output: &jobdb.TaskIO{Data: raw}}}}}}}}
		logical := jobdb.GetJobRunResponse{Execution: []jobdb.ExecutionHistoryEntry{
			{Record: jobdb.ExecutionRecord{Kind: "task", ID: "work"}},
			{Record: jobdb.ExecutionRecord{Kind: "attempt", ID: "work/attempt:1", Attempt: 1}},
			{Record: jobdb.ExecutionRecord{Kind: "result", ID: "work/attempt:1", Output: &jobdb.ExecutionSnapshot{Data: raw}}},
		}}
		for _, history := range []jobdb.GetJobRunResponse{physical, logical} {
			row := recordedTasks(history, nil, nil)[0]
			if string(row.Output) != `{"metrics":"business value"}` || row.Metrics == nil {
				t.Fatalf("unexpected %s output or metrics: %+v", kind, row)
			}
			producer := row.Metrics.Producer.Report.Mutations[0]
			if producer.Value != int64(9007199254740993) || row.Metrics.Runtime.Report.Mutations[0].Value != 1.25 {
				t.Fatalf("lost metric precision: %+v", row.Metrics)
			}
		}
	}
	for _, raw := range []string{`{"metrics":{"runtime":{"status":"valid"}}}`, `{"v":1,"kind":"internal_result","metrics":` + reports + `}`} {
		var row recordedTask
		row.readOutput(json.RawMessage(raw))
		if row.Metrics != nil {
			t.Fatalf("interpreted uninstrumented output as metrics: %s", raw)
		}
	}
}

func TestRecordedPhysicalTaskTimes(t *testing.T) {
	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Second)
	run := jobdb.GetJobRunResponse{Attempts: []jobdb.JobAttempt{{Tasks: []jobdb.TaskRun{{Attempts: []jobdb.TaskAttempt{
		{Ordinal: 3, CreatedAt: end.Add(time.Second), Outcome: jobdb.TaskOutcome{Status: jobdb.TaskOutcomeStatusSucceeded}},
		{Ordinal: 4, State: jobdb.TaskAttemptStateReady},
	}}}}}}
	rows := recordedTasks(run, nil, map[int64]taskTimes{3: {started: &start, finished: &end}})
	if len(rows) != 2 || rows[0].Started != &start || rows[0].Finished != &end || rows[1].Started != nil || rows[1].Finished != nil {
		t.Fatalf("unexpected physical task times: %+v", rows)
	}
}
