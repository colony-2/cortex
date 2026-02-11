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

func TestStoryReplay_ConsumesContextPatchChapterAndAppliesJobPatch(t *testing.T) {
	// The recipe parser validates op existence against the global ops registry.
	type in struct {
		Foo string `json:"foo"`
	}
	type out struct {
		Ok bool `json:"ok"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_activity"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{Ok: true}, nil
	}))

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_activity
    inputs:
      foo: bar
outputs:
  author: "{{ context.git.author }}"
`

	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	patch := coretasks.ContextPatch{
		Job: map[string]any{
			"git": map[string]any{
				"author": "new",
			},
		},
	}
	patchEnv, err := coretasks.NewOutputEnvelope(coretasks.OutputKindContextPatch, patch)
	if err != nil {
		t.Fatalf("NewOutputEnvelope(context_patch): %v", err)
	}
	patchBytes, err := json.Marshal(patchEnv)
	if err != nil {
		t.Fatalf("marshal patch envelope: %v", err)
	}

	actPayload := map[string]any{
		"git":          map[string]any{},
		"nextTaskType": "",
		"output":       map[string]any{"ok": true},
	}
	actEnv, err := coretasks.NewOutputEnvelope(coretasks.OutputKindActivityInvocationOutput, actPayload)
	if err != nil {
		t.Fatalf("NewOutputEnvelope(activity_output): %v", err)
	}
	actBytes, err := json.Marshal(actEnv)
	if err != nil {
		t.Fatalf("marshal activity envelope: %v", err)
	}

	now := time.Date(2026, 2, 6, 12, 0, 0, 0, time.UTC)
	tasks := []swf.TaskRun{
		{
			TaskRunID: "recipe-worker:1",
			TaskType:  "recipe-worker",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   1,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: now,
					Input:     &swf.TaskIO{Data: json.RawMessage(`{"kind":"context_patch"}`)},
					Output:    &swf.TaskIO{Data: patchBytes},
					State:     swf.TaskAttemptStateSucceeded,
					Outcome:   swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
				},
			},
		},
		{
			TaskRunID: "test_activity:test_activity:2",
			TaskType:  "test_activity:test_activity",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   2,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: now.Add(1 * time.Second),
					Input: &swf.TaskIO{Data: json.RawMessage(
						`{"input":{"foo":"bar"},"context":{"InvokeSeq":1}}`,
					)},
					Output:  &swf.TaskIO{Data: actBytes},
					State:   swf.TaskAttemptStateSucceeded,
					Outcome: swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
				},
			},
		},
	}

	jobContext := contextual.JobContext{
		GitBase: contextual.GitBaseContext{GitAuthor: "old"},
	}
	res := runRecipeReplay(t, &rec, map[string]interface{}{}, jobContext, contextual.GitCommitContext{}, swf.JobStatusActive, tasks)
	if res.err != nil {
		t.Fatalf("ExecuteRecipe: %v", res.err)
	}

	root := res.exec.Root()
	if root == nil {
		t.Fatalf("expected story root")
	}

	outMap, ok := root.Output.(map[string]interface{})
	if !ok {
		t.Fatalf("expected root output to be a map, got %T", root.Output)
	}
	if outMap["author"] != "new" {
		t.Fatalf("expected patched author 'new', got %#v", outMap["author"])
	}

	// Walk recipe -> sequence -> op.
	if len(root.Children) != 1 || root.Children[0] == nil {
		t.Fatalf("expected 1 child under root")
	}
	seq := root.Children[0]
	if len(seq.Children) != 1 || seq.Children[0] == nil {
		t.Fatalf("expected 1 child under sequence")
	}
	op := seq.Children[0]
	if len(op.Children) < 2 {
		t.Fatalf("expected op to contain contextPatch and step children, got %d children", len(op.Children))
	}
	foundPatch := false
	for _, ch := range op.Children {
		if ch == nil {
			continue
		}
		if ch.Kind == model.JobRunStoryNodeKindContextPatch {
			foundPatch = true
			if ch.TaskOrdinal == nil || *ch.TaskOrdinal != 1 {
				t.Fatalf("expected contextPatch task_ordinal=1, got %#v", ch.TaskOrdinal)
			}
		}
	}
	if !foundPatch {
		t.Fatalf("expected a contextPatch node under op")
	}
}
