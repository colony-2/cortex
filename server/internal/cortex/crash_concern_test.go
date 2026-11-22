package cortex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/gorilla/mux"
)

type clearRecorder struct {
	calls []jobdb.ClearCrashConcernRequest
	err   error
}

func (c *clearRecorder) ClearCrashConcern(_ context.Context, req jobdb.ClearCrashConcernRequest) error {
	c.calls = append(c.calls, req)
	return c.err
}
func TestClearCrashConcernRequest(t *testing.T) {
	for _, tc := range []struct {
		body, query, id, method string
		want                    int
	}{
		{"{}", "", "selected", "POST", 204}, {"", "", "selected", "POST", 204},
		{"null", "", "selected", "POST", 400}, {"[]", "", "selected", "POST", 400}, {"{", "", "selected", "POST", 400},
		{"{} {}", "", "selected", "POST", 400}, {`{"reason":"override"}`, "", "selected", "POST", 400},
		{strings.Repeat(" ", 1025), "", "selected", "POST", 400}, {"{}", "?actor=human", "selected", "POST", 400},
		{"{}", "", "self", "POST", 400}, {"{}", "", "%20", "POST", 400},
		{"{}", "", "selected", "GET", 405},
	} {
		t.Run(tc.body+tc.query+tc.id+tc.method, func(t *testing.T) {
			e := &leaseEventsEngine{}
			c := &clearRecorder{}
			s := &Server{engine: e, crashConcern: c}
			router := mux.NewRouter()
			router.HandleFunc("/projects/{projectId}/jobs/{jobId}/clear", s.handleClearCrashConcern).Methods("POST")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tc.method, "/projects/tenant/jobs/"+tc.id+"/clear"+tc.query, strings.NewReader(tc.body)))
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if tc.want != 204 {
				if len(c.calls) != 0 {
					t.Fatal(c.calls)
				}
				return
			}
			if len(c.calls) != 1 || c.calls[0] != (jobdb.ClearCrashConcernRequest{JobKey: jobdb.JobKey{TenantId: "tenant", JobId: "selected"}, WorkerID: "cortex", Reason: "Crash concern cleared from Cortex"}) || w.Body.Len() != 0 {
				t.Fatal(c.calls, w.Body)
			}
			if len(e.visibility) != 1 || e.visibility[0].IncludeHidden || len(e.visibility[0].TenantIds) != 1 || e.visibility[0].TenantIds[0] != "tenant" || len(e.visibility[0].JobKeys) != 1 || e.visibility[0].JobKeys[0] != (jobdb.JobKey{TenantId: "tenant", JobId: "selected"}) {
				t.Fatal(e.visibility)
			}
		})
	}
}

func TestClearCrashConcernCORS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		origins []string
		origin  string
		allowed bool
	}{
		{name: "default origin", origin: "http://localhost:3000", allowed: true},
		{name: "configured origin", origins: []string{"https://cortex.example"}, origin: "https://cortex.example", allowed: true},
		{name: "unlisted origin", origins: []string{"https://cortex.example"}, origin: "https://unlisted.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t), CORSOrigins: tc.origins})
			if err != nil {
				t.Fatal(err)
			}
			e := &leaseEventsEngine{}
			c := &clearRecorder{}
			s.engine, s.crashConcern = e, c
			const path = "/api/projects/tenant/jobs/selected/clear-crash-concern"
			r := httptest.NewRequest(http.MethodOptions, path, nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Access-Control-Request-Method", http.MethodPost)
			r.Header.Set("Access-Control-Request-Headers", "content-type")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
				t.Fatalf("preflight: status %d, body %q", w.Code, w.Body.String())
			}
			if len(c.calls) != 0 || len(e.visibility) != 0 {
				t.Fatalf("preflight performed work: clears %v, lookups %v", c.calls, e.visibility)
			}
			if !tc.allowed {
				if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
					t.Fatalf("unlisted origin allowed: %q", got)
				}
				return
			}
			for header, want := range map[string]string{
				"Access-Control-Allow-Origin":      tc.origin,
				"Access-Control-Allow-Credentials": "true",
				"Vary":                             "Origin",
			} {
				if got := w.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}
			if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "POST") || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Content-Type") {
				t.Fatalf("JSON POST not allowed: %v", w.Header())
			}
			r = httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", "application/json")
			w = httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != tc.origin || w.Body.Len() != 0 {
				t.Fatalf("POST: status %d, headers %v, body %q", w.Code, w.Header(), w.Body.String())
			}
			if len(c.calls) != 1 || c.calls[0].JobKey != (jobdb.JobKey{TenantId: "tenant", JobId: "selected"}) || len(e.visibility) != 1 {
				t.Fatalf("POST: clears %v, lookups %v", c.calls, e.visibility)
			}
		})
	}
}

