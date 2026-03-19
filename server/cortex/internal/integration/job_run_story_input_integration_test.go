package integration

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sort"
	"testing"
	"time"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	inputop "github.com/colony-2/colony2/server/recipe-input/pkg/input"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	workerwf "github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/colony2/server/workflow/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	directruntime "github.com/colony-2/swf-go/pkg/swf/runtime/direct"
	directtestsupport "github.com/colony-2/swf-go/pkg/swf/runtime/direct/testsupport"
	"github.com/stretchr/testify/require"
)

func TestGetJobRunStory_WaitingOnInput_ShowsPendingInputOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restoreOps := captureAndClearOpsRegistry(t)

	type empty struct{}
	type stepOut struct {
		Ok bool `json:"ok"`
	}
	multiStep := coreops.NewOp().
		WithType("test_multistep_story").
		AddStep("start", coreops.NewStep(func(_ context.Context, _ empty) (stepOut, error) { return stepOut{Ok: true}, nil })).
		AddStep("finish", coreops.NewStep(func(_ context.Context, _ stepOut) (stepOut, error) { return stepOut{Ok: true}, nil })).
		BuildOrPanic()

	inputOp := inputop.GetOp()
	coreops.Register(multiStep, inputOp, inputop.GetAutoFillOp())
	t.Cleanup(restoreOps)

	engine := startEmbeddedEngine(t, ctx)

	// Initialize input op management service (installs capability handlers used by NoTask steps).
	wfCtl := workerwf.SWFWorkflowControl{Engine: engine}
	deps := coreops.NewServiceDepsBuilder().
		WithWorkflowControl(&wfCtl).
		WithSSEManager(inputop.NewSimpleSSEManager()).
		Build()
	require.NoError(t, inputOp.GetManagementService().Initialize(deps))
	t.Cleanup(func() { inputOp.GetManagementService().Close() })

	recipeYAML := `
---
id: test-story-waiting-input
version: "0.0.1"
input_schema:
  prompt:
    type: string
    default_value: "Are you there?"
inputs:
  p1: "{{ inputs.prompt }}"
sequence:
  - id: one
    op: test_multistep_story
  - id: two
    op: test_multistep_story
  - id: q1
    op: input
    inputs:
      form:
        question: "{{ inputs.p1 }}"
outputs:
  ok: true
`
	rec, err := recipe.LoadRecipeFromString([]byte(recipeYAML))
	require.NoError(t, err)

	jobKey := startRecipeJob(t, ctx, engine, rec, map[string]any{"prompt": "Are you there?"})

	// Wait until the workflow is blocked waiting on input.
	require.Eventually(t, func() bool {
		run, err := engine.GetJobRun(ctx, swf.GetJobRunRequest{
			JobKey:               jobKey,
			IncludeInputs:        true,
			IncludeOutputs:       true,
			IncludeArtifacts:     true,
			IncludeAttemptInputs: true,
		})
		require.NoError(t, err)
		return hasWaitingCollectUserInput(run.Attempts)
	}, 30*time.Second, 100*time.Millisecond)

	svc, err := workflow.New(workflow.ServiceConfig{Engine: engine})
	require.NoError(t, err)
	story, err := svc.GetJobRunStory(ctx, workflow.GetJobRunStoryRequest{
		ProjectID: "test-tenant",
		JobID:     jobKey.JobId,
	})
	require.NoError(t, err)
	require.NotNil(t, story)
	require.NotNil(t, story.Root)
	require.Equal(t, workflow.WorkflowStatusRunning, story.Status)

	var (
		foundInputOp        *workflow.JobRunStoryNode
		foundCollectWaiting *workflow.JobRunStoryNode
		multiStepOps        []*workflow.JobRunStoryNode
	)

	var walk func(n *workflow.JobRunStoryNode)
	walk = func(n *workflow.JobRunStoryNode) {
		if n == nil {
			return
		}
		if string(n.Kind) == "op" && n.OpID == "input" {
			foundInputOp = n
		}
		if string(n.Kind) == "op" && n.OpID == "test_multistep_story" {
			multiStepOps = append(multiStepOps, n)
		}
		if string(n.Kind) == "opStep" && n.StepID == "collect_user_input" {
			foundCollectWaiting = n
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(story.Root)

	require.Len(t, multiStepOps, 2, "expected two separate invocations of the multistep op in the story")
	require.NotNil(t, foundInputOp, "expected input op node to be present while waiting on input")
	require.Equal(t, workflow.JobRunStoryNodeStatus("running"), foundInputOp.Status)
	require.NotNil(t, foundCollectWaiting, "expected collect_user_input step node to be present while waiting on input")
	require.Equal(t, workflow.JobRunStoryNodeStatus("running"), foundCollectWaiting.Status)
}

func TestGetJobRunStory_WaitingOnInput_WithClearedOpsRegistry_ReturnsMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restoreOps := captureAndClearOpsRegistry(t)

	inputOp := inputop.GetOp()
	coreops.Register(inputOp, inputop.GetAutoFillOp())
	t.Cleanup(restoreOps)

	engine := startEmbeddedEngine(t, ctx)

	wfCtl := workerwf.SWFWorkflowControl{Engine: engine}
	deps := coreops.NewServiceDepsBuilder().
		WithWorkflowControl(&wfCtl).
		WithSSEManager(inputop.NewSimpleSSEManager()).
		Build()
	require.NoError(t, inputOp.GetManagementService().Initialize(deps))
	t.Cleanup(func() { inputOp.GetManagementService().Close() })

	recipeYAML := `
---
id: test-story-waiting-input-missing-registry
version: "0.0.1"
sequence:
  - id: q1
    op: input
    inputs:
      form:
        question: "Hello?"
outputs:
  ok: true
`
	rec, err := recipe.LoadRecipeFromString([]byte(recipeYAML))
	require.NoError(t, err)

	jobKey := startRecipeJob(t, ctx, engine, rec, map[string]any{})

	require.Eventually(t, func() bool {
		run, err := engine.GetJobRun(ctx, swf.GetJobRunRequest{JobKey: jobKey, IncludeOutputs: true, IncludeAttemptInputs: true})
		require.NoError(t, err)
		return hasWaitingCollectUserInput(run.Attempts)
	}, 30*time.Second, 100*time.Millisecond)

	// Simulate the API process not having the recipe ops registry populated.
	coreops.Clear()

	svc, err := workflow.New(workflow.ServiceConfig{Engine: engine})
	require.NoError(t, err)
	_, err = svc.GetJobRunStory(ctx, workflow.GetJobRunStoryRequest{
		ProjectID: "test-tenant",
		JobID:     jobKey.JobId,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, workflow.ErrJobRunStoryMismatch), "expected ErrJobRunStoryMismatch, got %v", err)
}

