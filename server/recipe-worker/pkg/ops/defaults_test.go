package ops

import (
	"reflect"
	"testing"
)

func TestInjectDefaults_TopLevel(t *testing.T) {
	type TestInput struct {
		Name  string `json:"name" default:"test"`
		Count int    `json:"count" default:"5"`
	}

	inputMap := map[string]interface{}{
		"name": "override",
	}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inputMap["name"] != "override" {
		t.Errorf("expected name to be 'override', got %v", inputMap["name"])
	}

	if inputMap["count"] != "5" {
		t.Errorf("expected count to be '5', got %v", inputMap["count"])
	}
}

func TestInjectDefaults_Nested(t *testing.T) {
	type DatabaseConfig struct {
		Host string `json:"host" default:"localhost"`
		Port int    `json:"port" default:"5432"`
	}

	type TestInput struct {
		Name   string         `json:"name" default:"test"`
		Config DatabaseConfig `json:"config"`
	}

	inputMap := map[string]interface{}{
		"config": map[string]interface{}{
			"host": "prod.example.com",
		},
	}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Top-level default injected
	if inputMap["name"] != "test" {
		t.Errorf("expected name to be 'test', got %v", inputMap["name"])
	}

	// Nested structure preserved
	configMap, ok := inputMap["config"].(map[string]interface{})
	if !ok {
		t.Fatal("expected config to be a map")
	}

	if configMap["host"] != "prod.example.com" {
		t.Errorf("expected host to be 'prod.example.com', got %v", configMap["host"])
	}

	if configMap["port"] != "5432" {
		t.Errorf("expected port to be '5432', got %v", configMap["port"])
	}
}

func TestInjectDefaults_TemplateExpression(t *testing.T) {
	type TestInput struct {
		Branch string `json:"branch" default:"{{ context.git.branch }}"`
		Port   int    `json:"port" default:"{{ inputs.base_port + 1000 }}"`
	}

	inputMap := map[string]interface{}{}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Template strings injected (will be resolved later)
	if inputMap["branch"] != "{{ context.git.branch }}" {
		t.Errorf("expected branch template, got %v", inputMap["branch"])
	}

	if inputMap["port"] != "{{ inputs.base_port + 1000 }}" {
		t.Errorf("expected port template, got %v", inputMap["port"])
	}
}

func TestInjectDefaults_EmptyNestedStruct(t *testing.T) {
	type Config struct {
		Setting string `json:"setting" default:"value"`
	}

	type TestInput struct {
		Config Config `json:"config"`
	}

	inputMap := map[string]interface{}{}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Nested map created with default
	configMap, ok := inputMap["config"].(map[string]interface{})
	if !ok {
		t.Fatal("expected config to be created")
	}

	if configMap["setting"] != "value" {
		t.Errorf("expected setting to be 'value', got %v", configMap["setting"])
	}
}

func TestInjectDefaults_NoDefaults(t *testing.T) {
	type Config struct {
		Setting string `json:"setting"` // no default
	}

	type TestInput struct {
		Config Config `json:"config"`
	}

	inputMap := map[string]interface{}{}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No nested map created since no defaults exist
	_, exists := inputMap["config"]
	if exists {
		t.Error("expected config to not exist when no defaults present")
	}
}

func TestInjectDefaults_PointerFields(t *testing.T) {
	type Config struct {
		Setting string `json:"setting" default:"value"`
	}

	type TestInput struct {
		Config *Config `json:"config"`
	}

	inputMap := map[string]interface{}{}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Nested map created for pointer struct
	configMap, ok := inputMap["config"].(map[string]interface{})
	if !ok {
		t.Fatal("expected config to be created for pointer type")
	}

	if configMap["setting"] != "value" {
		t.Errorf("expected setting to be 'value', got %v", configMap["setting"])
	}
}

