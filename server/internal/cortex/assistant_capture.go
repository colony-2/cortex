package cortex

// Request-boundary source reads. The released assistant validates and packages
// these records; workers never resolve a mutable Cortex page themselves.
import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/story"
)

// Limits are supplied by facade composition, below its encoded snapshot envelope.
// Bytes counts retained source bytes; encoded JSON still needs facade validation.
type assistantCaptureLimits struct{ DocumentBytes, TotalBytes, Documents int }
type assistantCapturedDocument struct {
	ID            string
	Ref           *artifacts.Ref
	MIME, Outcome string
	Text          string
	Size          int64
}
type assistantCapture struct {
	TenantID  string
	Source    AssistantSource
	At        time.Time
	Documents []assistantCapturedDocument
}

func (s *Server) captureAssistantSource(ctx context.Context, tenant string, source AssistantSource, limits assistantCaptureLimits) (assistantCapture, error) {
	capture := assistantCapture{TenantID: tenant, Source: source, At: time.Now().UTC(), Documents: []assistantCapturedDocument{}}
	if !assistantToken(tenant) || !source.valid() {
		return capture, ErrAssistantInvalid
	}
	if source.Kind == "global" {
		return capture, nil
	}
	if limits.DocumentBytes <= 0 || limits.TotalBytes <= 0 || limits.Documents <= 0 {
		return capture, ErrAssistantUnavailable
	}
	switch source.Kind {
	case "review":
		if s.inputs == nil || s.engine == nil {
			return capture, ErrAssistantUnavailable
		}
		form, err := s.inputs.GetForm(ctx, tenant, source.JobID)
		if err != nil {
			if errors.Is(err, input.ErrInputNotPending) {
				return capture, ErrAssistantConflict
			}
			return capture, ErrAssistantUnavailable
		}
		if form.Kind != "review" || form.RequestID != source.RequestID {
			return capture, ErrAssistantConflict
		}
		if len(form.Documents) > limits.Documents {
			return capture, ErrAssistantInvalid
		}
		ids := make([]string, 0, len(form.Documents))
		for id := range form.Documents {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		remaining := limits.TotalBytes
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return capture, err
			}
			ref := form.Documents[id]
			d := assistantCapturedDocument{ID: id, Outcome: "unavailable"}
			// Only the authoritative stored reference is eligible. A URL is never fetched.
			if ref.Kind != artifacts.RefKindStored || ref.Stored == nil || ref.Validate() != nil {
				capture.Documents = append(capture.Documents, d)
				continue
			}
			d.Ref = &ref
			d.MIME = assistantCaptureMIME(ref.Stored.Key.Name)
			artifact, err := s.engine.GetArtifact(tenant, ref.Stored.Key)
			if err != nil {
				capture.Documents = append(capture.Documents, d)
				continue
			}
			d.Size = artifact.Size()
			if d.Size < 0 {
				capture.Documents = append(capture.Documents, d)
				continue
			}
			if d.MIME == "" {
				d.Outcome = "unsupported"
				capture.Documents = append(capture.Documents, d)
				continue
			}
			reader, err := artifact.Open()
			if err != nil {
				capture.Documents = append(capture.Documents, d)
				continue
			}
			if remaining == 0 {
				_ = reader.Close()
				d.Outcome = "budget_omitted"
				capture.Documents = append(capture.Documents, d)
				continue
			}
			max := min(limits.DocumentBytes, remaining)
			data, readErr := io.ReadAll(io.LimitReader(reader, int64(max)+1))
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil {
				capture.Documents = append(capture.Documents, d)
				continue
			}
			d.Outcome = "included"
			if len(data) > max {
				d.Size = max64(d.Size, int64(len(data)))
				d.Outcome = "truncated"
				data = data[:max]
				// Retain a complete UTF-8 prefix at the transport cap.
				for len(data) > 0 && !utf8.Valid(data) && len(data) > max-utf8.UTFMax {
					data = data[:len(data)-1]
				}
			} else {
				d.Size = int64(len(data))
			}
			if !utf8.Valid(data) || (d.MIME == "application/json" && d.Outcome == "included" && !json.Valid(data)) {
				d.Outcome = "unsupported"
				capture.Documents = append(capture.Documents, d)
				continue
			}
			d.Text = string(data)
			remaining -= len(data)
			capture.Documents = append(capture.Documents, d)
		}
		// Capture is rejected if the selected request changed while bytes were read.
		current, err := s.inputs.GetForm(ctx, tenant, source.JobID)
		if err != nil {
			if errors.Is(err, input.ErrInputNotPending) {
				return capture, ErrAssistantConflict
			}
			return capture, ErrAssistantUnavailable
		}
		if current.RequestID != source.RequestID || current.Kind != "review" || !reflect.DeepEqual(form.Documents, current.Documents) {
			return capture, ErrAssistantConflict
		}
	case "story":
		if s.story == nil {
			return capture, ErrAssistantUnavailable
		}
		value, err := s.story.GetJobRunStory(ctx, story.GetJobRunStoryRequest{ProjectID: tenant, JobID: source.JobID})
		if err != nil {
			return capture, ErrAssistantUnavailable
		}
		if value == nil || value.JobID != source.JobID {
			return capture, ErrAssistantUnavailable
		}
		data, err := json.Marshal(value)
		if err != nil {
			return capture, ErrAssistantUnavailable
		}
		d := assistantCapturedDocument{ID: "story", MIME: "application/json", Outcome: "included", Size: int64(len(data))}
		max := min(limits.DocumentBytes, limits.TotalBytes)
		if len(data) > max {
			data = data[:max]
			d.Outcome = "truncated"
			for len(data) > 0 && !utf8.Valid(data) {
				data = data[:len(data)-1]
			}
		}
		d.Text = string(data)
		capture.Documents = append(capture.Documents, d)
	}
	return capture, nil
}

func assistantCaptureMIME(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt", ".log", ".csv":
		return "text/plain"
	case ".json":
		return "application/json"
	default:
		return ""
	}
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
