package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/execution"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

const submissionRequirements = `{"execution":{"schema_version":4,"recipe_resolution":"resolved","effective":{"base":"macOS-26-arm","resources":{"cpu":"4","memory":"8Gi"},"capabilities":["docker","network"]}}}`
const publishedRequirements = `{"c2j":{"execution":{"schema_version":4,"recipe_resolution":"resolved","effective":{"resources":{"memory":"16Gi"}}}},"opaque":9007199254740993}`

func TestJobExecutionRetainedProjection(t *testing.T) {
	for _, status := range []string{"specified", "unspecified", "unresolved", "malformed", "unsupported", "in_flight", "not_waiting"} {
		t.Run(status, func(t *testing.T) {
			job := recipejob.RecipeJob{Metadata: json.RawMessage(submissionRequirements), ClientPayload: json.RawMessage(publishedRequirements), ClientPayloadRevision: 9007199254740993,
				Execution: execution.View{Status: status, Source: "unavailable"}}
			before, _ := json.Marshal(job)
			got := withExecutionRequirements(job)
			after, _ := json.Marshal(job)
			if !bytes.Equal(before, after) || !reflect.DeepEqual(got.Execution, job.Execution) {
				t.Fatal("projection mutated original job/current demand")
			}
			retained := status == "in_flight" || status == "not_waiting"
			if (got.RecordedExecution != nil) != retained {
				t.Fatalf("wrong retained projection: %+v", got)
			}
			if retained && !reflect.DeepEqual(*got.RecordedExecution, execution.Inspect(job.Metadata, job.ClientPayload)) {
				t.Fatal("interpreter result changed")
			}
			raw, err := json.Marshal(got)
			if err != nil || !bytes.Contains(raw, []byte(`"opaque":9007199254740993`)) || !bytes.Contains(raw, []byte(`"client_payload_revision":9007199254740993`)) {
				t.Fatalf("lost exact payload/revision: %s, %v", raw, err)
			}
			if !retained && bytes.Contains(raw, []byte(`recorded_execution`)) {
				t.Fatal("waiting response acquired retained field")
			}
		})
	}
}

func TestJobExecutionSourceReplacement(t *testing.T) {
	for _, tc := range []struct{ name, meta, payload, status, source, cpu, memory string }{
		{"published replaces", submissionRequirements, publishedRequirements, "specified", "yield", "", "16Gi"},
		{"removed snapshot", submissionRequirements, `{"other":9007199254740993}`, "specified", "submission", "4", "8Gi"},
		{"null", submissionRequirements, `null`, "specified", "submission", "4", "8Gi"},
		{"absent", submissionRequirements, ``, "specified", "submission", "4", "8Gi"},
		{"both removed", `{}`, ``, "unresolved", "absent", "", ""},
		{"empty", `{}`, `{"c2j":{"execution":{"schema_version":4,"recipe_resolution":"resolved","effective":{}}}}`, "unspecified", "yield", "", ""},
		{"partial", `{}`, `{"c2j":{"execution":{"schema_version":4,"recipe_resolution":"unresolved","effective":{"resources":{"cpu":"500m"}}}}}`, "unresolved", "yield", "500m", ""},
		{"array", submissionRequirements, `[]`, "malformed", "yield", "", ""},
		{"unsupported", submissionRequirements, `{"c2j":{"execution":{"schema_version":999}}}`, "unsupported", "yield", "", ""},
		{"legacy", `{}`, `{"c2j":{"execution":{"schema_version":3,"recipe_resolution":"resolved","effective":{"image":"alpine:3","platform":"linux/amd64"}}}}`, "specified", "yield", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := withExecutionRequirements(recipejob.RecipeJob{Metadata: json.RawMessage(tc.meta), ClientPayload: json.RawMessage(tc.payload), ClientPayloadRevision: 19, Execution: execution.View{Status: "in_flight"}}).RecordedExecution
			if got.Status != tc.status || got.Source != tc.source {
				t.Fatalf("%+v", got)
			}
			if tc.status == "malformed" || tc.status == "unsupported" {
				if got.Diagnostic == "" || got.Demand != nil {
					t.Fatalf("invalid data fell back: %+v", got)
				}
				return
			}
			if got.Demand == nil {
				if tc.cpu != "" || tc.memory != "" {
					t.Fatal("lost values")
				}
				return
			}
			cpu, memory := got.Demand.Effective.Resources.CPU, got.Demand.Effective.Resources.Memory
			text := func(v *string) string {
				if v == nil {
					return ""
				}
				return *v
			}
			if text(cpu) != tc.cpu || text(memory) != tc.memory {
				t.Fatalf("wrong resources: %+v", got.Demand.Effective.Resources)
			}
			if tc.name == "published replaces" && got.Demand.Effective.Base != nil {
				t.Fatal("merged submission base")
			}
		})
	}
}

