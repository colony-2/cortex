package story

import (
	"errors"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
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
	Input        any                       `json:"input"`
	GitTaskCtx   gitstate.GlobalGitTaskContext `json:"context"`
	ArtifactKeys []swf.ArtifactKey         `json:"artifact_keys,omitempty"`
	Artifacts    map[string]swf.ArtifactKey `json:"artifacts,omitempty"`
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

		evNode, nextState, err := e.evalTransitions(stateDef.Transitions, resCtx, currentState)
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

func (e *executor) evalTransitions(transitions []recipe.Transition, resCtx *template.ResolutionContext, fromState string) (*model.JobRunStoryNode, string, error) {
	// Temporary context sharing CEL env + template data.
	evalCtx := &template.ResolutionContext{
		ScopeType:    resCtx.ScopeType,
		TemplateData: resCtx.TemplateData,
		CELEnv:       resCtx.CELEnv,
	}

	node := e.tree.newNode(model.JobRunStoryNodeKindTransitionEval, "evaluate transitions")
	node.FromStateID = fromState
	node.Status = model.JobRunStoryNodeStatusSucceeded
	node.Path = append([]string(nil), e.tree.segments...)
	node.Path = append(node.Path, "transitionEval")
	node.InvokeSeq = resCtx.TaskExecutionContext().Invocation.InvokeSeq
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

	prefix := op + ":"
	taskType, ok := e.jobCtx.NextTaskTypeForPrefix(prefix)
	if !ok {
		return fmt.Errorf("%w: no task runs found for op %q", ErrReplayMismatch, op)
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

		stepID := stepIDFromTaskType(op, currentTask)
		stepNode := e.tree.newNode(model.JobRunStoryNodeKindOpStep, "step "+stepID)
		stepNode.StepID = stepID
		stepNode.StepType = "other"
		stepNode.Status = model.JobRunStoryNodeStatusRunning
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
		var env activityInvocationOutput
		if err := json.Unmarshal(raw, &env); err != nil {
			opNode.Status = model.JobRunStoryNodeStatusFailed
			e.tree.pop()
			e.tree.pop()
			return fmt.Errorf("decode activity output envelope: %w", err)
		}
		resCtx.UpdateGitState(env.GitResult)
		finalOut = env.OpOutput

		if strings.TrimSpace(env.NextTask) == "" {
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
		currentTask = env.NextTask
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
		fillAttemptOnNode(&pa, taskType, a, e.jobID)
		prior = append(prior, &pa)
	}
	n.PriorAttempts = prior
	n.Attempt = final.Attempt

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

	// Try to decode the activity invocation request so input and invoke_seq are recipe-centric.
	var req activityInvocationRequest
	if att.Input != nil && len(att.Input.Data) > 0 && json.Unmarshal(att.Input.Data, &req) == nil {
		n.Input = req.Input
		n.InvokeSeq = req.GitTaskCtx.InvokeSeq
	}

	// Try to decode the activity output envelope so output is recipe-centric.
	var env activityInvocationOutput
	if att.Output != nil && len(att.Output.Data) > 0 && json.Unmarshal(att.Output.Data, &env) == nil {
		n.Output = env.OpOutput
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
