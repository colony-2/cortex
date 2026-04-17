package setup

import (
	"github.com/colony-2/c2j/pkg/template/colonycel"
	"github.com/colony-2/c2j/pkg/template/funcregistry"
)

func registerArtifactCELFunctions(builder *funcregistry.Builder) {
	colonycel.RegisterArtifactFunctions(builder)
}
