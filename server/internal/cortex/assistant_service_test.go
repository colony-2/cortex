package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/secrets"
	workerworkflow "github.com/colony-2/c2j/pkg/worker/workflow"
	ca "github.com/colony-2/cortex-assistant/assistant"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/workflow"
)

func servingAssistantFixture(t *testing.T) (context.Context, *Server, *assistantFacadeService) {
	t.Helper()
	ctx := jobdb.WithPrimaryGoalSelectionAuthorizer(celltest.Context(context.Background()), func(context.Context, jobdb.GoalKey, jobdb.ResolvedCell) error { return nil })
	rt, e := sqlite.NewFromConfig(ctx, sqlite.Config{DBPath: filepath.Join(t.TempDir(), "assistant.db")})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if _, e = rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "tenant", Name: "root", Repo: "github.com/example/root"}); e != nil {
		t.Fatal(e)
	}
	eng, e := workflow.NewEngineBuilder().WithRuntime(rt).WithWorkerTenantId("tenant").BuildEngine()
	if e != nil {
		t.Fatal(e)
	}
	cfg := AssistantRuntimeConfig{Root: recipejob.ResolvedTarget{CellName: "root", CellRepo: "github.com/example/root", RepositorySource: "https://github.com/example/root.git", DefaultRef: "main"}, Bindings: ca.OrdinaryBindings{BaseURL: "https://fixture.invalid/exact/root", APIKey: secrets.Reference{Name: "fixture_key"}, Provider: "openai", Model: "fixture"}, CursorKey: []byte(strings.Repeat("key!", 8)), ExecutionReady: true}
	s, e := NewServer(Config{DefaultTenantID: "tenant", AssistantRuntime: &cfg}, jobdbschema.WorkflowEngine{Engine: eng, Registry: rt})
	if e != nil {
		t.Fatal(e)
	}
	return ctx, s, s.cfg.Assistant.Service.(*assistantFacadeService)
}

var servingCaller = AssistantCaller{TenantID: "tenant", Principal: AssistantPrincipal{ID: "cortex-discussion"}}

func servingStart(a *assistantFacadeService, id string) AssistantStart {
	return AssistantStart{Generation: a.generation, RequestID: id, SubmittedAt: time.Unix(100, 0), Prompt: "Selected global prompt", Source: AssistantSource{Kind: "global"}}
}

