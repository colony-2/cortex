package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"vibethis/core/pkg/core"
)

// mockStorage implements core.Storage for testing
type mockStorage struct {
	positions    map[string]core.Position
	containerIDs map[string]string
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		positions:    make(map[string]core.Position),
		containerIDs: make(map[string]string),
	}
}

func (m *mockStorage) SavePosition(ctx context.Context, pos core.Position) error {
	m.positions[pos.NodeID] = pos
	return nil
}

func (m *mockStorage) GetPositions(ctx context.Context) ([]core.Position, error) {
	var positions []core.Position
	for _, pos := range m.positions {
		positions = append(positions, pos)
	}
	return positions, nil
}

func (m *mockStorage) DeletePosition(ctx context.Context, nodeID string) error {
	delete(m.positions, nodeID)
	return nil
}

func (m *mockStorage) SaveContainerID(ctx context.Context, nodeID, containerID string) error {
	m.containerIDs[nodeID] = containerID
	return nil
}

func (m *mockStorage) GetContainerID(ctx context.Context, nodeID string) (string, error) {
	id, exists := m.containerIDs[nodeID]
	if !exists {
		return "", ErrNotFound
	}
	return id, nil
}

func (m *mockStorage) DeleteContainerID(ctx context.Context, nodeID string) error {
	delete(m.containerIDs, nodeID)
	return nil
}

func (m *mockStorage) Close() error {
	return nil
}

var ErrNotFound = fmt.Errorf("not found")

func TestPositionHandlers(t *testing.T) {
	// Create mock storage
	storage := newMockStorage()
	
	// Create handlers
	h := &Handlers{
		storage: storage,
	}

	// Test GET positions (empty)
	req := httptest.NewRequest("GET", "/api/positions", nil)
	w := httptest.NewRecorder()
	h.GetPositions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var positions []core.Position
	err := json.NewDecoder(w.Body).Decode(&positions)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(positions) != 0 {
		t.Errorf("Expected empty positions, got %d", len(positions))
	}

	// Test POST positions
	testPositions := []core.Position{
		{NodeID: "node1", X: 100.0, Y: 200.0},
		{NodeID: "node2", X: 300.0, Y: 400.0},
	}

	body, err := json.Marshal(testPositions)
	if err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader(body))
	w = httptest.NewRecorder()
	h.SavePositions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test GET positions (after save)
	req = httptest.NewRequest("GET", "/api/positions", nil)
	w = httptest.NewRecorder()
	h.GetPositions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	positions = nil
	err = json.NewDecoder(w.Body).Decode(&positions)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(positions) != len(testPositions) {
		t.Errorf("Expected %d positions, got %d", len(testPositions), len(positions))
	}

	// Verify positions were saved correctly
	posMap := make(map[string]core.Position)
	for _, pos := range positions {
		posMap[pos.NodeID] = pos
	}

	for _, expected := range testPositions {
		actual, exists := posMap[expected.NodeID]
		if !exists {
			t.Errorf("Position for node %s not found", expected.NodeID)
			continue
		}

		if actual.X != expected.X || actual.Y != expected.Y {
			t.Errorf("Position mismatch for node %s: expected (%.1f, %.1f), got (%.1f, %.1f)",
				expected.NodeID, expected.X, expected.Y, actual.X, actual.Y)
		}
	}

	// Test POST with invalid JSON
	req = httptest.NewRequest("POST", "/api/positions", bytes.NewReader([]byte("invalid json")))
	w = httptest.NewRecorder()
	h.SavePositions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", w.Code)
	}

	// Test POST with empty body
	req = httptest.NewRequest("POST", "/api/positions", nil)
	w = httptest.NewRecorder()
	h.SavePositions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty body, got %d", w.Code)
	}
}