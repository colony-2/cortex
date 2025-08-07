package recipe

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// HashComputer computes canonical hashes for recipes
type HashComputer struct{}

// NewHashComputer creates a new hash computer
func NewHashComputer() *HashComputer {
	return &HashComputer{}
}

// ComputeRecipeHash computes a deterministic hash of recipe content
func (h *HashComputer) ComputeRecipeHash(recipe *Recipe) string {
	// Create a normalized representation
	normalized := h.normalizeRecipe(recipe)
	
	// Convert to JSON for consistent serialization
	data, err := json.Marshal(normalized)
	if err != nil {
		// Fallback to a simple hash on error
		return fmt.Sprintf("%x", sha256.Sum256([]byte(recipe.Name+recipe.Version)))
	}
	
	// Compute SHA-256 hash
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// normalizeRecipe creates a normalized representation of a recipe
func (h *HashComputer) normalizeRecipe(recipe *Recipe) map[string]interface{} {
	result := make(map[string]interface{})
	
	// Core identity (always included)
	result["name"] = strings.TrimSpace(recipe.Name)
	result["version"] = strings.TrimSpace(recipe.Version)
	result["description"] = strings.TrimSpace(recipe.Description)
	
	// Normalize unified recipe definition
	if recipe.Recipe != nil {
		result["recipe"] = h.normalizeWorkflow(recipe.Recipe)
	}
	
	return result
}

// normalizeWorkflow normalizes a workflow structure
func (h *HashComputer) normalizeWorkflow(workflow interface{}) map[string]interface{} {
	// Convert workflow to a normalized map representation
	// This handles the dynamic nature of workflow definitions
	normalized := make(map[string]interface{})
	
	// Convert to JSON and back to normalize the structure
	data, err := json.Marshal(workflow)
	if err != nil {
		return normalized
	}
	
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return normalized
	}
	
	// Sort maps recursively
	if sorted := h.sortMaps(temp); sorted != nil {
		if m, ok := sorted.(map[string]interface{}); ok {
			return m
		}
	}
	return normalized
}

// normalizeActivity normalizes an activity structure
func (h *HashComputer) normalizeActivity(activity interface{}) map[string]interface{} {
	// Similar to normalizeWorkflow
	normalized := make(map[string]interface{})
	
	data, err := json.Marshal(activity)
	if err != nil {
		return normalized
	}
	
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return normalized
	}
	
	// Sort maps recursively
	if sorted := h.sortMaps(temp); sorted != nil {
		if m, ok := sorted.(map[string]interface{}); ok {
			return m
		}
	}
	return normalized
}

// normalizeAgent normalizes an agent structure
func (h *HashComputer) normalizeAgent(agent interface{}) map[string]interface{} {
	// Similar to normalizeWorkflow
	normalized := make(map[string]interface{})
	
	data, err := json.Marshal(agent)
	if err != nil {
		return normalized
	}
	
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return normalized
	}
	
	// Sort maps recursively
	if sorted := h.sortMaps(temp); sorted != nil {
		if m, ok := sorted.(map[string]interface{}); ok {
			return m
		}
	}
	return normalized
}

// sortMaps recursively sorts all maps in a structure by key
func (h *HashComputer) sortMaps(v interface{}) interface{} {
	switch value := v.(type) {
	case map[string]interface{}:
		// Create a new sorted map
		result := make(map[string]interface{})
		
		// Get sorted keys
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		
		// Add values in sorted order
		for _, k := range keys {
			// Recursively sort nested structures
			switch nested := value[k].(type) {
			case map[string]interface{}:
				result[k] = h.sortMaps(nested)
			case []interface{}:
				result[k] = h.sortSlice(nested)
			default:
				// Normalize strings by trimming whitespace
				if str, ok := nested.(string); ok {
					result[k] = strings.TrimSpace(str)
				} else {
					result[k] = nested
				}
			}
		}
		
		return result
		
	case []interface{}:
		return h.sortSlice(value)
	case string:
		return strings.TrimSpace(value)
	default:
		// For other types, return as-is
		return v
	}
}

// sortSlice recursively processes slices
func (h *HashComputer) sortSlice(slice []interface{}) []interface{} {
	result := make([]interface{}, len(slice))
	
	for i, item := range slice {
		switch value := item.(type) {
		case map[string]interface{}:
			result[i] = h.sortMaps(value)
		case []interface{}:
			result[i] = h.sortSlice(value)
		case string:
			result[i] = strings.TrimSpace(value)
		default:
			result[i] = value
		}
	}
	
	return result
}