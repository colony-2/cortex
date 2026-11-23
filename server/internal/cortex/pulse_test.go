package cortex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

func TestPulseReadsAndLogDownload(t *testing.T) {
	const token = "p.A+/=?"
	const provider = `{"name":"runner a","type":"reverse","discovery":"known","bases_complete":false,"bases":[{"name":"linux","capabilities":["gpu"]}],"health":{"state":"ready"}}`
	const inventory = `{"items":[` + provider + `]}`
	var checkpoints []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("received browser credentials")
		}
		switch r.URL.Path {
		case "/api/providers":
			_, _ = w.Write([]byte(inventory))
		case "/api/providers/runner a/capabilities":
			_, _ = w.Write([]byte(provider))
		case "/api/instances":
			if r.URL.Query().Get("launch_id") != "launch-1" {
				t.Error(r.URL)
			}
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{"items":[],"complete":false,"errors":[{"provider":"offline","error":"timeout"}]}`))
		case "/api/providers/runner a/launches/launch-1/logs":
			cursor := r.URL.Query().Get("page_token")
			checkpoints = append(checkpoints, cursor)
			if r.URL.Query().Get("page_size") != "100" {
				t.Error(r.URL)
			}
			switch cursor {
			case "":
				_, _ = w.Write([]byte(`{"records":[{"data":"AP8="}],"has_more":true,"next_page_token":"p.A+/=?"}`))
			case token:
				_, _ = w.Write([]byte(`{"records":[{"data":"Cg=="}],"has_more":false,"next_page_token":"p.end"}`))
			default:
				t.Error(cursor)
			}
		default:
			t.Error(r.URL)
		}
	}))
	defer upstream.Close()
	s := &Server{cfg: Config{PulseURL: upstream.URL + "/api/"}}
	router := mux.NewRouter()
	s.registerPulseRoutes(router)
	read := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", target, nil)
		req.Header.Set("Authorization", "test")
		req.Header.Set("Cookie", "test=true")
		router.ServeHTTP(w, req)
		return w
	}
	for path, body := range map[string]string{"/pulse/providers": inventory, "/pulse/providers/runner%20a/capabilities": provider} {
		if w := read(path); w.Code != 200 || w.Body.String() != body {
			t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := read("/pulse/instances?launch_id=launch-1")
	if w.Code != 502 || !json.Valid(w.Body.Bytes()) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = read("/pulse/providers/runner%20a/launches/launch-1/download")
	if w.Code != 200 || w.Body.String() != "\x00\xff\n" || !reflect.DeepEqual(checkpoints, []string{"", token}) || w.Header().Get("Content-Disposition") == "" {
		t.Fatal(w.Code, w.Body.Bytes(), checkpoints)
	}
	if got := normalizeConfig(Config{}).PulseURL; got != "http://localhost:8080" {
		t.Fatalf("default Pulse URL = %q", got)
	}
}

func TestRunnerHistoryQueriesAcquisitionsByTenantJobAndWorker(t *testing.T) {
	engine := &leaseEventsEngine{page: jobdb.ListLeaseEventsResponse{Events: []jobdb.LeaseEvent{}, NextPageToken: "next"}}
	s := &Server{leaseEvents: leaseEventsRuntime{engine: engine}}
	router := mux.NewRouter()
	s.registerPulseRoutes(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/projects/tenant/runner-launches?jobId=job&launchId=launch&pageToken=opaque%2B", nil))
	want := jobdb.ListLeaseEventsRequest{TenantID: "tenant", JobIDs: []string{"job"}, WorkerIDs: []string{"launch"}, EventTypes: []jobdb.LeaseEventType{jobdb.LeaseEventAcquired}, IncludeHidden: true, PageSize: 100, PageToken: "opaque+"}
	if w.Code != 200 || !reflect.DeepEqual(engine.reads, []jobdb.ListLeaseEventsRequest{want}) {
		t.Fatal(w.Code, engine.reads, w.Body.String())
	}
}
