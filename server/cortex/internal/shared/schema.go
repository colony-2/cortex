package shared

import (
	"github.com/divisive-ai/vibethis/server/cortex/pkg/schema"
)

// SchemaManager manages schema generation and formatting
type SchemaManager struct {
	registryManager *RegistryManager
}

// NewSchemaManager creates a new schema manager
func NewSchemaManager(rm *RegistryManager) *SchemaManager {
	return &SchemaManager{registryManager: rm}
}

// GenerateCompleteSchema generates a complete schema with all activity details
func (sm *SchemaManager) GenerateCompleteSchema(filterActivity string, includeVersion bool) (map[string]interface{}, error) {
	// Use the new automated generator for everything
	generator := schema.NewGenerator()
	
	var schemaResult map[string]interface{}
	var err error
	
	if filterActivity != "" {
		// Generate schema for specific activity
		schemaResult, err = generator.GenerateActivitySchema(filterActivity)
		if err != nil {
			return nil, err
		}
	} else {
		// Generate complete recipe schema
		schemaResult, err = generator.GenerateRecipeSchema()
		if err != nil {
			return nil, err
		}
	}
	
	// Add version if requested
	if includeVersion {
		schemaResult["version"] = "1.0.0"
	}
	
	return schemaResult, nil
}