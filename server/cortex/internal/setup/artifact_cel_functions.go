package setup

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
)

func registerArtifactCELFunctions(b *funcregistry.Builder) {
	// NOTE:
	// These are advanced CEL-only registrations used by artifact helpers.
	// For normal function integration, prefer funcregistry.AddZeroFuncWithContext /
	// AddZeroFunc / AddUnaryFunc / AddBinaryFunc so functions are available in both CEL and Go templates.
	// If one of these helpers must also be callable from Go templates, add a matching
	// b.WithTemplateFunc(...) registration.
	//
	// Registered in cortex (not recipe-template) via the CELOptionsProvider extension point.
	b.WithBuiltin("artifact_set", func(adapter types.Adapter, _ funcregistry.ContextProvider) cel.EnvOption {
		return artifactSetLikeEnvOption(adapter, "artifact_set")
	})
	b.WithBuiltin("artifact_concat", func(adapter types.Adapter, _ funcregistry.ContextProvider) cel.EnvOption {
		return artifactSetLikeEnvOption(adapter, "artifact_concat")
	})
	b.WithBuiltin("artifact_filter", func(adapter types.Adapter, _ funcregistry.ContextProvider) cel.EnvOption {
		return cel.Function(
			"artifact_filter",
			cel.Overload(
				"artifact_filter_dyn_dyn",
				[]*cel.Type{cel.DynType, cel.DynType},
				cel.ListType(cel.DynType),
				cel.BinaryBinding(func(arts ref.Val, opts ref.Val) ref.Val {
					keys, err := normalizeArtifactKeys("artifact_filter", arts)
					if err != nil {
						return types.NewErr("%v", err)
					}

					f, err := parseArtifactFilterOpts(opts)
					if err != nil {
						return types.NewErr("%v", err)
					}
					filtered := make([]swf.ArtifactKey, 0, len(keys))
					for _, k := range keys {
						if f.match(k) {
							filtered = append(filtered, k)
						}
					}
					return adapter.NativeToValue(filtered)
				}),
			),
		)
	})
	b.WithBuiltin("artifact_names", func(adapter types.Adapter, _ funcregistry.ContextProvider) cel.EnvOption {
		return cel.Function(
			"artifact_names",
			cel.Overload(
				"artifact_names_dyn",
				[]*cel.Type{cel.DynType},
				cel.ListType(cel.StringType),
				cel.UnaryBinding(func(arts ref.Val) ref.Val {
					keys, err := normalizeArtifactKeys("artifact_names", arts)
					if err != nil {
						return types.NewErr("%v", err)
					}
					out := make([]string, 0, len(keys))
					for _, k := range keys {
						out = append(out, k.Name)
					}
					return adapter.NativeToValue(out)
				}),
			),
		)
	})
	b.WithBuiltin("artifact_unique", func(adapter types.Adapter, _ funcregistry.ContextProvider) cel.EnvOption {
		return cel.Function(
			"artifact_unique",
			cel.Overload(
				"artifact_unique_dyn",
				[]*cel.Type{cel.DynType},
				cel.ListType(cel.DynType),
				cel.UnaryBinding(func(arts ref.Val) ref.Val {
					keys, err := normalizeArtifactKeys("artifact_unique", arts)
					if err != nil {
						return types.NewErr("%v", err)
					}
					return adapter.NativeToValue(dedupeArtifactKeys(keys, "name"))
				}),
			),
			cel.Overload(
				"artifact_unique_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType},
				cel.ListType(cel.DynType),
				cel.BinaryBinding(func(arts ref.Val, byVal ref.Val) ref.Val {
					keys, err := normalizeArtifactKeys("artifact_unique", arts)
					if err != nil {
						return types.NewErr("%v", err)
					}
					by, ok := toString(byVal)
					if !ok || strings.TrimSpace(by) == "" {
						by = "name"
					}
					out, err := dedupeArtifactKeysChecked(keys, by)
					if err != nil {
						return types.NewErr("artifact_unique: %v", err)
					}
					return adapter.NativeToValue(out)
				}),
			),
		)
	})
}

