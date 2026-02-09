package story

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	coretasks "github.com/colony-2/colony2/server/recipe-core/pkg/task"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
)

type activityInvocationOutput struct {
	GitResult contextual.GitCommitContext `json:"git,omitempty"`
	NextTask  string                      `json:"nextTaskType,omitempty"`
	OpOutput  any                         `json:"output"`
}

type activityInvocationRequest struct {
	Input        any                           `json:"input"`
	GitTaskCtx   gitstate.GlobalGitTaskContext `json:"context"`
	ArtifactKeys []swf.ArtifactKey             `json:"artifact_keys,omitempty"`
	Artifacts    map[string]swf.ArtifactKey    `json:"artifacts,omitempty"`
}

type executor struct {
	projectID string
	jobID     string
	jobCtx    *StoryBuildingContext

	tree *treeBuilder
	root *model.JobRunStoryNode

	// consumeTarget points to the node currently associated with the next DoTask call.
	consumeTarget *model.JobRunStoryNode
}

func splitInvocationNodePath(nodePath string) []string {
	nodePath = strings.TrimSpace(nodePath)
	if nodePath == "" {
		return make([]string, 0)
	}
	parts := strings.Split(nodePath, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func setStoryNodePath(n *model.JobRunStoryNode, invocationNodePath string, extra ...string) {
	if n == nil {
		return
	}
	path := splitInvocationNodePath(invocationNodePath)
	for _, seg := range extra {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		path = append(path, seg)
	}
	n.Path = path
}

func newExecutor(projectID, jobID string, jobCtx *StoryBuildingContext) *executor {
	e := &executor{
		projectID: projectID,
		jobID:     jobID,
		jobCtx:    jobCtx,
		tree:      newTreeBuilder(),
	}
	if jobCtx != nil {
		jobCtx.SetOnConsume(e.onConsume)
	}
	return e
}

func (e *executor) Root() *model.JobRunStoryNode {
	return e.root
}

func (e *executor) ExecuteRecipe(r *recipe.Recipe, inputs map[string]interface{}, execCtx contextual.JobContext, commitCtx contextual.GitCommitContext) (map[string]interface{}, []swf.Artifact, error) {
	if r == nil {
		return nil, nil, fmt.Errorf("nil recipe")
	}

	rCtx, err := template.NewRecipeResolutionContext(&commitCtx, inputs, execCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resolution context: %w", err)
	}

	root := e.tree.newNode(model.JobRunStoryNodeKindRecipe, "recipe "+strings.TrimSpace(r.GetMetadata().ID))
	root.RecipeID = strings.TrimSpace(r.GetMetadata().ID)
	root.Invocation = map[string]interface{}{"args": inputs}
	root.Input = inputs
	root.InvokeSeq = rCtx.TaskExecutionContext().Invocation.InvokeSeq
	setStoryNodePath(root, rCtx.TaskExecutionContext().Invocation.NodePath)
	e.tree.push("root", root)
	e.root = root

	meta := r.GetMetadata().NodeMetadata
	var execErr error
	switch t := r.RecipeImpl.(type) {
	case *recipe.RecipeState:
		execErr = e.ExecuteStateMachine(rCtx, meta, t.Outputs, t.StateMachineData.States)
	case *recipe.RecipeOp:
		execErr = e.ExecuteOp(rCtx, meta, t.OpData.Op)
	case *recipe.RecipeSequence:
		execErr = e.ExecuteSequence(rCtx, meta, t.Outputs, t.SequenceData.Sequence)
	default:
		return nil, nil, fmt.Errorf("unsupported recipe type: %T", t)
	}

	out := rCtx.GetLastExecution()
	arts := rCtx.GetLastArtifacts()

	// Mark root terminal status + output.
	if execErr != nil {
		if errors.Is(execErr, ErrReplayInProgress) {
			root.Status = model.JobRunStoryNodeStatusRunning
		} else {
			root.Status = model.JobRunStoryNodeStatusFailed
		}
		root.Output = out
		e.tree.pop()
		return out, arts, execErr
	}
	root.Status = model.JobRunStoryNodeStatusSucceeded
	root.Output = out
	e.tree.pop()
	return out, arts, nil
}

func (e *executor) ExecuteNode(parentCtx *template.ResolutionContext, n *recipe.Node) error {
	if n == nil {
		return nil
	}
	metadata := n.GetMetadata()
	switch t := n.NodeImpl.(type) {
	case *recipe.NodeState:
		return e.ExecuteStateMachine(parentCtx, metadata, t.Outputs, t.StateMachineData.States)
	case *recipe.NodeOp:
		return e.ExecuteOp(parentCtx, metadata, t.OpData.Op)
	case *recipe.NodeSequence:
		return e.ExecuteSequence(parentCtx, metadata, t.Outputs, t.SequenceData.Sequence)
	case *recipe.NodeShared:
		return nil
	default:
		return fmt.Errorf("unsupported node type: %T", t)
	}
}

func (e *executor) ExecuteSequence(parentCtx *template.ResolutionContext, metadata recipe.NodeMetadata, outputTemplate map[string]interface{}, sequence []recipe.Node) error {
	if parentCtx == nil {
		return fmt.Errorf("nil parent context")
	}

	resolvedInputs, err := parentCtx.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve sequence inputs: %w", err)
	}

	seqID := template.ScopeID(metadata, "", template.ScopeSequence)
	node := e.tree.newNode(model.JobRunStoryNodeKindSequence, "sequence "+seqID)
	node.SequenceID = seqID
	node.Input = resolvedInputs
	node.Status = model.JobRunStoryNodeStatusRunning
	e.tree.push("sequence:"+seqID, node)

	resCtx, err := parentCtx.NewChildContext(template.ScopeSequence, metadata, "", resolvedInputs)
	if err != nil {
		node.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("failed to create sequence resolution context: %w", err)
	}
	node.InvokeSeq = resCtx.TaskExecutionContext().Invocation.InvokeSeq
	setStoryNodePath(node, resCtx.TaskExecutionContext().Invocation.NodePath)

	for i := range sequence {
		nodeDef := sequence[i]
		if err := e.ExecuteNode(resCtx, &nodeDef); err != nil {
			if errors.Is(err, ErrReplayInProgress) {
				node.Status = model.JobRunStoryNodeStatusRunning
			} else {
				node.Status = model.JobRunStoryNodeStatusFailed
			}
			e.tree.pop()
			return fmt.Errorf("sequence node %d failed: %w", i, err)
		}
	}

	outputs, err := resCtx.ResolveMap(outputTemplate)
	if err != nil {
		node.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("failed to resolve sequence outputs: %w", err)
	}
	parentCtx.AddExecutionWithArtifacts(outputs, lastSequenceArtifacts(resCtx, sequence))

	node.Output = outputs
	node.Status = model.JobRunStoryNodeStatusSucceeded
	e.tree.pop()
	return nil
}

func (e *executor) ExecuteStateMachine(parentCtx *template.ResolutionContext, metadata recipe.NodeMetadata, outputTemplate map[string]interface{}, stateMap *recipe.StateMap) error {
	if parentCtx == nil {
		return fmt.Errorf("nil parent context")
	}
	if stateMap == nil {
		return fmt.Errorf("nil state map")
	}

	resolvedInputs, err := parentCtx.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve state machine inputs: %w", err)
	}

	smID := template.ScopeID(metadata, "", template.ScopeStateMachine)
	node := e.tree.newNode(model.JobRunStoryNodeKindStateMachine, "state machine "+smID)
	node.StateMachineID = smID
	node.Input = resolvedInputs
	node.Status = model.JobRunStoryNodeStatusRunning
	e.tree.push("stateMachine:"+smID, node)

	resCtx, err := parentCtx.NewChildContext(template.ScopeStateMachine, metadata, "", resolvedInputs)
	if err != nil {
		node.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("failed to create state machine context: %w", err)
	}
	node.InvokeSeq = resCtx.TaskExecutionContext().Invocation.InvokeSeq
	setStoryNodePath(node, resCtx.TaskExecutionContext().Invocation.NodePath)

	currentState := strings.TrimSpace(stateMap.Initial)
	if currentState == "" {
		node.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("state machine initial state is required")
	}
	if _, ok := stateMap.States[currentState]; !ok {
		node.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("state '%s' not found", currentState)
	}

	firstState := true
	for {
		stateDef, ok := stateMap.States[currentState]
		if !ok {
			node.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			return fmt.Errorf("state '%s' not found", currentState)
		}

		stNode := e.tree.newNode(model.JobRunStoryNodeKindState, "state "+currentState)
		stNode.StateID = currentState
		if firstState {
			v := true
			stNode.IsInitial = &v
		} else {
			v := false
			stNode.IsInitial = &v
		}
		stNode.Status = model.JobRunStoryNodeStatusRunning
		e.tree.push("state:"+currentState, stNode)

		stateResCtx, err := resCtx.NewChildContext(template.ScopeState, stateDef.GetMetadata(), currentState, nil)
		if err != nil {
			stNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			node.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			return fmt.Errorf("failed to create state context: %w", err)
		}
		stNode.InvokeSeq = stateResCtx.TaskExecutionContext().Invocation.InvokeSeq
		setStoryNodePath(stNode, stateResCtx.TaskExecutionContext().Invocation.NodePath)

		if err := e.ExecuteNode(stateResCtx, &stateDef.Node); err != nil {
			if errors.Is(err, ErrReplayInProgress) {
				stNode.Status = model.JobRunStoryNodeStatusRunning
				node.Status = model.JobRunStoryNodeStatusRunning
			} else {
				stNode.Status = model.JobRunStoryNodeStatusFailed
				node.Status = model.JobRunStoryNodeStatusFailed
			}
			e.tree.pop()
			e.tree.pop()
			return fmt.Errorf("state '%s' execution failed: %w", currentState, err)
		}
		stNode.Status = model.JobRunStoryNodeStatusSucceeded

		// Terminal states end the machine after they run.
		if isTerminalState(currentState, stateMap.States) {
			e.tree.pop()
			break
		}

			evNode, nextState, err := e.evalTransitions(stateDef.Transitions, resCtx, stateResCtx, currentState)
			if err != nil {
				if errors.Is(err, ErrReplayInProgress) {
					stNode.Status = model.JobRunStoryNodeStatusRunning
					node.Status = model.JobRunStoryNodeStatusRunning
			} else {
				stNode.Status = model.JobRunStoryNodeStatusFailed
				node.Status = model.JobRunStoryNodeStatusFailed
			}
			e.tree.pop()
			e.tree.pop()
			return err
		}
		stNode.Children = append(stNode.Children, evNode)

		e.tree.pop()

		if nextState == "" {
			break
		}
		currentState = nextState
		firstState = false
	}

	finalStateDef := recipe.State{}
	if currentState != "" {
		finalStateDef = stateMap.States[currentState]
	}
	resolvedOutputs, err := resCtx.ResolveMap(outputTemplate)
	if err != nil {
		node.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("failed to resolve state machine outputs: %w", err)
	}
	parentCtx.AddExecutionWithArtifacts(resolvedOutputs, stateArtifacts(resCtx, currentState, finalStateDef))

	node.Output = resolvedOutputs
	node.Status = model.JobRunStoryNodeStatusSucceeded
	e.tree.pop()
	return nil
}

