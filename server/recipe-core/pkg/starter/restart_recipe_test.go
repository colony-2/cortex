package starter

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/task"
	"github.com/colony-2/swf-go/pkg/swf"
)

type captureEngine struct {
	last *swf.RestartJob
}

func (c *captureEngine) ReplayJobRun(ctx context.Context, req swf.ReplayRunRequest) (swf.JobData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *captureEngine) RegisterWorkers(*swf.WorkSet) error { return nil }
func (c *captureEngine) Run(context.Context)                {}
func (c *captureEngine) StartJob(context.Context, swf.StartJob) (swf.JobKey, error) {
	return swf.JobKey{}, nil
}
func (c *captureEngine) RestartJob(_ context.Context, req swf.RestartJob) (swf.JobKey, error) {
	c.last = &req
	return swf.JobKey{TenantId: req.PriorJobKey.TenantId, JobId: "restarted"}, nil
}
func (c *captureEngine) CancelJob(context.Context, swf.CancelJob) error { return nil }
func (c *captureEngine) CheckJobStatus(context.Context, swf.JobKey) (swf.JobStatus, error) {
	return swf.JobStatusCompleted, nil
}
func (c *captureEngine) GetJobResult(context.Context, swf.JobKey) (swf.TaskData, error) {
	return nil, nil
}
func (c *captureEngine) FindTasksWaitingForCapability(context.Context, string, string, []string) ([]swf.TaskHandle, error) {
	return nil, nil
}
func (c *captureEngine) GetWaitingTask(context.Context, swf.JobKey) (swf.TaskHandle, error) {
	return nil, nil
}
func (c *captureEngine) GetArtifact(string, swf.ArtifactKey) (swf.Artifact, error) { return nil, nil }
func (c *captureEngine) ListJobs(context.Context, swf.ListJobsRequest) (swf.ListJobsResponse, error) {
	return swf.ListJobsResponse{}, nil
}
func (c *captureEngine) GetJobRun(context.Context, swf.GetJobRunRequest) (swf.GetJobRunResponse, error) {
	return swf.GetJobRunResponse{}, nil
}

var _ swf.SWFEngine = &captureEngine{}

func TestRestartRecipeJob_NoPatch(t *testing.T) {
	engine := &captureEngine{}
	prior := swf.JobKey{TenantId: "t1", JobId: "j1"}

	_, err := RestartRecipeJob(context.Background(), engine, prior, 3, nil)
	if err != nil {
		t.Fatalf("RestartRecipeJob: %v", err)
	}
	if engine.last == nil {
		t.Fatalf("expected RestartJob call")
	}
	if engine.last.PriorJobKey != prior {
		t.Fatalf("unexpected prior job key: %#v", engine.last.PriorJobKey)
	}
	if engine.last.LastStepToKeep != 2 {
		t.Fatalf("expected LastStepToKeep=2, got %d", engine.last.LastStepToKeep)
	}
	if engine.last.ExtraTaskOutput != nil {
		t.Fatalf("expected no ExtraTaskOutput")
	}
}

func TestRestartRecipeJob_WithPatch_InjectsEnvelope(t *testing.T) {
	engine := &captureEngine{}
	prior := swf.JobKey{TenantId: "t1", JobId: "j1"}
	patch := &task.ContextPatch{
		Job: map[string]any{"git": map[string]any{"author": "new-author"}},
	}

	_, err := RestartRecipeJob(context.Background(), engine, prior, 0, patch)
	if err != nil {
		t.Fatalf("RestartRecipeJob: %v", err)
	}
	if engine.last == nil {
		t.Fatalf("expected RestartJob call")
	}
	if engine.last.LastStepToKeep != -1 {
		t.Fatalf("expected LastStepToKeep=-1 for stepOffset=0, got %d", engine.last.LastStepToKeep)
	}
	if engine.last.ExtraTaskInput == nil {
		t.Fatalf("expected ExtraTaskInput")
	}
	if engine.last.ExtraTaskOutput == nil {
		t.Fatalf("expected ExtraTaskOutput")
	}

	raw, err := engine.last.ExtraTaskOutput.GetData()
	if err != nil {
		t.Fatalf("get ExtraTaskOutput data: %v", err)
	}
	var env task.OutputEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Kind != task.OutputKindContextPatch {
		t.Fatalf("expected kind %q, got %q", task.OutputKindContextPatch, env.Kind)
	}
	var decoded task.ContextPatch
	if err := env.DecodePayload(&decoded); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if decoded.Job["git"] == nil {
		t.Fatalf("expected decoded job patch")
	}
}
