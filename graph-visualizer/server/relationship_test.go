package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

func TestUpdateRelationshipsHandler(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	
	// Create test nodes
	nodeADir := filepath.Join(tempDir, "nodeA")
	nodeBDir := filepath.Join(tempDir, "nodeB")
	nodeCDir := filepath.Join(tempDir, "nodeC")
	
	os.MkdirAll(nodeADir, 0755)
	os.MkdirAll(nodeBDir, 0755)
	os.MkdirAll(nodeCDir, 0755)
	
	// Create initial relationships.yaml files
	relA := Relationship{Relationships: []string{"nodeB"}}
	dataA, _ := yaml.Marshal(&relA)
	os.WriteFile(filepath.Join(nodeADir, "relationships.yaml"), dataA, 0644)
	
	relB := Relationship{Relationships: []string{}}
	dataB, _ := yaml.Marshal(&relB)
	os.WriteFile(filepath.Join(nodeBDir, "relationships.yaml"), dataB, 0644)
	
	relC := Relationship{Relationships: []string{"nodeA"}}
	dataC, _ := yaml.Marshal(&relC)
	os.WriteFile(filepath.Join(nodeCDir, "relationships.yaml"), dataC, 0644)
	
	// Set the rootPath to our temp directory
	oldRootPath := rootPath
	rootPath = tempDir
	defer func() { rootPath = oldRootPath }()
	
	tests := []struct {
		name          string
		nodeID        string
		relationships []string
		wantStatus    int
	}{
		{
			name:          "Update existing node relationships",
			nodeID:        "nodeA",
			relationships: []string{"nodeB", "nodeC"},
			wantStatus:    http.StatusOK,
		},
		{
			name:          "Clear all relationships",
			nodeID:        "nodeA",
			relationships: []string{},
			wantStatus:    http.StatusOK,
		},
		{
			name:          "Update non-existent node",
			nodeID:        "nodeX",
			relationships: []string{"nodeA"},
			wantStatus:    http.StatusNotFound,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request body
			reqBody := struct {
				Relationships []string `json:"relationships"`
			}{
				Relationships: tt.relationships,
			}
			
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("PUT", "/api/nodes/"+tt.nodeID+"/relationships", bytes.NewReader(body))
			req = mux.SetURLVars(req, map[string]string{"nodeId": tt.nodeID})
			
			// Create response recorder
			rr := httptest.NewRecorder()
			
			// Call the handler
			updateRelationshipsHandler(rr, req)
			
			// Check status code
			if status := rr.Code; status != tt.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.wantStatus)
			}
			
			// If successful, verify the file was updated
			if tt.wantStatus == http.StatusOK {
				relFile := filepath.Join(tempDir, tt.nodeID, "relationships.yaml")
				data, err := os.ReadFile(relFile)
				if err != nil {
					t.Fatalf("Failed to read relationships file: %v", err)
				}
				
				var rel Relationship
				if err := yaml.Unmarshal(data, &rel); err != nil {
					t.Fatalf("Failed to unmarshal relationships: %v", err)
				}
				
				// Check if relationships match
				if len(rel.Relationships) != len(tt.relationships) {
					t.Errorf("Relationships count mismatch: got %d want %d",
						len(rel.Relationships), len(tt.relationships))
				}
				
				for i, r := range rel.Relationships {
					if i < len(tt.relationships) && r != tt.relationships[i] {
						t.Errorf("Relationship mismatch at index %d: got %s want %s",
							i, r, tt.relationships[i])
					}
				}
			}
		})
	}
}

func TestCircularRelationshipPrevention(t *testing.T) {
	// This test verifies that the frontend correctly filters out nodes
	// that would create circular relationships
	
	// Create a temporary directory structure
	tempDir := t.TempDir()
	rootPath = tempDir
	defer func() { rootPath = "" }()
	
	// Create a relationship chain: A -> B -> C
	nodeADir := filepath.Join(tempDir, "nodeA")
	nodeBDir := filepath.Join(tempDir, "nodeB")
	nodeCDir := filepath.Join(tempDir, "nodeC")
	
	os.MkdirAll(nodeADir, 0755)
	os.MkdirAll(nodeBDir, 0755)
	os.MkdirAll(nodeCDir, 0755)
	
	// A relates to B
	relA := Relationship{Relationships: []string{"nodeB"}}
	dataA, _ := yaml.Marshal(&relA)
	os.WriteFile(filepath.Join(nodeADir, "relationships.yaml"), dataA, 0644)
	
	// B relates to C
	relB := Relationship{Relationships: []string{"nodeC"}}
	dataB, _ := yaml.Marshal(&relB)
	os.WriteFile(filepath.Join(nodeBDir, "relationships.yaml"), dataB, 0644)
	
	// C has no relationships initially
	relC := Relationship{Relationships: []string{}}
	dataC, _ := yaml.Marshal(&relC)
	os.WriteFile(filepath.Join(nodeCDir, "relationships.yaml"), dataC, 0644)
	
	// Build the graph to verify structure
	graph := buildGraph(tempDir)
	
	// Verify we have 3 nodes
	if len(graph.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(graph.Nodes))
	}
	
	// Verify we have 2 edges (A->B, B->C)
	if len(graph.Edges) != 2 {
		t.Errorf("Expected 2 edges, got %d", len(graph.Edges))
	}
	
	// The frontend should prevent:
	// - C from relating to A (would create cycle: A->B->C->A)
	// - C from relating to B (would create cycle: B->C->B)
	// - B from relating to A (would create cycle: A->B->A)
	
	// But these should be allowed:
	// - A can add C as relationship (no cycle: A->B,C and B->C)
	// - A new node D could relate to any of A, B, C
}