func (e *executor) evalTransitions(transitions []recipe.Transition, evalResCtx, invResCtx *template.ResolutionContext, fromState string) (*model.JobRunStoryNode, string, error) {
	if evalResCtx == nil {
		return nil, "", fmt.Errorf("nil transition evaluation context")
	}
	if invResCtx == nil {
		invResCtx = evalResCtx
	}

	// Temporary context sharing CEL env + template data.
	evalCtx := &template.ResolutionContext{
		ScopeType:    evalResCtx.ScopeType,
		TemplateData: evalResCtx.TemplateData,
		CELEnv:       evalResCtx.CELEnv,
	}

	node := e.tree.newNode(model.JobRunStoryNodeKindTransitionEval, "evaluate transitions")
	node.FromStateID = fromState
	node.Status = model.JobRunStoryNodeStatusSucceeded
	node.InvokeSeq = invResCtx.TaskExecutionContext().Invocation.InvokeSeq
	setStoryNodePath(node, invResCtx.TaskExecutionContext().Invocation.NodePath, "transitionEval")
	node.Attempt = 1
	node.PriorAttempts = make([]*model.JobRunStoryNode, 0)
	node.Input = nil
	node.Output = nil
	node.ArtifactKeys = make([]swf.ArtifactKey, 0)
	node.Children = make([]*model.JobRunStoryNode, 0)

	evals := make([]model.JobRunStoryTransitionEval, 0, len(transitions))
	nextState := ""
	for _, tr := range transitions {
		expr := strings.TrimSpace(tr.When.String())
		ok, err := evalCtx.EvaluateCEL(expr)
		if err != nil {
			return nil, "", fmt.Errorf("failed to evaluate transition condition: %w", err)
		}
		ev := model.JobRunStoryTransitionEval{
			ToStateID:  tr.To,
			Expression: expr,
			Result:     ok,
			Reason:     nil,
		}
		evals = append(evals, ev)
		if nextState == "" && ok {
			nextState = tr.To
		}
	}
	node.Evaluations = evals
	decision := &model.JobRunStoryTransitionDecision{}
	if nextState != "" {
		decision.Kind = "state"
		decision.ToStateID = &nextState
	} else {
		decision.Kind = "fallthrough"
	}
	node.Decision = decision
	return node, nextState, nil
}

