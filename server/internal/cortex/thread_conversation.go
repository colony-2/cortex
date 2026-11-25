package cortex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/input/openapi"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type threadObservationKey struct{}

// A cursor is issued only for a complete, job-scoped thread. Keep just the
// next ordinal and row fingerprints. The browser owns the actual messages.
type threadCursor struct {
	authority   jobdb.CellAuthority
	key         jobdb.JobKey
	token       string
	nextOrdinal int64
	rows        [][sha256.Size]byte
}

type threadCursorCache struct {
	mu     sync.Mutex
	recent [128]*threadCursor
	next   int
}

func (c *threadCursorCache) find(key jobdb.JobKey, token string) *threadCursor {
	if token == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, prior := range c.recent {
		if prior != nil && prior.key == key && prior.token == token {
			return prior
		}
	}
	return nil
}

func (c *threadCursorCache) remember(cursor *threadCursor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recent[c.next] = cursor
	c.next = (c.next + 1) % len(c.recent)
}

// The recipe visibility check and the run projection use the same exact job
// summary and detail throughout a single HTTP request.
type threadObservation struct {
	reader interface {
		recipejob.Lister
		GetJob(context.Context, jobdb.JobKey) (jobdb.JobInfo, error)
	}
	prior   *threadCursor
	key     jobdb.JobKey
	summary *jobdb.ListJobsResponse
	info    *jobdb.JobInfo
	job     recipejob.RecipeJob
}

func (t *threadObservation) ListJobs(ctx context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	if len(req.JobKeys) != 1 || req.JobKeys[0] != t.key {
		return jobdb.ListJobsResponse{}, fmt.Errorf("unexpected thread job lookup")
	}
	if t.summary == nil {
		req.IncludeHidden = true
		page, err := t.reader.ListJobs(ctx, req)
		if err != nil {
			return page, err
		}
		t.summary = &page
	}
	return *t.summary, nil
}
func (t *threadObservation) GetJob(ctx context.Context, key jobdb.JobKey) (jobdb.JobInfo, error) {
	if key != t.key {
		return jobdb.JobInfo{}, fmt.Errorf("unexpected thread job lookup")
	}
	if t.info == nil {
		info, err := t.reader.GetJob(ctx, key)
		if err != nil {
			return info, err
		}
		t.info = &info
	}
	return *t.info, nil
}

func (s *Server) handleThreadConversation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	key := jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}
	prior := s.threadCursors.find(key, r.URL.Query().Get("since_snapshot"))
	authority, _ := jobdb.CellAuthorityFromContext(r.Context())
	if prior != nil && !reflect.DeepEqual(prior.authority, authority) {
		prior = nil
	}
	if reader, ok := s.chapters.(chapterLookup); ok && prior != nil {
		_, err := reader.GetChapter(r.Context(), jobdb.ChapterRef{JobKey: key, Ordinal: prior.nextOrdinal})
		if errors.Is(err, jobdb.ErrChapterNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	// Ignore paging and selection parameters on the complete thread view.
	u := *r.URL
	u.RawQuery = ""
	r = r.Clone(r.Context())
	r.URL = &u
	// The engine implements detail and list reads; the chapter reader is attached
	// when the history service projects the run.
	t := &threadObservation{key: key, reader: s.engine, prior: prior}
	job, err := recipejob.GetRecipeJob(r.Context(), t, recipejob.GetRecipeJobRequest{TenantID: key.TenantId, JobID: key.JobId})
	if err != nil {
		writeError(w, err)
		return
	}
	t.job = job
	s.handleConversation(w, r.WithContext(context.WithValue(r.Context(), threadObservationKey{}, t)))
}

// Embedded only in the complete thread response. A null input means that this
// observation has no matching live wait.
type threadInput struct {
	CurrentInput *input.UserInputDetails `json:"current_input"`
	EntryID      string                  `json:"current_input_entry_id,omitempty"`
}

func (t *threadObservation) pendingInput(page *input.JobInputStoryPage, inventory *conversationInventory) (*threadInput, error) {
	job := t.job
	wait := job.TaskWait
	if job.Status != jobdb.JobStatusReady || job.Store == jobdb.JobStoreArchived || job.NextRoute == nil || job.NextRoute.TaskType != "input:collect_user_input" || wait == nil || job.CancelRequested {
		return &threadInput{}, nil
	}
	if inventory.run == nil {
		return nil, input.ErrHistoryUnsupported
	}
	if wait.OutputOrdinal != inventory.nextOrdinal {
		return &threadInput{}, nil
	}
	for _, e := range page.Entries {
		if e.State != "pending" || e.AcceptedResponse.Present || !slices.Contains(e.Locator.CollectionOrdinals, wait.InputOrdinal) {
			continue
		}
		form, err := threadForm(e.FormFields)
		if err != nil {
			return nil, err
		}
		if fallback := e.FormFields["fallback"]; len(fallback) > 0 && string(fallback) != "null" {
			for _, record := range inventory.run.Execution {
				if record.Ordinal == wait.InputOrdinal && record.Record.Kind == "wait" && record.Record.AlternateAt != nil {
					form.FallbackAt = record.Record.AlternateAt
				}
			}
		}
		return &threadInput{EntryID: e.EntryID, CurrentInput: &input.UserInputDetails{JobId: t.key.JobId, Status: "pending", StartTime: job.CreatedAt, Form: form}}, nil
	}
	return &threadInput{}, nil
}

func threadForm(frozen map[string]json.RawMessage) (openapi.InputFormConfig, error) {
	var form openapi.InputFormConfig
	fields := maps.Clone(frozen)
	// The frozen form stores durations in nanoseconds and may contain empty
	// timestamp strings. The input API exposes seconds and optional dates.
	for _, name := range []string{"requested_at", "fallback_at"} {
		var at time.Time
		if json.Unmarshal(fields[name], &at) != nil || at.IsZero() {
			delete(fields, name)
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return form, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&form); err != nil {
		return form, err
	}
	if timeout := form.Timeout; timeout != nil {
		*timeout /= int(time.Second)
	}
	if form.Context != nil && *form.Context == (openapi.FormContext{}) {
		form.Context = nil
	}
	if form.Fields != nil {
		for i := range *form.Fields {
			field := &(*form.Fields)[i]
			if field.Validation != nil && *field.Validation == (openapi.FieldValidation{}) {
				field.Validation = nil
			}
		}
	}
	return form, nil
}
