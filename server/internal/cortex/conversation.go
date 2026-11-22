package cortex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

func historyError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "history_unavailable"
	if upstreamStatus, upstreamCode, ok := jobdb.CellErrorStatus(err); ok {
		writeJSON(w, upstreamStatus, map[string]string{"error": err.Error(), "code": upstreamCode})
		return
	}
	switch {
	case errors.Is(err, input.ErrHistorySnapshotExpired):
		status, code = http.StatusGone, "history_snapshot_expired"
	case errors.Is(err, input.ErrHistoryInvalidCursor):
		status, code = http.StatusBadRequest, "history_invalid_cursor"
	case errors.Is(err, input.ErrHistoryEntryNotFound), errors.Is(err, input.ErrHistoryArtifactNotFound), errors.Is(err, jobdb.ErrJobNotFound):
		status, code = http.StatusNotFound, "history_not_found"
	case errors.Is(err, input.ErrHistoryUnsupported):
		status, code = http.StatusNotImplemented, "history_unsupported"
	case errors.Is(err, input.ErrArtifactUnavailable):
		status, code = http.StatusGone, "history_artifact_unavailable"
	}
	writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
}

// The history service builds its snapshot from a JobRunReader. Capture the
// same chapters on the first page so the prompt and input tab share its inventory.
type conversationInventory struct {
	jobdb.JobRunReader
	key         jobdb.JobKey
	run         *jobdb.GetJobRunResponse
	rows        *[]inputDocument
	nextOrdinal int64
}
type inventoryContextKey struct{}

func (r *conversationInventory) ListChapters(ctx context.Context, req jobdb.ListChaptersRequest) ([]jobdb.Chapter, error) {
	chapters, err := r.JobRunReader.ListChapters(ctx, req)
	if err == nil && req.JobKey == r.key {
		for _, ch := range chapters {
			r.nextOrdinal = max(r.nextOrdinal, ch.Ordinal+1)
		}
		if rows, inventoryErr := documentInventory(req.JobKey.JobId, chapters); inventoryErr == nil {
			r.rows = &rows
		}
	}
	return chapters, err
}

type conversationHistoryReader struct {
	input.HistoryReader
	runtime jobdb.JobRunReader
}

func (r conversationHistoryReader) GetJobRun(ctx context.Context, req jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	capture, _ := ctx.Value(inventoryContextKey{}).(*conversationInventory)
	if capture == nil || req.JobKey != capture.key {
		return r.HistoryReader.GetJobRun(ctx, req)
	}
	return capture.readRun(ctx, r.runtime, req)
}

func (capture *conversationInventory) readRun(ctx context.Context, runtime jobdb.JobRunReader, req jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	capture.JobRunReader = runtime
	run, err := jobdb.GetJobRun(ctx, capture, req)
	if err == nil {
		capture.run = &run
	}
	return run, err
}

// Give the native c2j projections the already observed run for this request.
type storyRunKey struct{}
type snapshotStoryEngine struct{ jobdbschema.WorkflowEngine }

func (e snapshotStoryEngine) GetJobRun(ctx context.Context, req jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	if run, ok := ctx.Value(storyRunKey{}).(*jobdb.GetJobRunResponse); ok && run.Job.JobKey == req.JobKey {
		return *run, nil
	}
	if capture, ok := ctx.Value(inventoryContextKey{}).(*conversationInventory); ok && capture.key == req.JobKey {
		if reader, ok := e.Registry.(jobdb.JobRunReader); ok {
			return capture.readRun(ctx, reader, req)
		}
	}
	return e.WorkflowEngine.GetJobRun(ctx, req)
}

