package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
)

func TestIntegrationPositionPersistence(t *testing.T) {
	// Setup temporary directory and storage
	tempDir, err := os.MkdirTemp("", "integration_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Save original values
	oldStorage := storage
	oldRootPath := rootPath

	// Set test values
	rootPath = "../example"
	storage, err = NewPositionStorage(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Restore original values after test
	defer func() {
		storage.Close()
		storage = oldStorage
		rootPath = oldRootPath
	}()

	// Create router with our handlers
	router := mux.NewRouter()
	router.HandleFunc("/api/positions", getPositionsHandler).Methods("GET")
	router.HandleFunc("/api/positions", savePositionsHandler).Methods("POST")

	// Test 1: Initial GET should return empty positions
	req := httptest.NewRequest("GET", "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var initialPositions map[string]NodePosition
	err = json.NewDecoder(w.Body).Decode(&initialPositions)
	if err != nil {
		t.Fatalf("Failed to decode initial positions: %v", err)
	}

	if len(initialPositions) != 0 {
		t.Errorf("Expected 0 initial positions, got %d", len(initialPositions))
	}

	// Test 2: Save some positions
	testPositions := []NodePosition{
		{NodeID: "api", X: 123.45, Y: 678.90},
		{NodeID: "frontend", X: 234.56, Y: 789.01},
		{NodeID: "database", X: 345.67, Y: 890.12},
	}

	body, _ := json.Marshal(testPositions)
	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for POST, got %d", w.Code)
	}

	// Test 3: GET should now return saved positions
	req = httptest.NewRequest("GET", "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var savedPositions map[string]NodePosition
	err = json.NewDecoder(w.Body).Decode(&savedPositions)
	if err != nil {
		t.Fatalf("Failed to decode saved positions: %v", err)
	}

	if len(savedPositions) != len(testPositions) {
		t.Errorf("Expected %d saved positions, got %d", len(testPositions), len(savedPositions))
	}

	// Verify each position
	for _, expectedPos := range testPositions {
		savedPos, exists := savedPositions[expectedPos.NodeID]
		if !exists {
			t.Errorf("Position for node %s not found", expectedPos.NodeID)
			continue
		}

		if savedPos.X != expectedPos.X || savedPos.Y != expectedPos.Y {
			t.Errorf("Position mismatch for %s: expected (%.2f, %.2f), got (%.2f, %.2f)",
				expectedPos.NodeID, expectedPos.X, expectedPos.Y, savedPos.X, savedPos.Y)
		}
	}

	// Test 4: Update positions
	updatedPositions := []NodePosition{
		{NodeID: "api", X: 999.99, Y: 888.88},
		{NodeID: "frontend", X: 777.77, Y: 666.66},
	}

	body, _ = json.Marshal(updatedPositions)
	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for update POST, got %d", w.Code)
	}

	// Test 5: Verify updates were saved
	req = httptest.NewRequest("GET", "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var finalPositions map[string]NodePosition
	err = json.NewDecoder(w.Body).Decode(&finalPositions)
	if err != nil {
		t.Fatalf("Failed to decode final positions: %v", err)
	}

	// Check updated positions
	for _, updatedPos := range updatedPositions {
		savedPos, exists := finalPositions[updatedPos.NodeID]
		if !exists {
			t.Errorf("Updated position for node %s not found", updatedPos.NodeID)
			continue
		}

		if savedPos.X != updatedPos.X || savedPos.Y != updatedPos.Y {
			t.Errorf("Updated position mismatch for %s: expected (%.2f, %.2f), got (%.2f, %.2f)",
				updatedPos.NodeID, updatedPos.X, updatedPos.Y, savedPos.X, savedPos.Y)
		}
	}

	// Database node should still have original position
	if dbPos, exists := finalPositions["database"]; exists {
		if dbPos.X != 345.67 || dbPos.Y != 890.12 {
			t.Errorf("Database position changed unexpectedly: got (%.2f, %.2f)",
				dbPos.X, dbPos.Y)
		}
	} else {
		t.Error("Database position was lost after update")
	}
}

func TestInvalidRequests(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "invalid_test")
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

	// Test invalid JSON
	req := httptest.NewRequest("POST", "/api/positions", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	savePositionsHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", w.Code)
	}

	// Test empty body
	req = httptest.NewRequest("POST", "/api/positions", nil)
	w = httptest.NewRecorder()
	savePositionsHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty body, got %d", w.Code)
	}
}

// Helper function to print positions for debugging
func printPositions(positions map[string]NodePosition) {
	fmt.Println("Current positions:")
	for id, pos := range positions {
		fmt.Printf("  %s: (%.2f, %.2f)\n", id, pos.X, pos.Y)
	}
}