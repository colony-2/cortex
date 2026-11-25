package cortex

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/colony-2/c2j/pkg/jobrestart"
	"github.com/colony-2/c2j/pkg/story/live"
	"github.com/colony-2/c2j/pkg/task"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type recordedTask struct {
	ID         string                    `json:"id"`
	LaunchID   string                    `json:"launch_id,omitempty"`
	Invocation string                    `json:"invocation_id,omitempty"`
	Title      string                    `json:"title"`
	Status     string                    `json:"status"`
	Ordinal    int64                     `json:"task_ordinal"`
	JobAttempt int                       `json:"job_attempt,omitempty"`
	Attempt    int                       `json:"attempt"`
	Started    *time.Time                `json:"started_at,omitempty"`
	Finished   *time.Time                `json:"finished_at,omitempty"`
	Path       []string                  `json:"path,omitempty"`
	InvokeSeq  *int64                    `json:"invoke_seq,omitempty"`
	Inbox      []jobdb.ExecutionArtifact `json:"input_artifacts,omitempty"`
	Input      json.RawMessage           `json:"input,omitempty"`
	Output     json.RawMessage           `json:"output,omitempty"`
	Artifacts  []jobdb.ArtifactKey       `json:"artifact_keys,omitempty"`
	Error      *traceError               `json:"error,omitempty"`
	Scope      string                    `json:"restart_scope_id,omitempty"`
	Boundary   *int64                    `json:"restart_chapter_ordinal,omitempty"`
}
type traceError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (s *Server) handleRecordedTrace(w http.ResponseWriter, r *http.Request) {
	s.writeRecordedRun(w, r, true)
}

func (s *Server) handleRestartPoints(w http.ResponseWriter, r *http.Request) {
	s.writeRecordedRun(w, r, false)
}

func (s *Server) writeRecordedRun(w http.ResponseWriter, r *http.Request, tasks bool) {
	req := jobdb.GetJobRunRequest{
		JobKey:        jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]},
		IncludeInputs: tasks, IncludeOutputs: tasks, IncludeArtifacts: tasks, IncludeAttemptInputs: true,
	}
	inventory := conversationInventory{key: req.JobKey, launches: map[int64]string{}}
	var run jobdb.GetJobRunResponse
	var err error
	if reader, ok := s.chapters.(jobdb.JobRunReader); tasks && ok {
		// Capture chapter worker IDs from the same history read as the task data.
		run, err = inventory.readRun(r.Context(), reader, req)
	} else {
		run, err = s.engine.GetJobRun(r.Context(), req)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	points := live.RecordedRestartScopes(run)
	w.Header().Set("Cache-Control", "no-store")
	if !tasks {
		writeJSON(w, http.StatusOK, points)
		return
	}
	rows := recordedTasks(run, points)
	for _, row := range rows {
		if launch := inventory.launches[row.Ordinal]; launch != "" {
			row.LaunchID = launch
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Tasks    []*recordedTask `json:"tasks"`
		Terminal bool            `json:"terminal"`
	}{rows, run.Job.ArchivedAt != nil || run.Job.Status == jobdb.JobStatusCancelled || run.Job.Status == jobdb.JobStatusCompleted})
}

func tracePayload(data json.RawMessage) json.RawMessage {
	if len(data) == 0 || json.Valid(data) {
		return data
	}
	value, _ := json.Marshal(string(data))
	return value
}

