package story

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	coretasks "github.com/colony-2/colony2/server/recipe-core/pkg/task"
	"github.com/colony-2/swf-go/pkg/swf"
	"gopkg.in/yaml.v3"
)

func TestStoryReplay_DoesNotChainAcrossSeparateOpInvocations(t *testing.T) {
	coreops.Register(coreops.NewOp().
		WithType("test_chain_twice").
		AddStep("first", coreops.NewStep(func(_ context.Context, _ map[string]any) (map[string]any, error) { return map[string]any{}, nil })).
		AddStep("second", coreops.NewStep(func(_ context.Context, _ map[string]any) (map[string]any, error) { return map[string]any{}, nil })).
		BuildOrPanic(),
	)

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: one
    op: test_chain_twice
  - id: two
    op: test_chain_twice
outputs:
  ok: true
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	stepEnv := func(out map[string]any) []byte {
		payload := map[string]any{"git": map[string]any{}, "output": out}
		env, err := coretasks.NewOutputEnvelope(coretasks.OutputKindActivityInvocationOutput, payload)
		if err != nil {
			t.Fatalf("NewOutputEnvelope: %v", err)
		}
		b, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		return b
	}

	now := time.Date(2026, 2, 8, 3, 30, 0, 0, time.UTC)
	tasks := []swf.TaskRun{
		{TaskRunID: "test_chain_twice:first:1", TaskType: "test_chain_twice:first", Attempts: []swf.TaskAttempt{{Ordinal: 1, Attempt: 1, WorkerID: "w1", CreatedAt: now, Input: &swf.TaskIO{Data: []byte(`{"input":{},"context":{"InvokeSeq":1}}`)}, Output: &swf.TaskIO{Data: stepEnv(map[string]any{"inv": 1, "step": "one"})}, State: swf.TaskAttemptStateSucceeded, Outcome: swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded}}}},
		{TaskRunID: "test_chain_twice:second:2", TaskType: "test_chain_twice:second", Attempts: []swf.TaskAttempt{{Ordinal: 2, Attempt: 1, WorkerID: "w1", CreatedAt: now.Add(1 * time.Second), Input: &swf.TaskIO{Data: []byte(`{"input":{},"context":{"InvokeSeq":1}}`)}, Output: &swf.TaskIO{Data: stepEnv(map[string]any{"inv": 1, "step": "two"})}, State: swf.TaskAttemptStateSucceeded, Outcome: swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded}}}},
		{TaskRunID: "test_chain_twice:first:3", TaskType: "test_chain_twice:first", Attempts: []swf.TaskAttempt{{Ordinal: 3, Attempt: 1, WorkerID: "w1", CreatedAt: now.Add(2 * time.Second), Input: &swf.TaskIO{Data: []byte(`{"input":{},"context":{"InvokeSeq":2}}`)}, Output: &swf.TaskIO{Data: stepEnv(map[string]any{"inv": 2, "step": "one"})}, State: swf.TaskAttemptStateSucceeded, Outcome: swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded}}}},
		{TaskRunID: "test_chain_twice:second:4", TaskType: "test_chain_twice:second", Attempts: []swf.TaskAttempt{{Ordinal: 4, Attempt: 1, WorkerID: "w1", CreatedAt: now.Add(3 * time.Second), Input: &swf.TaskIO{Data: []byte(`{"input":{},"context":{"InvokeSeq":2}}`)}, Output: &swf.TaskIO{Data: stepEnv(map[string]any{"inv": 2, "step": "two"})}, State: swf.TaskAttemptStateSucceeded, Outcome: swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded}}}},
	}

	attempts := []swf.JobAttempt{{Attempt: 1, Tasks: tasks}}
	jobCtx := NewStoryBuildingContext(nil, "tenant", swf.JobKey{TenantId: "tenant", JobId: "job"}, "recipe", swf.JobStatusActive, attempts, nil)
	exec := newExecutor("tenant", "job", jobCtx)
	_, _, execErr := exec.ExecuteRecipe(&rec, map[string]interface{}{}, contextual.JobContext{}, contextual.GitCommitContext{})
	if execErr != nil {
		t.Fatalf("ExecuteRecipe: %v", execErr)
	}

	root := exec.Root()
	if root == nil || len(root.Children) != 1 || root.Children[0] == nil {
		t.Fatalf("expected recipe -> sequence structure")
	}
	seq := root.Children[0]
	if len(seq.Children) != 2 || seq.Children[0] == nil || seq.Children[1] == nil {
		t.Fatalf("expected 2 op children under sequence, got %d", len(seq.Children))
	}
	if len(seq.Children[0].Children) != 2 {
		t.Fatalf("expected first op to have 2 steps, got %d", len(seq.Children[0].Children))
	}
	if len(seq.Children[1].Children) != 2 {
		t.Fatalf("expected second op to have 2 steps, got %d", len(seq.Children[1].Children))
	}
}
