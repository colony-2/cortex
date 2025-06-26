package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSPAHandler(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Create test files
	indexContent := `<!DOCTYPE html><html><body>SPA Index</body></html>`
	if err := os.WriteFile(filepath.Join(tempDir, "index.html"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create a static file
	if err := os.MkdirAll(filepath.Join(tempDir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "assets", "app.js"), []byte("console.log('app');"), 0644); err != nil {
		t.Fatal(err)
	}
	
	handler := newSPAHandler(tempDir)
	
	tests := []struct {
		path           string
		expectedStatus int
		expectIndex    bool
		description    string
	}{
		{"/", http.StatusOK, true, "Root should serve index.html"},
		{"/boxes", http.StatusOK, true, "SPA route should serve index.html"},
		{"/box/api/files", http.StatusOK, true, "Nested SPA route should serve index.html"},
		{"/box/api/config/env", http.StatusOK, true, "Deep SPA route should serve index.html"},
		{"/assets/app.js", http.StatusOK, false, "Static file should be served"},
		{"/nonexistent.js", http.StatusOK, true, "Non-existent file should serve index.html"},
	}
	
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			
			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}
			
			body := rr.Body.String()
			if tt.expectIndex {
				if body != indexContent {
					t.Errorf("Expected index.html content, got %s", body)
				}
			}
		})
	}
}