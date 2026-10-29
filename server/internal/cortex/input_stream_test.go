package cortex

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

type inputStreamEngine struct {
	jobworkflow.Engine
	snapshots chan []jobdb.JobSummary
	requests  chan jobdb.ListJobsRequest
}

func (e *inputStreamEngine) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	e.requests <- req
	select {
	case snapshot := <-e.snapshots:
		return jobdb.ListJobsResponse{Jobs: snapshot}, nil
	case <-ctx.Done():
		return jobdb.ListJobsResponse{}, ctx.Err()
	}
}
func streamSummary(revision int64) []jobdb.JobSummary {
	return []jobdb.JobSummary{{JobKey: jobdb.JobKey{TenantId: "tenant-a", JobId: "same-job"}, JobType: "recipe", Status: jobdb.JobStatusReady, ClientPayloadRevision: revision}}
}

func TestInputStreamTracksSequentialTasksAndRemoval(t *testing.T) {
	engine := &inputStreamEngine{
		snapshots: make(chan []jobdb.JobSummary, 4),
		requests:  make(chan jobdb.ListJobsRequest, 10),
	}
	engine.snapshots <- streamSummary(7)
	engine.snapshots <- streamSummary(7) // no duplicate event
	engine.snapshots <- streamSummary(9) // no empty snapshot between prompts
	engine.snapshots <- nil              // expiry/cancellation by another process
	s := &Server{engine: engine}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handleInputStream(w, mux.SetURLVars(r, map[string]string{"projectId": "tenant-a"}))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	next := func() (string, map[string]any) {
		t.Helper()
		var event string
		var data map[string]any
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" && event != "" {
				return event, data
			}
			if strings.HasPrefix(line, "event: ") {
				event = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data); err != nil {
					t.Fatal(err)
				}
			}
		}
		t.Fatalf("stream ended: %v", scanner.Err())
		return "", nil
	}
	if event, _ := next(); event != "connected" {
		t.Fatalf("first event = %s", event)
	}
	for _, ordinal := range []float64{7, 9} {
		event, data := next()
		if event != "inputs_changed" || len(data["inputs"].([]any)) != 1 || data["inputs"].([]any)[0].(map[string]any)["client_payload_revision"] != ordinal {
			t.Fatalf("expected pending task %v, got %s %v", ordinal, event, data)
		}
	}
	if event, data := next(); event != "inputs_changed" || len(data["inputs"].([]any)) != 0 {
		t.Fatalf("expected removed input, got %s %v", event, data)
	}
	for range 4 {
		req := <-engine.requests
		if !reflect.DeepEqual(req.TenantIds, []string{"tenant-a"}) || !reflect.DeepEqual(req.JobTasks, []jobdb.JobTaskFilter{{JobType: "recipe", TaskType: "input:collect_user_input"}}) {
			t.Fatalf("incorrect task filter: %+v", req)
		}
	}
}
