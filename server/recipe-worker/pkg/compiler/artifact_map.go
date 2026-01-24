package compiler

import (
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	"github.com/colony-2/swf-go/pkg/swf"
)

func artifactsToMap(artifacts []swf.Artifact) map[string]swf.Artifact {
	out := make(map[string]swf.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		name := artifact.Name()
		//if name == "" || name == gitstate.ThinPackArtifactName {
		//	continue
		//}
		out[name] = artifact
	}
	return out
}

func lastSequenceArtifacts(resCtx *template.ResolutionContext, sequence []recipe.Node) map[string]swf.Artifact {
	if len(sequence) == 0 || resCtx == nil {
		return nil
	}
	lastNode := sequence[len(sequence)-1]
	metadata := lastNode.GetMetadata()

	var scopeType template.ScopeType
	fallback := ""
	switch t := lastNode.NodeImpl.(type) {
	case *recipe.NodeOp:
		scopeType = template.ScopeOp
		fallback = t.OpData.Op
	case *recipe.NodeSequence:
		scopeType = template.ScopeSequence
	case *recipe.NodeState:
		scopeType = template.ScopeStateMachine
	default:
		return nil
	}

	scopeID := template.ScopeID(metadata, fallback, scopeType)
	if scopeID == "" {
		return nil
	}
	step, ok := resCtx.TemplateData.Sequence[scopeID]
	if !ok {
		return nil
	}
	return step.Artifacts
}

func stateArtifacts(resCtx *template.ResolutionContext, stateName string, stateDef recipe.State) map[string]swf.Artifact {
	if resCtx == nil {
		return nil
	}
	scopeID := template.ScopeID(stateDef.GetMetadata(), stateName, template.ScopeState)
	if scopeID == "" {
		return nil
	}
	step, ok := resCtx.TemplateData.States[scopeID]
	if !ok {
		return nil
	}
	return step.Artifacts
}
