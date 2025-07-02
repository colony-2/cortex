package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"vibethis/core/pkg/core"
	"vibethis/files/pkg/files"
)

// mockFilesBrowser implements files.Browser for testing
type mockFilesBrowser struct {
	listFilesCalled   bool
	readFileCalled    bool
	writeFileCalled   bool
	receivedNodePath  string
	receivedFilePath  string
	receivedContent   []byte
	listFilesReturn   []files.FileInfo
	listFilesError    error
	readFileReturn    []byte
	readFileError     error
	writeFileError    error
}

func (m *mockFilesBrowser) ListFiles(ctx context.Context, nodePath string) ([]files.FileInfo, error) {
	m.listFilesCalled = true
	m.receivedNodePath = nodePath
	return m.listFilesReturn, m.listFilesError
}

func (m *mockFilesBrowser) ReadFile(ctx context.Context, nodePath, filePath string) ([]byte, error) {
	m.readFileCalled = true
	m.receivedNodePath = nodePath
	m.receivedFilePath = filePath
	return m.readFileReturn, m.readFileError
}

func (m *mockFilesBrowser) WriteFile(ctx context.Context, nodePath, filePath string, content []byte) error {
	m.writeFileCalled = true
	m.receivedNodePath = nodePath
	m.receivedFilePath = filePath
	m.receivedContent = content
	return m.writeFileError
}

func (m *mockFilesBrowser) CreateDirectory(ctx context.Context, nodePath, dirPath string) error {
	return nil
}

func (m *mockFilesBrowser) Delete(ctx context.Context, nodePath, path string) error {
	return nil
}

// mockGraphBuilder implements core.GraphBuilder for testing
type mockGraphBuilder struct {
	nodes map[string]*core.Node
}

func (m *mockGraphBuilder) BuildGraph(ctx context.Context) (*core.Graph, error) {
	return nil, nil
}

func (m *mockGraphBuilder) GetNode(ctx context.Context, nodeID string) (*core.Node, error) {
	node, ok := m.nodes[nodeID]
	if !ok {
		return nil, context.DeadlineExceeded // simulating node not found
	}
	return node, nil
}

func TestGetFiles_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockFiles := &mockFilesBrowser{
		listFilesReturn: []files.FileInfo{
			{Name: "test.txt", Path: "test.txt", IsDir: false, Size: 100, Type: "text"},
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
		files: mockFiles,
		graph: mockGraph,
	}
	
	// Create request
	req := httptest.NewRequest("GET", "/api/nodes/test-node/files", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
	w := httptest.NewRecorder()
	
	// Call handler
	h.GetFiles(w, req)
	
	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	
	// Verify the file browser was called with the actual path, not the node ID
	if !mockFiles.listFilesCalled {
		t.Error("ListFiles was not called")
	}
	if mockFiles.receivedNodePath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockFiles.receivedNodePath)
	}
}

func TestGetFile_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockFiles := &mockFilesBrowser{
		readFileReturn: []byte("test content"),
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
		files: mockFiles,
		graph: mockGraph,
	}
	
	// Create request
	req := httptest.NewRequest("GET", "/api/nodes/test-node/files/subdir/file.txt", nil)
	req = mux.SetURLVars(req, map[string]string{
		"nodeId":   "test-node",
		"filePath": "subdir/file.txt",
	})
	w := httptest.NewRecorder()
	
	// Call handler
	h.GetFile(w, req)
	
	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	
	// Verify the file browser was called with the actual path, not the node ID
	if !mockFiles.readFileCalled {
		t.Error("ReadFile was not called")
	}
	if mockFiles.receivedNodePath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockFiles.receivedNodePath)
	}
	if mockFiles.receivedFilePath != "subdir/file.txt" {
		t.Errorf("Expected file path 'subdir/file.txt', got '%s'", mockFiles.receivedFilePath)
	}
}

func TestPutFile_NodeIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockFiles := &mockFilesBrowser{}
	
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
		files: mockFiles,
		graph: mockGraph,
	}
	
	// Create request body
	reqBody := map[string]string{
		"content": "new file content",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	
	// Create request
	req := httptest.NewRequest("PUT", "/api/nodes/test-node/files/newfile.txt", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{
		"nodeId":   "test-node",
		"filePath": "newfile.txt",
	})
	w := httptest.NewRecorder()
	
	// Call handler
	h.PutFile(w, req)
	
	// Verify response
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	
	// Verify the file browser was called with the actual path, not the node ID
	if !mockFiles.writeFileCalled {
		t.Error("WriteFile was not called")
	}
	if mockFiles.receivedNodePath != "/actual/path/to/node" {
		t.Errorf("Expected path '/actual/path/to/node', got '%s'", mockFiles.receivedNodePath)
	}
	if mockFiles.receivedFilePath != "newfile.txt" {
		t.Errorf("Expected file path 'newfile.txt', got '%s'", mockFiles.receivedFilePath)
	}
	if string(mockFiles.receivedContent) != "new file content" {
		t.Errorf("Expected content 'new file content', got '%s'", string(mockFiles.receivedContent))
	}
}

func TestFileHandlers_NodeNotFound(t *testing.T) {
	// Create mocks with no nodes
	mockFiles := &mockFilesBrowser{}
	mockGraph := &mockGraphBuilder{
		nodes: map[string]*core.Node{},
	}
	
	// Create handlers
	h := &Handlers{
		files: mockFiles,
		graph: mockGraph,
	}
	
	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"GetFiles", "GET", "/api/nodes/nonexistent/files", nil},
		{"GetFile", "GET", "/api/nodes/nonexistent/files/file.txt", nil},
		{"PutFile", "PUT", "/api/nodes/nonexistent/files/file.txt", []byte(`{"content":"test"}`)},
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
			
			// Set URL vars based on path
			vars := map[string]string{"nodeId": "nonexistent"}
			if tt.name == "GetFile" || tt.name == "PutFile" {
				vars["filePath"] = "file.txt"
			}
			req = mux.SetURLVars(req, vars)
			
			w := httptest.NewRecorder()
			
			// Call appropriate handler
			switch tt.name {
			case "GetFiles":
				h.GetFiles(w, req)
			case "GetFile":
				h.GetFile(w, req)
			case "PutFile":
				h.PutFile(w, req)
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