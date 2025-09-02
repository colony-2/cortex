package main

import (
    "testing"
    "gopkg.in/yaml.v3"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

func TestDebugParse(t *testing.T) {
    yamlStr := `id: hello_world
desc: A minimal recipe that demonstrates basic structure
version: "1.0"
op: echo_activity
inputs:
  message: "Hello, World!"
  name: "test"`
    
    var r recipe.Recipe
    err := yaml.Unmarshal([]byte(yamlStr), &r)
    if err != nil {
        t.Fatalf("Error: %v", err)
    }
    
    t.Logf("Success! Type: %T", r.RecipeImpl)
    if op, ok := r.RecipeImpl.(*recipe.RecipeOp); ok {
        t.Logf("Op: %s", op.OpData.Op)
        t.Logf("Version: %s", op.RecipeMetadata.Version)
        t.Logf("ID: %s", op.RecipeMetadata.NodeMetadata.ID)
        t.Logf("Desc: %s", op.RecipeMetadata.NodeMetadata.Desc)
    }
    
    // Test GetMetdata
    metadata := r.GetMetdata()
    t.Logf("GetMetdata ID: %s", metadata.ID)
    t.Logf("GetMetdata Version: %s", metadata.Version)
    
    // Check for nil
    if metadata.ID == "" {
        t.Error("ID is empty! This will cause nil pointer dereference in executor")
    }
}