package schema

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestGenerateRecipeSchema(t *testing.T) {
	generator := NewGenerator()
	schema, err := generator.GenerateRecipeSchema()
	if err != nil {
		t.Fatalf("Failed to generate recipe schema: %v", err)
	}
	
	// Verify basic structure
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Error("Missing or incorrect $schema")
	}
	
	if schema["title"] != "Recipe Schema" {
		t.Error("Missing or incorrect title")
	}
	
	// Check for properties
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing properties")
	}
	
	// Check required fields
	if _, ok := props["name"]; !ok {
		t.Error("Missing 'name' property")
	}
	
	if _, ok := props["version"]; !ok {
		t.Error("Missing 'version' property")
	}
	
	// Check oneOf constraint for node types
	oneOf, ok := schema["oneOf"].([]map[string]interface{})
	if !ok || len(oneOf) != 4 {
		t.Error("Missing or incorrect oneOf constraint")
	}
	
	// Check definitions
	defs, ok := schema["definitions"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing definitions")
	}
	
	// Verify key definitions exist
	expectedDefs := []string{
		"RetryPolicy",
		"Node",
		"SequenceNode",
		"ParallelNode",
		"StateNode",
		"State",
	}
	
	for _, defName := range expectedDefs {
		if _, ok := defs[defName]; !ok {
			t.Errorf("Missing definition: %s", defName)
		}
	}
	
	// Output schema for inspection
	schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
	t.Logf("Generated schema:\n%s", string(schemaJSON))
}

func TestGenerateActivitySchema(t *testing.T) {
	tests := []struct {
		name         string
		activityType string
		checkInputs  []string
		checkOutputs []string
	}{
		{
			name:         "Sleep Operation",
			activityType: "sleep",
			checkInputs:  []string{"duration"},
			checkOutputs: []string{"start_time", "end_time", "completed"},
		},
		{
			name:         "Command Execution",
			activityType: "command_execution",
			checkInputs:  []string{"run"},
			checkOutputs: []string{"stdout", "stderr", "exit_code"},
		},
		{
			name:         "LLM Inference",
			activityType: "llm_inference",
			checkInputs:  []string{"prompt", "model"},
			checkOutputs: []string{"response", "model", "finish_reason"},
		},
	}
	
	generator := NewGenerator()
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := generator.GenerateActivitySchema(tt.activityType)
			if err != nil {
				t.Fatalf("Failed to generate schema for %s: %v", tt.activityType, err)
			}
			
			// Check basic structure
			if schema["title"] != "Activity Schema: "+tt.activityType {
				t.Error("Incorrect title")
			}
			
			props, ok := schema["properties"].(map[string]interface{})
			if !ok {
				t.Fatal("Missing properties")
			}
			
			// Check operation schema
			opSchema, ok := props["operation"].(map[string]interface{})
			if !ok {
				t.Fatal("Missing operation schema")
			}
			
			// Check for const discriminator
			opProps, ok := opSchema["properties"].(map[string]interface{})
			if !ok {
				t.Fatal("Missing operation properties")
			}
			
			opField, ok := opProps["op"].(map[string]interface{})
			if !ok {
				t.Fatal("Missing op field")
			}
			
			if opField["const"] != tt.activityType {
				t.Errorf("Expected op const to be %s, got %v", tt.activityType, opField["const"])
			}
			
			// Check inputs exist
			if _, ok := props["inputs"]; !ok {
				t.Error("Missing inputs schema")
			}
			
			// Check outputs exist
			if _, ok := props["outputs"]; !ok {
				t.Error("Missing outputs schema")
			}
			
			// Output schema for inspection
			schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
			t.Logf("Generated %s schema:\n%s", tt.activityType, string(schemaJSON))
		})
	}
}

func TestDurationValidation(t *testing.T) {
	d := Duration("30s")
	
	if pattern := d.SchemaPattern(); pattern != "^[0-9]+(s|m|h)$" {
		t.Errorf("Incorrect pattern: %s", pattern)
	}
	
	if desc := d.SchemaDescription(); desc == "" {
		t.Error("Missing description")
	}
}

