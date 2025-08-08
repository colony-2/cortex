package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/gorilla/mux"
)

func TestIntegrationPositionPersistence(t *testing.T) {
	// Create mock storage
	storage := newMockStorage()

	// Create handlers
	h := &Handlers{
		storage: storage,
	}

	// Create router with our handlers
	router := mux.NewRouter()
	router.HandleFunc("/api/positions", h.GetPositions).Methods("GET")
	router.HandleFunc("/api/positions", h.SavePositions).Methods("POST")

	// Test 1: Initial GET should return empty positions
	req := httptest.NewRequest("GET", "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var positions []core.Position
	if err := json.NewDecoder(w.Body).Decode(&positions); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(positions) != 0 {
		t.Errorf("Expected 0 positions initially, got %d", len(positions))
	}

	// Test 2: Save some positions
	testPositions := []core.Position{
		{CellID: "cell1", X: 100.5, Y: 200.5},
		{CellID: "cell2", X: 300.0, Y: 400.0},
		{CellID: "cell3", X: 500.5, Y: 600.5},
	}

	body, err := json.Marshal(testPositions)
	if err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Test 3: GET should now return the saved positions
	req = httptest.NewRequest("GET", "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	positions = nil
	if err := json.NewDecoder(w.Body).Decode(&positions); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(positions) != len(testPositions) {
		t.Errorf("Expected %d positions, got %d", len(testPositions), len(positions))
	}

	// Verify each position
	posMap := make(map[string]core.Position)
	for _, pos := range positions {
		posMap[pos.CellID] = pos
	}

	for _, expected := range testPositions {
		actual, exists := posMap[expected.CellID]
		if !exists {
			t.Errorf("Position for cell %s not found", expected.CellID)
			continue
		}

		if actual.X != expected.X || actual.Y != expected.Y {
			t.Errorf("Position mismatch for cell %s: expected (%.1f, %.1f), got (%.1f, %.1f)",
				expected.CellID, expected.X, expected.Y, actual.X, actual.Y)
		}
	}

	// Test 4: Update positions (partial update)
	updatedPositions := []core.Position{
		{CellID: "cell1", X: 150.0, Y: 250.0}, // Updated
		{CellID: "cell4", X: 700.0, Y: 800.0}, // New
	}

	body, err = json.Marshal(updatedPositions)
	if err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Test 5: Verify updates
	req = httptest.NewRequest("GET", "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	positions = nil
	if err := json.NewDecoder(w.Body).Decode(&positions); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should now have 4 positions (3 original + 1 new, with 1 updated)
	if len(positions) != 4 {
		t.Errorf("Expected 4 positions after update, got %d", len(positions))
	}

	// Verify cell1 was updated
	posMap = make(map[string]core.Position)
	for _, pos := range positions {
		posMap[pos.CellID] = pos
	}

	cell1 := posMap["cell1"]
	if cell1.X != 150.0 || cell1.Y != 250.0 {
		t.Errorf("cell1 not updated correctly: expected (150.0, 250.0), got (%.1f, %.1f)",
			cell1.X, cell1.Y)
	}

	// Verify cell4 was added
	if _, exists := posMap["cell4"]; !exists {
		t.Error("cell4 was not added")
	}

	// Verify cell2 and cell3 remain unchanged
	cell2 := posMap["cell2"]
	if cell2.X != 300.0 || cell2.Y != 400.0 {
		t.Error("cell2 was unexpectedly modified")
	}

	cell3 := posMap["cell3"]
	if cell3.X != 500.5 || cell3.Y != 600.5 {
		t.Error("cell3 was unexpectedly modified")
	}
}

func TestIntegrationErrorHandling(t *testing.T) {
	// Create mock storage
	storage := newMockStorage()

	// Create handlers
	h := &Handlers{
		storage: storage,
	}

	// Create router
	router := mux.NewRouter()
	router.HandleFunc("/api/positions", h.SavePositions).Methods("POST")

	// Test 1: Invalid JSON
	req := httptest.NewRequest("POST", "/api/positions", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", w.Code)
	}

	// Test 2: Wrong data type
	wrongData := map[string]string{"wrong": "type"}
	body, _ := json.Marshal(wrongData)

	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for wrong data type, got %d", w.Code)
	}

	// Test 3: Empty array (should succeed)
	emptyPositions := []core.Position{}
	body, _ = json.Marshal(emptyPositions)

	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for empty array, got %d", w.Code)
	}
}
