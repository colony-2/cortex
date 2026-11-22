package cortex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/task"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

var errReviewProjection = errors.New("review chapter needs native history projection")

// An appended chapter invalidates these locators. Digests distinguish records
// sharing an ordinal in a retained restart prefix. Document bytes stay in JobDB.
type reviewIndex struct {
	NextOrdinal  int64
	Completeness string
	Diagnostics  []input.HistoryDiagnostic
	Reviews      map[string][]reviewOccurrence
}

type reviewOccurrence struct {
	EntryID      string
	RequestID    string
	Locator      input.HistoryLocator
	FormDigest   string
	OutputDigest string
}

type chapterLookup interface {
	GetChapter(context.Context, jobdb.ChapterRef) (jobdb.Chapter, error)
}
type artifactLookup interface {
	OpenArtifact(context.Context, jobdb.ArtifactRef) (jobdb.ArtifactReader, error)
}

func reviewDigest(value any) string {
	raw, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func reviewID(form map[string]json.RawMessage) string {
	var presentation struct{ Type string }
	var request struct {
		ID string `json:"review_id"`
	}
	if json.Unmarshal(form["presentation"], &presentation) != nil || presentation.Type != "review/v2" || json.Unmarshal(form["request"], &request) != nil {
		return ""
	}
	return request.ID
}

func (s *Server) reviewIndexPath(key jobdb.JobKey) string {
	identity, _ := json.Marshal([]string{s.cfg.WorkingDir, s.cfg.JobDBURL, key.TenantId, key.JobId})
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "cortex", "review-index-v1", fmt.Sprintf("%x.json", sha256.Sum256(identity)))
}

func (s *Server) saveReviewIndex(key jobdb.JobKey, index reviewIndex) error {
	if len(index.Reviews) == 0 || s.cfg.WorkingDir == "" {
		return nil
	}
	path := s.reviewIndexPath(key)
	if path == "" {
		return fmt.Errorf("user cache directory unavailable")
	}
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}
	if existing, _ := os.ReadFile(path); bytes.Equal(existing, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".index-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}

// Continuation pages read the same in-memory c2j snapshot without JobDB I/O.
func (s *Server) indexReviewPages(ctx context.Context, key jobdb.JobKey, nextOrdinal int64, first *input.JobInputStoryPage, selected string) ([]input.JobInputStoryEntry, error) {
	index := reviewIndex{NextOrdinal: nextOrdinal, Completeness: first.Completeness, Diagnostics: first.Diagnostics, Reviews: map[string][]reviewOccurrence{}}
	matches := []input.JobInputStoryEntry{}
	for page := first; ; {
		for _, entry := range page.Entries {
			id := reviewID(entry.FormFields)
			if id == "" {
				continue
			}
			index.Reviews[id] = append(index.Reviews[id], reviewOccurrence{EntryID: entry.EntryID, RequestID: entry.RequestID, Locator: entry.Locator,
				FormDigest: reviewDigest(entry.FormFields), OutputDigest: reviewDigest(entry.AcceptedResponse.Raw)})
			if id == selected {
				matches = append(matches, entry)
			}
		}
		if page.NextPageToken == "" {
			break
		}
		var err error
		page, err = s.history.ListJobInputStory(ctx, input.ListJobInputStoryRequest{TenantID: key.TenantId, JobID: key.JobId, PageToken: page.NextPageToken, PageSize: 500})
		if err != nil {
			return nil, err
		}
	}
	if nextOrdinal > 0 {
		if err := s.saveReviewIndex(key, index); err != nil {
			s.logger.Warn("Could not save review chapter index", "error", err)
		}
	}
	return matches, nil
}

func chapterRecord(ch jobdb.Chapter) (jobdb.ExecutionRecord, error) {
	switch body := ch.Body.(type) {
	case jobdb.ExecutionChapter:
		return jobdb.DecodeExecutionRecord(body.Record)
	case *jobdb.ExecutionChapter:
		if body != nil {
			return jobdb.DecodeExecutionRecord(body.Record)
		}
	}
	return jobdb.ExecutionRecord{}, errReviewProjection
}

func (s *Server) reviewPayload(ctx context.Context, key jobdb.JobKey, ordinal int64, fromInput bool) (json.RawMessage, error) {
	reader := s.chapters.(chapterLookup)
	ch, err := reader.GetChapter(ctx, jobdb.ChapterRef{JobKey: key, Ordinal: ordinal})
	if err != nil {
		return nil, err
	}
	record, err := chapterRecord(ch)
	if err != nil {
		return nil, errReviewProjection
	}
	if fromInput && record.Input != nil {
		var source struct {
			Input    json.RawMessage
			Artifact string `json:"input_artifact"`
		}
		if err := json.Unmarshal(record.Input.Data, &source); err != nil {
			return nil, err
		}
		if source.Artifact == "" {
			return source.Input, nil
		}
		// Large frozen inputs are recorded in an invocation-owned artifact.
		for _, a := range record.Input.Artifacts {
			if a.Name != source.Artifact || a.Key == nil {
				continue
			}
			store, ok := s.chapters.(artifactLookup)
			if !ok {
				break
			}
			artifact, err := store.OpenArtifact(ctx, jobdb.ArtifactRef{JobKey: jobdb.JobKey{TenantId: key.TenantId, JobId: a.Key.JobId}, Ordinal: a.Key.TaskOrdinal, Name: a.Key.Name})
			if err != nil {
				return nil, err
			}
			body, err := artifact.Open()
			if err != nil {
				return nil, err
			}
			defer body.Close()
			var payload struct{ Input json.RawMessage }
			err = json.NewDecoder(body).Decode(&payload)
			return payload.Input, err
		}
		return nil, fmt.Errorf("frozen review input unavailable")
	}
	if record.Output == nil || fromInput {
		return nil, errReviewProjection
	}
	raw := record.Output.Data
	var env task.OutputEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.Version != 0 {
		if (env.Version != 1 && env.Version != task.OutputEnvelopeVersion) || env.Kind != task.OutputKindActivityInvocationOutput {
			return nil, errReviewProjection
		}
		raw = env.Payload
	}
	var out struct{ Output json.RawMessage }
	err = json.Unmarshal(raw, &out)
	if err == nil && len(out.Output) == 0 {
		err = errReviewProjection
	}
	return out.Output, err
}

func (s *Server) indexedReview(ctx context.Context, key jobdb.JobKey, selected string) (*input.JobInputStoryPage, error) {
	reader, ok := s.chapters.(chapterLookup)
	if !ok || s.cfg.WorkingDir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.reviewIndexPath(key))
	var index reviewIndex
	if err != nil || json.Unmarshal(data, &index) != nil || index.NextOrdinal < 1 || index.Completeness == "" || len(index.Reviews[selected]) == 0 {
		return nil, nil
	}
	_, err = reader.GetChapter(ctx, jobdb.ChapterRef{JobKey: key, Ordinal: index.NextOrdinal})
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, jobdb.ErrChapterNotFound) {
		return nil, err
	}
	result := []input.JobInputStoryEntry{}
	for _, occ := range index.Reviews[selected] {
		ordinal := occ.Locator.GenerationOrdinal
		fromInput := ordinal == nil
		if fromInput {
			if len(occ.Locator.CollectionOrdinals) == 0 {
				return nil, nil
			}
			ordinal = &occ.Locator.CollectionOrdinals[0]
		}
		raw, err := s.reviewPayload(ctx, key, *ordinal, fromInput)
		if err != nil {
			return nil, err
		}
		entry := input.JobInputStoryEntry{EntryID: occ.EntryID, RequestID: occ.RequestID, Locator: occ.Locator}
		if json.Unmarshal(raw, &entry.FormFields) != nil || reviewID(entry.FormFields) != selected || reviewDigest(entry.FormFields) != occ.FormDigest {
			return nil, nil
		}
		var requestID string
		if json.Unmarshal(entry.FormFields["request_id"], &requestID) != nil || requestID != occ.RequestID {
			return nil, nil
		}
		if accepted := occ.Locator.AcceptedOrdinal; accepted != nil {
			output, err := s.reviewPayload(ctx, key, *accepted, false)
			if err != nil {
				return nil, err
			}
			if reviewDigest(output) != occ.OutputDigest {
				return nil, nil
			}
			entry.AcceptedResponse = input.HistoryValue{Present: true, Raw: output}
			var out struct{ Receipt json.RawMessage }
			if err = json.Unmarshal(output, &out); err != nil {
				return nil, err
			}
			entry.Actor.Receipt = out.Receipt
		}
		result = append(result, entry)
	}
	return &input.JobInputStoryPage{TenantID: key.TenantId, JobID: key.JobId, Entries: result, Completeness: index.Completeness, Diagnostics: index.Diagnostics}, nil
}

