package compiler

import "github.com/colony-2/colony2/server/recipe-template/pkg/template"

type ExecutionMode string

const (
	ExecutionModeRun      ExecutionMode = "run"
	ExecutionModeValidate ExecutionMode = "validate"
)

type ValidationMode string

const (
	ValidateAll      ValidationMode = "all"
	ValidatePathOnly ValidationMode = "path_only"
)

type ValidationOptions struct {
	Mode       ValidationMode
	CollectAll bool
}

type ExecutionOptions struct {
	Mode       ExecutionMode
	Validation ValidationOptions
}

func normalizeExecutionOptions(opts []ExecutionOptions) ExecutionOptions {
	if len(opts) == 0 {
		return ExecutionOptions{Mode: ExecutionModeRun}
	}

	out := opts[0]
	if out.Mode == "" {
		out.Mode = ExecutionModeRun
	}
	if out.Validation.Mode == "" {
		out.Validation.Mode = ValidateAll
	}
	return out
}

func resolutionOptionsFromExecution(opts ExecutionOptions) template.ResolutionOptions {
	resolution := template.DefaultResolutionOptions()
	if opts.Mode == ExecutionModeValidate {
		resolution.Mode = string(ExecutionModeValidate)
		resolution.ClampSliceIndex = true
		resolution.AllowFutureStepRefs = true
		resolution.ValidationMode = string(opts.Validation.Mode)
	} else {
		resolution.Mode = string(ExecutionModeRun)
	}
	return resolution
}
