package story

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	coretasks "github.com/colony-2/colony2/server/recipe-core/pkg/task"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

type fakeReplayEngine struct {
	jobInput    swf.JobData
	taskOutputs []swf.TaskData
}

func (e *fakeReplayEngine) ReplayJobRun(ctx context.Context, req swf.ReplayRunRequest) (swf.JobData, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.JobWorker == nil {
		return nil, swf.ReplayCacheMissError{JobKey: req.JobKey, Ordinal: 0, Attempt: 1, Reason: swf.ReplayCacheMissJobResultMissing}
	}
	obs := req.Observer
	if obs != nil {
		obs.OnJobStart(swf.JobStartEvent{JobKey: req.JobKey, AttemptNumber: 1, Input: e.jobInput})
	}
	jc := &fakeReplayJobContext{
		jobKey:      req.JobKey,
		observer:    obs,
		nextOrdinal: 1,
		outputs:     append([]swf.TaskData{}, e.taskOutputs...),
	}
	out, err := req.JobWorker.Run(jc, e.jobInput)
	if obs != nil {
		obs.OnJobEnd(swf.JobEndEvent{JobKey: req.JobKey, AttemptNumber: 1, Output: out, Err: err})
	}
	return out, err
}

type fakeReplayJobContext struct {
	jobKey   swf.JobKey
	observer swf.ReplayObserver

	nextOrdinal int64
	outputs     []swf.TaskData
}

func (c *fakeReplayJobContext) GetJobKey() swf.JobKey { return c.jobKey }
func (c *fakeReplayJobContext) Logger() *slog.Logger  { return nil }

func (c *fakeReplayJobContext) AwaitDuration(_ swf.Duration) error { return nil }
func (c *fakeReplayJobContext) AwaitJobs(_ ...string) error        { return nil }

func (c *fakeReplayJobContext) DoTask(_ swf.RunPolicy, taskType string, input swf.TaskData) (swf.TaskData, error) {
	ord := c.nextOrdinal
	c.nextOrdinal++
	if c.observer != nil {
		c.observer.OnTaskStart(swf.TaskStartEvent{
			JobKey:        c.jobKey,
			TaskType:      taskType,
			Ordinal:       ord,
			AttemptNumber: 1,
			Input:         input,
		})
	}

	if len(c.outputs) == 0 {
		err := swf.ReplayCacheMissError{
			JobKey:   c.jobKey,
			TaskType: taskType,
			Ordinal:  ord,
			Attempt:  1,
			Reason:   swf.ReplayCacheMissTaskResultMissing,
		}
		if c.observer != nil {
			c.observer.OnTaskEnd(swf.TaskEndEvent{JobKey: c.jobKey, TaskType: taskType, Ordinal: ord, AttemptNumber: 1, Output: nil, Err: err})
		}
		return nil, err
	}

	out := c.outputs[0]
	c.outputs = c.outputs[1:]
	if c.observer != nil {
		c.observer.OnTaskEnd(swf.TaskEndEvent{JobKey: c.jobKey, TaskType: taskType, Ordinal: ord, AttemptNumber: 1, Output: out, Err: nil})
	}
	return out, nil
}

var _ swf.JobContext = (*fakeReplayJobContext)(nil)

func TestJobRunStoryReplay_BuildsTreeAndNormalizesMissingOutputKeys(t *testing.T) {
	type in struct{}
	type out struct {
		Present string `json:"present"`
		Missing string `json:"missing,omitempty"`
	}
	coreops.Register(coreops.NewActivityMappedOpV2[in, out](coreops.OpMetadata{Type: "test_story_normalize"}, func(_ coreops.OpDependencies, _ context.Context, _ in) (out, error) {
		return out{Present: "ok"}, nil
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
	recipeArt := swf.NewArtifactFromBytes("test"+starter.RecipeArtifactSuffix, []byte(recipeYAML))
	start := workflowctl.StartJob{
		RecipeName: "test",
		GitRef:     "main",
		Inputs:     map[string]any{},
		JobContext: contextual.JobContext{
			GitBase: contextual.GitBaseContext{BaseRepo: "/src", BaseRef: "main"},
			Workflow: contextual.WorkflowContext{
				CellPath: "server",
				CellName: "test",
			},
		},
	}
	jobInput, err := swf.NewTaskData(start, recipeArt)
	if err != nil {
		t.Fatalf("NewTaskData(job start): %v", err)
	}

	env, err := coretasks.NewOutputEnvelope(coretasks.OutputKindActivityInvocationOutput, map[string]any{
		"git":          map[string]any{},
		"nextTaskType": "",
		"output":       map[string]any{"present": "ok"},
	})
	if err != nil {
		t.Fatalf("NewOutputEnvelope: %v", err)
	}
	outBytes, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	taskOut := &swf.SimpleTaskData{Data: json.RawMessage(outBytes)}

	engine := &fakeReplayEngine{
		jobInput:    swf.JobData(jobInput),
		taskOutputs: []swf.TaskData{taskOut},
	}

	jobKey := swf.JobKey{TenantId: "tenant", JobId: "job"}
	st, err := BuildJobRunStory(context.Background(), engine, jobKey, nil, nil)
	if err != nil {
		t.Fatalf("BuildJobRunStory: %v", err)
	}
	if st == nil || st.Root == nil || st.Root.Kind != model.JobRunStoryNodeKindRecipe {
		t.Fatalf("expected recipe root, got %#v", st)
	}
	if st.Status != model.WorkflowStatusCompleted {
		t.Fatalf("expected story status completed, got %q", st.Status)
	}
	outMap, ok := st.Root.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected root output map, got %T", st.Root.Output)
	}
	if outMap["present"] != "ok" {
		t.Fatalf("expected present=%q, got %#v", "ok", outMap["present"])
	}
	if outMap["missing"] != "" {
		t.Fatalf("expected missing=%q, got %#v", "", outMap["missing"])
	}
}

func TestJobRunStoryReplay_UsesInjectedCELOptionsProvider(t *testing.T) {
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
	recipeArt := swf.NewArtifactFromBytes("test"+starter.RecipeArtifactSuffix, []byte(recipeYAML))
	start := workflowctl.StartJob{
		RecipeName: "test",
		GitRef:     "main",
		Inputs:     map[string]any{},
		JobContext: contextual.JobContext{},
	}
	jobInput, err := swf.NewTaskData(start, recipeArt)
	if err != nil {
		t.Fatalf("NewTaskData(job start): %v", err)
	}

	engine := &fakeReplayEngine{jobInput: swf.JobData(jobInput)}
	jobKey := swf.JobKey{TenantId: "tenant", JobId: "job"}
	st, err := BuildJobRunStory(context.Background(), engine, jobKey, builder, nil)
	if err != nil {
		t.Fatalf("BuildJobRunStory: %v", err)
	}
	if st == nil || st.Root == nil {
		t.Fatalf("expected root")
	}
	outMap, ok := st.Root.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected root output map, got %T", st.Root.Output)
	}
	if outMap["greet"] != "hi" {
		t.Fatalf("expected greet=%q, got %#v", "hi", outMap["greet"])
	}
}
