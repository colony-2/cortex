package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/jobref"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/secrets"
	workerworkflow "github.com/colony-2/c2j/pkg/worker/workflow"
	ca "github.com/colony-2/cortex-assistant/assistant"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

type assistantLostSubmissionReceipt struct {
	recipejob.Submitter
	lose bool
}

var errAssistantFixtureReceiptLost = errors.New("fixture: accepted submission response lost")

func (s *assistantLostSubmissionReceipt) SubmitJobRef(ctx context.Context, job jobdb.SubmitJob) (jobref.Ref, error) {
	ref, err := s.Submitter.SubmitJobRef(ctx, job)
	if err == nil && s.lose {
		s.lose = false
		return jobref.Ref{}, errAssistantFixtureReceiptLost
	}
	return ref, err
}

// This bounded runtime experiment reproduces the hosting prerequisite. It
// prepares/submits/cancels real queued jobs on SQLite, with no worker, inference,
// browser or new application storage. It is not CTX-023 acceptance.
func TestAssistantPreparedReopenRequiresOriginalBytesAfterLostReceipt(t *testing.T) {
	ctx := celltest.Context(context.Background())
	// Trusted fixture authority only; no browser field grants goal selection.
	ctx = jobdb.WithPrimaryGoalSelectionAuthorizer(ctx, func(_ context.Context, goal jobdb.GoalKey, target jobdb.ResolvedCell) error {
		if goal.TenantId != "tenant" || target.Cell != "root" || target.CellRepo != "github.com/example/root" {
			return jobdb.ErrPrimaryGoalSelectionForbidden
		}
		return nil
	})
	rt, err := sqlite.NewFromConfig(ctx, sqlite.Config{DBPath: filepath.Join(t.TempDir(), "retention.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)
	if _, err = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "root", Repo: "github.com/example/root"}); err != nil {
		t.Fatal(err)
	}
	engine, err := jobworkflow.NewEngineBuilder().WithRuntime(rt).WithWorkerTenantId("tenant").BuildEngine()
	if err != nil {
		t.Fatal(err)
	}
	wrapped := jobdbschema.WorkflowEngine{Engine: engine, Registry: rt}
	server, err := NewServer(Config{DefaultTenantID: "tenant"}, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	ctl := &workerworkflow.SWFWorkflowControl{Engine: wrapped}
	inputs, err := input.NewRuntime(ctl, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := recipejob.ResolvedTarget{CellName: "root", CellRepo: "github.com/example/root", RepositorySource: "https://github.com/example/root.git", DefaultRef: "main"}
	loss := &assistantLostSubmissionReceipt{Submitter: nativeSubmitter{wrapped}}
	host := ca.Host{Goals: rt, History: wrapped, Control: ctl, Story: server.story, Input: inputs, Submitter: loss,
		Authorize:   func(context.Context, ca.Caller, string) error { return nil },
		ResolveCell: func(context.Context, ca.Caller, string) (recipejob.ResolvedTarget, error) { return root, nil },
	}
	cfg := ca.Config{Protocol: ca.GoalProtocol, Root: root, Recipe: assistantGoalRecipe, RecipeCommit: assistantRecipeCommit, CursorKey: []byte(strings.Repeat("fixture-key-", 4)),
		Bindings: &ca.OrdinaryBindings{BaseURL: "https://fixture.invalid/root", APIKey: secrets.Reference{Name: "fixture_key"}, Provider: "openai", Model: "fixture"},
	}
	client, err := ca.New(host, cfg)
	if err != nil {
		t.Fatal(err)
	}
	caller := ca.Caller{TenantID: "tenant", Actor: input.Actor{Kind: "human", ID: "discussion"}}
	snapshot, selected, err := server.captureFrozenAssistantSource(ctx, caller.TenantID, AssistantSource{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	op, err := client.PrepareStartFrozen(ctx, caller, ca.StartInput{RequestID: "original-start", SubmittedAt: time.Unix(100, 0), Prompt: "opening prompt", Source: snapshot.Source}, snapshot, selected)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.SubmitFrozenStart(ctx, caller, op)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := client.Close(ctx, caller, first.JobID); err != nil || !state.Terminal {
		t.Fatal("captured queued cancellation", state, err)
	}
	at := time.Unix(200, 0)
	prepared, err := client.PrepareReopen(ctx, caller, first.JobID, "original-reopen", at)
	if err != nil {
		t.Fatal(err)
	}
	// Saving this value in a local variable demonstrates the facade's wire
	// contract, not durable application storage across Cortex replacement.
	saved, err := json.Marshal(prepared)
	if err != nil {
		t.Fatal(err)
	}
	loss.lose = true
	if _, err = client.SubmitFrozenStart(ctx, caller, prepared); !errors.Is(err, errAssistantFixtureReceiptLost) || loss.lose {
		t.Fatal("fixture did not lose an actually accepted receipt", err)
	}
	bindings := *cfg.Bindings
	bindings.Model = "replacement-model"
	cfg.Bindings = &bindings
	replacement, err := ca.New(host, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = replacement.PrepareReopen(ctx, caller, first.JobID, "original-reopen", at); !errors.Is(err, ca.ErrStale) {
		t.Fatal("accepted successor should make rebuilding from the predecessor stale", err)
	}
	var loaded ca.FrozenStart
	if err = json.Unmarshal(saved, &loaded); err != nil {
		t.Fatal(err)
	}
	recovered, err := replacement.SubmitFrozenStart(ctx, caller, loaded)
	if err != nil || recovered.GoalID != first.GoalID || recovered.JobID == first.JobID {
		t.Fatal("original operation did not reconcile", recovered, err)
	}
	again, err := replacement.SubmitFrozenStart(ctx, caller, loaded)
	if err != nil || again.JobID != recovered.JobID || again.GoalID != recovered.GoalID {
		t.Fatal("receipt/destination changed", again, err)
	}
	t.Log("real SQLite accepted-but-lost sequential reopen: rebuilding from the predecessor is stale; reloading the original complete FrozenStart recovers the same successor and goal despite host binding changes")
}
