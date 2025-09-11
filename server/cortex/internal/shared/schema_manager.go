package shared

import (
    "encoding/json"

    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

// SchemaManager provides JSON Schema generation helpers for cortex
type SchemaManager struct{}

func NewSchemaManager() *SchemaManager { return &SchemaManager{} }

// GenerateCompleteSchema returns the recipe JSON schema as a generic map
// filterActivity currently unused; includeVersion appends a version field
func (sm *SchemaManager) GenerateCompleteSchema(_ string, includeVersion bool) (map[string]interface{}, error) {
    s, err := recipe.GenerateSchemaString()
    if err != nil {
        return nil, err
    }
    var out map[string]interface{}
    if err := json.Unmarshal([]byte(s), &out); err != nil {
        return nil, err
    }
    if includeVersion {
        out["version"] = "1.0.0"
    }
    return out, nil
}
