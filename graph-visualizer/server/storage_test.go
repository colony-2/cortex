package main

import (
	"os"
	"testing"
)

func TestPositionStorage(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "graph_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage instance
	storage, err := NewPositionStorage(tempDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test data
	graphPath := "test/path"
	positions := []NodePosition{
		{NodeID: "node1", X: 100.5, Y: 200.5},
		{NodeID: "node2", X: 300.0, Y: 400.0},
		{NodeID: "node3", X: 500.5, Y: 600.5},
	}

	// Test saving positions
	err = storage.SavePositions(graphPath, positions)
	if err != nil {
		t.Fatalf("Failed to save positions: %v", err)
	}

	// Test retrieving positions
	retrieved, err := storage.GetPositions(graphPath)
	if err != nil {
		t.Fatalf("Failed to get positions: %v", err)
	}

	// Verify all positions were saved and retrieved correctly
	if len(retrieved) != len(positions) {
		t.Errorf("Expected %d positions, got %d", len(positions), len(retrieved))
	}

	for _, pos := range positions {
		retrievedPos, exists := retrieved[pos.NodeID]
		if !exists {
			t.Errorf("Position for node %s not found", pos.NodeID)
			continue
		}

		if retrievedPos.X != pos.X || retrievedPos.Y != pos.Y {
			t.Errorf("Position mismatch for node %s: expected (%.1f, %.1f), got (%.1f, %.1f)",
				pos.NodeID, pos.X, pos.Y, retrievedPos.X, retrievedPos.Y)
		}
	}

	// Test retrieving positions for a different graph path
	otherPositions, err := storage.GetPositions("other/path")
	if err != nil {
		t.Fatalf("Failed to get positions for other path: %v", err)
	}

	if len(otherPositions) != 0 {
		t.Errorf("Expected 0 positions for other path, got %d", len(otherPositions))
	}

	// Test updating a position
	positions[0].X = 150.0
	positions[0].Y = 250.0
	err = storage.SavePosition(graphPath, positions[0])
	if err != nil {
		t.Fatalf("Failed to update position: %v", err)
	}

	// Verify the update
	retrieved, err = storage.GetPositions(graphPath)
	if err != nil {
		t.Fatalf("Failed to get positions after update: %v", err)
	}

	updatedPos := retrieved["node1"]
	if updatedPos.X != 150.0 || updatedPos.Y != 250.0 {
		t.Errorf("Position update failed: expected (150.0, 250.0), got (%.1f, %.1f)",
			updatedPos.X, updatedPos.Y)
	}
}

func TestPositionStorageWithEmptyDatabase(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "graph_test_empty")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := NewPositionStorage(tempDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test getting positions from empty database
	positions, err := storage.GetPositions("any/path")
	if err != nil {
		t.Fatalf("Failed to get positions from empty database: %v", err)
	}

	if len(positions) != 0 {
		t.Errorf("Expected 0 positions from empty database, got %d", len(positions))
	}
}