package compiler

import (
	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	"github.com/colony-2/swf-go/pkg/swf"
)

func artifactsToMap(artifacts []swf.Artifact) map[string]swf.Artifact {
	out := make(map[string]swf.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		name := artifact.Name()
		if name == "" || name == gitstate.ThinPackArtifactName {
			continue
		}
		out[name] = artifact
	}
	return out
}