func TestAssistantServingFrozenRetryAndAdmission(t *testing.T) {
	ctx, _, a := servingAssistantFixture(t)
	// Simulate losing the actual native submission receipt at the facade boundary.
	lost := &assistantLostSubmissionReceipt{Submitter: nativeSubmitter{a.server.engine}, lose: true}
	ctl := &workerworkflow.SWFWorkflowControl{Engine: a.server.engine}
	in, e := input.NewRuntime(ctl, nil)
	if e != nil {
		t.Fatal(e)
	}
	root := a.server.cfg.AssistantRuntime.Root
	original, e := ca.New(ca.Host{Goals: a.server.engine.(ca.GoalReader), History: a.server.engine, Control: ctl, Story: a.server.story, Input: in, Submitter: lost, Authorize: func(context.Context, ca.Caller, string) error { return nil }, ResolveCell: func(context.Context, ca.Caller, string) (recipejob.ResolvedTarget, error) { return root, nil }}, ca.Config{Protocol: ca.GoalProtocol, Root: root, Recipe: assistantGoalRecipe, RecipeCommit: assistantRecipeCommit, CursorKey: a.server.cfg.AssistantRuntime.CursorKey, Bindings: &a.server.cfg.AssistantRuntime.Bindings})
	if e != nil {
		t.Fatal(e)
	}
	a.client = original
	request := servingStart(a, "lost")
	if _, e = a.Start(ctx, servingCaller, request); e == nil || lost.lose {
		t.Fatal("did not lose actual accepted receipt", e)
	}
	op := a.operations[assistantOperationKey{servingCaller, "start", "lost"}]
	before, _ := json.Marshal(op.frozen)
	if len(before) == 0 {
		t.Fatal("not retained before publication")
	}
	// Replacing the current client cannot change the retained operation's binding.
	_, _, other := servingAssistantFixture(t)
	a.client = other.client
	v, e := a.Start(ctx, servingCaller, request)
	if e != nil || v.Lifecycle != "goal" {
		t.Fatal(v, e)
	}
	again, e := a.Start(ctx, servingCaller, request)
	if e != nil || again.JobID != v.JobID || again.GoalID != v.GoalID {
		t.Fatal("retargeted retry", again, e)
	}
	after, _ := json.Marshal(op.frozen)
	if string(before) != string(after) || op.client != original {
		t.Fatal("prepared wrapper/configuration changed")
	}
	conflicting := request
	conflicting.Source = AssistantSource{Kind: "story", JobID: "inaccessible"}
	if _, e = a.Start(ctx, servingCaller, conflicting); !errors.Is(e, ErrAssistantConflict) {
		t.Fatal("key reuse reached capture", e)
	}
	old := servingStart(a, "old")
	old.Generation = "previous-process"
	old.Source = conflicting.Source
	if _, e = a.Start(ctx, servingCaller, old); !errors.Is(e, ErrAssistantUnresolved) {
		t.Fatal("old generation reached capture", e)
	}
	// Aggregate admission includes in-flight reservations and retains prior records.
	a.mu.Lock()
	a.reserved = assistantRetainedBytes
	a.mu.Unlock()
	if _, e = a.Start(ctx, servingCaller, servingStart(a, "full")); !errors.Is(e, ErrAssistantUnavailable) {
		t.Fatal("aggregate admission", e)
	}
	if len(a.operations) != 1 {
		t.Fatal("evicted or admitted record")
	}
	if _, e = a.Start(ctx, servingCaller, request); e != nil {
		t.Fatal("full map prevented frozen retry", e)
	}
}
func TestAssistantServingConcurrentDuplicatePreparation(t *testing.T) {
	ctx, _, a := servingAssistantFixture(t)
	in := servingStart(a, "parallel")
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); v, e := a.Start(ctx, servingCaller, in); ids <- v.JobID; errs <- e }()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("multiple destinations")
		}
	}
	if len(a.operations) != 1 || a.reserved >= assistantOperationBytes {
		t.Fatal("duplicate reservations or unbounded retained bytes", a.reserved)
	}
	// Count admission independently from aggregate memory.
	for i := 1; i < assistantRetainedOperations; i++ {
		a.operations[assistantOperationKey{servingCaller, "start", fmt.Sprint(i)}] = &assistantPreparedOperation{}
	}
	if _, e := a.Start(ctx, servingCaller, servingStart(a, "count-full")); !errors.Is(e, ErrAssistantUnavailable) {
		t.Fatal("count admission", e)
	}
}
func TestAssistantInvalidBindingsLeaveOrdinaryPagesAvailable(t *testing.T) {
	_, s, _ := servingAssistantFixture(t)
	cfg := *s.cfg.AssistantRuntime
	cfg.Bindings.Model = ""
	wrapped := s.engine.(jobdbschema.WorkflowEngine)
	eng, err := workflow.NewEngineBuilder().WithRuntime(wrapped.Registry.(jobdb.WorkflowRuntime)).WithWorkerTenantId("tenant").BuildEngine()
	if err != nil {
		t.Fatal(err)
	}
	replacement, e := NewServer(Config{DefaultTenantID: "tenant", AssistantRuntime: &cfg}, jobdbschema.WorkflowEngine{Engine: eng, Registry: wrapped.Registry})
	if e != nil {
		t.Fatal(e)
	}
	if replacement.cfg.Assistant != nil {
		t.Fatal("invalid bindings became available")
	}
}

func TestAssistantServingCancellationUsesRuntimeStatus(t *testing.T) {
	ctx, _, a := servingAssistantFixture(t)
	s, err := a.Start(ctx, servingCaller, servingStart(a, "queued-close"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "queued" {
		t.Fatal("unstarted job presented as processing", s)
	}
	closed, err := a.Close(ctx, servingCaller, s.ID, AssistantLifecycleRequest{JobID: s.JobID})
	if err != nil || closed.Status != "cancelled" || closed.CurrentRequestID != "" {
		t.Fatal(closed, err)
	}
	restored, err := a.Get(ctx, servingCaller, s.ID)
	if err != nil || restored.Status != "cancelled" || restored.JobID != s.JobID {
		t.Fatal(restored, err)
	}
	for _, failure := range []error{input.ErrInputNotPending, fmt.Errorf("wrapped: %w", input.ErrStaleRequest)} {
		if !errors.Is(assistantFacadeError(failure), ErrAssistantConflict) {
			t.Fatal("concurrent loser treated as transport failure")
		}
	}
}

func TestAssistantTrustedConfigBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assistant.json")
	for _, raw := range []string{`{"untrusted":true}`, `{}` + strings.Repeat(" ", 64<<10), `{} {}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadAssistantRuntimeConfig(path); err == nil {
			t.Fatal("accepted invalid/oversized configuration")
		}
	}
}