func artifactSetLikeEnvOption(adapter types.Adapter, fnName string) cel.EnvOption {
	// cel-go doesn't support true varargs; provide the common arities plus a list-form.
	return cel.Function(
		fnName,
		cel.Overload(
			fnName+"_dyn",
			[]*cel.Type{cel.DynType},
			cel.ListType(cel.DynType),
			cel.UnaryBinding(func(a ref.Val) ref.Val {
				keys, err := normalizeArtifactKeys(fnName, a)
				if err != nil {
					return types.NewErr("%v", err)
				}
				return adapter.NativeToValue(keys)
			}),
		),
		cel.Overload(
			fnName+"_dyn_dyn",
			[]*cel.Type{cel.DynType, cel.DynType},
			cel.ListType(cel.DynType),
			cel.BinaryBinding(func(a, b ref.Val) ref.Val {
				var out []swf.ArtifactKey
				if err := appendArtifactKeys(fnName, &out, a); err != nil {
					return types.NewErr("%v", err)
				}
				if err := appendArtifactKeys(fnName, &out, b); err != nil {
					return types.NewErr("%v", err)
				}
				return adapter.NativeToValue(out)
			}),
		),
		cel.Overload(
			fnName+"_dyn_dyn_dyn",
			[]*cel.Type{cel.DynType, cel.DynType, cel.DynType},
			cel.ListType(cel.DynType),
			cel.FunctionBinding(func(args ...ref.Val) ref.Val {
				var out []swf.ArtifactKey
				for _, a := range args {
					if err := appendArtifactKeys(fnName, &out, a); err != nil {
						return types.NewErr("%v", err)
					}
				}
				return adapter.NativeToValue(out)
			}),
		),
		cel.Overload(
			fnName+"_dyn_dyn_dyn_dyn",
			[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType},
			cel.ListType(cel.DynType),
			cel.FunctionBinding(func(args ...ref.Val) ref.Val {
				var out []swf.ArtifactKey
				for _, a := range args {
					if err := appendArtifactKeys(fnName, &out, a); err != nil {
						return types.NewErr("%v", err)
					}
				}
				return adapter.NativeToValue(out)
			}),
		),
	)
}

func normalizeArtifactKeys(fnName string, v ref.Val) ([]swf.ArtifactKey, error) {
	out := []swf.ArtifactKey{}
	if err := appendArtifactKeys(fnName, &out, v); err != nil {
		return nil, err
	}
	return out, nil
}

func appendArtifactKeys(fnName string, out *[]swf.ArtifactKey, v ref.Val) error {
	if v == nil || v == types.NullValue {
		return nil
	}

	// Prefer trait-based access so map/list elements are adapted consistently.
	if l, ok := v.(traits.Lister); ok {
		sizeVal := l.Size()
		size, ok := sizeVal.(types.Int)
		if !ok {
			return fmt.Errorf("%s: unsupported input type %T", fnName, v.Value())
		}
		for i := int64(0); i < int64(size); i++ {
			item := l.Get(types.Int(i))
			if err := appendArtifactKeys(fnName, out, item); err != nil {
				return err
			}
		}
		return nil
	}

	if m, ok := v.(traits.Mapper); ok {
		it := m.Iterator()
		keys := make([]string, 0)
		for it.HasNext() == types.True {
			k := it.Next()
			ks, ok := toString(k)
			if !ok {
				return fmt.Errorf("%s: unsupported input type %T", fnName, v.Value())
			}
			keys = append(keys, ks)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := m.Get(types.String(k))
			if err := appendArtifactKeys(fnName, out, val); err != nil {
				return err
			}
		}
		return nil
	}

	return appendArtifactKeysNative(fnName, out, v.Value())
}

func appendArtifactKeysNative(fnName string, out *[]swf.ArtifactKey, native any) error {
	if native == nil {
		return nil
	}

	if keyer, ok := native.(interface {
		ArtifactKey() (swf.ArtifactKey, error)
	}); ok {
		key, err := keyer.ArtifactKey()
		if err != nil {
			return fmt.Errorf("%s: %w", fnName, err)
		}
		*out = append(*out, key)
		return nil
	}

	switch v := native.(type) {
	case swf.ArtifactKey:
		*out = append(*out, v)
		return nil
	case *swf.ArtifactKey:
		if v == nil {
			return nil
		}
		*out = append(*out, *v)
		return nil
	case []swf.ArtifactKey:
		*out = append(*out, v...)
		return nil
	case []*swf.ArtifactKey:
		for _, item := range v {
			if item != nil {
				*out = append(*out, *item)
			}
		}
		return nil
	case []interface{}:
		for _, item := range v {
			if err := appendArtifactKeysNative(fnName, out, item); err != nil {
				return err
			}
		}
		return nil
	case []ref.Val:
		for _, item := range v {
			if err := appendArtifactKeys(fnName, out, item); err != nil {
				return err
			}
		}
		return nil
	case map[string]swf.ArtifactKey:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			*out = append(*out, v[k])
		}
		return nil
	case map[string]*swf.ArtifactKey:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v[k] != nil {
				*out = append(*out, *v[k])
			}
		}
		return nil
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := appendArtifactKeysNative(fnName, out, v[k]); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: unsupported input type %T", fnName, native)
	}
}

