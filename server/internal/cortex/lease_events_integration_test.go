package cortex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
)

func TestLeaseEventsHTTPRuntimeIntegration(t *testing.T) {
	for _, mode := range []string{"toy", "remote"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{WorkingDir: writeC2JConfig(t)}
			var server *Server
			var err error
			if mode == "toy" {
				server, err = NewUITestServer(cfg)
			} else {
				backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(toy.New())))
				defer backend.Close()
				cfg.JobDBURL = backend.URL
				server, err = NewRemoteServer(cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			api := httptest.NewServer(testjobdb.Handler(server))
			defer api.Close()
			ctx := testjobdb.Context(context.Background())
			runtime := server.leaseEvents
			for _, tenant := range []string{"tenant", "other"} {
				_, err := celltest.Submit(runtime, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: "same", Cell: "platform", CellRepo: "github.com/acme/platform", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(1)}})
				if err != nil {
					t.Fatal(err)
				}
				lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: tenant, JobId: "same"}, WorkerID: "worker-" + tenant, Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
				if err != nil || lease == nil {
					t.Fatalf("lease %v %v", lease, err)
				}
				defer lease.StopKeepAlive()
				if err := lease.KeepAlive(ctx); err != nil {
					t.Fatal(err)
				}
			}
			base := api.URL + "/api/projects/tenant/jobs/same/events"
			first := getJSON[jobdb.ListLeaseEventsResponse](t, base+"?pageSize=1")
			if len(first.Events) != 1 || first.Events[0].EventType != jobdb.LeaseEventRenewed || first.NextPageToken == "" {
				t.Fatalf("first page %+v", first)
			}
			second := getJSON[jobdb.ListLeaseEventsResponse](t, base+"?pageSize=1&pageToken="+url.QueryEscape(first.NextPageToken))
			if len(second.Events) != 1 || second.Events[0].EventType != jobdb.LeaseEventAcquired || second.NextPageToken != "" {
				t.Fatalf("second page %+v", second)
			}
			for _, event := range append(first.Events, second.Events...) {
				if event.JobKey != (jobdb.JobKey{TenantId: "tenant", JobId: "same"}) || event.WorkerID != "worker-tenant" {
					t.Fatalf("mixed tenant %+v", event)
				}
			}
			// Existing list/detail/story paths do not need the event service.
			server.leaseEvents = nil
			for _, path := range []string{"/api/projects/tenant/jobs?status=all", "/api/projects/tenant/jobs/same", "/api/projects/tenant/jobs/same/story"} {
				response, err := http.Get(api.URL + path)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != 200 {
					t.Fatalf("ordinary path %s: %d %s", path, response.StatusCode, body)
				}
			}
			response, err := http.Get(base)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 501 {
				t.Fatalf("missing runtime: %d", response.StatusCode)
			}
			server.leaseEvents = runtime
			if _, err := server.jobDeletion.DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: "tenant", JobIDs: []string{"same"}}); err != nil {
				t.Fatal(err)
			}
			hidden := getJSON[jobdb.ListLeaseEventsResponse](t, base+"?pageSize=1")
			if !reflect.DeepEqual(hidden, first) {
				t.Fatalf("hidden history changed: %+v", hidden)
			}
			response, err = http.Get(api.URL + "/api/projects/tenant/jobs/missing/events")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 404 {
				t.Fatalf("missing %d", response.StatusCode)
			}
		})
	}
}

func TestLeaseEventsRegisteredRoute(t *testing.T) {
	server, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	engine := &leaseEventsEngine{page: jobdb.ListLeaseEventsResponse{Events: []jobdb.LeaseEvent{}}}
	server.engine = engine
	server.leaseEvents = leaseEventsRuntime{engine: engine}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects/tenant/jobs/selected/events?pageToken=private-token", nil))
	if w.Code != 200 || len(engine.visibility) != 1 || len(engine.reads) != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var page jobdb.ListLeaseEventsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Events == nil {
		t.Fatal("lost canonical empty page")
	}
}
