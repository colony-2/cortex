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

func TestStoryReplay_IgnoresJobTypeTaskRuns(t *testing.T) {
	type in struct{}
	type out struct {
		Ok bool `json:"ok"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_ignore_jobtype"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{Ok: true}, nil
	}))

	recipeYAML := `
id: test
version: "1.0.0"
sequence:
  - id: run
    op: test_ignore_jobtype
outputs:
  ok: true
`
	var rec recipe.Recipe
	if err := yaml.Unmarshal([]byte(recipeYAML), &rec); err != nil {
		t.Fatalf("parse recipe: %v", err)
	}

	stepEnv := func(outPayload map[string]any) []byte {
		env, err := coretasks.NewOutputEnvelope(coretasks.OutputKindActivityInvocationOutput, outPayload)
		if err != nil {
			t.Fatalf("NewOutputEnvelope: %v", err)
		}
		b, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		return b
	}

	now := time.Date(2026, 2, 9, 20, 10, 0, 0, time.UTC)
	tasks := []swf.TaskRun{
		// Job-level "recipe" chapters can appear in the task list and should not block story replay.
		{
			TaskRunID: "recipe:0",
			TaskType:  "recipe",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   0,
					Attempt:   1,
					WorkerID:  "w0",
					CreatedAt: now,
					State:     swf.TaskAttemptStateSucceeded,
					Outcome:   swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
					Output:    &swf.TaskIO{Data: []byte(`{"job_level":"ok"}`)},
				},
			},
		},
		{
			TaskRunID: "test_ignore_jobtype:test_ignore_jobtype:1",
			TaskType:  "test_ignore_jobtype:test_ignore_jobtype",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   1,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: now.Add(1 * time.Second),
					State:     swf.TaskAttemptStateSucceeded,
					Outcome:   swf.TaskOutcome{Status: swf.TaskOutcomeStatusSucceeded},
					Input:     &swf.TaskIO{Data: []byte(`{"input":{},"context":{"NodePath":"root/run/test_ignore_jobtype","InvokeSeq":0}}`)},
					Output: &swf.TaskIO{Data: stepEnv(map[string]any{
						"git":    map[string]any{},
						"output": map[string]any{"ok": true},
					})},
				},
			},
		},
	}

	attempts := []swf.JobAttempt{{Attempt: 1, Tasks: tasks}}
	jobCtx := NewStoryBuildingContext(nil, "tenant", swf.JobKey{TenantId: "tenant", JobId: "job"}, "recipe", swf.JobStatusCompleted, attempts, nil)
	exec := newExecutor("tenant", "job", jobCtx)
	_, _, execErr := exec.ExecuteRecipe(&rec, map[string]interface{}{}, contextual.JobContext{}, contextual.GitCommitContext{})
	if execErr != nil {
		t.Fatalf("expected replay success, got %v", execErr)
	}
}
