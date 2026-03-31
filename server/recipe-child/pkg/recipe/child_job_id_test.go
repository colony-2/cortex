package recipe

import (
	"context"
	"reflect"
	"testing"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/segmentio/ksuid"
)

func TestDeterministicChildJobID(t *testing.T) {
	invocation := contextual.Invocation{NodePath: "workflow/steps/start", InvokeSeq: 7}

	id := deterministicChildJobID("parent-job", invocation, 0)
	if id != deterministicChildJobID("parent-job", invocation, 0) {
		t.Fatal("expected identical input to produce the same child job id")
	}
	if _, err := ksuid.Parse(id); err != nil {
		t.Fatalf("expected child job id to be parseable as ksuid, got %q: %v", id, err)
	}
	if other := deterministicChildJobID("parent-job", invocation, 1); other == id {
		t.Fatal("expected different recipe indices to produce different child job ids")
	}
	if other := deterministicChildJobID("parent-job", contextual.Invocation{NodePath: "workflow/steps/other", InvokeSeq: 7}, 0); other == id {
		t.Fatal("expected different node paths to produce different child job ids")
	}
	if other := deterministicChildJobID("parent-job", contextual.Invocation{NodePath: invocation.NodePath, InvokeSeq: 8}, 0); other == id {
		t.Fatal("expected different invocation sequences to produce different child job ids")
	}
}

func TestStartSingleJobPassesDeterministicJobID(t *testing.T) {
	ctl := &fakeWorkflowControl{}
	deps := coreops.NewOpDependenciesBuilder().
		WithWorkflowControl(ctl).
		WithJobTool(&fakeJobTool{key: swf.JobKey{TenantId: "tenant", JobId: "parent-job"}}).
		WithGitContext(coreops.GitExecutionContext{NodePath: "workflow/steps/start", InvokeSeq: 5}).
		Build()

	out, err := startSingleJob(deps, context.Background(), SingleRecipeWithRef{
		SingleRecipe: SingleRecipe{Name: "child-recipe"},
		GitRef:       "main",
	})
	if err != nil {
		t.Fatalf("startSingleJob: %v", err)
	}
	if len(ctl.startRequests) != 1 {
		t.Fatalf("expected one child start, got %d", len(ctl.startRequests))
	}

	want := deterministicChildJobID("parent-job", contextual.Invocation{NodePath: "workflow/steps/start", InvokeSeq: 5}, 0)
	if ctl.startRequests[0].JobID != want {
		t.Fatalf("expected explicit child job id %q, got %q", want, ctl.startRequests[0].JobID)
	}
	if out.JobId != want {
		t.Fatalf("expected started job output to reuse deterministic job id %q, got %q", want, out.JobId)
	}
}

func TestStartMultipleJobsReusesSameIDsAcrossReruns(t *testing.T) {
	ctl := &fakeWorkflowControl{}
	deps := coreops.NewOpDependenciesBuilder().
		WithWorkflowControl(ctl).
		WithJobTool(&fakeJobTool{key: swf.JobKey{TenantId: "tenant", JobId: "parent-job"}}).
		WithGitContext(coreops.GitExecutionContext{NodePath: "workflow/steps/fanout", InvokeSeq: 11}).
		Build()

	input := MultipleRecipes{
		GitRef: "main",
		Recipes: []SingleRecipe{
			{Name: "child-a"},
			{Name: "child-b"},
		},
	}

	first, err := startMultipleJobs(deps, context.Background(), input)
	if err != nil {
		t.Fatalf("first startMultipleJobs: %v", err)
	}
	second, err := startMultipleJobs(deps, context.Background(), input)
	if err != nil {
		t.Fatalf("second startMultipleJobs: %v", err)
	}

	if !reflect.DeepEqual(first.JobIDs, second.JobIDs) {
		t.Fatalf("expected reruns to return the same ordered child job ids, got %v and %v", first.JobIDs, second.JobIDs)
	}
	if len(ctl.startRequests) != 4 {
		t.Fatalf("expected four start requests across two runs, got %d", len(ctl.startRequests))
	}
	if ctl.startRequests[0].JobID != ctl.startRequests[2].JobID || ctl.startRequests[1].JobID != ctl.startRequests[3].JobID {
		t.Fatalf("expected reruns to submit the same per-index child job ids, got %q/%q and %q/%q",
			ctl.startRequests[0].JobID, ctl.startRequests[2].JobID, ctl.startRequests[1].JobID, ctl.startRequests[3].JobID)
	}
	if ctl.startRequests[0].JobID == ctl.startRequests[1].JobID {
		t.Fatal("expected different fan-out indices to use different child job ids")
	}
}
