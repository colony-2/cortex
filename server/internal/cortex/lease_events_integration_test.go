package cortex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

func TestLeaseEventsHTTPRuntimeIntegration(t *testing.T) {
	for _, mode := range []string{"toy", "remote", "sqlite", "sqlite-remote"} {
		t.Run(mode, func(t *testing.T) {
			runtime := retirementRuntime(t, mode)
			server := retirementServer(t, runtime, Config{WorkingDir: writeC2JConfig(t)})
			api := httptest.NewServer(testjobdb.Handler(server))
			defer api.Close()
			ctx := testjobdb.Context(context.Background())
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
				if err := lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: "success", Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"ok":true}`)}}}}}); err != nil {
					t.Fatal(err)
				}
			}
			base := api.URL + "/api/projects/tenant/jobs/same/events"
			expected, err := runtime.ListLeaseEvents(ctx, jobdb.ListLeaseEventsRequest{TenantID: "tenant", JobIDs: []string{"same"}, IncludeHidden: true, PageSize: 1})
			if err != nil {
				t.Fatal(err)
			}
			first := getJSON[jobdb.ListLeaseEventsResponse](t, base+"?pageSize=1")
			if !reflect.DeepEqual(first, expected) {
				t.Fatalf("event identity changed: %+v => %+v", expected, first)
			}
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
			archived, err := runtime.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"tenant"}, JobKeys: []jobdb.JobKey{{TenantId: "tenant", JobId: "same"}}, Stores: []jobdb.JobStore{jobdb.JobStoreArchived}, IncludeHidden: true, PageSize: 1})
			if err != nil || len(archived.Jobs) != 1 {
				t.Fatalf("hidden archive missing: %+v %v", archived, err)
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

// Real setup handles remain separate from the narrow reader exercised by Cortex.
func retirementRuntime(t *testing.T, mode string) jobdb.WorkflowRuntime {
	t.Helper()
	var runtime jobdb.WorkflowRuntime = toy.New()
	if mode == "sqlite" || mode == "sqlite-remote" {
		embedded, err := sqlite.StartEmbeddedRuntime(testjobdb.Context(context.Background()))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(embedded.Shutdown)
		runtime = embedded.Runtime
	}
	if mode == "remote" || mode == "sqlite-remote" {
		backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(runtime)))
		t.Cleanup(backend.Close)
		var err error
		runtime, err = remote.New(backend.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	return runtime
}
func retirementServer(t *testing.T, runtime jobdb.WorkflowRuntime, cfg Config) *Server {
	t.Helper()
	engine, err := jobworkflow.NewEngineBuilder().WithRuntime(runtime).WithWorkerTenantId("tenant").BuildEngine()
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(cfg, jobdbschema.WorkflowEngine{Engine: engine, Registry: runtime.(jobdb.JobSchemaRegistry)})
	if err != nil {
		t.Fatal(err)
	}
	server.chapters = runtime
	server.leaseEvents = runtime
	server.jobDeletion = runtime.(jobdb.JobDeletionRuntime)
	return server
}

// Count the actual native reads while keeping the owned event port read-only.
type leaseCountingEngine struct {
	jobworkflow.Engine
	mu       sync.Mutex
	requests []jobdb.ListJobsRequest
}

func (e *leaseCountingEngine) ListJobs(ctx context.Context, q jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	e.mu.Lock()
	e.requests = append(e.requests, q)
	e.mu.Unlock()
	return e.Engine.ListJobs(ctx, q)
}

type leaseCountingReader struct {
	reader   jobdb.LeaseEventReader
	mu       sync.Mutex
	requests []jobdb.ListLeaseEventsRequest
	empty    atomic.Bool
}

func (e *leaseCountingReader) ListLeaseEvents(ctx context.Context, q jobdb.ListLeaseEventsRequest) (jobdb.ListLeaseEventsResponse, error) {
	e.mu.Lock()
	e.requests = append(e.requests, q)
	e.mu.Unlock()
	page, err := e.reader.ListLeaseEvents(ctx, q)
	// Model a filtered retained page, preserving its native cursor. No runtime
	// mutation or new event is introduced; both HTTP transports remain real.
	if e.empty.Load() {
		page.Events = []jobdb.LeaseEvent{}
	}
	return page, err
}
func TestLeaseEventsNativePageBoundary(t *testing.T) {
	for _, mode := range []string{"sqlite", "sqlite-remote"} {
		t.Run(mode, func(t *testing.T) {
			ctx := testjobdb.Context(context.Background())
			native := retirementRuntime(t, "sqlite")
			var denied atomic.Bool
			var emptyPage atomic.Bool
			withAuthority := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if denied.Load() {
						r = r.WithContext(jobdb.WithCellAuthority(r.Context(), jobdb.CellAuthority{Principal: "cortex-tests", TenantID: "tenant", Permissions: map[string]bool{"jobs.list": true}, Cells: map[string]bool{}}))
					} else {
						r = r.WithContext(testjobdb.Context(r.Context()))
					}
					next.ServeHTTP(w, r)
				})
			}
			runtime := native
			if mode == "sqlite-remote" {
				nativeHandler := remote.NewServer(native)
				backend := httptest.NewServer(withAuthority(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !emptyPage.Load() || !strings.HasSuffix(r.URL.Path, "/lease-events/query") {
						nativeHandler.ServeHTTP(w, r)
						return
					}
					recorder := httptest.NewRecorder()
					nativeHandler.ServeHTTP(recorder, r)
					for name, values := range recorder.Header() {
						w.Header()[name] = values
					}
					if recorder.Code != 200 {
						w.WriteHeader(recorder.Code)
						_, _ = w.Write(recorder.Body.Bytes())
						return
					}
					var page jobdb.ListLeaseEventsResponse
					if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					page.Events = []jobdb.LeaseEvent{}
					writeJSON(w, 200, page)
				})))
				defer backend.Close()
				var err error
				runtime, err = remote.New(backend.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			handle, err := celltest.Submit(native, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", JobID: "selected", Cell: "platform", CellRepo: "github.com/acme/platform", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(1)}})
			if err != nil {
				t.Fatal(err)
			}
			lease, err := native.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: handle.JobKey, WorkerID: "native-history", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
			if err != nil || lease == nil {
				t.Fatalf("%v %v", lease, err)
			}
			defer lease.StopKeepAlive()
			if err := lease.KeepAlive(ctx); err != nil {
				t.Fatal(err)
			}
			s := retirementServer(t, runtime, Config{WorkingDir: writeC2JConfig(t)})
			engine := &leaseCountingEngine{Engine: s.engine}
			reader := &leaseCountingReader{reader: runtime}
			s.engine = engine
			s.leaseEvents = reader
			api := httptest.NewServer(withAuthority(s))
			defer api.Close()
			assertCounts := func(vis, events int) {
				t.Helper()
				engine.mu.Lock()
				defer engine.mu.Unlock()
				reader.mu.Lock()
				defer reader.mu.Unlock()
				if len(engine.requests) != vis || len(reader.requests) != events {
					t.Fatalf("reads visibility=%d events=%d; want %d/%d", len(engine.requests), len(reader.requests), vis, events)
				}
				for _, q := range engine.requests {
					if !q.IncludeHidden || q.PageSize != 1 || !reflect.DeepEqual(q.TenantIds, []string{"tenant"}) || !reflect.DeepEqual(q.JobKeys, []jobdb.JobKey{handle.JobKey}) || !reflect.DeepEqual(q.Stores, []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}) {
						t.Fatalf("visibility %+v", q)
					}
				}
			}
			base := api.URL + "/api/projects/tenant/jobs/selected/events?pageSize=1"
			if mode == "sqlite" {
				reader.empty.Store(true)
			} else {
				emptyPage.Store(true)
			}
			empty := getJSON[jobdb.ListLeaseEventsResponse](t, base)
			if len(empty.Events) != 0 || empty.NextPageToken == "" {
				t.Fatalf("empty continuation %+v", empty)
			}
			assertCounts(1, 1)
			reader.empty.Store(false)
			emptyPage.Store(false)
			next := getJSON[jobdb.ListLeaseEventsResponse](t, base+"&pageToken="+url.QueryEscape(empty.NextPageToken))
			if len(next.Events) != 1 || next.Events[0].EventType != jobdb.LeaseEventAcquired || next.Events[0].JobKey != handle.JobKey {
				t.Fatalf("continuation %+v", next)
			}
			assertCounts(2, 2)
			denied.Store(true)
			response, err := http.Get(base)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 403 && response.StatusCode != 404 {
				t.Fatalf("denied %d", response.StatusCode)
			}
			assertCounts(3, 2)
		})
	}
}