func TestInjectDefaults_UserProvidedNonMap(t *testing.T) {
	type Config struct {
		Setting string `json:"setting" default:"value"`
	}

	type TestInput struct {
		Config Config `json:"config"`
	}

	inputMap := map[string]interface{}{
		"config": "some-string", // wrong type
	}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// User's invalid value preserved, no defaults injected
	if inputMap["config"] != "some-string" {
		t.Errorf("expected config to be preserved as 'some-string', got %v", inputMap["config"])
	}
}

func TestInjectDefaults_DeepNesting(t *testing.T) {
	type Level3 struct {
		Value string `json:"value" default:"deep"`
	}

	type Level2 struct {
		Level3 Level3 `json:"level3"`
		Name   string `json:"name" default:"middle"`
	}

	type Level1 struct {
		Level2 Level2 `json:"level2"`
		Count  int    `json:"count" default:"10"`
	}

	inputMap := map[string]interface{}{}

	err := InjectDefaults(reflect.TypeOf(Level1{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check top level
	if inputMap["count"] != "10" {
		t.Errorf("expected count to be '10', got %v", inputMap["count"])
	}

	// Check level 2
	level2, ok := inputMap["level2"].(map[string]interface{})
	if !ok {
		t.Fatal("expected level2 to be created")
	}

	if level2["name"] != "middle" {
		t.Errorf("expected name to be 'middle', got %v", level2["name"])
	}

	// Check level 3
	level3, ok := level2["level3"].(map[string]interface{})
	if !ok {
		t.Fatal("expected level3 to be created")
	}

	if level3["value"] != "deep" {
		t.Errorf("expected value to be 'deep', got %v", level3["value"])
	}
}

func TestInjectDefaults_PartialNesting(t *testing.T) {
	type Inner struct {
		Field1 string `json:"field1" default:"default1"`
		Field2 string `json:"field2" default:"default2"`
	}

	type Outer struct {
		Inner Inner  `json:"inner"`
		Name  string `json:"name" default:"outer"`
	}

	inputMap := map[string]interface{}{
		"inner": map[string]interface{}{
			"field1": "provided",
		},
	}

	err := InjectDefaults(reflect.TypeOf(Outer{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Top level default
	if inputMap["name"] != "outer" {
		t.Errorf("expected name to be 'outer', got %v", inputMap["name"])
	}

	inner, ok := inputMap["inner"].(map[string]interface{})
	if !ok {
		t.Fatal("expected inner to be a map")
	}

	// User provided field1
	if inner["field1"] != "provided" {
		t.Errorf("expected field1 to be 'provided', got %v", inner["field1"])
	}

	// Default for field2
	if inner["field2"] != "default2" {
		t.Errorf("expected field2 to be 'default2', got %v", inner["field2"])
	}
}

func TestInjectDefaults_OmitEmpty(t *testing.T) {
	type TestInput struct {
		Required string `json:"required" default:"value"`
		Optional string `json:"optional,omitempty" default:"optval"`
	}

	inputMap := map[string]interface{}{}

	err := InjectDefaults(reflect.TypeOf(TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inputMap["required"] != "value" {
		t.Errorf("expected required to be 'value', got %v", inputMap["required"])
	}

	if inputMap["optional"] != "optval" {
		t.Errorf("expected optional to be 'optval', got %v", inputMap["optional"])
	}
}

func TestInjectDefaults_NonStructType(t *testing.T) {
	inputMap := map[string]interface{}{}

	// Should handle gracefully
	err := InjectDefaults(reflect.TypeOf("string"), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(inputMap) != 0 {
		t.Error("expected input map to remain empty")
	}
}

func TestInjectDefaults_PtrToStruct(t *testing.T) {
	type TestInput struct {
		Name string `json:"name" default:"test"`
	}

	inputMap := map[string]interface{}{}

	// Pass pointer type
	err := InjectDefaults(reflect.TypeOf(&TestInput{}), inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inputMap["name"] != "test" {
		t.Errorf("expected name to be 'test', got %v", inputMap["name"])
	}
}
