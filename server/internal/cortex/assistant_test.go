package cortex

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

type assistantBoundaryFixture struct {
	calls    int
	caller   AssistantCaller
	failure  error
	download string
	filename string
	deadline bool
}

func (f *assistantBoundaryFixture) called(ctx context.Context, c AssistantCaller) error {
	f.calls++
	f.caller = c
	end, ok := ctx.Deadline()
	f.deadline = ok && time.Until(end) <= 20*time.Second
	return f.failure
}
func (f *assistantBoundaryFixture) Start(ctx context.Context, c AssistantCaller, in AssistantStart) (AssistantSession, error) {
	return AssistantSession{ID: in.RequestID, Source: in.Source}, f.called(ctx, c)
}
func (f *assistantBoundaryFixture) List(ctx context.Context, c AssistantCaller, _ AssistantPage) (AssistantSessions, error) {
	return AssistantSessions{Sessions: []AssistantSession{}}, f.called(ctx, c)
}
func (f *assistantBoundaryFixture) Get(ctx context.Context, c AssistantCaller, id string) (AssistantSession, error) {
	return AssistantSession{ID: id}, f.called(ctx, c)
}
func (f *assistantBoundaryFixture) Replay(ctx context.Context, c AssistantCaller, _ string, _ AssistantPage) (AssistantReplay, error) {
	return AssistantReplay{}, f.called(ctx, c)
}
func (f *assistantBoundaryFixture) Respond(ctx context.Context, c AssistantCaller, _ string, in AssistantSubmission) (AssistantReceipt, error) {
	return AssistantReceipt{RequestID: in.RequestID, SubmissionID: in.SubmissionID, Outcome: "accepted"}, f.called(ctx, c)
}
func (f *assistantBoundaryFixture) Reopen(ctx context.Context, c AssistantCaller, _ string) (AssistantSession, error) {
	return AssistantSession{}, f.called(ctx, c)
}
func (f *assistantBoundaryFixture) OpenArtifact(ctx context.Context, c AssistantCaller, _, _ string) (AssistantDownload, error) {
	return AssistantDownload{ReadCloser: io.NopCloser(strings.NewReader(f.download)), Filename: f.filename, Size: int64(len(f.download)), MIME: "text/html"}, f.called(ctx, c)
}

type assistantIdentityKey struct{}

