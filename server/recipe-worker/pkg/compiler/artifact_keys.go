package compiler

import (
	"fmt"

	"github.com/colony-2/swf-go/pkg/swf"
)

func collectArtifactKeys(value interface{}, out map[string]swf.ArtifactKey) error {
	if keyer, ok := value.(interface {
		ArtifactKey() (swf.ArtifactKey, error)
	}); ok {
		key, err := keyer.ArtifactKey()
		if err != nil {
			return err
		}
		if err := key.Validate(); err != nil {
			return err
		}
		out[artifactKeyIdentity(key)] = key
		return nil
	}

	switch v := value.(type) {
	case swf.ArtifactKey:
		if err := v.Validate(); err != nil {
			return err
		}
		out[artifactKeyIdentity(v)] = v
	case *swf.ArtifactKey:
		if v == nil {
			return nil
		}
		if err := v.Validate(); err != nil {
			return err
		}
		out[artifactKeyIdentity(*v)] = *v
	case []swf.ArtifactKey:
		for _, item := range v {
			if err := collectArtifactKeys(item, out); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, item := range v {
			if err := collectArtifactKeys(item, out); err != nil {
				return err
			}
		}
	case map[string]swf.ArtifactKey:
		for _, item := range v {
			if err := collectArtifactKeys(item, out); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for _, item := range v {
			if err := collectArtifactKeys(item, out); err != nil {
				return err
			}
		}
	}
	return nil
}

func artifactKeyIdentity(key swf.ArtifactKey) string {
	return fmt.Sprintf("%s:%d:%s", key.JobId, key.TaskOrdinal, key.Name)
}

func collectArtifactKeysFromInput(input map[string]interface{}) ([]swf.ArtifactKey, error) {
	if len(input) == 0 {
		return nil, nil
	}
	seen := make(map[string]swf.ArtifactKey)
	for _, value := range input {
		if err := collectArtifactKeys(value, seen); err != nil {
			return nil, err
		}
	}
	out := make([]swf.ArtifactKey, 0, len(seen))
	for _, key := range seen {
		out = append(out, key)
	}
	return out, nil
}

func appendArtifactKeys(existing []swf.ArtifactKey, bindings map[string]swf.ArtifactKey) []swf.ArtifactKey {
	if len(bindings) == 0 {
		return existing
	}
	seen := make(map[string]swf.ArtifactKey, len(existing)+len(bindings))
	for _, key := range existing {
		seen[artifactKeyIdentity(key)] = key
	}
	for _, key := range bindings {
		seen[artifactKeyIdentity(key)] = key
	}
	out := make([]swf.ArtifactKey, 0, len(seen))
	for _, key := range seen {
		out = append(out, key)
	}
	return out
}
