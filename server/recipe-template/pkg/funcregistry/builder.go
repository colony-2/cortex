package funcregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// ContextProvider returns the TaskExecutionContext in scope for the current CEL evaluation.
// It may be nil when functions are evaluated without a task context (e.g., validation).
type ContextProvider func() contextual.TaskExecutionContext

// BuiltinFactory produces a cel.EnvOption given the adapter (needed for native conversions)
// and, optionally, a TaskExecutionContext provider for context-aware functions.
type BuiltinFactory func(adapter types.Adapter, ctxProvider ContextProvider) cel.EnvOption

// Builder holds a non-global set of builtin function factories and type declarations.
type Builder struct {
	factories map[string]BuiltinFactory
	typeDecls []reflect.Type
}

// NewBuilder constructs an empty builder.
func NewBuilder() *Builder {
	return &Builder{factories: map[string]BuiltinFactory{}}
}

// WithDefaults installs the legacy builtin set (json_parse, json_stringify, jq, string overloads).
func (b *Builder) WithDefaults() *Builder {
	for name, f := range defaultBuiltins() {
		b.factories[name] = f
	}
	return b
}

// WithBuiltin registers or overwrites a builtin factory.
func (b *Builder) WithBuiltin(name string, factory BuiltinFactory) *Builder {
	b.factories[name] = factory
	return b
}

// AddUnaryFunc registers a typed unary function with static output typing.
func AddUnaryFunc[In any, Out any](b *Builder, name string, impl func(context.Context, In) (Out, error)) *Builder {
	inT := reflect.TypeOf((*In)(nil)).Elem()
	outT := reflect.TypeOf((*Out)(nil)).Elem()
	b.registerType(inT)
	b.registerType(outT)

	b.factories[name] = func(adapter types.Adapter, _ ContextProvider) cel.EnvOption {
		return cel.Function(
			name,
			cel.Overload(
				name+"_unary",
				[]*cel.Type{celTypeFor(inT)},
				celTypeFor(outT),
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					inVal, err := decodeVal[In](arg)
					if err != nil {
						return types.NewErr("%s: %v", name, err)
					}
					out, err := impl(context.Background(), inVal)
					if err != nil {
						return types.NewErr("%s: %v", name, err)
					}
					return adapter.NativeToValue(out)
				}),
			),
		)
	}
	return b
}

// AddBinaryFunc registers a typed binary function with static typing.
func AddBinaryFunc[A any, B any, Out any](b *Builder, name string, impl func(context.Context, A, B) (Out, error)) *Builder {
	aT := reflect.TypeOf((*A)(nil)).Elem()
	bT := reflect.TypeOf((*B)(nil)).Elem()
	outT := reflect.TypeOf((*Out)(nil)).Elem()
	b.registerType(aT)
	b.registerType(bT)
	b.registerType(outT)

	b.factories[name] = func(adapter types.Adapter, _ ContextProvider) cel.EnvOption {
		return cel.Function(
			name,
			cel.Overload(
				name+"_binary",
				[]*cel.Type{celTypeFor(aT), celTypeFor(bT)},
				celTypeFor(outT),
				cel.BinaryBinding(func(lhs ref.Val, rhs ref.Val) ref.Val {
					aVal, err := decodeVal[A](lhs)
					if err != nil {
						return types.NewErr("%s: %v", name, err)
					}
					bVal, err := decodeVal[B](rhs)
					if err != nil {
						return types.NewErr("%s: %v", name, err)
					}
					out, err := impl(context.Background(), aVal, bVal)
					if err != nil {
						return types.NewErr("%s: %v", name, err)
					}
					return adapter.NativeToValue(out)
				}),
			),
		)
	}
	return b
}

// TypeOptions returns EnvOptions that must be supplied at env construction time.
func (b *Builder) TypeOptions() []cel.EnvOption {
	if len(b.typeDecls) == 0 {
		return nil
	}
	ptrTypes := make([]any, 0, len(b.typeDecls))
	for _, t := range b.typeDecls {
		ptrTypes = append(ptrTypes, reflect.New(t).Interface())
	}
	return []cel.EnvOption{cel.Types(ptrTypes...)}
}

