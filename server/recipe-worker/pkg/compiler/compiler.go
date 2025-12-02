package compiler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/template"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflow"
)

func ExecuteRecipe(ctx workflow.Context, activityRegistry *workerops.ActivityRegistry, r recipe.Recipe, recipeInputs map[string]interface{}, execCtx workerops.ExecutionContext) (map[string]interface{}, error) {

	// TODO: validate the inputSchema matches the actual inputs.

	rCtx, err := template.NewRecipeResolutionContext(recipeInputs, execCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolution context: %w", err)
	}

	metadata := r.GetMetadata().NodeMetadata

	switch t := r.RecipeImpl.(type) {
	case *recipe.RecipeState:
		err = executeStateMachine(ctx, rCtx, activityRegistry, metadata, t.Outputs, t.StateData.States)
	case *recipe.RecipeOp:
		err = executeOp(ctx, rCtx, activityRegistry, metadata, t.OpData.Op)
	case *recipe.RecipeSequence:
		err = executeSequence(ctx, rCtx, activityRegistry, metadata, t.Outputs, t.SequenceData.Sequence)
	default:
		return nil, fmt.Errorf("unsupported recipe type: %T", t)
	}

	if err != nil {
		return nil, err
	}
	return rCtx.GetLastExecution(), nil
}

// ExecuteWorkflow implements the WorkflowExecutor interface for unified recipes
func executeNode(ctx workflow.Context, parentResCtx *template.ResolutionContext, activityRegistry *workerops.ActivityRegistry, n *recipe.Node) error {
	metadata := n.GetMetadata()
	switch t := n.NodeImpl.(type) {
	case *recipe.NodeState:
		return executeStateMachine(ctx, parentResCtx, activityRegistry, metadata, t.Outputs, t.StateData.States)
	case *recipe.NodeOp:
		return executeOp(ctx, parentResCtx, activityRegistry, metadata, t.OpData.Op)
	case *recipe.NodeSequence:
		return executeSequence(ctx, parentResCtx, activityRegistry, metadata, t.Outputs, t.SequenceData.Sequence)
	default:
		return fmt.Errorf("unsupported recipe type: %T", t)
	}
}

// WorkflowState maintains the runtime state of a workflow
type WorkflowState struct {
	Inputs  ops.RawMessageOrStruct
	Steps   map[string]StepResult
	Outputs ops.RawMessageOrStruct
	Context map[string]interface{}
}

// StepResult stores the result of a workflow step
type StepResult struct {
	Outputs ops.RawMessageOrStruct
}

// executeOperation executes a single operation node
func executeOp(ctx workflow.Context, parentResolutionContext *template.ResolutionContext, activityRegistry *workerops.ActivityRegistry, metadata recipe.NodeMetadata, op string) error {

	resCtx, err := parentResolutionContext.NewChildContext(template.ScopeOp, metadata, op, nil)
	if err != nil {
		return fmt.Errorf("failed to create resolution context: %w", err)
	}
	resolvedNodeInputs, err := resCtx.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve templates op inputs: %w", err)
	}

	// Execute the operation
	retry := swf.RetryPolicy{}
	if metadata.Retry != nil {
		retry = *metadata.Retry
	}

	runPolicy := swf.RunPolicy{
		Retry: retry,
	}
	if metadata.Timeout > 0 {
		timeout := swf.Duration(metadata.Timeout)
		runPolicy.TotalTimeout = &timeout
	}

	workspacePayload, err := workspacePayloadFromContext(resCtx.TaskExecutionContext())
	if err != nil {
		return fmt.Errorf("build workspace payload: %w", err)
	}

	invocation := workerops.ActivityInvocationRequest{
		Input:     resolvedNodeInputs,
		TaskCtx:   resCtx.TaskExecutionContext(),
		Workspace: workspacePayload,
	}

	taskData, err := swf.NewTaskData(invocation)
	if err != nil {
		return err
	}

	out, err := ctx.DoTask(
		runPolicy,
		op,
		taskData,
	)

	if err != nil {
		return err
	}

	outputData, err := out.GetData()
	if err != nil {
		return err
	}

	var envelope workerops.ActivityInvocationOutput
	if err := json.Unmarshal(outputData, &envelope); err != nil {
		return fmt.Errorf("decode activity output envelope: %w", err)
	}

	if err != nil {
		return err
	}

	parentResolutionContext.AddExecution(envelope.OpOutput)
	// TODO: capture the output of the git workspace...
	return nil
}

