package cortex

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	ca "github.com/colony-2/cortex-assistant/assistant"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestAssistantFrozenCaptureUsesReleasedValidation(t *testing.T) {
	ctx := context.Background()
	s := &Server{}
	global, selection, err := s.captureFrozenAssistantSource(ctx, "tenant", AssistantSource{Kind: "global"})
	if err != nil || len(global.Documents) != 0 || ca.ValidateFrozenSnapshot(global, selection) != nil {
		t.Fatal("global snapshot", global, err)
	}
	name, body := "notes/child.md", "Selected child text 世界 after the first thousand bytes: "+strings.Repeat("grounded ", 180)
	ref := artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "original-child", TaskOrdinal: 17, Name: name, SizeBytes: int64(len(body))})
	s.engine = &assistantCaptureEngine{data: map[string][]byte{name: []byte(body)}}
	s.inputs = &reviewRuntimeStub{forms: map[string]input.InputForm{"parent": {Kind: "review", RequestID: "exact", Documents: map[string]artifacts.Ref{"selected": ref}}}}
	frozen, selected, err := s.captureFrozenAssistantSource(ctx, "tenant", AssistantSource{Kind: "review", JobID: "parent", RequestID: "exact"})
	if err != nil || len(frozen.Documents) != 1 {
		t.Fatal("review snapshot", err)
	}
	d := frozen.Documents[0]
	if d.Producer.JobID != "original-child" || d.Producer.Task != 17 || d.Ref.Stored.Key != ref.Stored.Key || d.Text != body || d.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) {
		t.Fatalf("lost source identity or bytes: %+v", d)
	}
	// A forged producer must not also change the independent selected reference.
	frozen.Documents[0].Ref.Stored.Key.JobId = "forged"
	if selected.Documents[d.ID].Ref.Stored.Key.JobId != "original-child" || ca.ValidateFrozenSnapshot(frozen, selected) == nil {
		t.Fatal("source substitution changed trusted selection")
	}
	if _, _, err = s.captureFrozenAssistantSource(ctx, "tenant", AssistantSource{Kind: "review", JobID: "parent", RequestID: "replacement"}); err != ErrAssistantConflict {
		t.Fatal("replaced review", err)
	}
}

func TestAssistantFrozenEncodedBudgetKeepsEveryCoverageRecord(t *testing.T) {
	text := strings.Repeat("<\n", 12_000) // Fits raw document bytes; escapes exceed the entire JSON budget.
	capture := assistantCapture{TenantID: "tenant", Source: AssistantSource{Kind: "story", JobID: "source"}, At: time.Unix(100, 0), Documents: []assistantCapturedDocument{
		{ID: "escaped", MIME: "text/plain", Outcome: "included", Text: text, Size: int64(len(text))},
		{ID: "small", MIME: "text/plain", Outcome: "included", Text: "readable evidence", Size: 17},
		{ID: "expired", MIME: "text/plain", Outcome: "unavailable"},
	}}
	frozen, selected, err := freezeAssistantCapture(capture)
	if err != nil || len(frozen.Documents) != 3 || len(selected.Documents) != 3 {
		t.Fatal("coverage", err)
	}
	if d := frozen.Documents[0]; d.Outcome != "budget_omitted" || d.Text != "" || d.SHA256 != "" {
		t.Fatal("escaped text reported as read", d)
	}
	if d := frozen.Documents[1]; d.Outcome != "included" || d.Text != "readable evidence" {
		t.Fatal("small following document omitted", d)
	}
	if ca.ValidateFrozenSnapshot(frozen, selected) != nil {
		t.Fatal("released snapshot validation rejected encoded budgeting")
	}
}
