package cortex

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	ca "github.com/colony-2/cortex-assistant/assistant"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestAssistantProjectionPreservesOriginsAndOmitsActionFields(t *testing.T) {
	entry := ca.Entry{Origin: ca.Locator{JobID: "original-job", Task: 17, Attempt: 2, Invocation: "original-invocation", Path: []string{"original-path"}}, Role: "assistant", Status: "succeeded", Text: "grounded reply", Citations: []string{"selected"}, Receipt: &input.Receipt{RequestID: "original-request", SubmissionID: "original-submission"}, Proposal: &ca.Proposal{ID: "executable-proposal"}, Destination: "backend-destination", Error: "private-provider-error"}
	entry.Receipt.SubmittedAt = time.Unix(100, 0)
	entry.Artifacts = []ca.Artifact{{ID: "original-artifact-id", Ref: artifacts.NewStoredRef(jobdb.ArtifactKey{JobId: "earlier-producer", TaskOrdinal: 8, Name: "original.txt"}), Name: "original.txt", Available: true}}
	page := ca.ReplayPage{Entries: []ca.Entry{entry}, Complete: true, Watermark: "actual-watermark"}
	got := projectAssistantReplay(page)
	e := got.Entries[0]
	if e.Status != "unavailable" {
		t.Fatal("recorded model failure appeared successful")
	}
	if e.Origin.JobID != entry.Origin.JobID || e.Origin.Task != 17 || e.Origin.Attempt != 2 || e.Origin.Invocation != entry.Origin.Invocation || e.Receipt.RequestID != "original-request" || e.Receipt.SubmissionID != "original-submission" || len(e.Citations) != 1 || got.Watermark != page.Watermark {
		t.Fatalf("lost recorded origin: %+v", got)
	}
	if e.Receipt.SubmittedAt == nil || !e.Receipt.SubmittedAt.Equal(entry.Receipt.SubmittedAt) || len(e.Origin.Path) != 1 || e.Origin.Path[0] != "original-path" || e.Artifacts[0].ID != "original-artifact-id" || e.Artifacts[0].Origin.JobID != "earlier-producer" || e.Artifacts[0].Origin.Task != 8 || e.Artifacts[0].Origin.Attempt != 0 {
		t.Fatal("original receipt/artifact origin changed or attempt invented", e)
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"executable-proposal", "backend-destination", "private-provider-error", "proposal", "destination"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("browser envelope includes %s", forbidden)
		}
	}
	entry.Origin.Attempt++
	if projectAssistantReplay(ca.ReplayPage{Entries: []ca.Entry{entry}}).Entries[0].Key == e.Key {
		t.Fatal("different actual attempts share an entry identity")
	}
	entry.Origin.Attempt--
	entry.Origin.JobID = "replacement-job"
	if projectAssistantReplay(ca.ReplayPage{Entries: []ca.Entry{entry}}).Entries[0].Key == e.Key {
		t.Fatal("different actual jobs share an entry identity")
	}
}

func TestAssistantProjectionRequiresExplicitLifecycle(t *testing.T) {
	s := ca.Session{ID: "legacy-session", JobID: "legacy-execution", GoalID: "goal", Source: ca.Source{Kind: "global"}}
	legacy, err := projectAssistantSession(s, "legacy")
	if err != nil || legacy.ID != s.ID || legacy.JobID != s.JobID || legacy.Lifecycle != "legacy" {
		t.Fatal("legacy identity reinterpreted", legacy, err)
	}
	if _, err = projectAssistantSession(s, "goal"); err == nil {
		t.Fatal("legacy session silently assigned goal semantics")
	}
	s.ID = s.GoalID
	if _, err = projectAssistantSession(s, ""); err == nil {
		t.Fatal("matching identifiers silently selected a lifecycle")
	}
	if goal, err := projectAssistantSession(s, "goal"); err != nil || goal.ID != "goal" || goal.JobID != "legacy-execution" {
		t.Fatal("distinct session/job identity", goal, err)
	}
}