func (e *executor) ExecuteOp(parentCtx *template.ResolutionContext, metadata recipe.NodeMetadata, op string) error {
	if parentCtx == nil {
		return fmt.Errorf("nil parent context")
	}
	op = strings.TrimSpace(op)
	if op == "" {
		return fmt.Errorf("missing op name")
	}

	opNode := e.tree.newNode(model.JobRunStoryNodeKindOp, "op "+op)
	opNode.OpID = op
	opNode.OpType = "custom"
	opNode.Status = model.JobRunStoryNodeStatusRunning
	e.tree.push("op:"+op, opNode)

	resCtx, err := parentCtx.NewChildContext(template.ScopeOp, metadata, op, nil)
	if err != nil {
		opNode.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("failed to create op resolution context: %w", err)
	}
	opNode.InvokeSeq = resCtx.TaskExecutionContext().Invocation.InvokeSeq
	setStoryNodePath(opNode, resCtx.TaskExecutionContext().Invocation.NodePath)

	prefix := op + ":"
	taskType, ok := e.jobCtx.NextTaskTypeForPrefix(prefix)
	if !ok {
		// When a job is still running, SWF may not have recorded any task runs for the next op
		// yet (or may only show a runtime placeholder). Treat that as in-progress rather than
		// a deterministic mismatch.
		if e.jobCtx != nil && isJobStatusRunning(e.jobCtx.jobStatus) {
			opNode.Status = model.JobRunStoryNodeStatusRunning
			e.tree.pop()
			return fmt.Errorf("%w: op %q not started (no task runs)", ErrReplayInProgress, op)
		}
		opNode.Status = model.JobRunStoryNodeStatusFailed
		e.tree.pop()
		return fmt.Errorf("%w: no task runs found for op %q", ErrReplayMismatch, op)
	}

	var (
		finalOut    any
		finalArts   map[string]swf.Artifact
		stepNodes   []*model.JobRunStoryNode
		currentTask = taskType
	)

	for i := 0; i < 64; i++ {
		if strings.TrimSpace(currentTask) == "" {
			opNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			return fmt.Errorf("%w: op %q task chain ended unexpectedly", ErrReplayMismatch, op)
		}

		// Restart-injected chapters (e.g. context patches) may appear in the story stream even
		// though they do not match the expected task type for this op step. Consume and apply
		// them before attempting the next task.
		for {
			nextTy, nextRun, ok := e.jobCtx.peekNextRunAny()
			if !ok {
				break
			}
			if strings.TrimSpace(nextTy) == strings.TrimSpace(currentTask) {
				break
			}

			// Only consume "out-of-order" chapters as context patches when the chapter is clearly
			// a context patch. Otherwise, treat it as a replay mismatch.
			if !isContextPatchRun(nextRun) {
				// If the job is still running and the next chapter is in a non-terminal state, stop
				// replay and surface "running" instead of treating it as a deterministic mismatch.
				if e.jobCtx != nil && isJobStatusRunning(e.jobCtx.jobStatus) && isTaskRunInProgress(nextRun) {
					opNode.Status = model.JobRunStoryNodeStatusRunning
					e.tree.pop() // op
					return fmt.Errorf("%w: op %q waiting on runtime task %q", ErrReplayInProgress, op, nextTy)
				}
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop() // op
				return fmt.Errorf("%w: unexpected task %q before %q", ErrReplayMismatch, nextTy, currentTask)
			}

			ord := int64(-1)
			if o, ok := taskRunFirstOrdinal(nextRun); ok {
				ord = o
			}

			patchNode := e.tree.newNode(model.JobRunStoryNodeKindContextPatch, "context patch")
			patchNode.Status = model.JobRunStoryNodeStatusRunning
			patchNode.InvokeSeq = resCtx.TaskExecutionContext().Invocation.InvokeSeq
			setStoryNodePath(patchNode, resCtx.TaskExecutionContext().Invocation.NodePath, fmt.Sprintf("contextPatch:%d", ord))
			e.tree.push(fmt.Sprintf("contextPatch:%d", ord), patchNode)

			e.consumeTarget = patchNode
			_, td, err := e.jobCtx.ConsumeNextAny()
			e.consumeTarget = nil
			if err != nil {
				patchNode.Status = statusFromErr(err, patchNode.Status)
				if errors.Is(err, ErrReplayInProgress) {
					opNode.Status = model.JobRunStoryNodeStatusRunning
				} else {
					opNode.Status = model.JobRunStoryNodeStatusFailed
				}
				e.tree.pop() // contextPatch
				e.tree.pop() // op
				return err
			}

			raw, err := td.GetData()
			if err != nil {
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop() // contextPatch
				e.tree.pop() // op
				return err
			}

			var env coretasks.OutputEnvelope
			if err := json.Unmarshal(raw, &env); err != nil {
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop() // contextPatch
				e.tree.pop() // op
				return fmt.Errorf("decode task output envelope: %w", err)
			}
			if env.Version != coretasks.OutputEnvelopeVersion {
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop() // contextPatch
				e.tree.pop() // op
				return fmt.Errorf("%w: unsupported task output envelope version %d", ErrReplayMismatch, env.Version)
			}
			if env.Kind != coretasks.OutputKindContextPatch {
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop() // contextPatch
				e.tree.pop() // op
				return fmt.Errorf("%w: unexpected task output kind %q before %q", ErrReplayMismatch, env.Kind, currentTask)
			}

			var patch coretasks.ContextPatch
			if err := env.DecodePayload(&patch); err != nil {
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop() // contextPatch
				e.tree.pop() // op
				return fmt.Errorf("decode context patch payload: %w", err)
			}
			if err := resCtx.ApplyContextPatch(patch); err != nil {
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop() // contextPatch
				e.tree.pop() // op
				return fmt.Errorf("apply context patch: %w", err)
			}

			patchNode.Status = model.JobRunStoryNodeStatusSucceeded
			e.tree.pop() // contextPatch
		}

		stepID := stepIDFromTaskType(op, currentTask)
		stepNode := e.tree.newNode(model.JobRunStoryNodeKindOpStep, "step "+stepID)
		stepNode.StepID = stepID
		stepNode.StepType = "other"
		stepNode.Status = model.JobRunStoryNodeStatusRunning
		stepNode.InvokeSeq = resCtx.TaskExecutionContext().Invocation.InvokeSeq
		setStoryNodePath(stepNode, resCtx.TaskExecutionContext().Invocation.NodePath, "step:"+stepID)
		e.tree.push("step:"+stepID, stepNode)
		stepNodes = append(stepNodes, stepNode)

		e.consumeTarget = stepNode
		td, err := e.jobCtx.DoTask(swf.RunPolicy{}, currentTask, swf.NewTaskDataOrPanic(map[string]interface{}{}))
		e.consumeTarget = nil
		if err != nil {
			// Step node was populated by onConsume; keep tree as-is and bubble the error to stop replay.
			stepNode.Status = statusFromErr(err, stepNode.Status)
			if errors.Is(err, ErrReplayInProgress) {
				opNode.Status = model.JobRunStoryNodeStatusRunning
			} else {
				opNode.Status = model.JobRunStoryNodeStatusFailed
			}
			e.tree.pop() // step
			e.tree.pop() // op
			return err
		}

		raw, err := td.GetData()
		if err != nil {
			opNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			e.tree.pop()
			return err
		}
		var outEnv coretasks.OutputEnvelope
		if err := json.Unmarshal(raw, &outEnv); err != nil {
			opNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			e.tree.pop()
			return fmt.Errorf("decode task output envelope: %w", err)
		}
		if outEnv.Version != coretasks.OutputEnvelopeVersion {
			opNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			e.tree.pop()
			return fmt.Errorf("%w: unsupported task output envelope version %d", ErrReplayMismatch, outEnv.Version)
		}
		if outEnv.Kind != coretasks.OutputKindActivityInvocationOutput {
			opNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			e.tree.pop()
			return fmt.Errorf("%w: unexpected task output kind %q for taskType=%q", ErrReplayMismatch, outEnv.Kind, currentTask)
		}

		var out activityInvocationOutput
		if err := outEnv.DecodePayload(&out); err != nil {
			opNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			e.tree.pop()
			return fmt.Errorf("decode activity output payload: %w", err)
		}

		resCtx.UpdateGitState(out.GitResult)
		finalOut = out.OpOutput

		next := strings.TrimSpace(out.NextTask)
		if next == "" {
			// If the chapter didn't explicitly provide nextTaskType, use the op definition
			// (registered in the global ops registry) to determine the next step. This avoids
			// guessing based on "whatever task run happens to come next", which can incorrectly
			// chain multiple invocations of the same op.
			if regNext, ok := nextTaskFromRegistry(op, currentTask); ok {
				next = strings.TrimSpace(regNext)
			}
		}
		if next == "" {
			arts, err := td.GetArtifacts()
			if err != nil {
				opNode.Status = model.JobRunStoryNodeStatusFailed
				e.tree.pop()
				e.tree.pop()
				return err
			}
			finalArts = artifactsToMap(arts)
			stepNode.Status = model.JobRunStoryNodeStatusSucceeded
			e.tree.pop() // step
			break
		}

		stepNode.Status = model.JobRunStoryNodeStatusSucceeded
		e.tree.pop() // step
		currentTask = next
	}

	if finalOut == nil {
		finalOut = map[string]interface{}{}
	}
	resCtx.AddExecutionWithArtifacts(asMap(finalOut), finalArts)

	// Attach op-level input/output.
	if len(stepNodes) > 0 {
		opNode.Input = stepNodes[0].Input
		opNode.Output = stepNodes[len(stepNodes)-1].Output
	}

	// Flatten single-step op where taskType == opId:opId.
	if len(opNode.Children) == 1 {
		ch := opNode.Children[0]
		if ch != nil && ch.Kind == model.JobRunStoryNodeKindOpStep && ch.StepID == op {
			opNode.Attempt = ch.Attempt
			opNode.PriorAttempts = ch.PriorAttempts
			opNode.Input = ch.Input
			opNode.Output = ch.Output
			opNode.ArtifactKeys = ch.ArtifactKeys
			opNode.TaskOrdinal = ch.TaskOrdinal
			opNode.RestartFromOrdinal = ch.RestartFromOrdinal
			opNode.StartedAt = ch.StartedAt
			opNode.FinishedAt = ch.FinishedAt
			opNode.Error = ch.Error
			opNode.Children = make([]*model.JobRunStoryNode, 0)
			opNode.InvokeSeq = ch.InvokeSeq
			opNode.Status = ch.Status
		}
	}

	// If not flattened, derive status from steps.
	if opNode.Status == model.JobRunStoryNodeStatusRunning {
		opNode.Status = deriveContainerStatus(opNode.Children)
	}

	e.tree.pop() // op
	return nil
}

