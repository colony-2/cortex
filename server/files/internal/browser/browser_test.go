package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/divisive-ai/vibethis/server/files/internal/security"
)

func TestListFiles(t *testing.T) {
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
		filepath.Join(tempDir, "api", "main.go"):            "package main",
		filepath.Join(tempDir, "api", "config.yaml"):        "config: test",
		filepath.Join(tempDir, "frontend", "src", "App.tsx"): "export default App",
	}

	for path, content := range testFiles {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create browser
	validator := security.NewValidator(tempDir)
	b := New(validator, nil, 0)
	ctx := context.Background()

	// Test listing files in api directory
	files, err := b.ListFiles(ctx, filepath.Join(tempDir, "api"))
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files in api directory, got %d", len(files))
	}

	// Check that both files are present
	fileMap := make(map[string]bool)
	for _, f := range files {
		fileMap[f.Name] = true
	}

	expectedFiles := []string{"main.go", "config.yaml"}
	for _, expected := range expectedFiles {
		if !fileMap[expected] {
			t.Errorf("Expected file %s not found", expected)
		}
	}

	// Test listing files in frontend/src directory
	files, err = b.ListFiles(ctx, filepath.Join(tempDir, "frontend", "src"))
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file in frontend/src directory, got %d", len(files))
	}

	if len(files) > 0 && files[0].Name != "App.tsx" {
		t.Errorf("Expected App.tsx, got %s", files[0].Name)
	}
}

func TestReadFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	nodeDir := filepath.Join(tempDir, "testnode")
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		t.Fatal(err)
	}

	testContent := "test file content"
	testFile := filepath.Join(nodeDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create browser
	validator := security.NewValidator(tempDir)
	b := New(validator, nil, 0)
	ctx := context.Background()

	// Test reading file
	content, err := b.ReadFile(ctx, filepath.Join(tempDir, "testnode"), "test.txt")
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Expected content %q, got %q", testContent, string(content))
	}

	// Test reading non-existent file
	_, err = b.ReadFile(ctx, filepath.Join(tempDir, "testnode"), "nonexistent.txt")
	if err == nil {
		t.Error("Expected error when reading non-existent file")
	}
}

func TestWriteFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test node directory
	nodeDir := filepath.Join(tempDir, "testnode")
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create browser
	validator := security.NewValidator(tempDir)
	b := New(validator, nil, 0)
	ctx := context.Background()

	// Test writing new file
	testContent := []byte("new file content")
	err = b.WriteFile(ctx, filepath.Join(tempDir, "testnode"), "new.txt", testContent)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Verify file was written
	writtenContent, err := os.ReadFile(filepath.Join(nodeDir, "new.txt"))
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(writtenContent) != string(testContent) {
		t.Errorf("Expected content %q, got %q", string(testContent), string(writtenContent))
	}

	// Test overwriting existing file
	newContent := []byte("updated content")
	err = b.WriteFile(ctx, filepath.Join(tempDir, "testnode"), "new.txt", newContent)
	if err != nil {
		t.Fatalf("Failed to overwrite file: %v", err)
	}

	// Verify file was overwritten
	writtenContent, err = os.ReadFile(filepath.Join(nodeDir, "new.txt"))
	if err != nil {
		t.Fatalf("Failed to read overwritten file: %v", err)
	}

	if string(writtenContent) != string(newContent) {
		t.Errorf("Expected content %q, got %q", string(newContent), string(writtenContent))
	}
}

func TestFileTypes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files with different extensions
	nodeDir := filepath.Join(tempDir, "testnode")
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		t.Fatal(err)
	}

	testFiles := map[string]string{
		"main.go":        "go",
		"style.css":      "css",
		"index.html":     "html",
		"script.js":      "javascript",
		"data.json":      "json",
		"config.yaml":    "yaml",
		"readme.md":      "markdown",
		"Dockerfile":     "dockerfile",
		"unknown.xyz":    "file",
		"image.png":      "image",
		".gitignore":     "git",
		"Makefile":       "makefile",
		"package.json":   "json",
		"tsconfig.json":  "json",
	}

	// Create all test files
	for filename := range testFiles {
		if err := os.WriteFile(filepath.Join(nodeDir, filename), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create browser
	validator := security.NewValidator(tempDir)
	b := New(validator, nil, 0)
	ctx := context.Background()

	// List files and check types
	files, err := b.ListFiles(ctx, filepath.Join(tempDir, "testnode"))
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	// Create map of actual file types
	actualTypes := make(map[string]string)
	for _, f := range files {
		actualTypes[f.Name] = f.Type
	}

	// Verify file types
	for filename, expectedType := range testFiles {
		actualType, exists := actualTypes[filename]
		if !exists {
			t.Errorf("File %s not found in listing", filename)
			continue
		}

		if actualType != expectedType {
			t.Errorf("File %s: expected type %s, got %s", filename, expectedType, actualType)
		}
	}
}