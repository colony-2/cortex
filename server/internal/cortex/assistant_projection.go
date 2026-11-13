package cortex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/colony-2/c2j/pkg/input"
	ca "github.com/colony-2/cortex-assistant/assistant"
)

func assistantFacadeError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ca.ErrForbidden):
		return ErrAssistantDenied
	case errors.Is(err, ca.ErrConflict), errors.Is(err, ca.ErrStale), errors.Is(err, ca.ErrNotAtPrompt), errors.Is(err, input.ErrInputNotPending), errors.Is(err, input.ErrStaleRequest):
		return ErrAssistantConflict
	case errors.Is(err, ca.ErrInvalid), errors.Is(err, ca.ErrLimit):
		return ErrAssistantInvalid
	default:
		return ErrAssistantUnavailable
	}
}

func projectAssistantReplay(in ca.ReplayPage) AssistantReplay {
	out := AssistantReplay{Entries: make([]AssistantEntry, 0, len(in.Entries)), Cursor: in.Cursor, HasMore: in.HasMore, Complete: in.Complete, Watermark: in.Watermark}
	for _, entry := range in.Entries {
		origin := AssistantOrigin{JobID: entry.Origin.JobID, Task: entry.Origin.Task, Attempt: entry.Origin.Attempt, Invocation: entry.Origin.Invocation, Path: append([]string(nil), entry.Origin.Path...)}
		// Include the full original locator in identity; offsets and replacement
		// job IDs would merge distinct retained tasks or attempts incorrectly.
		raw, _ := json.Marshal(entry.Origin)
		sum := sha256.Sum256(raw)
		value := AssistantEntry{Key: hex.EncodeToString(sum[:]), Origin: &origin, Role: entry.Role, Status: entry.Status, Text: entry.Text, Citations: append([]string(nil), entry.Citations...)}
		if entry.Role == "assistant" && entry.Error != "" {
			// A successfully recorded task can contain a failed model response.
			// Expose that outcome without forwarding the underlying error text.
			value.Status = "unavailable"
		}
		if entry.Receipt != nil {
			r := entry.Receipt
			value.Receipt = &AssistantReceipt{RequestID: r.RequestID, SubmissionID: r.SubmissionID, Outcome: "accepted"}
			if !r.SubmittedAt.IsZero() {
				at := r.SubmittedAt
				value.Receipt.SubmittedAt = &at
			}
		}
		for _, artifact := range entry.Artifacts {
			a := AssistantArtifact{ID: artifact.ID, Name: artifact.Name, MIME: artifact.MIME, Size: artifact.Size, Available: artifact.Available}
			if artifact.Ref.Stored != nil {
				key := artifact.Ref.Stored.Key
				a.Origin = &AssistantOrigin{JobID: key.JobId, Task: key.TaskOrdinal}
				if key.JobId == origin.JobID && key.TaskOrdinal == origin.Task {
					a.Origin.Attempt = origin.Attempt
				}
			}
			value.Artifacts = append(value.Artifacts, a)
		}
		// Executable proposals, model routing/usage, raw errors and submitted
		// references do not belong in the discussion-only browser envelope.
		out.Entries = append(out.Entries, value)
	}
	return out
}

// lifecycle is selected by the adapter's explicit release descriptor. Do not
// infer a goal contract merely because two identifiers happen to be equal.
func projectAssistantSession(in ca.Session, lifecycle string) (AssistantSession, error) {
	if lifecycle != "goal" && lifecycle != "legacy" || lifecycle == "goal" && (in.GoalID == "" || in.ID != in.GoalID) {
		return AssistantSession{}, ErrAssistantUnavailable
	}
	out := AssistantSession{ID: in.ID, JobID: in.JobID, GoalID: in.GoalID, Lifecycle: lifecycle, Status: in.Status, Source: AssistantSource(in.Source), Replay: projectAssistantReplay(in.Replay)}
	if in.Current != nil && in.Current.Form.RequestId != nil {
		out.CurrentRequestID = *in.Current.Form.RequestId
	}
	if in.Snapshot != nil {
		at := in.Snapshot.At
		out.CapturedAt = &at
		for _, d := range in.Snapshot.Documents {
			coverage := AssistantCoverage{ID: d.ID, Name: d.ID, Outcome: d.Outcome, ProducerJobID: d.Producer.JobID}
			if outcome, ok := in.Coverage[d.ID]; ok {
				coverage.Outcome = outcome
			}
			if d.Ref != nil && d.Ref.Stored != nil {
				coverage.Name = d.Ref.Stored.Key.Name
				if d.Outcome == "included" && d.SHA256 != "" {
					coverage.ArtifactID = assistantSourceArtifactID(d)
				}
				coverage.ProducerTask = strconv.FormatInt(d.Producer.Task, 10)
			}
			out.Coverage = append(out.Coverage, coverage)
		}
	}
	return out, nil
}

// This ID describes only a facade-validated captured source, never a browser
// supplied producer. The frozen full-byte hash is rechecked on download.
func assistantSourceArtifactID(d ca.SnapshotDocument) string {
	raw, _ := json.Marshal(d)
	sum := sha256.Sum256(raw)
	return "source-" + hex.EncodeToString(sum[:])
}
