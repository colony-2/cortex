package setup

import (
	"github.com/colony-2/colony2/server/recipe-template/pkg/colonycel"
	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
)

func registerArtifactCELFunctions(builder *funcregistry.Builder) {
	colonycel.RegisterArtifactFunctions(builder)
}
