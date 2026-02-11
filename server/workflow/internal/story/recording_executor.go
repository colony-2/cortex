package story

import (
	"strings"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	coreworkflow "github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
)

type recordingExecutor struct {
	inner compiler.DefaultRecipeExecutor
	tree  *treeBuilder
	root  *model.JobRunStoryNode
	rec   *replayStoryRecorder
}

func newRecordingExecutor(inner compiler.DefaultRecipeExecutor, tree *treeBuilder, rec *replayStoryRecorder) *recordingExecutor {
	return &recordingExecutor{inner: inner, tree: tree, rec: rec}
}

func (e *recordingExecutor) Root() *model.JobRunStoryNode { return e.root }

func (e *recordingExecutor) ExecuteRecipe(ctx coreworkflow.Context, r recipe.Recipe, rawRecipeInputs map[string]interface{}, execCtx contextual.JobContext, commitContext contextual.GitCommitContext, opts ...compiler.ExecutionOptions) (map[string]interface{}, []swf.Artifact, error) {
	recipeID := strings.TrimSpace(r.GetMetadata().ID)
	root := e.tree.newNode(model.JobRunStoryNodeKindRecipe, "recipe "+recipeID)
	root.RecipeID = recipeID
	root.Invocation = map[string]interface{}{"args": rawRecipeInputs}
	root.Input = rawRecipeInputs
	root.Status = model.JobRunStoryNodeStatusRunning
	e.tree.push("root", root)
	e.root = root
	if e.rec != nil {
		e.rec.SetRecipeMeta(recipeID, strings.TrimSpace(r.GetMetadata().Version))
		e.rec.SetRoot(root)
	}

	out, arts, err := e.inner.WithDelegate(e).ExecuteRecipe(ctx, r, rawRecipeInputs, execCtx, commitContext, opts...)
	if err != nil {
		root.Status = statusFromErr(err, root.Status)
		root.Output = out
		e.tree.pop()
		return out, arts, err
	}
	root.Status = model.JobRunStoryNodeStatusSucceeded
	root.Output = out
	e.tree.pop()
	return out, arts, nil
}

func (e *recordingExecutor) ExecuteNode(ctx coreworkflow.Context, parentResCtx *template.ResolutionContext, n *recipe.Node) error {
	if parentResCtx != nil && parentResCtx.ScopeType == template.ScopeState {
		nodePath := ""
		if tec := parentResCtx.TaskExecutionContext(); strings.TrimSpace(tec.Invocation.NodePath) != "" {
			nodePath = tec.Invocation.NodePath
		}

		title := "state"
		if segs := splitInvocationNodePath(nodePath); len(segs) > 0 {
			title = "state " + segs[len(segs)-1]
		}

		stateNode := e.tree.newNode(model.JobRunStoryNodeKindState, title)
		stateNode.Status = model.JobRunStoryNodeStatusRunning
		setStoryNodePath(stateNode, nodePath)
		e.tree.push("state", stateNode)

		err := e.inner.WithDelegate(e).ExecuteNode(ctx, parentResCtx, n)
		if err != nil {
			stateNode.Status = statusFromErr(err, stateNode.Status)
			e.tree.pop()
			return err
		}
		stateNode.Status = deriveContainerStatus(stateNode.Children)
		e.tree.pop()
		return nil
	}

	return e.inner.WithDelegate(e).ExecuteNode(ctx, parentResCtx, n)
}

func (e *recordingExecutor) ExecuteSequence(ctx coreworkflow.Context, rCtx *template.ResolutionContext, metadata recipe.NodeMetadata, outputTemplate map[string]interface{}, sequence []recipe.Node) error {
	seqID := template.ScopeID(metadata, "", template.ScopeSequence)
	node := e.tree.newNode(model.JobRunStoryNodeKindSequence, "sequence "+seqID)
	node.SequenceID = seqID
	node.Status = model.JobRunStoryNodeStatusRunning
	if rCtx != nil {
		if resolved, err := rCtx.ResolveMap(metadata.Inputs); err == nil {
			node.Input = resolved
		}
	}
	e.tree.push("sequence:"+seqID, node)

	err := e.inner.WithDelegate(e).ExecuteSequence(ctx, rCtx, metadata, outputTemplate, sequence)
	if err != nil {
		node.Status = statusFromErr(err, node.Status)
		e.tree.pop()
		return err
	}
	node.Status = deriveContainerStatus(node.Children)
	if rCtx != nil {
		node.Output = rCtx.GetLastExecution()
	}
	e.tree.pop()
	return nil
}

func (e *recordingExecutor) ExecuteStateMachine(ctx coreworkflow.Context, parentContext *template.ResolutionContext, metadata recipe.NodeMetadata, outputTemplate map[string]interface{}, stateMap *recipe.StateMap) error {
	smID := template.ScopeID(metadata, "", template.ScopeStateMachine)
	node := e.tree.newNode(model.JobRunStoryNodeKindStateMachine, "stateMachine "+smID)
	node.StateMachineID = smID
	node.Status = model.JobRunStoryNodeStatusRunning
	if parentContext != nil {
		if resolved, err := parentContext.ResolveMap(metadata.Inputs); err == nil {
			node.Input = resolved
		}
	}
	e.tree.push("stateMachine:"+smID, node)

	err := e.inner.WithDelegate(e).ExecuteStateMachine(ctx, parentContext, metadata, outputTemplate, stateMap)
	if err != nil {
		node.Status = statusFromErr(err, node.Status)
		e.tree.pop()
		return err
	}
	node.Status = deriveContainerStatus(node.Children)
	if parentContext != nil {
		node.Output = parentContext.GetLastExecution()
	}
	e.tree.pop()
	return nil
}

func (e *recordingExecutor) ExecuteOp(ctx coreworkflow.Context, parentResolutionContext *template.ResolutionContext, metadata recipe.NodeMetadata, opID string) error {
	opID = strings.TrimSpace(opID)
	opNode := e.tree.newNode(model.JobRunStoryNodeKindOp, "op "+opID)
	opNode.OpID = opID
	opNode.OpType = "custom"
	opNode.Status = model.JobRunStoryNodeStatusRunning
	e.tree.push("op:"+opID, opNode)

	if e.rec != nil {
		e.rec.SetCurrentOpNode(opNode)
	}
	err := e.inner.WithDelegate(e).ExecuteOp(ctx, parentResolutionContext, metadata, opID)
	if e.rec != nil {
		e.rec.SetCurrentOpNode(nil)
	}

	if err != nil {
		opNode.Status = statusFromErr(err, opNode.Status)
		e.tree.pop()
		return err
	}

	// Attach op-level input/output from steps.
	first := findFirstChildStep(opNode.Children)
	last := findLastChildStep(opNode.Children)
	if first != nil {
		opNode.Input = first.Input
	}
	if last != nil {
		opNode.Output = last.Output
	}

	// Flatten single-step op where the only child step is "<opId>".
	if len(opNode.Children) == 1 {
		ch := opNode.Children[0]
		if ch != nil && ch.Kind == model.JobRunStoryNodeKindOpStep && ch.StepID == opID {
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
			opNode.Path = append([]string{}, ch.Path...)
			opNode.Status = ch.Status
			e.tree.pop()
			return nil
		}
	}

	opNode.Status = deriveContainerStatus(opNode.Children)
	if opNode.Status == model.JobRunStoryNodeStatusUnknown {
		opNode.Status = model.JobRunStoryNodeStatusSucceeded
	}

	e.tree.pop()
	return nil
}
