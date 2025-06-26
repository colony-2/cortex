package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gorilla/mux"
)

func TestGitHandlers(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test node directory
	nodeDir := filepath.Join(tempDir, "test-node")
	err = os.Mkdir(nodeDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repository
	repo, err := git.PlainInit(nodeDir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create test files
	testFile := filepath.Join(nodeDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create storage and server
	storage, err := NewPositionStorage(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	server := NewServer(tempDir, storage)

	t.Run("TestGitStatus", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/test-node/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var status GitStatus
		err := json.NewDecoder(w.Body).Decode(&status)
		if err != nil {
			t.Fatal(err)
		}

		// Should have one untracked file
		if len(status.Untracked) != 1 {
			t.Errorf("Expected 1 untracked file, got %d", len(status.Untracked))
		}
		if status.TotalCount != 1 {
			t.Errorf("Expected total count 1, got %d", status.TotalCount)
		}
	})

	// Add and commit the file for further tests
	worktree, _ := repo.Worktree()
	worktree.Add("test.txt")
	worktree.Commit("[test-node] initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})

	// Modify the file
	err = os.WriteFile(testFile, []byte("modified content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("TestGitStatusWithModified", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/test-node/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var status GitStatus
		err := json.NewDecoder(w.Body).Decode(&status)
		if err != nil {
			t.Fatal(err)
		}

		// Should have one modified file
		if len(status.Modified) != 1 {
			t.Errorf("Expected 1 modified file, got %d", len(status.Modified))
		}
	})

	t.Run("TestGitCommit", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/nodes/test-node/git/commit", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitCommit(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		if err != nil {
			t.Fatal(err)
		}

		if response["message"] != "Commit created successfully" {
			t.Errorf("Expected success message, got %s", response["message"])
		}

		// Verify commit message format
		if response["commitMessage"][:11] != "[test-node]" {
			t.Errorf("Expected commit message to start with [test-node], got %s", response["commitMessage"])
		}
	})

	t.Run("TestGitHistory", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/test-node/git/history", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitHistory(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var commits []GitCommit
		err := json.NewDecoder(w.Body).Decode(&commits)
		if err != nil {
			t.Fatal(err)
		}

		// Should have at least 2 commits (initial + the one we just made)
		if len(commits) < 2 {
			t.Errorf("Expected at least 2 commits, got %d", len(commits))
		}

		// All commits should start with [test-node]
		for _, commit := range commits {
			if len(commit.Message) < 11 || commit.Message[:11] != "[test-node]" {
				t.Errorf("Expected commit message to start with [test-node], got %s", commit.Message)
			}
		}
	})

	t.Run("TestGitDiff", func(t *testing.T) {
		// Create another file to have some diff
		anotherFile := filepath.Join(nodeDir, "another.txt")
		err = os.WriteFile(anotherFile, []byte("another content"), 0644)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("GET", "/api/nodes/test-node/git/diff", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitDiff(w, req)

		// Note: diff might be empty if comparing with HEAD
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("TestNonGitRepository", func(t *testing.T) {
		// Create a non-git directory
		nonGitDir := filepath.Join(tempDir, "non-git-node")
		err = os.Mkdir(nonGitDir, 0755)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("GET", "/api/nodes/non-git-node/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "non-git-node"})
		w := httptest.NewRecorder()

		server.handleGitStatus(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("TestNodeNotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/non-existent/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "non-existent"})
		w := httptest.NewRecorder()

		server.handleGitStatus(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}