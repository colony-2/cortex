package story

import (
	"context"
	"errors"
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
	"gopkg.in/yaml.v3"
)

func TestStoryReplay_RuntimeLeasedTask_DoesNotMarkFailed(t *testing.T) {
	type in struct{}
	type out struct {
		Ok bool `json:"ok"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_activity_leased"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{Ok: true}, nil
	}))

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_activity_leased
outputs:
  ok: true
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	// Simulate a running job where SWF surfaces a runtime task attempt in LEASED state that
	// does not match the next replayable op task type yet.
	tasks := []swf.TaskRun{
		{
			TaskRunID: "recipe:1",
			TaskType:  "recipe",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:  1,
					Attempt:  1,
					State:    swf.TaskAttemptStateLeased,
					Outcome:  swf.TaskOutcome{},
					Input:    &swf.TaskIO{Data: []byte(`{"context":{"NodePath":"root"}}`)},
					Output:   &swf.TaskIO{Data: []byte(`null`)},
					WorkerID: "",
				},
			},
		},
	}

	jobCtx := NewStoryBuildingContext(nil, "tenant", swf.JobKey{TenantId: "tenant", JobId: "job"}, "recipe", swf.JobStatusActive, tasks, nil)
	exec := newExecutor("tenant", "job", jobCtx)

	_, _, execErr := exec.ExecuteRecipe(&rec, map[string]interface{}{}, contextual.JobContext{}, contextual.GitCommitContext{})
	if execErr == nil || !errors.Is(execErr, ErrReplayInProgress) {
		t.Fatalf("expected ErrReplayInProgress, got %v", execErr)
	}

	root := exec.Root()
	if root == nil {
		t.Fatalf("expected story root")
	}
	if root.Status != model.JobRunStoryNodeStatusRunning {
		t.Fatalf("expected root status running, got %q", root.Status)
	}
}

func TestStoryReplay_RuntimeLeasedTaskBeforeNextOp_DoesNotMismatch(t *testing.T) {
	type in struct{}
	type out struct {
		Ok bool `json:"ok"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_activity_leased2"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{Ok: true}, nil
	}))

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_activity_leased2
outputs:
  ok: true
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	tasks := []swf.TaskRun{
		{
			TaskRunID: "recipe:1",
			TaskType:  "recipe",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:  1,
					Attempt:  1,
					State:    swf.TaskAttemptStateLeased,
					Outcome:  swf.TaskOutcome{},
					Input:    &swf.TaskIO{Data: []byte(`{"context":{"NodePath":"root"}}`)},
					Output:   &swf.TaskIO{Data: []byte(`null`)},
					WorkerID: "",
				},
			},
		},
		{
			TaskRunID: "test_activity_leased2:test_activity_leased2:2",
			TaskType:  "test_activity_leased2:test_activity_leased2",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal: 2,
					Attempt: 1,
					State:   swf.TaskAttemptStateSucceeded,
					Outcome: swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
					Input:   &swf.TaskIO{Data: []byte(`{"input":{},"context":{"NodePath":"root/run/test_activity_leased2","InvokeSeq":0}}`)},
					Output:  &swf.TaskIO{Data: []byte(`null`)},
				},
			},
		},
	}

	jobCtx := NewStoryBuildingContext(nil, "tenant", swf.JobKey{TenantId: "tenant", JobId: "job"}, "recipe", swf.JobStatusActive, tasks, nil)
	exec := newExecutor("tenant", "job", jobCtx)

	_, _, execErr := exec.ExecuteRecipe(&rec, map[string]interface{}{}, contextual.JobContext{}, contextual.GitCommitContext{})
	if execErr == nil || !errors.Is(execErr, ErrReplayInProgress) {
		t.Fatalf("expected ErrReplayInProgress, got %v", execErr)
	}

	root := exec.Root()
	if root == nil {
		t.Fatalf("expected story root")
	}
	if root.Status != model.JobRunStoryNodeStatusRunning {
		t.Fatalf("expected root status running, got %q", root.Status)
	}
	if len(root.Children) != 1 || root.Children[0] == nil {
		t.Fatalf("expected recipe -> sequence child")
	}
	seq := root.Children[0]
	if len(seq.Children) != 1 || seq.Children[0] == nil {
		t.Fatalf("expected sequence -> op child")
	}
	op := seq.Children[0]
	if op.Status != model.JobRunStoryNodeStatusRunning {
		t.Fatalf("expected op status running, got %q", op.Status)
	}
}