type artifactFilter struct {
	namePrefix   string
	nameSuffix   string
	nameContains string
	nameRegex    *regexp.Regexp
	minSize      *int64
	maxSize      *int64
}

func parseArtifactFilterOpts(opts ref.Val) (artifactFilter, error) {
	if opts == nil || opts == types.NullValue {
		return artifactFilter{}, nil
	}
	m, ok := opts.(traits.Mapper)
	if !ok {
		return artifactFilter{}, fmt.Errorf("artifact_filter: expected opts to be a map")
	}
	f := artifactFilter{}

	// Strings
	if s, ok := mapGetString(m, "name_prefix"); ok {
		f.namePrefix = s
	}
	if s, ok := mapGetString(m, "name_suffix"); ok {
		f.nameSuffix = s
	}
	if s, ok := mapGetString(m, "name_contains"); ok {
		f.nameContains = s
	}
	if s, ok := mapGetString(m, "name_regex"); ok && strings.TrimSpace(s) != "" {
		re, err := regexp.Compile(s) // RE2
		if err != nil {
			return artifactFilter{}, fmt.Errorf("artifact_filter: invalid name_regex: %v", err)
		}
		f.nameRegex = re
	}

	// Ints
	if i, ok := mapGetInt64(m, "min_size"); ok {
		f.minSize = &i
	}
	if i, ok := mapGetInt64(m, "max_size"); ok {
		f.maxSize = &i
	}

	return f, nil
}

func (f artifactFilter) match(k swf.ArtifactKey) bool {
	name := k.Name
	if f.namePrefix != "" && !strings.HasPrefix(name, f.namePrefix) {
		return false
	}
	if f.nameSuffix != "" && !strings.HasSuffix(name, f.nameSuffix) {
		return false
	}
	if f.nameContains != "" && !strings.Contains(name, f.nameContains) {
		return false
	}
	if f.nameRegex != nil && !f.nameRegex.MatchString(name) {
		return false
	}

	size := k.SizeBytes
	if f.minSize != nil {
		// Unknown sizes (-1) fail min_size when min_size > -1.
		if size < 0 && *f.minSize > -1 {
			return false
		}
		if size >= 0 && size < *f.minSize {
			return false
		}
	}
	if f.maxSize != nil {
		// Unknown sizes pass max_size unless caller sets a negative bound explicitly.
		if size >= 0 && size > *f.maxSize {
			return false
		}
		if size < 0 && *f.maxSize < 0 {
			return false
		}
	}

	return true
}

func dedupeArtifactKeys(keys []swf.ArtifactKey, by string) []swf.ArtifactKey {
	out, err := dedupeArtifactKeysChecked(keys, by)
	if err != nil {
		// Shouldn't happen for internal/default usage; fall back to name.
		out, _ = dedupeArtifactKeysChecked(keys, "name")
	}
	return out
}

func dedupeArtifactKeysChecked(keys []swf.ArtifactKey, by string) ([]swf.ArtifactKey, error) {
	switch by {
	case "name", "":
		seen := map[string]struct{}{}
		out := make([]swf.ArtifactKey, 0, len(keys))
		for _, k := range keys {
			if _, ok := seen[k.Name]; ok {
				continue
			}
			seen[k.Name] = struct{}{}
			out = append(out, k)
		}
		return out, nil
	case "key":
		seen := map[string]struct{}{}
		out := make([]swf.ArtifactKey, 0, len(keys))
		for _, k := range keys {
			id := fmt.Sprintf("%s:%d:%s", k.JobId, k.TaskOrdinal, k.Name)
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, k)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid by: %s", by)
	}
}

func mapGetString(m traits.Mapper, key string) (string, bool) {
	v := m.Get(types.String(key))
	if v == nil || v == types.NullValue {
		return "", false
	}
	s, ok := toString(v)
	return s, ok
}

func mapGetInt64(m traits.Mapper, key string) (int64, bool) {
	v := m.Get(types.String(key))
	if v == nil || v == types.NullValue {
		return 0, false
	}
	i, ok := toInt64(v)
	return i, ok
}

func toString(v ref.Val) (string, bool) {
	if v == nil || v == types.NullValue {
		return "", false
	}
	switch s := v.(type) {
	case types.String:
		return string(s), true
	default:
		if raw, ok := v.Value().(string); ok {
			return raw, true
		}
		return "", false
	}
}

func toInt64(v ref.Val) (int64, bool) {
	if v == nil || v == types.NullValue {
		return 0, false
	}
	switch n := v.(type) {
	case types.Int:
		return int64(n), true
	case types.Uint:
		return int64(n), true
	default:
		switch raw := v.Value().(type) {
		case int:
			return int64(raw), true
		case int64:
			return raw, true
		case float64:
			return int64(raw), true
		default:
			return 0, false
		}
	}
}