func TestGetJobRunStory_InputAutoFill_DoesNotMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restoreOps := captureAndClearOpsRegistry(t)

	inputOp := inputop.GetOp()
	coreops.Register(inputOp, inputop.GetAutoFillOp())
	t.Cleanup(restoreOps)

	engine := startEmbeddedEngine(t, ctx)

	wfCtl := workerwf.SWFWorkflowControl{Engine: engine}
	deps := coreops.NewServiceDepsBuilder().
		WithWorkflowControl(&wfCtl).
		WithSSEManager(inputop.NewSimpleSSEManager()).
		Build()
	require.NoError(t, inputOp.GetManagementService().Initialize(deps))
	t.Cleanup(func() { inputOp.GetManagementService().Close() })

	recipeYAML := `
---
id: test-story-input-autofill
version: "0.0.1"
sequence:
  - id: q1
    op: input
    inputs:
      form:
        question: "Hello?"
        type: "short_answer"
        autofill:
          response: "APPROVE"
outputs:
  ok: true
`
	rec, err := recipe.LoadRecipeFromString([]byte(recipeYAML))
	require.NoError(t, err)

	jobKey := startRecipeJob(t, ctx, engine, rec, map[string]any{})

	require.Eventually(t, func() bool {
		st, err := engine.CheckJobStatus(ctx, jobKey)
		require.NoError(t, err)
		return st == swf.JobStatusCompleted
	}, 30*time.Second, 100*time.Millisecond)

	svc, err := workflow.New(workflow.ServiceConfig{Engine: engine})
	require.NoError(t, err)
	story, err := svc.GetJobRunStory(ctx, workflow.GetJobRunStoryRequest{
		ProjectID: "test-tenant",
		JobID:     jobKey.JobId,
	})
	require.NoError(t, err)
	require.NotNil(t, story)
	require.Equal(t, workflow.WorkflowStatusCompleted, story.Status)
}

