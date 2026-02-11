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

func TestStoryReplay_UnwrapsEnvelopePayloadForNodeInput(t *testing.T) {
	type in struct{}
	type out struct {
		Ok bool `json:"ok"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_unwrap_env_input"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{Ok: true}, nil
	}))

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_unwrap_env_input
outputs:
  ok: true
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	// Simulate a chapter that stores an OutputEnvelope in the attempt input.
	inPayload := map[string]any{"hello": "world"}
	inEnv, err := coretasks.NewOutputEnvelope(coretasks.OutputKindActivityInvocationOutput, inPayload)
	if err != nil {
		t.Fatalf("NewOutputEnvelope(input): %v", err)
	}
	inBytes, err := json.Marshal(inEnv)
	if err != nil {
		t.Fatalf("marshal input envelope: %v", err)
	}

	outPayload := map[string]any{"git": map[string]any{}, "output": map[string]any{"ok": true}}
	outEnv, err := coretasks.NewOutputEnvelope(coretasks.OutputKindActivityInvocationOutput, outPayload)
	if err != nil {
		t.Fatalf("NewOutputEnvelope(output): %v", err)
	}
	outBytes, err := json.Marshal(outEnv)
	if err != nil {
		t.Fatalf("marshal output envelope: %v", err)
	}

	now := time.Date(2026, 2, 8, 2, 0, 0, 0, time.UTC)
	tasks := []swf.TaskRun{
		{
			TaskRunID: "test_unwrap_env_input:test_unwrap_env_input:1",
			TaskType:  "test_unwrap_env_input:test_unwrap_env_input",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   1,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: now,
					Input:     &swf.TaskIO{Data: inBytes},
					Output:    &swf.TaskIO{Data: outBytes},
					State:     swf.TaskAttemptStateSucceeded,
					Outcome:   swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
				},
			},
		},
	}

	res := runRecipeReplay(t, &rec, map[string]interface{}{}, contextual.JobContext{}, contextual.GitCommitContext{}, swf.JobStatusActive, tasks)
	if res.err != nil {
		t.Fatalf("ExecuteRecipe: %v", res.err)
	}

	root := res.exec.Root()
	if root == nil || len(root.Children) == 0 || root.Children[0] == nil || len(root.Children[0].Children) == 0 || root.Children[0].Children[0] == nil {
		t.Fatalf("expected recipe -> sequence -> op structure")
	}
	op := root.Children[0].Children[0]

	inMap, ok := op.Input.(map[string]any)
	if !ok {
		t.Fatalf("expected op input to be a map, got %T", op.Input)
	}
	if inMap["hello"] != "world" {
		t.Fatalf("expected unwrapped payload in input, got %#v", op.Input)
	}
	if _, hasV := inMap["v"]; hasV {
		t.Fatalf("expected envelope wrapper removed from input, got %#v", op.Input)
	}
	if _, hasKind := inMap["kind"]; hasKind {
		t.Fatalf("expected envelope wrapper removed from input, got %#v", op.Input)
	}
	if _, hasPayload := inMap["payload"]; hasPayload {
		t.Fatalf("expected envelope wrapper removed from input, got %#v", op.Input)
	}
}
