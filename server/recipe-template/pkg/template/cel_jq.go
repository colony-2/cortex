package template

import (
	"encoding/json"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/itchyny/gojq"
)

// jqEnvOption registers the jq(input, expr) CEL function and optional member style.
func jqEnvOption(adapter types.Adapter) cel.EnvOption {
	return cel.Function(
		"jq",
		cel.Overload(
			"jq_dyn_string",
			[]*cel.Type{cel.DynType, cel.StringType},
			cel.DynType,
			cel.BinaryBinding(jqBinding(adapter)),
		),
		cel.MemberOverload(
			"jq_dyn_string_member",
			[]*cel.Type{cel.DynType, cel.StringType},
			cel.DynType,
			cel.BinaryBinding(jqBinding(adapter)),
		),
	)
}

// jsonStringifyEnvOption registers json_stringify(dyn) -> string.
func jsonStringifyEnvOption(adapter types.Adapter) cel.EnvOption {
	return cel.Function(
		"json_stringify",
		cel.Overload(
			"json_stringify_dyn",
			[]*cel.Type{cel.DynType},
			cel.StringType,
			cel.UnaryBinding(jsonStringifyBinding(adapter)),
		),
	)
}

// stringJSONEnvOption adds string(map) / string(list) overloads that JSON-encode structured values.
func stringJSONEnvOption(adapter types.Adapter) cel.EnvOption {
	return cel.Function(
		"string",
		cel.Overload(
			"string_map_json",
			[]*cel.Type{cel.MapType(cel.DynType, cel.DynType)},
			cel.StringType,
			cel.UnaryBinding(func(val ref.Val) ref.Val {
				return jsonStringifyBinding(adapter)(val)
			}),
		),
		cel.Overload(
			"string_list_json",
			[]*cel.Type{cel.ListType(cel.DynType)},
			cel.StringType,
			cel.UnaryBinding(func(val ref.Val) ref.Val {
				return jsonStringifyBinding(adapter)(val)
			}),
		),
	)
}

func jqBinding(adapter types.Adapter) func(ref.Val, ref.Val) ref.Val {
	return func(input ref.Val, exprVal ref.Val) ref.Val {
		expr, ok := toString(exprVal)
		if !ok {
			return types.NewErr("jq: invalid expression: expected string")
		}

		parsed, err := gojq.Parse(expr)
		if err != nil {
			return types.NewErr("jq: invalid expression: %v", err)
		}

		prog, err := gojq.Compile(parsed)
		if err != nil {
			return types.NewErr("jq: compile failed: %v", err)
		}

		results, err := drainIter(prog.Run(normalizeJQInput(input)))
		if err != nil {
			return types.NewErr("jq: execution failed: %v", err)
		}

		switch len(results) {
		case 0:
			return types.NullValue
		case 1:
			return adapter.NativeToValue(results[0])
		default:
			return adapter.NativeToValue(results)
		}
	}
}

func jsonStringifyBinding(adapter types.Adapter) func(ref.Val) ref.Val {
	return func(value ref.Val) ref.Val {
		native := normalizeJQInput(value)
		data, err := json.Marshal(native)
		if err != nil {
			return types.NewErr("json_stringify: failed to encode JSON: %v", err)
		}
		return types.String(data)
	}
}

func normalizeJQInput(val ref.Val) interface{} {
	if val == nil {
		return nil
	}
	return val.Value()
}

func drainIter(iter gojq.Iter) ([]interface{}, error) {
	results := []interface{}{}
	for {
		val, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := val.(error); ok {
			return nil, err
		}
		if nested, ok := val.(gojq.Iter); ok {
			nestedResults, err := drainIter(nested)
			if err != nil {
				return nil, err
			}
			results = append(results, nestedResults...)
			continue
		}
		results = append(results, val)
	}
	return results, nil
}

func toString(val ref.Val) (string, bool) {
	if val == nil {
		return "", false
	}

	switch v := val.(type) {
	case types.String:
		return string(v), true
	default:
		if s, ok := v.Value().(string); ok {
			return s, true
		}
	}
	return "", false
}
