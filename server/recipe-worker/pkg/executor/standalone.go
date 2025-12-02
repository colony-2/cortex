package executor

import (
	"context"
	"encoding/json"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.uber.org/zap"
)

// StandaloneExecutor executes recipes without a Temporal server
type StandaloneExecutor struct {
	registry *ops.ActivityRegistry
	logger   *zap.Logger
}

// NewStandaloneExecutor creates a new standalone recipe executor
func NewStandaloneExecutor(registry *ops.ActivityRegistry, logger *zap.Logger) (*StandaloneExecutor, error) {

	return &StandaloneExecutor{
		registry: registry,
		logger:   logger,
	}, nil
}

// Execute runs a recipe with the given inputs
func (e *StandaloneExecutor) Execute(
	ctx context.Context,
	r recipe.Recipe,
	inputs map[string]interface{},
	execCtx compiler.ExecutionContext,
) (map[string]interface{}, error) {

	workset, err := compiler.NewRecipeWorker(e.registry)
	if err != nil {
		return nil, err
	}

	eng := toy.NewToyEngine([]swf.WorkSet{*workset})

	job := compiler.StartJob{
		RecipeName: r.GetMetadata().ID,
		Inputs:     inputs,
		Context:    execCtx,
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
