package input

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/stretchr/testify/require"
)

type gen struct {
	count int
	max   int
}

func (g *gen) Generate(tenantId string) (swf.JobKey, error) {
	g.count++
	if g.count > g.max {
		return swf.JobKey{}, fmt.Errorf("too many jobs")
	}
	return swf.JobKey{TenantId: tenantId, JobId: fmt.Sprintf("job-%d", g.count)}, nil
}

func TestSimpleInput(t *testing.T) {
	op := GetOp()
	opR := op.GetManagementService().(*inputManagementService)
	coreops.Register(op)
	recipeYaml := `
---
id: test-recipe
op: input
inputs:
  form:
    question: "how old are you"
`

	testRecipe, err := recipe.LoadRecipeFromString([]byte(recipeYaml))
	require.NoError(t, err)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)
	g := gen{max: 1}
	eng := toy.NewToyEngine([]swf.WorkSet{}, toy.WithJobIDGenerator(g.Generate))

	wf := workflow.SWFWorkflowControl{
		Engine: eng,
	}
	deps := coreops.NewServiceDepsBuilder().
		WithWorkflowControl(&wf).
		WithSSEManager(NewSimpleSSEManager()).
		Build()
	require.NoError(t, op.GetManagementService().Initialize(deps))

	workSet, err := compiler.NewRecipeWorker(deps, registry)
	require.NoError(t, eng.RegisterWorkers(workSet))
	jobCtx, gitCtx := compiler.GenerateTestContext()
	in := map[string]interface{}{}

	job := workflowctl.StartJob{
		TenantId:   "test-tenant",
		RecipeName: testRecipe.GetMetadata().ID,
		Inputs:     in,
		JobContext: jobCtx,
		GitRef:     gitCtx.ParentRef,
	}

	errCh := make(chan error)
	go func() {
		_, err := starter.StartRecipeJob(context.Background(), job, eng, *testRecipe)
		errCh <- err
	}()

	time.Sleep(300 * time.Millisecond)

	inputs, err := opR.collectPendingInputs(context.Background(), "test-tenant")
	require.NoError(t, err)
	require.Equal(t, 1, len(inputs))
	pending := inputs[0]

	result := opR.getDetails(context.Background(), "test-tenant", pending.JobID)
	if result.hasError() {
		t.Fatalf("failed to get result: %v", result.err)
	}
	details := result.value
	require.Equal(t, "how old are you", details.Form.Question)
	res2 := opR.submitResponse(context.Background(), "test-tenant", pending.JobID, FormResponse{
		Response: "foolish",
		Hash:     "abc123",
	})
	if res2.hasError() {
		t.Fatalf("failed to submit response: %v", res2.err)
	}
	err = <-errCh
	require.NoError(t, err)

	res3, err := eng.GetJobResult(context.Background(), swf.JobKey{TenantId: "test-tenant", JobId: "job-1"})
	require.NoError(t, err)

	res4, err := res3.GetData()
	require.NoError(t, err)

	air := Output{}
	require.NoError(t, json.Unmarshal(res4, &air))

	require.Equal(t, "foolish", air.Response)

}