func assistantBoundaryServer(f *assistantBoundaryFixture) (*Server, http.Handler) {
	a := &AssistantIntegration{
		Service: f, Ready: func(context.Context) bool { return true },
		ResolvePrincipal: func(ctx context.Context, _ *http.Request) (AssistantPrincipal, error) {
			p, _ := ctx.Value(assistantIdentityKey{}).(AssistantPrincipal)
			if p.ID == "" {
				return p, ErrAssistantDenied
			}
			return p, nil
		},
		AuthorizeTenant: func(_ context.Context, p AssistantPrincipal, tenant string) error {
			if (p.ID == "alice" && tenant == "a") || (p.ID == "bob" && tenant == "b") {
				return nil
			}
			return ErrAssistantDenied
		},
	}
	s := &Server{cfg: Config{Assistant: a}}
	r := mux.NewRouter()
	s.registerAssistantRoutes(r.PathPrefix("/api").Subrouter())
	return s, r
}
func assistantBoundaryRequest(h http.Handler, method, endpoint, body, principal string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://cortex.test"+endpoint, strings.NewReader(body))
	r.Header.Set("Origin", "http://cortex.test")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Cookie", "email=alice@example.com")
	r.Header.Set("X-User", "alice")
	r.Header.Set("X-Tenant-ID", "a")
	if principal != "" {
		r = r.WithContext(context.WithValue(r.Context(), assistantIdentityKey{}, AssistantPrincipal{ID: principal}))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const assistantStartJSON = `{"request_id":"frozen-start","submitted_at":"2026-11-09T00:00:00Z","prompt":"Discuss the evidence","source":{"kind":"global"}}`
const assistantRespondJSON = `{"request_id":"prompt-1","submission_id":"submission-1","submitted_at":"2026-11-09T00:00:00Z","command":"send","prompt":"Continue"}`

// CTX-001, CTX-002, CTX-003 (HTTP boundary only; durable source checks are separate).
func TestAssistantBoundaryAuthorization(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"GET", "capabilities", ""}, {"GET", "sessions", ""}, {"POST", "sessions", assistantStartJSON},
		{"GET", "sessions/s", ""}, {"GET", "sessions/s/replay", ""},
		{"POST", "sessions/s/respond", assistantRespondJSON}, {"POST", "sessions/s/reopen", "{}"},
		{"GET", "sessions/s/artifacts/citation", ""},
	}
	for _, route := range routes {
		t.Run(route.method+route.path, func(t *testing.T) {
			f := &assistantBoundaryFixture{}
			_, h := assistantBoundaryServer(f)
			for _, principal := range []string{"", "bob"} {
				w := assistantBoundaryRequest(h, route.method, "/api/projects/a/assistant/"+route.path, route.body, principal)
				if w.Code != 403 || f.calls != 0 {
					t.Fatalf("spoofed/foreign identity: %d calls=%d body=%s", w.Code, f.calls, w.Body)
				}
			}
			w := assistantBoundaryRequest(h, route.method, "/api/projects/a/assistant/"+route.path, route.body, "alice")
			if w.Code != 200 {
				t.Fatalf("authorized request: %d %s", w.Code, w.Body)
			}
			if route.path != "capabilities" && (f.caller.Principal.ID != "alice" || f.caller.TenantID != "a" || !f.deadline) {
				t.Fatalf("caller/deadline lost: %+v", f)
			}
		})
	}
}
func TestAssistantMissingComposition(t *testing.T) {
	for _, missing := range []string{"integration", "service", "ready", "not ready"} {
		t.Run(missing, func(t *testing.T) {
			f := &assistantBoundaryFixture{}
			s, h := assistantBoundaryServer(f)
			switch missing {
			case "integration":
				s.cfg.Assistant = nil
			case "service":
				s.cfg.Assistant.Service = nil
			case "ready":
				s.cfg.Assistant.Ready = nil
			case "not ready":
				s.cfg.Assistant.Ready = func(context.Context) bool { return false }
			}
			w := assistantBoundaryRequest(h, "GET", "/api/projects/a/assistant/capabilities", "", "alice")
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"available":false`) {
				t.Fatalf("capabilities: %d %s", w.Code, w.Body)
			}
			w = assistantBoundaryRequest(h, "POST", "/api/projects/a/assistant/sessions", assistantStartJSON, "alice")
			if w.Code != 503 || f.calls != 0 {
				t.Fatalf("unconfigured send: %d calls=%d", w.Code, f.calls)
			}
		})
	}
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects", nil))
	if w.Code != 200 {
		t.Fatalf("ordinary pages unavailable: %d", w.Code)
	}
}

// CTX-022 and CTX-025: hostile browser fields must not reach durable work.
func TestAssistantMutationBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint, body, origin, content string
		status                                int
	}{
		{"origin", "sessions", assistantStartJSON, "https://evil.test", "application/json", 403},
		{"missing origin", "sessions", assistantStartJSON, "", "application/json", 403},
		{"text body", "sessions", assistantStartJSON, "http://cortex.test", "text/plain", 400},
		{"large body", "sessions", `{"prompt":"` + strings.Repeat("x", assistantBodyLimit) + `"}`, "http://cortex.test", "application/json", 413},
		{"browser model override", "sessions", strings.TrimSuffix(assistantStartJSON, "}") + `,"model":"evil"}`, "http://cortex.test", "application/json", 400},
		{"goal action", "sessions/s/respond", strings.Replace(assistantRespondJSON, `"send"`, `"confirm"`, 1), "http://cortex.test", "application/json", 400},
		{"review action", "sessions/s/respond", strings.Replace(assistantRespondJSON, `"send"`, `"approve"`, 1), "http://cortex.test", "application/json", 400},
		{"arbitrary source", "sessions", strings.Replace(assistantStartJSON, `"global"`, `"https://evil.test"`, 1), "http://cortex.test", "application/json", 400},
		{"global job", "sessions", strings.Replace(assistantStartJSON, `"kind":"global"`, `"kind":"global","job_id":"secret"`, 1), "http://cortex.test", "application/json", 400},
		{"review exact identity", "sessions", strings.Replace(assistantStartJSON, `"kind":"global"`, `"kind":"review","job_id":"job"`, 1), "http://cortex.test", "application/json", 400},
		{"trailing JSON", "sessions", assistantStartJSON + ` {}`, "http://cortex.test", "application/json", 400},
		{"reopen fields", "sessions/s/reopen", `{"shell":"echo unsafe"}`, "http://cortex.test", "application/json", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &assistantBoundaryFixture{}
			s, h := assistantBoundaryServer(f)
			s.cfg.CORSOrigins = []string{"*"}
			r := httptest.NewRequest("POST", "http://cortex.test/api/projects/a/assistant/"+tc.endpoint, strings.NewReader(tc.body))
			r = r.WithContext(context.WithValue(r.Context(), assistantIdentityKey{}, AssistantPrincipal{ID: "alice"}))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", tc.content)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || f.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, f.calls, w.Body)
			}
		})
	}
}
func TestAssistantSafeErrorsAndDownloads(t *testing.T) {
	f := &assistantBoundaryFixture{failure: errors.New("synthetic-key-do-not-publish provider failure")}
	_, h := assistantBoundaryServer(f)
	w := assistantBoundaryRequest(h, "GET", "/api/projects/a/assistant/sessions/s", "", "alice")
	if w.Code != 503 || strings.Contains(w.Body.String(), "synthetic-key") {
		t.Fatalf("unsafe error: %s", w.Body)
	}
	f.failure = nil
	f.download = "<script>untrusted source</script>"
	f.filename = "../unsafe\r\nname.html"
	w = assistantBoundaryRequest(h, "GET", "/api/projects/a/assistant/sessions/s/artifacts/citation", "", "alice")
	if w.Code != 200 || w.Body.String() != f.download || w.Header().Get("Content-Type") != "application/octet-stream" || strings.ContainsAny(w.Header().Get("Content-Disposition"), "\r\n") || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("unsafe download: %v %s", w.Header(), w.Body)
	}
	// Authorization is checked again after the first successful read.
	w = assistantBoundaryRequest(h, "GET", "/api/projects/a/assistant/sessions/s/artifacts/citation", "", "bob")
	if w.Code != 403 || strings.Contains(w.Body.String(), "untrusted source") {
		t.Fatalf("revoked access: %d %s", w.Code, w.Body)
	}
}
func TestAssistantReservedSessionNamesRemainOpaque(t *testing.T) {
	for _, id := range []string{"capabilities", "respond", "replay", "reopen"} {
		f := &assistantBoundaryFixture{}
		_, h := assistantBoundaryServer(f)
		w := assistantBoundaryRequest(h, "GET", "/api/projects/a/assistant/sessions/"+id, "", "alice")
		if w.Code != 200 || f.calls != 1 || !strings.Contains(w.Body.String(), `"id":"`+id+`"`) {
			t.Fatalf("opaque session id: %d %s", w.Code, w.Body)
		}
	}
}

func TestAssistantOriginAndPageBounds(t *testing.T) {
	for _, tc := range []struct {
		name, origin, forwarded, site string
		allowed                       bool
	}{
		{"same origin", "http://cortex.test", "", "same-origin", true},
		{"explicit configured origin", "https://browser.test", "", "same-site", true},
		{"wildcard is not authority", "https://attacker.test", "", "", false},
		{"forwarding claim", "https://attacker.test", "attacker.test", "", false},
		{"cross-site claim", "http://cortex.test", "", "cross-site", false},
		{"origin path", "http://cortex.test/path", "", "", false},
		{"userinfo", "http://attacker@cortex.test", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &assistantBoundaryFixture{}
			s, h := assistantBoundaryServer(f)
			s.cfg.CORSOrigins = []string{"*", "https://browser.test"}
			r := httptest.NewRequest("POST", "http://cortex.test/api/projects/a/assistant/sessions", strings.NewReader(assistantStartJSON))
			r = r.WithContext(context.WithValue(r.Context(), assistantIdentityKey{}, AssistantPrincipal{ID: "alice"}))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Forwarded-Host", tc.forwarded)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if tc.allowed && (w.Code != 200 || f.calls != 1) || !tc.allowed && (w.Code != 403 || f.calls != 0) {
				t.Fatalf("origin result: %d calls=%d %s", w.Code, f.calls, w.Body)
			}
		})
	}
	for _, query := range []string{"?limit=0", "?limit=51", "?limit=invalid", "?cursor=" + strings.Repeat("x", 4097)} {
		f := &assistantBoundaryFixture{}
		_, h := assistantBoundaryServer(f)
		w := assistantBoundaryRequest(h, "GET", "/api/projects/a/assistant/sessions"+query, "", "alice")
		if w.Code != 400 || f.calls != 0 {
			t.Fatalf("unbounded page accepted: %d calls=%d", w.Code, f.calls)
		}
	}
}

// CTX-001: approved permission deferral must not require new identity callbacks.
func TestAssistantExistingTenantComposition(t *testing.T) {
	f := &assistantBoundaryFixture{}
	s, h := assistantBoundaryServer(f)
	s.cfg.Assistant.ResolvePrincipal = nil
	s.cfg.Assistant.AuthorizeTenant = nil
	w := assistantBoundaryRequest(h, "POST", "/api/projects/a/assistant/sessions", assistantStartJSON, "")
	if w.Code != 200 || f.calls != 1 || f.caller.TenantID != "a" || f.caller.Principal.ID != "cortex-discussion" {
		t.Fatalf("existing tenant composition: %d caller=%+v calls=%d", w.Code, f.caller, f.calls)
	}
}

type assistantLifecycleFixture struct {
	*assistantBoundaryFixture
	operation         AssistantLifecycleRequest
	action, sessionID string
}

func (f *assistantLifecycleFixture) Close(ctx context.Context, c AssistantCaller, id string, in AssistantLifecycleRequest) (AssistantSession, error) {
	f.operation, f.action, f.sessionID = in, "close", id
	return AssistantSession{ID: id, GoalID: id, JobID: in.JobID, Lifecycle: "goal", Status: "closed"}, f.called(ctx, c)
}
func (f *assistantLifecycleFixture) ReopenGoal(ctx context.Context, c AssistantCaller, id string, in AssistantLifecycleRequest) (AssistantSession, error) {
	f.operation, f.action, f.sessionID = in, "reopen", id
	return AssistantSession{ID: id, GoalID: id, JobID: "replacement", Lifecycle: "goal", Status: "waiting"}, f.called(ctx, c)
}
func TestAssistantGoalLifecycleBoundary(t *testing.T) {
	for _, action := range []string{"close", "reopen"} {
		t.Run(action, func(t *testing.T) {
			f := &assistantLifecycleFixture{assistantBoundaryFixture: &assistantBoundaryFixture{}}
			s, h := assistantBoundaryServer(f.assistantBoundaryFixture)
			s.cfg.Assistant.Service = f
			body := `{"request_id":"frozen-operation","submitted_at":"2026-11-12T00:00:00Z","job_id":"observed-job"}`
			endpoint := "/api/projects/a/assistant/sessions/goal/" + action
			for range 2 {
				w := assistantBoundaryRequest(h, "POST", endpoint, body, "alice")
				if w.Code != 200 || f.action != action || f.sessionID != "goal" || f.operation.JobID != "observed-job" || f.operation.RequestID != "frozen-operation" || f.operation.SubmittedAt.IsZero() {
					t.Fatalf("%d %s %+v", w.Code, w.Body, f)
				}
			}
			for _, invalid := range []string{`{"request_id":"op"}`, `{"job_id":"job"}`, `{"request_id":"op","submitted_at":"2026-11-12T00:00:00Z","job_id":"job","model":"override"}`} {
				before := f.calls
				w := assistantBoundaryRequest(h, "POST", endpoint, invalid, "alice")
				if w.Code != 400 || f.calls != before {
					t.Fatalf("invalid lifecycle: %d calls=%d", w.Code, f.calls)
				}
			}
		})
	}
}
func TestAssistantCloseNeedsLifecycleComposition(t *testing.T) {
	f := &assistantBoundaryFixture{}
	_, h := assistantBoundaryServer(f)
	w := assistantBoundaryRequest(h, "POST", "/api/projects/a/assistant/sessions/goal/close", `{"request_id":"op","submitted_at":"2026-11-12T00:00:00Z","job_id":"job"}`, "alice")
	if w.Code != 503 || f.calls != 0 {
		t.Fatalf("uncomposed close: %d calls=%d", w.Code, f.calls)
	}
}
