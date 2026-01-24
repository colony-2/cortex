package compiler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"

	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
)

func ExecuteRecipe(ctx workflow.Context, r recipe.Recipe, rawRecipeInputs map[string]interface{}, execCtx contextual.JobContext, commitContext contextual.GitCommitContext, opts ...ExecutionOptions) (map[string]interface{}, []swf.Artifact, error) {

	jobKey := ctx.GetJobKey()
	execCtx.Workflow.JobID = jobKey.JobId
	execCtx.Workflow.ProjectId = jobKey.TenantId

	// we forward thin packs from one task to the next to maintain state.
	ctx.JobContext = newThinPackForwardingJobContext(ctx.JobContext)

	execOpts := normalizeExecutionOptions(opts)
	recipeInputs, err := prepareRecipeInputs(r.GetMetdata(), rawRecipeInputs, execOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("recipe inputs do not match schema. %w", err)
	}

	if execOpts.Mode == ExecutionModeValidate {
		ctx = wrapValidationContext(ctx, commitContext)
	}

	rCtx, err := template.NewRecipeResolutionContext(&commitContext, recipeInputs, execCtx, resolutionOptionsFromExecution(execOpts))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resolution context: %w", err)
	}

	metadata := r.GetMetadata().NodeMetadata

	switch t := r.RecipeImpl.(type) {
	case *recipe.RecipeState:
		err = executeStateMachine(ctx, rCtx, metadata, t.Outputs, t.StateData.States)
	case *recipe.RecipeOp:
		err = executeOp(ctx, rCtx, metadata, t.OpData.Op)
	case *recipe.RecipeSequence:
		err = executeSequence(ctx, rCtx, metadata, t.Outputs, t.SequenceData.Sequence)
	default:
		return nil, nil, fmt.Errorf("unsupported recipe type: %T", t)
	}

	if err != nil {
		return nil, nil, err
	}
	return rCtx.GetLastExecution(), rCtx.GetLastArtifacts(), nil
}

// ExecuteWorkflow implements the WorkflowExecutor interface for unified recipes
func executeNode(ctx workflow.Context, parentResCtx *template.ResolutionContext, n *recipe.Node) error {
	metadata := n.GetMetadata()
	switch t := n.NodeImpl.(type) {
	case *recipe.NodeState:
		return executeStateMachine(ctx, parentResCtx, metadata, t.Outputs, t.StateData.States)
	case *recipe.NodeOp:
		return executeOp(ctx, parentResCtx, metadata, t.OpData.Op)
	case *recipe.NodeSequence:
		return executeSequence(ctx, parentResCtx, metadata, t.Outputs, t.SequenceData.Sequence)
	default:
		return fmt.Errorf("unsupported recipe type: %T", t)
	}
}

// StepResult stores the result of a workflow step
type StepResult struct {
	Outputs map[string]interface{}
}

func executeOp(ctx workflow.Context, parentResolutionContext *template.ResolutionContext, metadata recipe.NodeMetadata, op string) error {
	l := slog.Default()
	l.Info("executing op", "op", op)
	err := executeOp2(ctx, parentResolutionContext, metadata, op)
	if err != nil {
		l.Error("failed to execute op", "op", op, "err", err)
		return err
	}

	l.Info("op executed successfully", "op", op)
	return nil
}

