package cortex

import (
	"sync"

	gitexport "github.com/colony-2/c2j/pkg/git/export"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/c2j/pkg/ops/extensions"
	recipeops "github.com/colony-2/c2j/pkg/ops/recipe"
	"github.com/colony-2/c2j/pkg/worker/compiler"
	workerexport "github.com/colony-2/c2j/pkg/worker/export"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

var registerReplayOps sync.Once

func configureReplay(engine jobworkflow.Engine) error {
	// Replay needs the same operation definitions as c2j to reconstruct task
	// inputs and the story tree. No task workers or polling loop run in Cortex.
	registerReplayOps.Do(func() {
		definitions := []ops.RegisterableOp{extensions.GetExecutionOp()}
		definitions = append(definitions, workerexport.GetAll()...)
		definitions = append(definitions, input.GetOp(), input.GetAutoFillOp())
		definitions = append(definitions, recipeops.GetOps()...)
		definitions = append(definitions, gitexport.GetAll()...)
		ops.Register(definitions...)
	})
	return engine.RegisterWorkers(&jobworkflow.WorkSet{
		JobWorker: compiler.NewRecipeJobWorker(compiler.RecipeJobWorkerOptions{ReadOnlyReplay: true}),
	})
}
