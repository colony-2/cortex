package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPositionAPI(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "api_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldStorage := storage
	storage, err = NewPositionStorage(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		storage.Close()
		storage = oldStorage
	}()

	// Test GET positions (empty)
	req := httptest.NewRequest("GET", "/api/positions", nil)
	w := httptest.NewRecorder()
	getPositionsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var positions map[string]NodePosition
	err = json.NewDecoder(w.Body).Decode(&positions)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(positions) != 0 {
		t.Errorf("Expected empty positions, got %d", len(positions))
	}

	// Test POST positions
	testPositions := []NodePosition{
		{NodeID: "api", X: 100, Y: 200},
		{NodeID: "frontend", X: 300, Y: 400},
	}

	body, _ := json.Marshal(testPositions)
	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	savePositionsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test GET positions after save
	req = httptest.NewRequest("GET", "/api/positions", nil)
	w = httptest.NewRecorder()
	getPositionsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	err = json.NewDecoder(w.Body).Decode(&positions)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(positions) != 2 {
		t.Errorf("Expected 2 positions, got %d", len(positions))
	}

	// Verify the positions
	for _, testPos := range testPositions {
		savedPos, exists := positions[testPos.NodeID]
		if !exists {
			t.Errorf("Position for %s not found", testPos.NodeID)
			continue
		}
		if savedPos.X != testPos.X || savedPos.Y != testPos.Y {
			t.Errorf("Position mismatch for %s: expected (%.0f, %.0f), got (%.0f, %.0f)",
				testPos.NodeID, testPos.X, testPos.Y, savedPos.X, savedPos.Y)
		}
	}
}