func (e *executor) onConsume(taskType string, run swf.TaskRun, final swf.TaskAttempt) {
	n := e.consumeTarget
	if n == nil {
		return
	}

	// Populate attempt/priorAttempts from the run's attempts.
	if len(run.Attempts) == 0 {
		return
	}

	// Fill latest attempt.
	fillAttemptOnNode(n, taskType, run.Attempts[len(run.Attempts)-1], e.jobID)

	// Build prior attempts as nodes of the same kind.
	prior := make([]*model.JobRunStoryNode, 0, len(run.Attempts)-1)
	for i := 0; i < len(run.Attempts)-1; i++ {
		a := run.Attempts[i]
		pa := *n
		pa.ID = fmt.Sprintf("%s_a%d", n.ID, a.Attempt)
		pa.Attempt = a.Attempt
		pa.PriorAttempts = make([]*model.JobRunStoryNode, 0)
		pa.Children = make([]*model.JobRunStoryNode, 0)
		pa.ArtifactKeys = make([]swf.ArtifactKey, 0)
		pa.TaskOrdinal = nil
		pa.RestartFromOrdinal = nil
		fillAttemptOnNode(&pa, taskType, a, e.jobID)
		prior = append(prior, &pa)
	}
	n.PriorAttempts = prior
	n.Attempt = final.Attempt

	// Expose a safe restart ordinal for the logical node: attempt 1 (or earliest ordinal fallback).
	var restartOrd *int64
	for i := range run.Attempts {
		if run.Attempts[i].Attempt == 1 {
			v := run.Attempts[i].Ordinal
			restartOrd = &v
			break
		}
	}
	if restartOrd == nil && len(run.Attempts) > 0 {
		min := run.Attempts[0].Ordinal
		for i := 1; i < len(run.Attempts); i++ {
			if run.Attempts[i].Ordinal < min {
				min = run.Attempts[i].Ordinal
			}
		}
		restartOrd = &min
	}
	n.RestartFromOrdinal = restartOrd

	// started_at is always attempt created time.
	t := run.Attempts[0].CreatedAt
	n.StartedAt = &t

	// Status from final attempt state/outcome, unless already set.
	n.Status = statusFromAttempt(final)
}

