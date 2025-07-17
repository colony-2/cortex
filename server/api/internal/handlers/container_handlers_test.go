package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/divisive-ai/vibethis/server/container/pkg/container"
	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/gorilla/mux"
)

// mockContainerManager implements container.Manager for testing
type mockContainerManager struct {
	createCalled   bool
	receivedPath   string
	containerIDRet string
	createError    error
}

func (m *mockContainerManager) Create(ctx context.Context, nodePath string) (containerID string, err error) {
	m.createCalled = true
	m.receivedPath = nodePath
	return m.containerIDRet, m.createError
}

func (m *mockContainerManager) Start(ctx context.Context, containerID string) error {
	return nil
}

func (m *mockContainerManager) Stop(ctx context.Context, containerID string) error {
	return nil
}

func (m *mockContainerManager) Restart(ctx context.Context, containerID string) error {
	return nil
}

func (m *mockContainerManager) Remove(ctx context.Context, containerID string) error {
	return nil
}

func (m *mockContainerManager) GetInfo(ctx context.Context, containerID string) (*container.Info, error) {
	return nil, nil
}

func (m *mockContainerManager) GetStatus(ctx context.Context, containerID string) (container.Status, error) {
	return container.StatusNone, nil
}

func (m *mockContainerManager) Exec(ctx context.Context, containerID string, command []string) (output string, err error) {
	return "", nil
}

func (m *mockContainerManager) AttachWebSocket(ctx context.Context, containerID string) (container.TerminalConnection, error) {
	return nil, nil
}

func TestCreateContainer_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockContainer := &mockContainerManager{
		containerIDRet: "test-container-123",
	}

	mockGraph := &mockGraphBuilder{
		nodes: map[string]*core.Node{
			"test-node": {
				ID:   "test-node",
				Name: "Test Node",
				Path: "/actual/path/to/node",
				Type: "app",
			},
		},
	}

	mockStorage := newMockStorage()

	// Create handlers
	h := &Handlers{
		container: mockContainer,
		graph:     mockGraph,
		storage:   mockStorage,
	}

	// Create request
	req := httptest.NewRequest("POST", "/api/nodes/test-node/container/create", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
	w := httptest.NewRecorder()

	// Call handler
	h.CreateContainer(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Verify the container manager was called with the actual path, not the node ID
	if !mockContainer.createCalled {
		t.Error("Create was not called")
	}
	if mockContainer.receivedPath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockContainer.receivedPath)
	}

	// Verify response contains container ID
	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if response["containerId"] != "test-container-123" {
		t.Errorf("Expected containerId 'test-container-123', got '%s'", response["containerId"])
	}

	// Verify container ID was saved to storage
	savedID, err := mockStorage.GetContainerID(context.Background(), "test-node")
	if err != nil {
		t.Errorf("Failed to get saved container ID: %v", err)
	}
	if savedID != "test-container-123" {
		t.Errorf("Expected saved container ID 'test-container-123', got '%s'", savedID)
	}
}

func TestCreateContainer_NodeNotFound(t *testing.T) {
	// Create mocks with no nodes
	mockContainer := &mockContainerManager{}
	mockGraph := &mockGraphBuilder{
		nodes: map[string]*core.Node{},
	}
	mockStorage := newMockStorage()

	// Create handlers
	h := &Handlers{
		container: mockContainer,
		graph:     mockGraph,
		storage:   mockStorage,
	}

	// Create request
	req := httptest.NewRequest("POST", "/api/nodes/nonexistent/container/create", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeId": "nonexistent"})
	w := httptest.NewRecorder()

	// Call handler
	h.CreateContainer(w, req)

	// Verify 404 response
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
	if body := w.Body.String(); !bytes.Contains([]byte(body), []byte("Node not found")) {
		t.Errorf("Expected 'Node not found' in response, got: %s", body)
	}

	// Verify container manager was not called
	if mockContainer.createCalled {
		t.Error("Create should not have been called")
	}
}