func TestRetryPolicySchema(t *testing.T) {
	generator := NewGenerator()
	rp := RetryPolicy{
		MaxAttempts:        3,
		InitialInterval:    Duration("5s"),
		BackoffCoefficient: 2.0,
		MaxInterval:        Duration("60s"),
	}
	
	schema := generator.generateFromType(reflect.TypeOf(rp), rp)
	
	// Check it's an object
	if schema["type"] != "object" {
		t.Error("RetryPolicy should be an object")
	}
	
	// Check required fields
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("Missing required fields")
	}
	
	expectedRequired := []string{"max_attempts", "initial_interval"}
	for _, field := range expectedRequired {
		found := false
		for _, req := range required {
			if req == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Missing required field: %s", field)
		}
	}
	
	// Check properties exist
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing properties")
	}
	
	// Check max_attempts has validation
	maxAttempts, ok := props["max_attempts"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing max_attempts property")
	}
	
	if maxAttempts["type"] != "integer" {
		t.Error("max_attempts should be integer")
	}
	
	if maxAttempts["minimum"] != 1 {
		t.Error("max_attempts should have minimum of 1")
	}
}

func TestWorkflowNodeSchema(t *testing.T) {
	generator := NewGenerator()
	node := WorkflowNode{
		ID:      "test-node",
		Desc:    "Test node",
		Timeout: Duration("30s"),
	}
	
	schema := generator.generateFromType(reflect.TypeOf(node), node)
	
	// Check properties
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing properties")
	}
	
	// Check common fields exist
	expectedFields := []string{"id", "desc", "timeout", "when", "retry"}
	for _, field := range expectedFields {
		if _, ok := props[field]; !ok {
			t.Errorf("Missing field: %s", field)
		}
	}
	
	// Check node type fields exist
	nodeTypeFields := []string{"op", "sequence", "parallel", "states"}
	for _, field := range nodeTypeFields {
		if _, ok := props[field]; !ok {
			t.Errorf("Missing node type field: %s", field)
		}
	}
}

func TestSequenceNodeSchema(t *testing.T) {
	generator := NewGenerator()
	seq := SequenceNode{
		Nodes: []WorkflowNode{
			{ID: "step1"},
			{ID: "step2"},
		},
	}
	
	schema := generator.generateFromType(reflect.TypeOf(seq), seq)
	
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing properties")
	}
	
	// Check sequence property
	seqProp, ok := props["sequence"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing sequence property")
	}
	
	if seqProp["type"] != "array" {
		t.Error("sequence should be an array")
	}
	
	if seqProp["minItems"] != 1 {
		t.Error("sequence should have minItems of 1")
	}
}

func TestCompleteRecipeExample(t *testing.T) {
	// Create a complete recipe using the new types
	recipe := Recipe{
		Name:        "build-and-test",
		Version:     "1.0",
		Description: "Build and test the application",
		WorkflowNode: WorkflowNode{
			Sequence: &SequenceNode{
				Nodes: []WorkflowNode{
					{
						ID:   "build",
						Desc: "Build the application",
						Operation: &NodeOperation{
							Command: &CommandExecutionOperation{
								Run: "go build ./...",
							},
						},
					},
					{
						ID:   "test",
						Desc: "Run tests",
						Operation: &NodeOperation{
							Command: &CommandExecutionOperation{
								Run:     "go test ./...",
								Timeout: Duration("5m"),
							},
						},
					},
				},
			},
		},
	}
	
	// Generate schema for this recipe
	generator := NewGenerator()
	schema, err := generator.GenerateRecipeSchema()
	if err != nil {
		t.Fatalf("Failed to generate schema: %v", err)
	}
	
	// The schema should be valid and complete
	if schema == nil {
		t.Fatal("Schema is nil")
	}
	
	// Marshal the recipe to JSON to verify it matches expected structure
	recipeJSON, err := json.MarshalIndent(recipe, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal recipe: %v", err)
	}
	
	t.Logf("Recipe JSON:\n%s", string(recipeJSON))
	
	// The recipe should be serializable and match our expected structure
	var decoded map[string]interface{}
	if err := json.Unmarshal(recipeJSON, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal recipe: %v", err)
	}
	
	if decoded["name"] != "build-and-test" {
		t.Error("Recipe name mismatch")
	}
	
	if decoded["version"] != "1.0" {
		t.Error("Recipe version mismatch")
	}
}