func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.history == nil {
		historyError(w, input.ErrHistoryUnsupported)
		return
	}
	key := jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}
	if selected := r.URL.Query().Get("review_id"); selected != "" {
		s.handleRecordedReview(w, r, key, selected)
		return
	}
	size := 50
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		var err error
		size, err = strconv.Atoi(raw)
		if err != nil || size < 1 || size > 500 {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("page_size must be between 1 and 500"))
			return
		}
	}
	ctx := r.Context()
	inventory := &conversationInventory{key: jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}}
	if r.URL.Query().Get("page_token") == "" {
		ctx = context.WithValue(ctx, inventoryContextKey{}, inventory)
	}
	page, err := s.history.ListJobInputStory(ctx, input.ListJobInputStoryRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], PageSize: size, PageToken: r.URL.Query().Get("page_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	if inventory.nextOrdinal > 0 {
		if _, err := s.indexReviewPages(ctx, key, inventory.nextOrdinal, page, ""); err != nil {
			s.logger.Warn("Could not index review chapters", "error", err)
		}
	}
	// Use the exact input page observation for publication and ordering data.
	// Older HistoryService implementations remain usable with explicit coverage.
	type conversationPage struct {
		InputArtifacts *[]inputDocument     `json:"input_artifacts,omitempty"`
		Activity       *conversationSection `json:"activity,omitempty"`
		FinalDocuments *conversationSection `json:"final_documents,omitempty"`
		*input.JobInputStoryPage
		Merges              []input.HistoryMergeEntry `json:"merges"`
		Events              []input.HistoryEvent      `json:"events"`
		PublicationCoverage string                    `json:"publication_coverage"`
	}
	result := conversationPage{InputArtifacts: inventory.rows, JobInputStoryPage: page, Merges: []input.HistoryMergeEntry{}, Events: []input.HistoryEvent{}, PublicationCoverage: "recorded"}
	if r.URL.Query().Get("include_sections") != "false" {
		activity, documents := s.conversationSections(r, inventory)
		result.Activity, result.FinalDocuments = &activity, &documents
	}
	snapshot, err := input.GetJobHistorySnapshot(r.Context(), s.history, input.GetJobHistorySnapshotRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], SnapshotToken: page.SnapshotToken})
	if errors.Is(err, input.ErrHistoryUnsupported) {
		result.PublicationCoverage = "unavailable"
	} else if err != nil {
		historyError(w, err)
		return
	} else {
		result.Events = snapshot.Events
		for _, merge := range snapshot.Merges {
			if merge.JobID != page.JobID || merge.TenantID != page.TenantID {
				continue
			}
			if merge.EvidenceAvailability == "recorded" && (merge.PublicationDisposition == "published" || merge.PublicationDisposition == "already_present") {
				result.Merges = append(result.Merges, merge)
			} else if merge.EvidenceAvailability != "recorded" {
				result.PublicationCoverage = "partial"
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleConversationEntry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.history == nil {
		historyError(w, input.ErrHistoryUnsupported)
		return
	}
	entry, err := s.history.GetJobInputStoryEntry(r.Context(), input.GetJobInputStoryEntryRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], EntryID: r.URL.Query().Get("entry_id"), SnapshotToken: r.URL.Query().Get("snapshot_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// Legacy manifest readers open lazy artifacts outside the request context.
func (s *Server) checkProducerVisibility(ctx context.Context, key jobdb.JobKey) error {
	page, err := s.engine.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{key.TenantId}, JobKeys: []jobdb.JobKey{key}, IncludeHidden: true, Stores: []jobdb.JobStore{jobdb.JobStoreActive, jobdb.JobStoreArchived}, PageSize: 1})
	if err != nil {
		return err
	}
	if len(page.Jobs) == 1 && page.Jobs[0].JobKey == key {
		return nil
	}
	return jobdb.ErrJobNotFound
}

func (s *Server) handleConversationArtifact(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.history == nil {
		historyError(w, input.ErrHistoryUnsupported)
		return
	}
	// The history resolver validates membership and resolves original producer
	// coordinates through the authorized engine; no arbitrary reference is accepted.
	// Recheck original producer access in the request context even for a cached
	// snapshot. Membership tokens identify bytes; they never grant read authority.
	detail, err := s.history.GetJobInputStoryEntry(r.Context(), input.GetJobInputStoryEntryRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], EntryID: r.URL.Query().Get("entry_id"), SnapshotToken: r.URL.Query().Get("snapshot_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	found := false
	for _, manifest := range [][]input.HistoryArtifactMember{detail.RequestManifest, detail.ResponseManifest} {
		for _, member := range manifest {
			if member.ArtifactID != r.URL.Query().Get("artifact_id") {
				continue
			}
			found = true
			if member.Ref.Kind == artifacts.RefKindExternal {
				historyError(w, input.ErrArtifactUnavailable)
				return
			}
			if key, stored := member.Ref.StoredKey(); stored {
				if s.engine == nil {
					historyError(w, input.ErrHistoryUnsupported)
					return
				}
				err = s.checkProducerVisibility(r.Context(), jobdb.JobKey{TenantId: s.tenantID(r), JobId: key.JobId})
				if err != nil {
					historyError(w, err)
					return
				}
			}
		}
	}
	if !found {
		historyError(w, input.ErrHistoryArtifactNotFound)
		return
	}
	artifact, err := s.history.OpenJobInputStoryArtifact(r.Context(), input.OpenJobInputStoryArtifactRequest{TenantID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], EntryID: r.URL.Query().Get("entry_id"), ArtifactID: r.URL.Query().Get("artifact_id"), SnapshotToken: r.URL.Query().Get("snapshot_token")})
	if err != nil {
		historyError(w, err)
		return
	}
	defer artifact.Close()
	// Buffer before sending headers so missing/corrupt retained bytes can still
	// produce an explicit error instead of a successful truncated document.
	data, err := io.ReadAll(io.LimitReader(artifact, 32<<20+1))
	if err != nil {
		historyError(w, err)
		return
	}
	if len(data) > 32<<20 {
		writeHTTPError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("historical attachment exceeds 32 MiB download limit"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(artifact.Name))
	w.Write(data)
}

// Mutable sections are observations separate from the immutable history snapshot.
type conversationSection struct {
	AttemptOrdinal *int64    `json:"attempt_ordinal,omitempty"`
	Availability   string    `json:"availability"`
	Value          any       `json:"value,omitempty"`
	Diagnostic     string    `json:"diagnostic,omitempty"`
	ObservedAt     time.Time `json:"observed_at"`
}

func (s *Server) conversationSections(r *http.Request, inventory *conversationInventory) (conversationSection, conversationSection) {
	unavailable := conversationSection{Availability: "unavailable", Diagnostic: "Current observation unavailable", ObservedAt: time.Now().UTC()}
	if s.engine == nil {
		return unavailable, unavailable
	}
	ctx := r.Context()
	key := jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}
	readJob := func() (recipejob.RecipeJob, error) {
		return recipejob.GetRecipeJob(ctx, archivedJobReader{s.engine}, recipejob.GetRecipeJobRequest{TenantID: key.TenantId, JobID: key.JobId})
	}
	before, err := readJob()
	if err != nil {
		return unavailable, unavailable
	}
	activity := conversationSection{Availability: "available", Value: before, ObservedAt: time.Now().UTC()}
	if before.Status != jobdb.JobStatusCompleted && before.Store != jobdb.JobStoreArchived {
		return activity, conversationSection{Availability: "available", Value: map[string]string{"job_id": key.JobId, "state": "pending"}, ObservedAt: time.Now().UTC()}
	}
	var run jobdb.GetJobRunResponse
	if inventory.run != nil {
		run = *inventory.run
		ctx = context.WithValue(ctx, storyRunKey{}, inventory.run)
	} else {
		run, err = s.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: key})
		if err != nil {
			return activity, unavailable
		}
	}
	if len(run.Attempts) > 0 {
		ordinal := run.Attempts[len(run.Attempts)-1].Ordinal
		activity.AttemptOrdinal = &ordinal
	}
	manifest, err := story.GetFinalDocuments(ctx, s.story, story.GetWorkflowOutcomeRequest{ProjectID: key.TenantId, JobID: key.JobId})
	if errors.Is(err, story.ErrOutcomePending) || errors.Is(err, jobdb.ErrJobNotComplete) {
		return activity, conversationSection{Availability: "available", Value: map[string]string{"job_id": key.JobId, "state": "pending"}, ObservedAt: time.Now().UTC()}
	}
	if err != nil || manifest == nil {
		return activity, unavailable
	}
	after, err := readJob()
	if err != nil {
		return unavailable, unavailable
	}
	activity.Value = after
	activity.AttemptOrdinal = nil
	later := run
	if reader, ok := s.chapters.(chapterLookup); ok && inventory.run != nil {
		// Detect a new attempt committed while composing the terminal sections.
		_, probeErr := reader.GetChapter(ctx, jobdb.ChapterRef{JobKey: key, Ordinal: inventory.nextOrdinal})
		if !errors.Is(probeErr, jobdb.ErrChapterNotFound) {
			return activity, unavailable
		}
	} else {
		later, err = s.engine.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: key})
	}
	if err == nil && len(later.Attempts) > 0 {
		ordinal := later.Attempts[len(later.Attempts)-1].Ordinal
		activity.AttemptOrdinal = &ordinal
	}
	if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(run.Attempts, later.Attempts) || manifest.JobID != key.JobId {
		return activity, unavailable
	}
	if len(later.Attempts) > 0 {
		latest := later.Attempts[len(later.Attempts)-1].Ordinal
		if manifest.AttemptOrdinal == nil || *manifest.AttemptOrdinal != latest {
			return activity, unavailable
		}
	} else if manifest.AttemptOrdinal != nil {
		return activity, unavailable
	}
	identity := finalManifestID(manifest)
	return activity, conversationSection{Availability: "available", Value: struct {
		*story.FinalDocuments
		ManifestID string `json:"manifest_id"`
	}{manifest, identity}, ObservedAt: time.Now().UTC()}
}
