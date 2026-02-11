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

func TestStoryReplay_TaskRunWithNoAttempts_IsInProgressAndDoesNotFailOp(t *testing.T) {
	type in struct{}
	type out struct {
		Ok bool `json:"ok"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_activity_no_attempts"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{Ok: true}, nil
	}))

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_activity_no_attempts
outputs:
  ok: true
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	// A TaskRun may exist before its first attempt is recorded; the story should treat this as
	// in-progress (running), not failed.
	tasks := []swf.TaskRun{
		{
			TaskRunID: "test_activity_no_attempts:test_activity_no_attempts:1",
			TaskType:  "test_activity_no_attempts:test_activity_no_attempts",
			Attempts:  nil,
		},
	}

	res := runRecipeReplay(t, &rec, map[string]interface{}{}, contextual.JobContext{}, contextual.GitCommitContext{}, swf.JobStatusReady, tasks)
	if res.err == nil || !errors.Is(res.err, ErrReplayInProgress) {
		t.Fatalf("expected ErrReplayInProgress, got %v", res.err)
	}

	root := res.exec.Root()
	if root == nil {
		t.Fatalf("expected story root")
	}
	if root.Status != model.JobRunStoryNodeStatusRunning {
		t.Fatalf("expected root status running, got %q", root.Status)
	}

	// Walk recipe -> sequence -> op -> step.
	if len(root.Children) != 1 || root.Children[0] == nil {
		t.Fatalf("expected 1 child under root")
	}
	seq := root.Children[0]
	if seq.Status != model.JobRunStoryNodeStatusRunning {
		t.Fatalf("expected sequence status running, got %q", seq.Status)
	}
	if len(seq.Children) != 1 || seq.Children[0] == nil {
		t.Fatalf("expected 1 child under sequence")
	}
	op := seq.Children[0]
	if op.Kind != model.JobRunStoryNodeKindOp {
		t.Fatalf("expected op kind, got %q", op.Kind)
	}
	if op.Status != model.JobRunStoryNodeStatusRunning {
		t.Fatalf("expected op status running, got %q", op.Status)
	}
	if len(op.Children) != 1 || op.Children[0] == nil {
		t.Fatalf("expected 1 child under op")
	}
	step := op.Children[0]
	if step.Kind != model.JobRunStoryNodeKindOpStep {
		t.Fatalf("expected opStep kind, got %q", step.Kind)
	}
	if step.Status != model.JobRunStoryNodeStatusRunning {
		t.Fatalf("expected step status running, got %q", step.Status)
	}
}
