package bolt

import (
	"context"
	"os"
	"testing"
	"vibethis/core/pkg/core"
)

func TestPositionStorage(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "storage_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage instance
	storage, err := New(tempDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Test data
	positions := []core.Position{
		{NodeID: "node1", X: 100.5, Y: 200.5},
		{NodeID: "node2", X: 300.0, Y: 400.0},
		{NodeID: "node3", X: 500.5, Y: 600.5},
	}

	// Test saving positions
	for _, pos := range positions {
		err = storage.SavePosition(ctx, pos)
		if err != nil {
			t.Fatalf("Failed to save position: %v", err)
		}
	}

	// Test retrieving positions
	retrieved, err := storage.GetPositions(ctx)
	if err != nil {
		t.Fatalf("Failed to get positions: %v", err)
	}

	// Verify all positions were saved and retrieved correctly
	if len(retrieved) != len(positions) {
		t.Errorf("Expected %d positions, got %d", len(positions), len(retrieved))
	}

	// Create a map for easy lookup
	retrievedMap := make(map[string]core.Position)
	for _, pos := range retrieved {
		retrievedMap[pos.NodeID] = pos
	}

	for _, pos := range positions {
		retrievedPos, exists := retrievedMap[pos.NodeID]
		if !exists {
			t.Errorf("Position for node %s was not retrieved", pos.NodeID)
			continue
		}

		if retrievedPos.X != pos.X || retrievedPos.Y != pos.Y {
			t.Errorf("Position mismatch for node %s: expected (%.1f, %.1f), got (%.1f, %.1f)",
				pos.NodeID, pos.X, pos.Y, retrievedPos.X, retrievedPos.Y)
		}
	}

	// Test updating a position
	updatedPos := core.Position{NodeID: "node1", X: 150.0, Y: 250.0}
	err = storage.SavePosition(ctx, updatedPos)
	if err != nil {
		t.Fatalf("Failed to update position: %v", err)
	}

	// Verify the update
	retrieved, err = storage.GetPositions(ctx)
	if err != nil {
		t.Fatalf("Failed to get positions after update: %v", err)
	}

	// Still should have the same number of positions
	if len(retrieved) != len(positions) {
		t.Errorf("Expected %d positions after update, got %d", len(positions), len(retrieved))
	}

	// Check that node1 was updated
	retrievedMap = make(map[string]core.Position)
	for _, pos := range retrieved {
		retrievedMap[pos.NodeID] = pos
	}

	if node1Pos, exists := retrievedMap["node1"]; exists {
		if node1Pos.X != updatedPos.X || node1Pos.Y != updatedPos.Y {
			t.Errorf("Updated position not saved correctly: expected (%.1f, %.1f), got (%.1f, %.1f)",
				updatedPos.X, updatedPos.Y, node1Pos.X, node1Pos.Y)
		}
	} else {
		t.Error("Updated node1 position not found")
	}

	// Test deleting a position
	err = storage.DeletePosition(ctx, "node2")
	if err != nil {
		t.Fatalf("Failed to delete position: %v", err)
	}

	retrieved, err = storage.GetPositions(ctx)
	if err != nil {
		t.Fatalf("Failed to get positions after delete: %v", err)
	}

	if len(retrieved) != len(positions)-1 {
		t.Errorf("Expected %d positions after delete, got %d", len(positions)-1, len(retrieved))
	}
}

func TestContainerIDStorage(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "storage_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage instance
	storage, err := New(tempDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Test saving container ID
	nodeID := "test-node"
	containerID := "abc123"
	
	err = storage.SaveContainerID(ctx, nodeID, containerID)
	if err != nil {
		t.Fatalf("Failed to save container ID: %v", err)
	}

	// Test retrieving container ID
	retrieved, err := storage.GetContainerID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Failed to get container ID: %v", err)
	}

	if retrieved != containerID {
		t.Errorf("Expected container ID %s, got %s", containerID, retrieved)
	}

	// Test retrieving non-existent container ID
	_, err = storage.GetContainerID(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent container ID")
	}

	// Test deleting container ID
	err = storage.DeleteContainerID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Failed to delete container ID: %v", err)
	}

	// Verify deletion
	_, err = storage.GetContainerID(ctx, nodeID)
	if err == nil {
		t.Error("Expected error when getting deleted container ID")
	}
}