func fillAttemptOnNode(n *model.JobRunStoryNode, taskType string, att swf.TaskAttempt, jobID string) {
	n.Attempt = att.Attempt
	n.Status = statusFromAttempt(att)
	t := att.CreatedAt
	n.StartedAt = &t
	n.TaskOrdinal = &att.Ordinal

	// Try to decode the activity invocation request so input and invoke_seq are recipe-centric.
	var req activityInvocationRequest
	if att.Input != nil && len(att.Input.Data) > 0 && json.Unmarshal(att.Input.Data, &req) == nil {
		n.Input = req.Input
		n.InvokeSeq = req.GitTaskCtx.InvokeSeq
		if strings.TrimSpace(req.GitTaskCtx.NodePath) != "" {
			if n.Kind == model.JobRunStoryNodeKindOpStep && strings.TrimSpace(n.StepID) != "" {
				setStoryNodePath(n, req.GitTaskCtx.NodePath, "step:"+strings.TrimSpace(n.StepID))
			} else {
				setStoryNodePath(n, req.GitTaskCtx.NodePath)
			}
		}
	}
	if n.Input == nil && att.Input != nil && len(att.Input.Data) > 0 {
		// Some non-activity chapters store a task envelope in the "input" field. For API consumers,
		// unwrap the envelope and return only the payload.
		if payload, ok := unwrapTaskEnvelopePayload(att.Input.Data); ok {
			n.Input = payload
		} else {
			// Best-effort: preserve raw input for unknown/non-envelope chapters.
			var raw any
			if json.Unmarshal(att.Input.Data, &raw) == nil {
				n.Input = raw
			}
		}
	}

	// Decode the task output envelope so output is recipe-centric.
	if att.Output != nil && len(att.Output.Data) > 0 {
		var outEnv coretasks.OutputEnvelope
		if json.Unmarshal(att.Output.Data, &outEnv) == nil && outEnv.Version == coretasks.OutputEnvelopeVersion {
			switch outEnv.Kind {
			case coretasks.OutputKindActivityInvocationOutput:
				var env activityInvocationOutput
				if outEnv.DecodePayload(&env) == nil {
					n.Output = env.OpOutput
				}
			case coretasks.OutputKindContextPatch:
				var patch coretasks.ContextPatch
				if outEnv.DecodePayload(&patch) == nil {
					n.Output = patch
				}
			default:
				// Unknown envelope kinds are preserved as raw bytes (best-effort).
				n.Output = map[string]any{"kind": string(outEnv.Kind)}
			}
		}
	}

	// Artifacts from output IO.
	keys := make([]swf.ArtifactKey, 0)
	if att.Output != nil {
		for _, info := range att.Output.Artifacts {
			if info.Key != nil {
				keys = append(keys, *info.Key)
				continue
			}
			keys = append(keys, swf.ArtifactKey{
				JobId:       jobID,
				TaskOrdinal: att.Ordinal,
				Name:        info.Name,
				SizeBytes:   info.SizeBytes,
			})
		}
	}
	n.ArtifactKeys = keys

	// Error object.
	if att.Outcome.Error != nil && strings.TrimSpace(att.Outcome.Error.Message) != "" {
		n.Error = &model.JobRunStoryError{
			Message: att.Outcome.Error.Message,
			Code:    strings.TrimSpace(att.Outcome.Error.Code),
		}
	}

	// If the task type doesn't match the node (rare), still keep what we can.
	_ = taskType
}

