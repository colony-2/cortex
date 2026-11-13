package cortex

// This file is the browser boundary. Durable policy and execution belong to the
// released assistant facade; an adapter supplies AssistantService at composition.
import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/mux"
)

var (
	ErrAssistantUnresolved  = errors.New("assistant prepared operation lost")
	ErrAssistantDenied      = errors.New("assistant access denied")
	ErrAssistantConflict    = errors.New("assistant state changed")
	ErrAssistantUnavailable = errors.New("assistant unavailable")
	ErrAssistantInvalid     = errors.New("invalid assistant request")
)

// Optional deployment identity hooks can impose additional existing controls.
// The assistant does not introduce a new identity or permission system.
type AssistantPrincipal struct{ ID string }
type AssistantCaller struct {
	Principal AssistantPrincipal
	TenantID  string
}
type AssistantIntegration struct {
	Service          AssistantService
	Generation       string
	ResolvePrincipal func(context.Context, *http.Request) (AssistantPrincipal, error)
	AuthorizeTenant  func(context.Context, AssistantPrincipal, string) error
	// Ready is true only after trusted model bindings and ordinary recipe
	// execution are composed. Missing configuration must return false.
	Ready func(context.Context) bool
}
type AssistantSource struct {
	Kind      string `json:"kind"`
	JobID     string `json:"job_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}
type AssistantStart struct {
	Generation  string          `json:"generation,omitempty"`
	RequestID   string          `json:"request_id"`
	SubmittedAt time.Time       `json:"submitted_at"`
	Prompt      string          `json:"prompt"`
	Source      AssistantSource `json:"source"`
}
type AssistantSubmission struct {
	RequestID    string    `json:"request_id"`
	SubmissionID string    `json:"submission_id"`
	SubmittedAt  time.Time `json:"submitted_at"`
	Command      string    `json:"command"`
	Prompt       string    `json:"prompt,omitempty"`
}

// Lifecycle requests freeze both the logical operation and the observed job.
// A delayed close must never be retargeted to a later job in the same goal.
type AssistantLifecycleRequest struct {
	Generation  string    `json:"generation,omitempty"`
	RequestID   string    `json:"request_id"`
	SubmittedAt time.Time `json:"submitted_at"`
	JobID       string    `json:"job_id"`
}

func (r AssistantLifecycleRequest) valid() bool {
	return assistantToken(r.RequestID) && !r.SubmittedAt.IsZero() && assistantToken(r.JobID)
}

type AssistantArtifact struct {
	Origin    *AssistantOrigin `json:"origin,omitempty"`
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	MIME      string           `json:"mime"`
	Size      int64            `json:"size"`
	Available bool             `json:"available"`
}
type AssistantCoverage struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Outcome       string `json:"outcome"`
	Detail        string `json:"detail,omitempty"`
	ProducerJobID string `json:"producer_job_id,omitempty"`
	ProducerTask  string `json:"producer_task,omitempty"`
	ArtifactID    string `json:"artifact_id,omitempty"`
}
type AssistantEntry struct {
	Receipt   *AssistantReceipt `json:"receipt,omitempty"`
	Origin    *AssistantOrigin  `json:"origin,omitempty"`
	Citations []string          `json:"citations,omitempty"`
	// Key is derived from the durable task origin, never an array offset.
	Key       string              `json:"key"`
	Role      string              `json:"role"`
	Status    string              `json:"status"`
	Text      string              `json:"text,omitempty"`
	Artifacts []AssistantArtifact `json:"artifacts,omitempty"`
}
type AssistantOrigin struct {
	JobID      string   `json:"job_id"`
	Task       int64    `json:"task"`
	Attempt    int      `json:"attempt,omitempty"`
	Invocation string   `json:"invocation,omitempty"`
	Path       []string `json:"path,omitempty"`
}
type AssistantReplay struct {
	Entries   []AssistantEntry `json:"entries"`
	Cursor    string           `json:"cursor,omitempty"`
	HasMore   bool             `json:"has_more"`
	Complete  bool             `json:"complete"`
	Watermark string           `json:"watermark"`
}
type AssistantSession struct {
	GoalID           string              `json:"goal_id,omitempty"`
	Lifecycle        string              `json:"lifecycle,omitempty"`
	ID               string              `json:"id"`
	JobID            string              `json:"job_id"`
	Status           string              `json:"status"`
	Source           AssistantSource     `json:"source"`
	CapturedAt       *time.Time          `json:"captured_at,omitempty"`
	Coverage         []AssistantCoverage `json:"coverage,omitempty"`
	CurrentRequestID string              `json:"current_request_id,omitempty"`
	Replay           AssistantReplay     `json:"replay"`
}
type AssistantSessions struct {
	Sessions []AssistantSession `json:"sessions"`
	Cursor   string             `json:"cursor,omitempty"`
}
type AssistantReceipt struct {
	RequestID    string     `json:"request_id"`
	SubmissionID string     `json:"submission_id"`
	Outcome      string     `json:"outcome"`
	SubmittedAt  *time.Time `json:"submitted_at,omitempty"`
}
type AssistantPage struct {
	Cursor string
	Limit  int
}
type AssistantDownload struct {
	io.ReadCloser
	Filename string
	MIME     string
	Size     int64
}

// Implementations must preserve tenant/session and original producer integrity.
// Only opaque recorded citation IDs may be opened.
// These methods deliberately expose no goal, review decision or arbitrary action.
type AssistantService interface {
	Start(context.Context, AssistantCaller, AssistantStart) (AssistantSession, error)
	List(context.Context, AssistantCaller, AssistantPage) (AssistantSessions, error)
	Get(context.Context, AssistantCaller, string) (AssistantSession, error)
	Replay(context.Context, AssistantCaller, string, AssistantPage) (AssistantReplay, error)
	Respond(context.Context, AssistantCaller, string, AssistantSubmission) (AssistantReceipt, error)
	Reopen(context.Context, AssistantCaller, string) (AssistantSession, error)
	OpenArtifact(context.Context, AssistantCaller, string, string) (AssistantDownload, error)
}

// Goal lifecycle is explicit so legacy sessions keep their released semantics.
// The adapter owns cancellation, prepared submission and retry reconciliation.
type AssistantLifecycleService interface {
	Close(context.Context, AssistantCaller, string, AssistantLifecycleRequest) (AssistantSession, error)
	ReopenGoal(context.Context, AssistantCaller, string, AssistantLifecycleRequest) (AssistantSession, error)
}

const assistantBodyLimit = 40 << 10

func (s *Server) registerAssistantRoutes(api *mux.Router) {
	base := "/projects/{projectId}/assistant"
	for _, route := range []struct{ path, method string }{
		{"/capabilities", "GET"}, {"/sessions", "GET"}, {"/sessions", "POST"},
		{"/sessions/{sessionId}", "GET"}, {"/sessions/{sessionId}/replay", "GET"},
		{"/sessions/{sessionId}/respond", "POST"}, {"/sessions/{sessionId}/close", "POST"}, {"/sessions/{sessionId}/reopen", "POST"},
		{"/sessions/{sessionId}/artifacts/{artifactId}", "GET"},
	} {
		api.HandleFunc(base+route.path, s.handleAssistant).Methods(route.method, http.MethodOptions).Name(route.path)
	}
}

func (s *Server) assistantCaller(ctx context.Context, r *http.Request) (AssistantCaller, bool, error) {
	a := s.cfg.Assistant
	if a == nil {
		return AssistantCaller{}, false, nil
	}
	tenant := mux.Vars(r)["projectId"]
	if !assistantToken(tenant) {
		return AssistantCaller{}, false, ErrAssistantInvalid
	}
	// Conversations use the existing tenant runtime. Identity isolation is
	// deferred; the fixed actor is server-owned, never taken from browser headers.
	p := AssistantPrincipal{ID: "cortex-discussion"}
	if a.ResolvePrincipal != nil {
		var err error
		p, err = a.ResolvePrincipal(ctx, r)
		if err != nil || !assistantToken(p.ID) {
			return AssistantCaller{}, false, ErrAssistantDenied
		}
	}
	if a.AuthorizeTenant != nil && a.AuthorizeTenant(ctx, p, tenant) != nil {
		return AssistantCaller{}, false, ErrAssistantDenied
	}
	return AssistantCaller{p, tenant}, a.Service != nil && a.Ready != nil && a.Ready(ctx), nil
}

func (s *Server) handleAssistant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	caller, ready, err := s.assistantCaller(ctx, r)
	if err != nil {
		assistantError(w, err)
		return
	}
	routeName := mux.CurrentRoute(r).GetName()
	if routeName == "/capabilities" {
		writeJSON(w, http.StatusOK, struct {
			Available  bool   `json:"available"`
			Generation string `json:"generation,omitempty"`
		}{ready, func() string {
			if ready {
				return s.cfg.Assistant.Generation
			}
			return ""
		}()})
		return
	}
	if !ready {
		assistantError(w, ErrAssistantUnavailable)
		return
	}
	vars := mux.Vars(r)
	id := vars["sessionId"]
	if id != "" && !assistantToken(id) {
		assistantError(w, ErrAssistantInvalid)
		return
	}
	if r.Method == http.MethodPost && !s.assistantOrigin(r) {
		assistantError(w, ErrAssistantDenied)
		return
	}
	service := s.cfg.Assistant.Service
	var result any
	switch {
	case vars["artifactId"] != "":
		aid := vars["artifactId"]
		if !assistantToken(aid) {
			assistantError(w, ErrAssistantInvalid)
			return
		}
		var download AssistantDownload
		download, err = service.OpenArtifact(ctx, caller, id, aid)
		if err == nil {
			if download.ReadCloser == nil {
				assistantError(w, ErrAssistantUnavailable)
				return
			}
			defer download.Close()
			if download.Size < 0 || download.Size > 8<<20 {
				assistantError(w, ErrAssistantUnavailable)
				return
			}
			// Buffer before headers so expired/truncated bytes cannot look successful.
			b, readErr := io.ReadAll(io.LimitReader(download, download.Size+1))
			if readErr != nil || int64(len(b)) != download.Size {
				assistantError(w, ErrAssistantUnavailable)
				return
			}
			name := path.Base(strings.ReplaceAll(download.Filename, "\\", "/"))
			name = strings.Map(func(r rune) rune {
				if r < 32 || r == 127 {
					return -1
				}
				return r
			}, name)
			if name == "" || name == "." {
				name = "source"
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			_, _ = w.Write(b)
			return
		}
	case routeName == "/sessions/{sessionId}/respond":
		var in AssistantSubmission
		if !assistantDecode(w, r, &in) {
			return
		}
		if !assistantToken(in.RequestID) || !assistantToken(in.SubmissionID) || in.SubmittedAt.IsZero() ||
			(in.Command != "send" && in.Command != "finish") ||
			(in.Command == "send" && !assistantPrompt(in.Prompt)) || (in.Command == "finish" && in.Prompt != "") {
			assistantError(w, ErrAssistantInvalid)
			return
		}
		result, err = service.Respond(ctx, caller, id, in)
	case routeName == "/sessions/{sessionId}/close" || routeName == "/sessions/{sessionId}/reopen":
		var in AssistantLifecycleRequest
		if !assistantDecode(w, r, &in) {
			return
		}
		if routeName == "/sessions/{sessionId}/reopen" && in == (AssistantLifecycleRequest{}) {
			// Explicit legacy route; a goal adapter must reject this for goal sessions.
			result, err = service.Reopen(ctx, caller, id)
			break
		}
		if !in.valid() {
			assistantError(w, ErrAssistantInvalid)
			return
		}
		lifecycle, ok := service.(AssistantLifecycleService)
		if !ok {
			assistantError(w, ErrAssistantUnavailable)
			return
		}
		if routeName == "/sessions/{sessionId}/close" {
			result, err = lifecycle.Close(ctx, caller, id, in)
		} else {
			result, err = lifecycle.ReopenGoal(ctx, caller, id, in)
		}
	case routeName == "/sessions/{sessionId}/replay":
		page, valid := assistantPage(r)
		if !valid {
			assistantError(w, ErrAssistantInvalid)
			return
		}
		result, err = service.Replay(ctx, caller, id, page)
	case id != "":
		result, err = service.Get(ctx, caller, id)
	case r.Method == http.MethodGet:
		page, valid := assistantPage(r)
		if !valid {
			assistantError(w, ErrAssistantInvalid)
			return
		}
		result, err = service.List(ctx, caller, page)
	default:
		var in AssistantStart
		if !assistantDecode(w, r, &in) {
			return
		}
		if !assistantToken(in.RequestID) || in.SubmittedAt.IsZero() || !assistantPrompt(in.Prompt) || !in.Source.valid() {
			assistantError(w, ErrAssistantInvalid)
			return
		}
		result, err = service.Start(ctx, caller, in)
	}
	if err != nil {
		assistantError(w, err)
		return
	}
	// Keep the encoded envelope below the released facade's page budget.
	encoded, encodeErr := json.Marshal(result)
	if encodeErr != nil || len(encoded) > 4<<20 {
		assistantError(w, ErrAssistantUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}

func assistantToken(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 512 && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n")
}
func assistantPrompt(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 32<<10 && utf8.ValidString(s)
}
func (s AssistantSource) valid() bool {
	switch s.Kind {
	case "global":
		return s.JobID == "" && s.RequestID == ""
	case "story":
		return assistantToken(s.JobID) && s.RequestID == ""
	case "review":
		return assistantToken(s.JobID) && assistantToken(s.RequestID)
	}
	return false
}
func assistantPage(r *http.Request) (AssistantPage, bool) {
	q := r.URL.Query()
	p := AssistantPage{Cursor: q.Get("cursor"), Limit: 20}
	if len(p.Cursor) > 4096 {
		return p, false
	}
	if q.Has("limit") {
		n, e := strconv.Atoi(q.Get("limit"))
		if e != nil || n < 1 || n > 50 {
			return p, false
		}
		p.Limit = n
	}
	return p, true
}
func assistantDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		assistantError(w, ErrAssistantInvalid)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, assistantBodyLimit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err == nil {
		var tail any
		err = d.Decode(&tail)
		if err == io.EOF {
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "Assistant request is too large", Code: "limit"})
		return false
	}
	assistantError(w, ErrAssistantInvalid)
	return false
}
func (s *Server) assistantOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
		return false
	}
	raw := r.Header.Get("Origin")
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if raw == scheme+"://"+r.Host {
		return true
	}
	for _, origin := range s.cfg.CORSOrigins {
		if origin != "*" && raw == strings.TrimSpace(origin) {
			return true
		}
	}
	return false
}
func assistantError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusServiceUnavailable, "unavailable", "Assistant is unavailable; refresh to recover accepted messages"
	switch {
	case errors.Is(err, ErrAssistantUnresolved):
		status, code, message = http.StatusConflict, "unresolved", "This request may have succeeded. Automatic recovery is unavailable after the server restarted. Use conversation history to inspect recorded conversations."
	case errors.Is(err, ErrAssistantDenied):
		status, code, message = http.StatusForbidden, "forbidden", "Assistant access is unavailable"
	case errors.Is(err, ErrAssistantConflict):
		status, code, message = http.StatusConflict, "conflict", "Conversation changed; refresh before retrying"
	case errors.Is(err, ErrAssistantInvalid):
		status, code, message = http.StatusBadRequest, "invalid", "Invalid assistant request"
	}
	// Never reflect provider/runtime errors, source names or credentials.
	writeJSON(w, status, errorResponse{Error: message, Code: code})
}
