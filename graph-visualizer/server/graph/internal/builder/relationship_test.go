package builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUpdateRelationships(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	
	// Create test nodes
	nodeADir := filepath.Join(tempDir, "nodeA")
	nodeBDir := filepath.Join(tempDir, "nodeB")
	nodeCDir := filepath.Join(tempDir, "nodeC")
	
	os.MkdirAll(nodeADir, 0755)
	os.MkdirAll(nodeBDir, 0755)
	os.MkdirAll(nodeCDir, 0755)
	
	// Create initial dependencies.yaml files
	depA := Dependency{Dependencies: []string{"nodeB"}}
	dataA, _ := yaml.Marshal(&depA)
	os.WriteFile(filepath.Join(nodeADir, "dependencies.yaml"), dataA, 0644)
	
	depB := Dependency{Dependencies: []string{}}
	dataB, _ := yaml.Marshal(&depB)
	os.WriteFile(filepath.Join(nodeBDir, "dependencies.yaml"), dataB, 0644)
	
	depC := Dependency{Dependencies: []string{}}
	dataC, _ := yaml.Marshal(&depC)
	os.WriteFile(filepath.Join(nodeCDir, "dependencies.yaml"), dataC, 0644)
	
	// Create builder
	builder := New(tempDir)
	ctx := context.Background()
	
	tests := []struct {
		name          string
		nodeID        string
		relationships []string
		wantErr       bool
	}{
		{
			name:          "Update existing relationships",
			nodeID:        "nodeA",
			relationships: []string{"nodeC"},
			wantErr:       false,
		},
		{
			name:          "Clear all relationships",
			nodeID:        "nodeA",
			relationships: []string{},
			wantErr:       false,
		},
		{
			name:          "Add relationships to empty node",
			nodeID:        "nodeB",
			relationships: []string{"nodeC"},
			wantErr:       false,
		},
		{
			name:          "Non-existent node",
			nodeID:        "nodeD",
			relationships: []string{"nodeA"},
			wantErr:       true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := builder.UpdateDependencies(ctx, tt.nodeID, tt.relationships)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateDependencies() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr {
				// Verify the update
				depFile := filepath.Join(tempDir, tt.nodeID, "dependencies.yaml")
				data, err := os.ReadFile(depFile)
				if err != nil {
					t.Fatalf("Failed to read dependencies file: %v", err)
				}
				
				var dep Dependency
				err = yaml.Unmarshal(data, &dep)
				if err != nil {
					t.Fatalf("Failed to unmarshal dependencies: %v", err)
				}
				
				if len(dep.Dependencies) != len(tt.relationships) {
					t.Errorf("Expected %d dependencies, got %d", len(tt.relationships), len(dep.Dependencies))
				}
				
				// Check each dependency
				for i, expected := range tt.relationships {
					if i >= len(dep.Dependencies) || dep.Dependencies[i] != expected {
						t.Errorf("Expected dependency %s at index %d", expected, i)
					}
				}
			}
		})
	}
}

func TestCircularDependencyValidation(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	
	// Create test nodes with circular dependencies
	nodeADir := filepath.Join(tempDir, "nodeA")
	nodeBDir := filepath.Join(tempDir, "nodeB")
	nodeCDir := filepath.Join(tempDir, "nodeC")
	
	os.MkdirAll(nodeADir, 0755)
	os.MkdirAll(nodeBDir, 0755)
	os.MkdirAll(nodeCDir, 0755)
	
	// nodeA depends on nodeB
	depA := Dependency{Dependencies: []string{"nodeB"}}
	dataA, _ := yaml.Marshal(&depA)
	os.WriteFile(filepath.Join(nodeADir, "dependencies.yaml"), dataA, 0644)
	
	// nodeB depends on nodeC
	depB := Dependency{Dependencies: []string{"nodeC"}}
	dataB, _ := yaml.Marshal(&depB)
	os.WriteFile(filepath.Join(nodeBDir, "dependencies.yaml"), dataB, 0644)
	
	// nodeC has no dependencies initially
	depC := Dependency{Dependencies: []string{}}
	dataC, _ := yaml.Marshal(&depC)
	os.WriteFile(filepath.Join(nodeCDir, "dependencies.yaml"), dataC, 0644)
	
	// Create builder
	builder := New(tempDir)
	ctx := context.Background()
	
	// Try to create a circular dependency: nodeC -> nodeA
	// This would create: nodeA -> nodeB -> nodeC -> nodeA
	err := builder.UpdateDependencies(ctx, "nodeC", []string{"nodeA"})
	if err == nil {
		t.Error("Expected error for circular dependency, got nil")
	}
	
	// Verify the original dependencies weren't changed
	data, _ := os.ReadFile(filepath.Join(nodeCDir, "dependencies.yaml"))
	var dep Dependency
	yaml.Unmarshal(data, &dep)
	
	if len(dep.Dependencies) != 0 {
		t.Error("Dependencies should not have been updated due to circular dependency")
	}
}

func TestSelfDependency(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	
	// Create test node
	nodeDir := filepath.Join(tempDir, "nodeA")
	os.MkdirAll(nodeDir, 0755)
	
	// Create initial dependencies.yaml
	dep := Dependency{Dependencies: []string{}}
	data, _ := yaml.Marshal(&dep)
	os.WriteFile(filepath.Join(nodeDir, "dependencies.yaml"), data, 0644)
	
	// Create builder
	builder := New(tempDir)
	ctx := context.Background()
	
	// Try to create self-dependency
	err := builder.UpdateDependencies(ctx, "nodeA", []string{"nodeA"})
	if err == nil {
		t.Error("Expected error for self-dependency, got nil")
	}
}

func TestNonExistentDependency(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	
	// Create test node
	nodeDir := filepath.Join(tempDir, "nodeA")
	os.MkdirAll(nodeDir, 0755)
	
	// Create initial dependencies.yaml
	dep := Dependency{Dependencies: []string{}}
	data, _ := yaml.Marshal(&dep)
	os.WriteFile(filepath.Join(nodeDir, "dependencies.yaml"), data, 0644)
	
	// Create builder
	builder := New(tempDir)
	ctx := context.Background()
	
	// Try to add non-existent node as dependency
	err := builder.UpdateDependencies(ctx, "nodeA", []string{"nonExistentNode"})
	if err == nil {
		t.Error("Expected error for non-existent dependency, got nil")
	}
}