type requirementsDetailEngine struct {
	*compactListEngine
	info        jobdb.JobInfo
	detailReads int
	detailError error
}

func (e *requirementsDetailEngine) GetJob(context.Context, jobdb.JobKey) (jobdb.JobInfo, error) {
	e.detailReads++
	return e.info, e.detailError
}

func TestJobExecutionEndpoints(t *testing.T) {
	for _, endpoint := range []string{"list", "goal list", "children", "detail", "denied"} {
		t.Run(endpoint, func(t *testing.T) {
			jobs := compactJobs(1)
			jobs[0].Status = jobdb.JobStatusActive
			jobs[0].GoalID = "goal"
			jobs[0].Metadata, jobs[0].ClientPayload = json.RawMessage(submissionRequirements), json.RawMessage(publishedRequirements)
			engine := &requirementsDetailEngine{compactListEngine: &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: jobs, NextPageToken: "next+/="}}}, info: jobdb.JobInfo{Status: jobdb.JobStatusActive, ClientPayload: jobs[0].ClientPayload, ClientPayloadRevision: 19}}
			if endpoint == "denied" {
				engine.detailError = jobdb.ErrCellForbidden
			}
			s := &Server{engine: engine}
			handler := s.handleListJobs
			query := "/jobs?status=all&pageSize=1"
			if endpoint == "goal list" {
				query += "&goal=goal"
			}
			if endpoint == "children" {
				handler = s.handleJobChildren
			}
			if endpoint == "detail" || endpoint == "denied" {
				handler = s.handleGetJob
			}
			w := httptest.NewRecorder()
			handler(w, mux.SetURLVars(httptest.NewRequest("GET", query, nil), map[string]string{"projectId": "tenant", "jobId": "0"}))
			if endpoint == "denied" {
				if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "recorded_execution") {
					t.Fatalf("access failure changed: %d %s", w.Code, w.Body)
				}
				return
			}
			if w.Code != http.StatusOK || len(engine.requests) != 1 {
				t.Fatalf("%d calls=%d %s", w.Code, len(engine.requests), w.Body)
			}
			var got jobWithExecutionRequirements
			if endpoint == "detail" {
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if engine.detailReads != 1 {
					t.Fatal("unexpected detail reads")
				}
			} else {
				var page struct {
					Jobs []jobWithExecutionRequirements `json:"jobs"`
					Next string                         `json:"next_page_token"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if page.Next != "next+/=" || len(page.Jobs) != 1 || engine.detailReads != 0 {
					t.Fatalf("bad pagination/read count: %s", w.Body)
				}
				got = page.Jobs[0]
			}
			if got.Execution.Status != "in_flight" || got.RecordedExecution == nil || got.RecordedExecution.Status != "specified" || *got.RecordedExecution.Demand.Effective.Resources.Memory != "16Gi" {
				t.Fatalf("wrong view: %s", w.Body)
			}
		})
	}
}
