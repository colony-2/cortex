package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
)

func TestGetFilesHandler(t *testing.T) {
	// Setup test directory structure
	tempDir, err := os.MkdirTemp("", "files_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test structure
	// tempDir/
	//   api/
	//     main.go
	//     config.yaml
	//   frontend/
	//     src/
	//       App.tsx
	testDirs := []string{
		filepath.Join(tempDir, "api"),
		filepath.Join(tempDir, "frontend", "src"),
	}

	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create test files
	testFiles := map[string]string{
		filepath.Join(tempDir, "api", "main.go"):         "package main",
		filepath.Join(tempDir, "api", "config.yaml"):     "config: test",
		filepath.Join(tempDir, "frontend", "src", "App.tsx"): "export default App",
	}

	for path, content := range testFiles {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Save old rootPath and restore after test
	oldRootPath := rootPath
	rootPath = tempDir
	defer func() { rootPath = oldRootPath }()

	// Create router
	router := mux.NewRouter()
	router.HandleFunc("/api/files/{nodeId}", getFilesHandler).Methods("GET")

	// Test 1: Get files for "api" node
	req := httptest.NewRequest("GET", "/api/files/api", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response FilesResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check we got the right files
	if len(response.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(response.Files))
	}

	// Verify file names
	fileNames := make(map[string]bool)
	for _, file := range response.Files {
		fileNames[file.Name] = true
	}

	if !fileNames["main.go"] || !fileNames["config.yaml"] {
		t.Errorf("Missing expected files. Got: %v", fileNames)
	}

	// Test 2: Get files for nested path
	req = httptest.NewRequest("GET", "/api/files/frontend?path=src", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.Files) != 1 {
		t.Errorf("Expected 1 file in src, got %d", len(response.Files))
	}

	if response.Files[0].Name != "App.tsx" {
		t.Errorf("Expected App.tsx, got %s", response.Files[0].Name)
	}

	// Test 3: Non-existent node
	req = httptest.NewRequest("GET", "/api/files/nonexistent", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent node, got %d", w.Code)
	}

	// Test 4: Invalid path
	req = httptest.NewRequest("GET", "/api/files/api?path=../../../etc", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should either return 404 or 500 depending on security
	if w.Code == http.StatusOK {
		t.Errorf("Should not allow path traversal, got status %d", w.Code)
	}
}

func TestFileInfoTypes(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "fileinfo_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create various file types with expected types
	testFiles := map[string]string{
		"test.go":   "go",
		"test.yaml": "yaml",
		"test.json": "json",
		"test.md":   "md",
		"test.tsx":  "tsx",
		"test":      "file", // no extension
	}

	apiDir := filepath.Join(tempDir, "api")
	os.MkdirAll(apiDir, 0755)

	for filename := range testFiles {
		path := filepath.Join(apiDir, filename)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Also create a subdirectory
	os.MkdirAll(filepath.Join(apiDir, "subdir"), 0755)

	oldRootPath := rootPath
	rootPath = tempDir
	defer func() { rootPath = oldRootPath }()

	// Make request
	router := mux.NewRouter()
	router.HandleFunc("/api/files/{nodeId}", getFilesHandler).Methods("GET")

	req := httptest.NewRequest("GET", "/api/files/api", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response FilesResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check file types
	for _, file := range response.Files {
		if file.IsDir {
			if file.Type != "folder" {
				t.Errorf("Directory %s should have type 'folder', got %s", file.Name, file.Type)
			}
		} else {
			expected, ok := testFiles[file.Name]
			if ok && file.Type != expected {
				t.Errorf("File %s should have type %s, got %s", file.Name, expected, file.Type)
			}
		}
	}
}