// executeOperation executes a single operation node
func executeOp2(ctx workflow.Context, parentResolutionContext *template.ResolutionContext, metadata recipe.NodeMetadata, op string) error {

	if metadata.Inputs == nil {
		metadata.Inputs = map[string]interface{}{}
	}
	artifactsDefined := metadata.Artifacts != nil
	if metadata.Artifacts == nil {
		metadata.Artifacts = recipe.InputMap{}
	}

	resCtx, err := parentResolutionContext.NewChildContext(template.ScopeOp, metadata, op, nil)
	if err != nil {
		return fmt.Errorf("failed to create resolution context: %w", err)
	}

	// Inject defaults before template resolution
	registeredOp, exists := ops.Get(op)
	if !exists {
		return fmt.Errorf("operation %s not found", op)
	}
	if artifactsDefined && !registeredOp.GetMetadata().AcceptsArtifacts {
		return fmt.Errorf("operation %s does not accept artifacts", op)
	}

	chain := registeredOp.TaskChain()
	if len(chain) > 0 {
		if err := workerops.InjectDefaults(chain[0].InputType, metadata.Inputs); err != nil {
			return fmt.Errorf("failed to inject defaults: %w", err)
		}
	}

	resolvedNodeInputs, err := resCtx.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve templates op inputs: %w", err)
	}

	resolvedArtifacts, err := resolveArtifactBindings(resCtx, map[string]interface{}(metadata.Artifacts))
	if err != nil {
		return err
	}

	if len(chain) > 0 {
		allowNulls := resCtx.Options.Mode == string(ExecutionModeValidate)
		if err := validateOpInputType(chain[0].InputType, resolvedNodeInputs, allowNulls); err != nil {
			return fmt.Errorf("op input validation failed: %w", err)
		}
	}

	artifactKeys, err := collectArtifactKeysFromInput(resolvedNodeInputs)
	if err != nil {
		return fmt.Errorf("failed to collect artifact keys: %w", err)
	}
	if len(resolvedArtifacts) > 0 {
		artifactKeys = appendArtifactKeys(artifactKeys, resolvedArtifacts)
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

	stepInput := resolvedNodeInputs

	var stepArtifacts map[string]swf.Artifact
	for i := 0; i < 64; i++ { // guard against accidental loops
		taskType := fmt.Sprintf("%s:%s", op, chain[i].Name)
		invocation := workerops.ActivityInvocationRequest{
			Input:          stepInput,
			GitTaskContext: *gitstate.NewGlobalGitTaskContext(resCtx.TaskExecutionContext()),
			ArtifactKeys:   artifactKeys,
			Artifacts:      resolvedArtifacts,
		}

		taskData, err := swf.NewTaskData(invocation)
		if err != nil {
			return err
		}

		out, err := ctx.DoTask(
			runPolicy,
			taskType,
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
		decoder := json.NewDecoder(strings.NewReader(string(outputData)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil {
			return fmt.Errorf("decode activity output envelope: %w", err)
		}

		gitResult := envelope.GitResult
		resCtx.UpdateGitState(gitResult)
		stepInput = normalizeOpOutput(chain[i].OutputType, envelope.OpOutput)
		if envelope.NextTask == "" {
			outputArtifacts, err := out.GetArtifacts()
			if err != nil {
				return err
			}
			stepArtifacts = artifactsToMap(outputArtifacts)
			break
		}
		taskType = envelope.NextTask
	}
	resCtx.AddExecutionWithArtifacts(stepInput, stepArtifacts)
	return nil
}

func innerSequence(ctx workflow.Context, parentCtx *template.ResolutionContext, metadata recipe.NodeMetadata, outputTemplate map[string]interface{}, sequence []recipe.Node) error {
	// Create resolution context for this sequence
	resolvedInputs, err := parentCtx.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve sequence inputs: %w", err)
	}

	resCtx, err := parentCtx.NewChildContext(template.ScopeSequence, metadata, "", resolvedInputs)
	if err != nil {
		return fmt.Errorf("failed to create resolution context: %w", err)
	}
	if err := seedSequencePlaceholders(resCtx, sequence); err != nil {
		return err
	}

	for i, node := range sequence {
		// Execute the node
		err := executeNode(ctx, resCtx, &node)
		if err != nil {
			return fmt.Errorf("sequence node %d failed: %w", i, err)
		}
	}

	outputs, err := resCtx.ResolveMap(outputTemplate)
	if err != nil {
		return fmt.Errorf("failed to resolve sequence outputs: %w", err)
	}

	// add resolved output to parent context.
	parentCtx.AddExecutionWithArtifacts(outputs, lastSequenceArtifacts(resCtx, sequence))
	return nil
}

func executeSequence(ctx workflow.Context, rCtx *template.ResolutionContext, metadata recipe.NodeMetadata, outputTemplate map[string]interface{}, sequence []recipe.Node) error {
	timeout := time.Duration(metadata.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	fn := func(inner workflow.Context) error {
		e := innerSequence(inner, rCtx, metadata, outputTemplate, sequence)
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