func TestClearCrashConcernErrors(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		err, visibility      error
		missing, unsupported bool
		want                 int
	}{
		{name: "invalid", err: jobdb.ErrInvalidCrashConcernRequest, want: 400},
		{name: "permission", err: jobdb.ErrLeasePermissionDenied, want: 403},
		{name: "missing", err: jobdb.ErrJobNotFound, want: 404},
		{name: "conflict", err: jobdb.ErrConflict, want: 409},
		{name: "transport", err: errors.New("connection lost"), want: 500},
		{name: "hidden", missing: true, want: 404}, {name: "denied read", visibility: jobdb.ErrLeasePermissionDenied, want: 403},
		{name: "unsupported", unsupported: true, want: 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &leaseEventsEngine{missing: tc.missing, visibilityError: tc.visibility}
			c := &clearRecorder{err: tc.err}
			s := &Server{engine: e, crashConcern: c}
			if tc.unsupported {
				s.crashConcern = nil
			}
			r := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
			r = mux.SetURLVars(r, map[string]string{"projectId": "tenant", "jobId": "selected"})
			w := httptest.NewRecorder()
			s.handleClearCrashConcern(w, r)
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if (tc.missing || tc.visibility != nil || tc.unsupported) && len(c.calls) != 0 {
				t.Fatal(c.calls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &clearRecorder{}
	s := &Server{crashConcern: c}
	r := mux.SetURLVars(httptest.NewRequest("POST", "/", nil).WithContext(ctx), map[string]string{"projectId": "tenant", "jobId": "selected"})
	s.handleClearCrashConcern(httptest.NewRecorder(), r)
	if len(c.calls) != 0 {
		t.Fatal(c.calls)
	}
}

func TestClearCrashConcernRemoteFailures(t *testing.T) {
	for _, status := range []int{400, 403, 404, 409, 501, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			count := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				if r.Method != "POST" || !strings.Contains(r.URL.Path, "selected") {
					t.Error(r.Method, r.URL.Path)
				}
				w.WriteHeader(status)
				fmt.Fprint(w, "upstream diagnostic")
			}))
			defer upstream.Close()
			runtime, err := remote.New(upstream.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			s := &Server{engine: &leaseEventsEngine{}, crashConcern: runtime}
			r := mux.SetURLVars(httptest.NewRequest("POST", "/", nil), map[string]string{"projectId": "tenant", "jobId": "selected"})
			w := httptest.NewRecorder()
			s.handleClearCrashConcern(w, r)
			want := status
			if status >= 500 {
				want = 500
			}
			if w.Code != want || count != 1 {
				t.Fatalf("%d %s calls %d", w.Code, w.Body, count)
			}
			if status >= 500 && (!strings.Contains(w.Body.String(), "upstream diagnostic") || strings.Contains(w.Body.String(), "unsupported_crash_concern")) {
				t.Fatal(w.Body)
			}
		})
	}
}

type mismatchedClearEngine struct{ *leaseEventsEngine }

func (e mismatchedClearEngine) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	page, err := e.leaseEventsEngine.ListJobs(ctx, req)
	if len(page.Jobs) > 0 {
		page.Jobs[0].JobKey.TenantId = "other"
	}
	return page, err
}
func TestClearCrashConcernExactVisibilityAndCancellation(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c := &clearRecorder{}
		e := &leaseEventsEngine{}
		s := &Server{engine: e, crashConcern: c}
		if mismatch {
			s.engine = mismatchedClearEngine{e}
		} else {
			e.afterVisibility = cancel
		}
		r := mux.SetURLVars(httptest.NewRequest("POST", "/", nil).WithContext(ctx), map[string]string{"projectId": "tenant", "jobId": "selected"})
		w := httptest.NewRecorder()
		s.handleClearCrashConcern(w, r)
		if len(c.calls) != 0 || w.Code == 204 {
			t.Fatal(w.Code, c.calls)
		}
		if mismatch && w.Code != 404 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, key := range []jobdb.JobKey{{TenantId: " ", JobId: "selected"}, {TenantId: "tenant", JobId: " "}} {
		c := &clearRecorder{}
		s := &Server{crashConcern: c}
		r := mux.SetURLVars(httptest.NewRequest("POST", "/", nil), map[string]string{"projectId": key.TenantId, "jobId": key.JobId})
		w := httptest.NewRecorder()
		s.handleClearCrashConcern(w, r)
		if w.Code != 400 || len(c.calls) != 0 {
			t.Fatal(w.Code, c.calls)
		}
	}
}
