package cortex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

func TestRestoredListFieldsWithoutInspection(t *testing.T) {
	for _, count := range []int{1, 200} {
		for _, archived := range []bool{false, true} {
			t.Run(fmt.Sprintf("rows=%d/archived=%v", count, archived), func(t *testing.T) {
				jobs := compactJobs(count)
				deadline := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
				payloads := []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[9007199254740993,"雪"]`), json.RawMessage(`{"other":{"n":9007199254740993}}`), json.RawMessage(`"` + strings.Repeat("x", 65534) + `"`)}
				for i := range jobs {
					job := &jobs[i]
					job.ClientPayload = payloads[i%len(payloads)]
					job.ClientPayloadRevision = 19
					job.WaitFor = []string{"second", "first"}
					job.ExecutionState.TaskWait = &jobdb.TaskWait{InputOrdinal: 0, OutputOrdinal: 12, InputHash: "input-hash", ResumeJobType: "recipe"}
					job.BlockingWaitSummary = &jobdb.DependencyWaitSummary{WaitID: "wait-id", Deadline: &deadline}
					if archived {
						job.Status = jobdb.JobStatusCompleted
						job.ArchivedAt = &deadline
						job.CompletionStatus = "future_outcome"
						if i%2 == 0 {
							job.CompletionDetail = "  failed <script> & 雪\n\tsecond line\n"
						}
					}
				}
				engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: jobs, NextPageToken: "cursor+unchanged/="}}}
				w := httptest.NewRecorder()
				req := mux.SetURLVars(httptest.NewRequest("GET", "/jobs?status=all&pageSize=200", nil), map[string]string{"projectId": "tenant"})
				(&Server{engine: engine}).handleListJobs(w, req)
				if w.Code != 200 || len(engine.requests) != 1 {
					t.Fatalf("status=%d calls=%d body=%s", w.Code, len(engine.requests), w.Body)
				}
				var page recipejob.ListRecipeJobsResponse
				if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if len(page.Jobs) != count || page.NextPageToken != "cursor+unchanged/=" {
					t.Fatalf("bad page: %+v", page)
				}
				for i, got := range page.Jobs {
					want := jobs[i]
					if got.CompletionDetail != want.CompletionDetail || got.CompletionStatus != want.CompletionStatus || !bytes.Equal(got.ClientPayload, want.ClientPayload) || got.ClientPayloadRevision != want.ClientPayloadRevision || !reflect.DeepEqual(got.TaskWait, want.ExecutionState.TaskWait) || !reflect.DeepEqual(got.WaitFor, want.WaitFor) || !reflect.DeepEqual(got.BlockingWaitSummary, want.BlockingWaitSummary) {
						t.Fatalf("stored fields changed on row %d: %+v", i, got)
					}
					if got.BlockingWait != nil || len(got.OtherGoalIDs) != 0 {
						t.Fatal("list acquired inspection fields")
					}
				}
			})
		}
	}
}

func TestListExecutionUsesC2JInterpretation(t *testing.T) {
	for _, tc := range []struct{ name, metadata, payload, status, source string }{
		{"absent", `{}`, ``, "unresolved", "absent"},
		{"submission", `{"execution":{"schema_version":3,"recipe_resolution":"resolved","effective":{"resources":{"cpu":"1"}}}}`, ``, "specified", "submission"},
		{"empty submission", `{"execution":{"schema_version":3,"recipe_resolution":"resolved","effective":{}}}`, ``, "unspecified", "submission"},
		{"published overrides", `{"execution":{"schema_version":3,"recipe_resolution":"resolved","effective":{"resources":{"cpu":"1"}}}}`, `{"c2j":{"execution":{"schema_version":3,"recipe_resolution":"resolved","effective":{"resources":{"cpu":"8"}}}}}`, "specified", "yield"},
		{"invalid", `{}`, `[]`, "malformed", "yield"},
		{"unsupported", `{}`, `{"c2j":{"execution":{"schema_version":999}}}`, "unsupported", "yield"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobs := compactJobs(1)
			jobs[0].Metadata, jobs[0].ClientPayload = json.RawMessage(tc.metadata), json.RawMessage(tc.payload)
			jobs[0].NextRoute = nil
			engine := &compactListEngine{pages: map[string]jobdb.ListJobsResponse{"": {Jobs: jobs}}}
			w := httptest.NewRecorder()
			(&Server{engine: engine}).handleListJobs(w, mux.SetURLVars(httptest.NewRequest("GET", "/jobs?status=all", nil), map[string]string{"projectId": "tenant"}))
			var page recipejob.ListRecipeJobsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || len(page.Jobs) != 1 || len(engine.requests) != 1 {
				t.Fatalf("response: %s", w.Body)
			}
			view := page.Jobs[0].Execution
			if view.Status != tc.status || view.Source != tc.source {
				t.Fatalf("view: %+v", view)
			}
			if tc.name == "published overrides" && (view.Demand == nil || *view.Demand.Effective.Resources.CPU != "8") {
				t.Fatalf("wrong demand: %+v", view)
			}
			if (tc.status == "malformed" || tc.status == "unsupported") && view.Diagnostic == "" {
				t.Fatal("lost diagnostic")
			}
		})
	}
}