func (s *Server) handleRecordedReview(w http.ResponseWriter, r *http.Request, key jobdb.JobKey, selected string) {
	ctx := r.Context()
	page, err := s.indexedReview(ctx, key, selected)
	if errors.Is(err, errReviewProjection) || errors.Is(err, jobdb.ErrChapterNotFound) {
		page = nil
		err = nil
	}
	if err == nil && page == nil {
		capture := &conversationInventory{key: key}
		ctx = context.WithValue(ctx, inventoryContextKey{}, capture)
		var first *input.JobInputStoryPage
		first, err = s.history.ListJobInputStory(ctx, input.ListJobInputStoryRequest{TenantID: key.TenantId, JobID: key.JobId, PageSize: 500})
		if err == nil {
			entries, indexErr := s.indexReviewPages(ctx, key, capture.nextOrdinal, first, selected)
			err = indexErr
			page = &input.JobInputStoryPage{TenantID: key.TenantId, JobID: key.JobId, Entries: entries, Completeness: first.Completeness, Diagnostics: first.Diagnostics}
		}
	}
	if err != nil {
		historyError(w, err)
		return
	}
	job, err := recipejob.GetRecipeJob(ctx, archivedJobReader{s.engine}, recipejob.GetRecipeJobRequest{TenantID: key.TenantId, JobID: key.JobId})
	activity := conversationSection{Availability: "unavailable"}
	if err == nil {
		activity.Availability = "available"
		activity.Value = job
	}
	writeJSON(w, http.StatusOK, struct {
		*input.JobInputStoryPage
		Activity conversationSection `json:"activity"`
		ReviewID string              `json:"review_id"`
	}{page, activity, selected})
}
