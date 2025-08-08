package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/gorilla/mux"
)

// mockFilesBrowser implements files.Browser for testing
type mockFilesBrowser struct {
	listFilesCalled  bool
	readFileCalled   bool
	writeFileCalled  bool
	receivedNodePath string
	receivedFilePath string
	receivedContent  []byte
	listFilesReturn  []files.FileInfo
	listFilesError   error
	readFileReturn   []byte
	readFileError    error
	writeFileError   error
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
	cells map[string]*core.Cell
}

func (m *mockGraphBuilder) BuildGraph(ctx context.Context) (*core.Graph, error) {
	return nil, nil
}

func (m *mockGraphBuilder) GetCell(ctx context.Context, cellID string) (*core.Cell, error) {
	cell, ok := m.cells[cellID]
	if !ok {
		return nil, context.DeadlineExceeded // simulating cell not found
	}
	return cell, nil
}

func TestGetFiles_CellIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockFiles := &mockFilesBrowser{
		listFilesReturn: []files.FileInfo{
			{Name: "test.txt", Path: "test.txt", IsDir: false, Size: 100, Type: "text"},
		},
	}

	mockGraph := &mockGraphBuilder{
		cells: map[string]*core.Cell{
			"test-cell": {
				ID:   "test-cell",
				Name: "Test Cell",
				Path: "/actual/path/to/cell",
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
	req := httptest.NewRequest("GET", "/api/cells/test-node/files", nil)
	req = mux.SetURLVars(req, map[string]string{"cellId": "test-cell"})
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
	if mockFiles.receivedNodePath != "/actual/path/to/cell" {
		t.Errorf("Expected path '/actual/path/to/cell', got '%s'", mockFiles.receivedNodePath)
	}
}

func TestGetFile_CellIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockFiles := &mockFilesBrowser{
		readFileReturn: []byte("test content"),
	}

	mockGraph := &mockGraphBuilder{
		cells: map[string]*core.Cell{
			"test-cell": {
				ID:   "test-cell",
				Name: "Test Cell",
				Path: "/actual/path/to/cell",
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
	req := httptest.NewRequest("GET", "/api/cells/test-node/files/subdir/file.txt", nil)
	req = mux.SetURLVars(req, map[string]string{
		"cellId":   "test-cell",
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
	if mockFiles.receivedNodePath != "/actual/path/to/cell" {
		t.Errorf("Expected path '/actual/path/to/cell', got '%s'", mockFiles.receivedNodePath)
	}
	if mockFiles.receivedFilePath != "subdir/file.txt" {
		t.Errorf("Expected file path 'subdir/file.txt', got '%s'", mockFiles.receivedFilePath)
	}
}

func TestPutFile_CellIDToPathTranslation(t *testing.T) {
	// Create mocks
	mockFiles := &mockFilesBrowser{}

	mockGraph := &mockGraphBuilder{
		cells: map[string]*core.Cell{
			"test-cell": {
				ID:   "test-cell",
				Name: "Test Cell",
				Path: "/actual/path/to/cell",
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
	req := httptest.NewRequest("PUT", "/api/cells/test-node/files/newfile.txt", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{
		"cellId":   "test-cell",
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
	if mockFiles.receivedNodePath != "/actual/path/to/cell" {
		t.Errorf("Expected path '/actual/path/to/cell', got '%s'", mockFiles.receivedNodePath)
	}
	if mockFiles.receivedFilePath != "newfile.txt" {
		t.Errorf("Expected file path 'newfile.txt', got '%s'", mockFiles.receivedFilePath)
	}
	if string(mockFiles.receivedContent) != "new file content" {
		t.Errorf("Expected content 'new file content', got '%s'", string(mockFiles.receivedContent))
	}
}

func TestFileHandlers_CellNotFound(t *testing.T) {
	// Create mocks with no cells
	mockFiles := &mockFilesBrowser{}
	mockGraph := &mockGraphBuilder{
		cells: map[string]*core.Cell{},
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
		{"GetFiles", "GET", "/api/cells/nonexistent/files", nil},
		{"GetFile", "GET", "/api/cells/nonexistent/files/file.txt", nil},
		{"PutFile", "PUT", "/api/cells/nonexistent/files/file.txt", []byte(`{"content":"test"}`)},
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
			vars := map[string]string{"cellId": "nonexistent"}
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
			if body := w.Body.String(); !bytes.Contains([]byte(body), []byte("Cell not found")) {
				t.Errorf("Expected 'Cell not found' in response, got: %s", body)
			}
		})
	}
}
