package template

type ResolutionOptions struct {
	Mode                string
	ValidationMode      string
	ClampSliceIndex     bool
	AllowFutureStepRefs bool
}

func DefaultResolutionOptions() ResolutionOptions {
	return ResolutionOptions{Mode: "run"}
}
