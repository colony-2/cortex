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
	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"gopkg.in/yaml.v3"
)

func TestStoryReplay_NormalizesOpOutputsToAvoidMissingKeyFailures(t *testing.T) {
	type in struct{}
	type out struct {
		Present string `json:"present"`
		Missing string `json:"missing"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_story_normalize"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{}, nil
	}))

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_story_normalize
outputs:
  present: "{{ sequence.run.outputs.present }}"
  missing: "{{ sequence.run.outputs.missing }}"
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	env, err := coretasks.NewOutputEnvelope(coretasks.OutputKindActivityInvocationOutput, map[string]any{
		"git":    map[string]any{},
		"output": map[string]any{"present": "ok"},
	})
	if err != nil {
		t.Fatalf("NewOutputEnvelope: %v", err)
	}
	outBytes, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	tasks := []swf.TaskRun{
		{
			TaskRunID: "test_story_normalize:test_story_normalize:1",
			TaskType:  "test_story_normalize:test_story_normalize",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   1,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: now,
					Input:     &swf.TaskIO{Data: []byte(`{"input":{},"context":{"InvokeSeq":1}}`)},
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
	if res.out == nil {
		t.Fatalf("expected output map")
	}
	if res.out["present"] != "ok" {
		t.Fatalf("expected present=%q, got %#v", "ok", res.out["present"])
	}
	if res.out["missing"] != "" {
		t.Fatalf("expected missing=%q, got %#v", "", res.out["missing"])
	}
}

func TestStoryReplay_UsesInjectedCELOptionsProvider(t *testing.T) {
	builder := funcregistry.NewBuilder().WithDefaults().WithBuiltin("hello", func(_ types.Adapter, _ funcregistry.ContextProvider) cel.EnvOption {
		return cel.Function(
			"hello",
			cel.Overload(
				"hello_0",
				[]*cel.Type{},
				cel.StringType,
				cel.FunctionBinding(func(...ref.Val) ref.Val { return types.String("hi") }),
			),
		)
	})

	recipeYAML := `
id: test
version: "1.0.0"
sequence: []
outputs:
  greet: "{{ hello() }}"
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	res := runRecipeReplay(
		t,
		&rec,
		map[string]interface{}{},
		contextual.JobContext{},
		contextual.GitCommitContext{},
		swf.JobStatusActive,
		[]swf.TaskRun{},
		compiler.ExecutionOptions{CELOptionsProvider: builder},
	)
	if res.err != nil {
		t.Fatalf("ExecuteRecipe: %v", res.err)
	}
	if res.out == nil || res.out["greet"] != "hi" {
		t.Fatalf("expected greet=%q, got %#v", "hi", res.out)
	}
}
