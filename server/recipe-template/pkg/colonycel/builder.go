package colonycel

import (
	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
)

type Options struct {
}

func NewBuilder(opts Options) *funcregistry.Builder {
	builder := funcregistry.NewBuilder().WithDefaults()
	RegisterArtifactFunctions(builder)
	return builder
}
