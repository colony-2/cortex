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
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
	"gopkg.in/yaml.v3"
)

func TestStoryReplay_StepChaining_UsesOpRegistryWhenNextTaskMissing(t *testing.T) {
	coreops.Register(coreops.NewOp().
		WithType("test_chain").
		AddStep("first", coreops.NewStep(func(_ context.Context, _ map[string]any) (map[string]any, error) { return map[string]any{}, nil })).
		AddStep("second", coreops.NewStep(func(_ context.Context, _ map[string]any) (map[string]any, error) { return map[string]any{}, nil })).
		BuildOrPanic(),
	)

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_chain
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

	now := time.Date(2026, 2, 8, 3, 0, 0, 0, time.UTC)
	tasks := []swf.TaskRun{
		{
			TaskRunID: "test_chain:first:1",
			TaskType:  "test_chain:first",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   1,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: now,
					Input:     &swf.TaskIO{Data: []byte(`{"input":{},"context":{"InvokeSeq":1}}`)},
					Output:    &swf.TaskIO{Data: stepEnv(map[string]any{"step": "one"})},
					State:     swf.TaskAttemptStateSucceeded,
					Outcome:   swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
				},
			},
		},
		{
			TaskRunID: "test_chain:second:2",
			TaskType:  "test_chain:second",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   2,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: now.Add(1 * time.Second),
					Input:     &swf.TaskIO{Data: []byte(`{"input":{},"context":{"InvokeSeq":1}}`)},
					Output:    &swf.TaskIO{Data: stepEnv(map[string]any{"step": "two"})},
					State:     swf.TaskAttemptStateSucceeded,
					Outcome:   swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
				},
			},
		},
	}

	jobCtx := NewStoryBuildingContext(nil, "tenant", swf.JobKey{TenantId: "tenant", JobId: "job"}, "recipe", swf.JobStatusActive, tasks, nil)
	exec := newExecutor("tenant", "job", jobCtx)
	_, _, execErr := exec.ExecuteRecipe(&rec, map[string]interface{}{}, contextual.JobContext{}, contextual.GitCommitContext{})
	if execErr != nil {
		t.Fatalf("ExecuteRecipe: %v", execErr)
	}

	root := exec.Root()
	if root == nil {
		t.Fatalf("expected story root")
	}
	if len(root.Children) != 1 || root.Children[0] == nil {
		t.Fatalf("expected 1 child under root")
	}
	seq := root.Children[0]
	if len(seq.Children) != 1 || seq.Children[0] == nil {
		t.Fatalf("expected 1 child under sequence")
	}
	op := seq.Children[0]
	if op.Kind != model.JobRunStoryNodeKindOp {
		t.Fatalf("expected op kind, got %q", op.Kind)
	}
	if op.Status != model.JobRunStoryNodeStatusSucceeded {
		t.Fatalf("expected op status succeeded, got %q", op.Status)
	}
	if len(op.Children) != 2 {
		t.Fatalf("expected 2 step children, got %d", len(op.Children))
	}
	for _, ch := range op.Children {
		if ch == nil {
			continue
		}
		if ch.Kind == model.JobRunStoryNodeKindContextPatch {
			t.Fatalf("unexpected contextPatch child under op")
		}
	}
}
