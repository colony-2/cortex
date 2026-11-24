package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/input/openapi"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type threadObservationKey struct{}

// The recipe visibility check and the run projection use the same exact job
// summary and detail throughout a single HTTP request.
type threadObservation struct {
	reader interface {
		recipejob.Lister
		GetJob(context.Context, jobdb.JobKey) (jobdb.JobInfo, error)
	}
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
	// Ignore paging and selection parameters on the complete thread view.
	u := *r.URL
	u.RawQuery = ""
	r = r.Clone(r.Context())
	r.URL = &u
	// The engine implements detail and list reads; the chapter reader is attached
	// when the history service projects the run.
	t := &threadObservation{key: key, reader: s.engine}
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