func statusFromAttempt(att swf.TaskAttempt) model.JobRunStoryNodeStatus {
	switch att.State {
	case swf.TaskAttemptStateReady, swf.TaskAttemptStateLeased, swf.TaskAttemptStateWaiting, swf.TaskAttemptStateRunning:
		return model.JobRunStoryNodeStatusRunning
	case swf.TaskAttemptStateSucceeded:
		return model.JobRunStoryNodeStatusSucceeded
	case swf.TaskAttemptStateFailed:
		return model.JobRunStoryNodeStatusFailed
	default:
	}
	if att.Outcome.Status == swf.TaskOutcomeStatusSucceeded {
		return model.JobRunStoryNodeStatusSucceeded
	}
	if att.Outcome.Status == swf.TaskOutcomeStatusFailed {
		return model.JobRunStoryNodeStatusFailed
	}
	return model.JobRunStoryNodeStatusUnknown
}

func statusFromErr(err error, fallback model.JobRunStoryNodeStatus) model.JobRunStoryNodeStatus {
	if err == nil {
		return fallback
	}
	if errors.Is(err, ErrReplayInProgress) {
		return model.JobRunStoryNodeStatusRunning
	}
	return model.JobRunStoryNodeStatusFailed
}

func deriveContainerStatus(children []*model.JobRunStoryNode) model.JobRunStoryNodeStatus {
	if len(children) == 0 {
		return model.JobRunStoryNodeStatusUnknown
	}
	last := children[len(children)-1]
	if last != nil && last.Status == model.JobRunStoryNodeStatusRunning {
		return model.JobRunStoryNodeStatusRunning
	}
	for _, ch := range children {
		if ch != nil && ch.Status == model.JobRunStoryNodeStatusFailed {
			return model.JobRunStoryNodeStatusFailed
		}
	}
	return model.JobRunStoryNodeStatusSucceeded
}