// FunctionOptions materializes cel.EnvOptions using the provided adapter.
func (b *Builder) FunctionOptions(adapter types.Adapter) ([]cel.EnvOption, error) {
	return b.FunctionOptionsWithContext(adapter, nil)
}

// FunctionOptionsWithContext materializes cel.EnvOptions using the provided adapter and task context provider.
func (b *Builder) FunctionOptionsWithContext(adapter types.Adapter, ctxProvider ContextProvider) ([]cel.EnvOption, error) {
	opts := make([]cel.EnvOption, 0, len(b.factories))
	for name, factory := range b.factories {
		if factory == nil {
			return nil, fmt.Errorf("builtin %q has nil factory", name)
		}
		opts = append(opts, factory(adapter, ctxProvider))
	}
	return opts, nil
}

// Helper: decode ref.Val into typed Go value via JSON roundtrip.
func decodeVal[T any](v ref.Val) (T, error) {
	var zero T
	raw := v.Value()
	data, err := json.Marshal(raw)
	if err != nil {
		return zero, fmt.Errorf("marshal input: %w", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return zero, fmt.Errorf("unmarshal input: %w", err)
	}
	return out, nil
}

// celTypeFor produces a cel.Type best-effort for static checking.
func celTypeFor(rt reflect.Type) *cel.Type {
	switch rt.Kind() {
	case reflect.Bool:
		return cel.BoolType
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cel.IntType
	case reflect.Float32, reflect.Float64:
		return cel.DoubleType
	case reflect.String:
		return cel.StringType
	case reflect.Slice:
		return cel.ListType(celTypeFor(rt.Elem()))
	case reflect.Map:
		key := celTypeFor(rt.Key())
		val := celTypeFor(rt.Elem())
		return cel.MapType(key, val)
	case reflect.Struct:
		return cel.DynType
	default:
		return cel.DynType
	}
}

func (b *Builder) registerType(rt reflect.Type) {
	// intentionally no-op: we avoid cel.Types registration to keep builtins simple/dyn.
}

// AddZeroFuncWithContext registers a zero-argument function that can access the TaskExecutionContext.
func AddZeroFuncWithContext[Out any](b *Builder, name string, impl func(context.Context, contextual.TaskExecutionContext) (Out, error)) *Builder {
	outT := reflect.TypeOf((*Out)(nil)).Elem()
	b.registerType(outT)

	b.factories[name] = func(adapter types.Adapter, ctxProvider ContextProvider) cel.EnvOption {
		return cel.Function(
			name,
			cel.Overload(
				name+"_zero",
				[]*cel.Type{},
				celTypeFor(outT),
				cel.FunctionBinding(func(values ...ref.Val) ref.Val {
					var taskCtx contextual.TaskExecutionContext
					if ctxProvider != nil {
						taskCtx = ctxProvider()
					}
					out, err := impl(context.Background(), taskCtx)
					if err != nil {
						return types.NewErr("%s: %v", name, err)
					}
					return toAdapterValue(adapter, out)
				}),
			),
		)
	}
	return b
}

// AddZeroFunc preserves the legacy zero-arg function registration without task context.
func AddZeroFunc[Out any](b *Builder, name string, impl func(context.Context) (Out, error)) *Builder {
	return AddZeroFuncWithContext(b, name, func(ctx context.Context, _ contextual.TaskExecutionContext) (Out, error) {
		return impl(ctx)
	})
}

// toAdapterValue converts Go values into CEL values via JSON round-trip to
// avoid type-registration requirements while keeping field names intact.
func toAdapterValue(adapter types.Adapter, v any) ref.Val {
	data, err := json.Marshal(v)
	if err != nil {
		return types.NewErr("convert value: %v", err)
	}
	var decoded interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return types.NewErr("convert value: %v", err)
	}
	return adapter.NativeToValue(decoded)
}
