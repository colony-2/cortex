package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/gorilla/mux"
)

// mockGitRepository implements git.Repository for testing
type mockGitRepository struct {
	getStatusCalled    bool
	getDiffCalled      bool
	getHistoryCalled   bool
	createCommitCalled bool
	stageFilesCalled   bool
	receivedPath       string
	receivedStaged     bool
	receivedLimit      int
	receivedMessage    string
	receivedFiles      []string
	statusReturn       *git.Status
	statusError        error
	diffReturn         string
	diffError          error
	historyReturn      []git.Commit
	historyError       error
	stageError         error
	commitError        error
}

func (m *mockGitRepository) GetStatus(ctx context.Context, path string) (*git.Status, error) {
	m.getStatusCalled = true
	m.receivedPath = path
	return m.statusReturn, m.statusError
}

func (m *mockGitRepository) GetDiff(ctx context.Context, path string, staged bool) (string, error) {
	m.getDiffCalled = true
	m.receivedPath = path
	m.receivedStaged = staged
	return m.diffReturn, m.diffError
}

func (m *mockGitRepository) GetHistory(ctx context.Context, path string, limit int) ([]git.Commit, error) {
	m.getHistoryCalled = true
	m.receivedPath = path
	m.receivedLimit = limit
	return m.historyReturn, m.historyError
}

func (m *mockGitRepository) StageFiles(ctx context.Context, path string, files []string) error {
	m.stageFilesCalled = true
	m.receivedPath = path
	m.receivedFiles = files
	return m.stageError
}

func (m *mockGitRepository) CreateCommit(ctx context.Context, path string, message string) error {
	m.createCommitCalled = true
	m.receivedPath = path
	m.receivedMessage = message
	return m.commitError
}

func (m *mockGitRepository) UnstageFiles(ctx context.Context, path string, files []string) error {
	return nil
}

func TestGetGitStatus_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockGit := &mockGitRepository{
		statusReturn: &git.Status{
			Branch: "main",
			Clean:  false,
			Files: []git.StatusFile{
				{Path: "file1.txt", Status: "M"},
				{Path: "new.txt", Status: "?"},
			},
			Ahead:     0,
			Behind:    0,
			HasRemote: true,
		},
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

	// Create handlers
	h := &Handlers{
		git:   mockGit,
		graph: mockGraph,
	}

	// Create request
	req := httptest.NewRequest("GET", "/api/nodes/test-node/git/status", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
	w := httptest.NewRecorder()

	// Call handler
	h.GetGitStatus(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Verify the git repository was called with the actual path, not the node ID
	if !mockGit.getStatusCalled {
		t.Error("GetStatus was not called")
	}
	if mockGit.receivedPath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockGit.receivedPath)
	}
}

func TestGetGitDiff_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockGit := &mockGitRepository{
		diffReturn: "diff --git a/file.txt b/file.txt\n...",
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

	// Create handlers
	h := &Handlers{
		git:   mockGit,
		graph: mockGraph,
	}

	// Create request with staged=true
	req := httptest.NewRequest("GET", "/api/nodes/test-node/git/diff?staged=true", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
	w := httptest.NewRecorder()

	// Call handler
	h.GetGitDiff(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Verify the git repository was called with the actual path, not the node ID
	if !mockGit.getDiffCalled {
		t.Error("GetDiff was not called")
	}
	if mockGit.receivedPath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockGit.receivedPath)
	}
	if !mockGit.receivedStaged {
		t.Error("Expected staged=true")
	}
}

func TestGetGitHistory_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockGit := &mockGitRepository{
		historyReturn: []git.Commit{
			{Hash: "abc123", Message: "Test commit", Author: "Test User"},
		},
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

	// Create handlers
	h := &Handlers{
		git:   mockGit,
		graph: mockGraph,
	}

	// Create request with limit
	req := httptest.NewRequest("GET", "/api/nodes/test-node/git/history?limit=10", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
	w := httptest.NewRecorder()

	// Call handler
	h.GetGitHistory(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Verify the git repository was called with the actual path, not the node ID
	if !mockGit.getHistoryCalled {
		t.Error("GetHistory was not called")
	}
	if mockGit.receivedPath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockGit.receivedPath)
	}
	if mockGit.receivedLimit != 10 {
		t.Errorf("Expected limit 10, got %d", mockGit.receivedLimit)
	}
}

func TestCreateGitCommit_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockGit := &mockGitRepository{}

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

	// Create handlers
	h := &Handlers{
		git:   mockGit,
		graph: mockGraph,
	}

	// Create request body
	reqBody := map[string]interface{}{
		"message": "Test commit message",
		"files":   []string{"file1.txt", "file2.txt"},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// Create request
	req := httptest.NewRequest("POST", "/api/nodes/test-node/git/commit", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
	w := httptest.NewRecorder()

	// Call handler
	h.CreateGitCommit(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Verify the git repository was called with the actual path, not the node ID
	if !mockGit.stageFilesCalled {
		t.Error("StageFiles was not called")
	}
	if !mockGit.createCommitCalled {
		t.Error("CreateCommit was not called")
	}
	if mockGit.receivedPath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockGit.receivedPath)
	}
	if mockGit.receivedMessage != "Test commit message" {
		t.Errorf("Expected message 'Test commit message', got '%s'", mockGit.receivedMessage)
	}
	if len(mockGit.receivedFiles) != 2 || mockGit.receivedFiles[0] != "file1.txt" {
		t.Errorf("Expected files [file1.txt, file2.txt], got %v", mockGit.receivedFiles)
	}
}

func TestGitHandlers_NodeNotFound(t *testing.T) {
	// Create mocks with no nodes
	mockGit := &mockGitRepository{}
	mockGraph := &mockGraphBuilder{
		nodes: map[string]*core.Node{},
	}

	// Create handlers
	h := &Handlers{
		git:   mockGit,
		graph: mockGraph,
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"GetGitStatus", "GET", "/api/nodes/nonexistent/git/status", nil},
		{"GetGitDiff", "GET", "/api/nodes/nonexistent/git/diff", nil},
		{"GetGitHistory", "GET", "/api/nodes/nonexistent/git/history", nil},
		{"CreateGitCommit", "POST", "/api/nodes/nonexistent/git/commit", []byte(`{"message":"test"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			var req *http.Request
			if tt.body != nil {
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}

			req = mux.SetURLVars(req, map[string]string{"nodeId": "nonexistent"})
			w := httptest.NewRecorder()

			// Call appropriate handler
			switch tt.name {
			case "GetGitStatus":
				h.GetGitStatus(w, req)
			case "GetGitDiff":
				h.GetGitDiff(w, req)
			case "GetGitHistory":
				h.GetGitHistory(w, req)
			case "CreateGitCommit":
				h.CreateGitCommit(w, req)
			}

			// Verify 404 response
			if w.Code != http.StatusNotFound {
				t.Errorf("Expected status 404, got %d", w.Code)
			}
			if body := w.Body.String(); !bytes.Contains([]byte(body), []byte("Node not found")) {
				t.Errorf("Expected 'Node not found' in response, got: %s", body)
			}
		})
	}
}