func isTerminalState(stateName string, states map[string]recipe.State) bool {
	state, ok := states[stateName]
	if !ok {
		return true
	}
	return len(state.Transitions) == 0
}

func stepIDFromTaskType(opID, taskType string) string {
	// Expected: "<opId>:<stepId>"
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return ""
	}
	parts := strings.SplitN(taskType, ":", 2)
	if len(parts) != 2 {
		return taskType
	}
	if strings.TrimSpace(parts[0]) != strings.TrimSpace(opID) {
		return parts[1]
	}
	return parts[1]
}

func asMap(v any) map[string]interface{} {
	if v == nil {
		return map[string]interface{}{}
	}
	m, ok := v.(map[string]interface{})
	if ok {
		return m
	}
	// Best-effort: if op output isn't a map, store it under a stable key.
	return map[string]interface{}{"value": v}
}

func isContextPatchRun(run swf.TaskRun) bool {
	if len(run.Attempts) == 0 {
		return false
	}
	final := run.Attempts[len(run.Attempts)-1]

	// Use explicit output envelope kind; do not guess based on task type ordering.
	if final.Output != nil && len(final.Output.Data) > 0 {
		var env coretasks.OutputEnvelope
		if json.Unmarshal(final.Output.Data, &env) == nil && env.Version == coretasks.OutputEnvelopeVersion {
			return env.Kind == coretasks.OutputKindContextPatch
		}
	}
	return false
}

func nextTaskFromRegistry(opID string, currentTask string) (string, bool) {
	opID = strings.TrimSpace(opID)
	if opID == "" {
		return "", false
	}
	def, ok := coreops.Get(opID)
	if !ok || def == nil {
		return "", false
	}
	stepID := strings.TrimSpace(stepIDFromTaskType(opID, currentTask))
	if stepID == "" {
		return "", false
	}
	chain := def.TaskChain()
	for i := range chain {
		st := chain[i]
		if strings.TrimSpace(st.Name) != stepID {
			continue
		}
		next := strings.TrimSpace(st.NextStepTask)
		if next == "" {
			return "", false
		}
		return next, true
	}
	return "", false
}

func unwrapTaskEnvelopePayload(raw []byte) (any, bool) {
	var env coretasks.OutputEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false
	}
	if env.Version != coretasks.OutputEnvelopeVersion || strings.TrimSpace(string(env.Kind)) == "" {
		return nil, false
	}
	if len(env.Payload) == 0 {
		return nil, true
	}
	var payload any
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return nil, false
	}
	return payload, true
}