// Activity inputs and outputs wrap the values displayed by Trace. Keep payload
// bytes intact so the browser can display numbers beyond JavaScript precision.
func (row *recordedTask) readInput(raw json.RawMessage) {
	var envelope struct {
		Input   json.RawMessage `json:"input"`
		Context *struct {
			InvokeSeq *int64 `json:"invoke_seq"`
			NodePath  string `json:"node_path"`
		} `json:"context"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Context != nil && envelope.Input != nil {
		row.Input = envelope.Input
		row.InvokeSeq = envelope.Context.InvokeSeq
		row.Path = strings.FieldsFunc(envelope.Context.NodePath, func(c rune) bool { return c == '/' })
	} else {
		row.Input = tracePayload(raw)
	}
}

func recordedOutput(raw json.RawMessage) json.RawMessage {
	var envelope task.OutputEnvelope
	if json.Unmarshal(raw, &envelope) == nil && (envelope.Version == 1 || envelope.Version == task.OutputEnvelopeVersion) {
		switch envelope.Kind {
		case task.OutputKindInternalResult:
			return envelope.Payload
		case task.OutputKindActivityInvocationOutput:
			var activity struct {
				Output json.RawMessage `json:"output"`
			}
			if json.Unmarshal(envelope.Payload, &activity) == nil {
				return activity.Output
			}
		}
	}
	return tracePayload(raw)
}

// A compact JobDB chapter can contain several logical events. Preserve their
// recorded order and match results to their invocation and attempt, not chapter alone.
func recordedTasks(run jobdb.GetJobRunResponse, points []jobrestart.RestartScope) []*recordedTask {
	rows := make([]*recordedTask, 0)
	type scopeVisit struct {
		id      string
		ordinal int64
	}
	scopes := map[scopeVisit]jobrestart.RestartScope{}
	for _, p := range points {
		scopes[scopeVisit{p.ScopeID, p.ChapterOrdinal}] = p
	}
	seenScopes := map[string]jobrestart.RestartScope{}
	tasks := map[string]*recordedTask{}
	attempts := map[string]*recordedTask{}
	jobAttempt := 1
	var completed []jobdb.JobAttempt
	for _, a := range run.Attempts {
		if a.Outcome.Status != "" {
			completed = append(completed, a)
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].Ordinal < completed[j].Ordinal })
	for _, event := range run.Execution {
		r := event.Record
		for len(completed) > 0 && completed[0].Ordinal < event.Ordinal {
			jobAttempt = completed[0].Attempt + 1
			completed = completed[1:]
			clear(seenScopes)
			clear(tasks)
			clear(attempts)
		}
		switch r.Kind {
		case "scope":
			if p, ok := scopes[scopeVisit{r.ID, event.Ordinal}]; ok {
				seenScopes[r.ID] = p
			} else {
				delete(seenScopes, r.ID)
			}
		case "task":
			var descriptor struct {
				Call struct {
					Type string `json:"task_type"`
					Site string `json:"site"`
				} `json:"call"`
			}
			_ = json.Unmarshal(r.Descriptor, &descriptor)
			title := descriptor.Call.Type
			if title == "" {
				title = descriptor.Call.Site
			}
			if title == "" {
				title = r.ID
			}
			row := &recordedTask{ID: r.ID, Invocation: r.ID, Title: title, Ordinal: event.Ordinal, JobAttempt: jobAttempt, Attempt: 1, Status: "pending", Started: &r.At}
			for parent := r.ID; parent != ""; {
				if p, ok := seenScopes[parent]; ok {
					row.Scope = p.ScopeID
					row.Boundary = &p.ChapterOrdinal
					break
				}
				i := strings.LastIndexByte(parent, '/')
				if i < 0 {
					break
				}
				parent = parent[:i]
			}
			tasks[r.ID] = row
			rows = append(rows, row)
		case "attempt":
			split := strings.LastIndex(r.ID, "/attempt:")
			if split < 0 {
				continue
			}
			parent := tasks[r.ID[:split]]
			if parent == nil {
				continue
			}
			row := parent
			if r.Attempt > 1 {
				row = &recordedTask{Invocation: parent.Invocation, Title: parent.Title, Scope: parent.Scope, Boundary: parent.Boundary, JobAttempt: jobAttempt}
				rows = append(rows, row)
			}
			row.ID = r.ID
			row.Status = "running"
			row.Ordinal = event.Ordinal
			row.Attempt = r.Attempt
			row.Started = &r.At
			attempts[r.ID] = row
		case "wait", "result":
			row := attempts[r.ID]
			if row == nil {
				continue
			}
			if r.Input != nil {
				row.readInput(r.Input.Data)
				row.Inbox = r.Input.Artifacts
			}
			row.Ordinal = event.Ordinal
			if r.Kind == "wait" {
				continue
			}
			row.Finished = &r.At
			row.Status = "succeeded"
			if r.ErrorKind != "" {
				row.Status = "failed"
				var detail traceError
				_ = json.Unmarshal(r.Error, &detail)
				if detail.Code == "" {
					detail.Code = r.ErrorKind
				}
				if detail.Message == "" {
					detail.Message = string(r.Error)
				}
				row.Error = &detail
			}
			if r.Output != nil {
				row.Output = recordedOutput(r.Output.Data)
				for _, art := range r.Output.Artifacts {
					if art.Key != nil {
						row.Artifacts = append(row.Artifacts, *art.Key)
					}
				}
			}
		}
	}
	if len(rows) > 0 {
		return rows
	}
	// Earlier histories store physical task attempts without logical task records.
	for _, job := range run.Attempts {
		for _, task := range job.Tasks {
			for _, a := range task.Attempts {
				row := &recordedTask{ID: task.TaskRunID, LaunchID: a.WorkerID, Title: task.TaskType, JobAttempt: job.Attempt, Attempt: a.Attempt, Ordinal: a.Ordinal, Status: strings.ToLower(a.State)}
				switch a.State {
				case jobdb.TaskAttemptStateReady:
					row.Status = "pending"
				case jobdb.TaskAttemptStateLeased, jobdb.TaskAttemptStateWaiting:
					row.Status = "running"
				}
				if a.Outcome.Status != "" {
					row.Finished = &a.CreatedAt
				}
				if a.Input != nil {
					row.readInput(a.Input.Data)
					for _, art := range a.Input.Artifacts {
						row.Inbox = append(row.Inbox, jobdb.ExecutionArtifact{Name: art.Name, Key: art.Key})
					}
				}
				if a.Output != nil {
					row.Output = recordedOutput(a.Output.Data)
					for _, art := range a.Output.Artifacts {
						if art.Key != nil {
							row.Artifacts = append(row.Artifacts, *art.Key)
						}
					}
				}
				if a.Outcome.Error != nil {
					row.Error = &traceError{Code: a.Outcome.Error.Code, Message: a.Outcome.Error.Message}
				}
				rows = append(rows, row)
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Ordinal < rows[j].Ordinal })
	return rows
}
