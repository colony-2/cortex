package executor

import (
	"context"
	"encoding/json"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	ops2 "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"go.uber.org/zap"
)

// StandaloneExecutor executes recipes without a Temporal server
type StandaloneExecutor struct {
	registry *ops.ActivityRegistry
	logger   *zap.Logger
	deps     ops2.ServiceDependencies2
}

// NewStandaloneExecutor creates a new standalone recipe executor
func NewStandaloneExecutor(deps ops2.ServiceDependencies2, registry *ops.ActivityRegistry, logger *zap.Logger) (*StandaloneExecutor, error) {
	return &StandaloneExecutor{
		registry: registry,
		logger:   logger,
		deps:     deps,
	}, nil
}

// Execute runs a recipe with the given inputs
func (e *StandaloneExecutor) Execute(
	ctx context.Context,
	r recipe.Recipe,
	inputs map[string]interface{},
	jobCtx contextual.JobContext,
	gitCtx contextual.GitCommitContext,
) (map[string]interface{}, error) {

	workset, err := compiler.NewRecipeWorker(e.deps, e.registry)
	if err != nil {
		return nil, err
	}

	eng := toy.NewToyEngine([]swf.WorkSet{*workset})

	job := workflowctl.StartJob{
		RecipeName: r.GetMetadata().ID,
		Inputs:     inputs,
		JobContext: jobCtx,
		GitContext: gitCtx,
	}

	id, err := compiler.StartRecipeJob(ctx, job, eng, r)
	if err != nil {
		return nil, err
	}
	out, err := eng.GetJobResult(ctx, id)
	if err != nil {
		return nil, err
	}
	d, err := out.GetData()
	if err != nil {
		return nil, err
	}
	outMap := make(map[string]interface{})
	err = json.Unmarshal(d, &outMap)
	return outMap, err
}

// GetActivityRegistry returns the activity registry
func (e *StandaloneExecutor) GetActivityRegistry() *ops.ActivityRegistry {
	return e.registry
}