func startEmbeddedEngine(t *testing.T, ctx context.Context) swf.SWFEngine {
	t.Helper()

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	workerDeps := coreops.NewServiceDepsBuilder().Build()
	workSet, err := compiler.NewRecipeWorker(workerDeps, registry, nil)
	require.NoError(t, err)

	dsn, stopPG, err := directtestsupport.StartEmbeddedPostgres()
	require.NoError(t, err)
	t.Cleanup(stopPG)

	sqlDB, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, directtestsupport.InstallPGWF(ctx, sqlDB))

	strata, err := directtestsupport.StartEmbeddedStrata()
	require.NoError(t, err)
	t.Cleanup(func() { strata.Shutdown() })

	taskWorkers := make([]swf.TaskWorker, 0, len(workSet.TaskWorkers))
	for _, tw := range workSet.TaskWorkers {
		taskWorkers = append(taskWorkers, tw)
	}
	swfRuntime, err := directruntime.NewFromConfig(dsn, strata.BaseURL, strata.APIKey)
	require.NoError(t, err)
	engine, err := swf.NewEngineBuilder().
		WithRuntime(swfRuntime).
		WithLogger(slog.Default()).
		WithMaxActive(100).
		PlusWorkers(workSet.JobWorker, taskWorkers...).
		BuildEngine()
	require.NoError(t, err)

	go engine.Run(ctx)
	return engine
}

func startRecipeJob(t *testing.T, ctx context.Context, engine swf.SWFEngine, rec *recipe.Recipe, inputs map[string]any) swf.JobKey {
	t.Helper()

	jobCtx, gitCtx := compiler.GenerateTestContext()
	job := workflowctl.StartJob{
		TenantId:   "test-tenant",
		RecipeName: rec.GetMetadata().ID,
		Inputs:     inputs,
		JobContext: jobCtx,
		GitRef:     gitCtx.ParentRef,
	}
	jobKey, err := starter.StartRecipeJob(ctx, job, engine, *rec)
	require.NoError(t, err)
	return jobKey
}

func hasWaitingCollectUserInput(attempts []swf.JobAttempt) bool {
	for _, ja := range attempts {
		for _, tr := range ja.Tasks {
			if tr.TaskType != "recipe:input:collect_user_input" && tr.TaskType != "input:collect_user_input" {
				continue
			}
			if len(tr.Attempts) == 0 {
				continue
			}
			st := tr.Attempts[len(tr.Attempts)-1].State
			switch st {
			case swf.TaskAttemptStateReady, swf.TaskAttemptStateWaiting, swf.TaskAttemptStateLeased, swf.TaskAttemptStateRunning:
				return true
			default:
			}
		}
	}
	return false
}

func captureAndClearOpsRegistry(t *testing.T) func() {
	t.Helper()

	orig := coreops.List()
	unique := make(map[string]coreops.RegisterableOp, len(orig))
	for _, op := range orig {
		if op == nil {
			continue
		}
		name := op.GetName()
		if name == "" {
			name = op.GetMetadata().Type
		}
		if name == "" {
			continue
		}
		unique[name] = op
	}
	coreops.Clear()

	return func() {
		coreops.Clear()
		ops := make([]coreops.RegisterableOp, 0, len(unique))
		for _, op := range unique {
			ops = append(ops, op)
		}
		// Keep ordering stable to make debugging easier if this ever flakes.
		sort.SliceStable(ops, func(i, j int) bool { return ops[i].GetName() < ops[j].GetName() })
		coreops.Register(ops...)
	}
}