func workspacePayloadFromContext(execCtx workerops.TaskExecutionContext) (gitstate.WorkspacePayload, error) {
	var payload gitstate.WorkspacePayload
	ctx := execCtx.Git

	if ctx.BaseRepo == "" {
		return payload, fmt.Errorf("execution context missing git.base_repo")
	}
	if ctx.BaseHash == "" {
		return payload, fmt.Errorf("execution context missing git.base_hash")
	}
	if ctx.WorktreePath == "" {
		return payload, fmt.Errorf("execution context missing environment.worktree_path")
	}
	if ctx.BlobStoreURI == "" {
		return payload, fmt.Errorf("execution context missing environment.blob_store_uri")
	}

	if ctx.PersistHash == "" {
		ctx.PersistHash = ctx.BaseHash
	}
	if ctx.PreviousHash == "" {
		ctx.PreviousHash = ctx.PersistHash
	}
	if ctx.GitAuthor == "" && ctx.GetCellName() != "" {
		ctx.GitAuthor = fmt.Sprintf("%s <%s@vibethis>", ctx.CellName, ctx.CellName)
	}

	payload = gitstate.WorkspacePayload{
		Context:        ctx,
		GitPersistHash: ctx.PersistHash,
		TicketID:       ctx.TicketID,
		CellName:       ctx.CellName,
	}
	return payload, nil
}

func innerSequence(ctx workflow.Context, parentCtx *template.ResolutionContext, activityRegistry *workerops.ActivityRegistry, metadata recipe.NodeMetadata, outputTemplate recipe.OutputMap, sequence []recipe.Node) error {
	// Create resolution context for this sequence
	resolvedInputs, err := parentCtx.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve sequence inputs: %w", err)
	}

	resCtx, err := parentCtx.NewChildContext(template.ScopeSequence, metadata, "", resolvedInputs)
	if err != nil {
		return fmt.Errorf("failed to create resolution context: %w", err)
	}

	for i, node := range sequence {
		// Execute the node
		err := executeNode(ctx, resCtx, activityRegistry, &node)
		if err != nil {
			return fmt.Errorf("sequence node %d failed: %w", i, err)
		}
	}

	outputs, err := resCtx.ResolveMap(outputTemplate)
	if err != nil {
		return fmt.Errorf("failed to resolve sequence outputs: %w", err)
	}

	// add resolved output to parent context.
	parentCtx.AddExecution(outputs)
	return nil
}

func executeSequence(ctx workflow.Context, rCtx *template.ResolutionContext, activityRegistry *workerops.ActivityRegistry, metadata recipe.NodeMetadata, outputTemplate recipe.OutputMap, sequence []recipe.Node) error {
	timeout := time.Duration(metadata.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	fn := func(inner workflow.Context) error {
		e := innerSequence(inner, rCtx, activityRegistry, metadata, outputTemplate, sequence)
		return e
	}
	err := executeCompositeInEnvelope(ctx, metadata.Retry, timeout, fn)

	return err
}

// executeCompositeInEnvelope executes a composite nodes in a retry/timeout envelope
func executeCompositeInEnvelope(ctx workflow.Context, retry *recipe.RetryPolicy, timeoutDuration time.Duration, fn func(inner workflow.Context) error) error {
	// TODO: update composite executions to respect retry policy and timeouts.
	return fn(ctx)
}
