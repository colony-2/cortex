package compiler

import (
	"fmt"

	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	"github.com/colony-2/swf-go/pkg/swf"
)

func resolveArtifactBindings(resCtx *template.ResolutionContext, bindings map[string]interface{}) (map[string]swf.ArtifactKey, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	resolved, err := resCtx.ResolveMap(bindings)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve templates op artifacts: %w", err)
	}
	out := make(map[string]swf.ArtifactKey, len(resolved))
	for name, value := range resolved {
		if name == "" {
			return nil, fmt.Errorf("artifact binding name cannot be empty")
		}
		key, err := coerceArtifactKey(value)
		if err != nil {
			return nil, fmt.Errorf("artifact binding %q is invalid: %w", name, err)
		}
		out[name] = key
	}
	return out, nil
}

func coerceArtifactKey(value interface{}) (swf.ArtifactKey, error) {
	switch v := value.(type) {
	case swf.ArtifactKey:
		if err := v.Validate(); err != nil {
			return swf.ArtifactKey{}, err
		}
		return v, nil
	case *swf.ArtifactKey:
		if v == nil {
			return swf.ArtifactKey{}, fmt.Errorf("artifact key is nil")
		}
		if err := v.Validate(); err != nil {
			return swf.ArtifactKey{}, err
		}
		return *v, nil
	default:
		return swf.ArtifactKey{}, fmt.Errorf("expected artifact key, got %T", value)
	}
}
