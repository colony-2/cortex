package template

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// CELOptionsProvider supplies type options (must be applied at env creation)
// and function options (require adapter) for CEL.
type CELOptionsProvider interface {
	TypeOptions() []cel.EnvOption
	FunctionOptions(adapter types.Adapter) ([]cel.EnvOption, error)
}

type ResolutionOptions struct {
	Mode                string
	ValidationMode      string
	ClampSliceIndex     bool
	AllowFutureStepRefs bool
	// CELOptionsProvider allows callers (server/api testserver, cortex) to inject CEL functions.
	// It is given the adapter to allow proper wrapping of complex return types.
	CELOptionsProvider CELOptionsProvider
}

func DefaultResolutionOptions() ResolutionOptions {
	return ResolutionOptions{Mode: "run"}
}
