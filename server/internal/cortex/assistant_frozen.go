package cortex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/colony-2/c2j/pkg/artifacts"
	ca "github.com/colony-2/cortex-assistant/assistant"
)

// The module and ordinary recipe identify the same accepted release. Historical
// descriptors must be selected explicitly rather than assigned this protocol.
const assistantRecipeCommit = "0561937f92a8e15c9eedb0d3045ab432f8e2c17a"
const assistantGoalRecipe = "git+https://github.com/colony-2/cortex-assistant.git//discussion-goal.yaml@" + assistantRecipeCommit

// captureFrozenAssistantSource is called only for a genuinely new operation,
// after checking for an already retained prepared request. Both the snapshot
// and its independent selection originate at this trusted source boundary.
func (s *Server) captureFrozenAssistantSource(ctx context.Context, tenant string, source AssistantSource) (ca.FrozenSnapshot, ca.SnapshotSelection, error) {
	capture, err := s.captureAssistantSource(ctx, tenant, source, assistantCaptureLimits{
		DocumentBytes: ca.SnapshotDocumentMaxBytes,
		TotalBytes:    ca.SnapshotMaxBytes,
		Documents:     ca.SnapshotDocumentCount,
	})
	if err != nil {
		return ca.FrozenSnapshot{}, ca.SnapshotSelection{}, err
	}
	return freezeAssistantCapture(capture)
}

func cloneAssistantRef(ref *artifacts.Ref) *artifacts.Ref {
	if ref == nil {
		return nil
	}
	copy := *ref
	if ref.Stored != nil {
		stored := *ref.Stored
		copy.Stored = &stored
	}
	return &copy
}

func freezeAssistantCapture(capture assistantCapture) (ca.FrozenSnapshot, ca.SnapshotSelection, error) {
	snapshot := ca.FrozenSnapshot{
		TenantID:  capture.TenantID,
		Source:    ca.Source(capture.Source),
		At:        capture.At,
		Documents: make([]ca.SnapshotDocument, 0, len(capture.Documents)),
	}
	selected := ca.SnapshotSelection{TenantID: snapshot.TenantID, Source: snapshot.Source, At: snapshot.At, Documents: map[string]ca.SnapshotDocument{}}
	// Reserve all coverage/provenance metadata before admitting source text.
	// Counting raw text alone misses JSON escaping and can exceed the release's
	// encoded snapshot budget even when every individual document fits.
	for _, captured := range capture.Documents {
		d := ca.SnapshotDocument{ID: captured.ID, MIME: captured.MIME, Outcome: captured.Outcome, Size: captured.Size, Ref: cloneAssistantRef(captured.Ref)}
		// A job-wide story or unavailable external reference has no singular
		// artifact task. Only stored refs assert original producer coordinates.
		d.Producer.JobID = capture.Source.JobID
		if d.Ref != nil && d.Ref.Stored != nil {
			d.Producer.JobID = d.Ref.Stored.Key.JobId
			d.Producer.Task = d.Ref.Stored.Key.TaskOrdinal
		}
		if d.Outcome == "included" || d.Outcome == "truncated" {
			d.Outcome = "budget_omitted"
		}
		snapshot.Documents = append(snapshot.Documents, d)
	}
	for i, captured := range capture.Documents {
		if captured.Outcome != "included" && captured.Outcome != "truncated" {
			continue
		}
		candidate := snapshot.Documents[i]
		candidate.Outcome, candidate.Text = captured.Outcome, captured.Text
		sum := sha256.Sum256([]byte(candidate.Text))
		candidate.SHA256 = hex.EncodeToString(sum[:])
		snapshot.Documents[i] = candidate
		raw, err := json.Marshal(snapshot)
		if err != nil || len(raw) > ca.SnapshotMaxBytes {
			candidate.Outcome, candidate.Text, candidate.SHA256 = "budget_omitted", "", ""
			snapshot.Documents[i] = candidate
		}
	}
	for _, d := range snapshot.Documents {
		// Mutating the value sent for validation must not mutate the trusted
		// reference against which the facade checks it.
		d.Ref = cloneAssistantRef(d.Ref)
		selected.Documents[d.ID] = d
	}
	if err := ca.ValidateFrozenSnapshot(snapshot, selected); err != nil {
		return ca.FrozenSnapshot{}, ca.SnapshotSelection{}, assistantFacadeError(err)
	}
	return snapshot, selected, nil
}
