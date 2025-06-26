package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/gorilla/mux"
)

func TestGitHandlersWithEmptyRepo(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "git-empty-test-*")
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

	// Initialize empty git repository
	_, err = git.PlainInit(nodeDir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create test file (but don't commit it)
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

	t.Run("TestGitStatusWithEmptyRepo", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/test-node/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
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
	})

	t.Run("TestGitDiffWithEmptyRepo", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/test-node/git/diff", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitDiff(w, req)

		// Should return OK with empty diff
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var diff GitDiff
		err := json.NewDecoder(w.Body).Decode(&diff)
		if err != nil {
			t.Fatal(err)
		}

		// Should have no files in diff
		if len(diff.Files) != 0 {
			t.Errorf("Expected 0 files in diff, got %d", len(diff.Files))
		}
	})

	t.Run("TestGitHistoryWithEmptyRepo", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/test-node/git/history", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitHistory(w, req)

		// Should return OK with empty history
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var commits []GitCommit
		err := json.NewDecoder(w.Body).Decode(&commits)
		if err != nil {
			t.Fatal(err)
		}

		// Should have no commits
		if len(commits) != 0 {
			t.Errorf("Expected 0 commits in history, got %d", len(commits))
		}
	})

	t.Run("TestGitCommitWithEmptyRepo", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/nodes/test-node/git/commit", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w := httptest.NewRecorder()

		server.handleGitCommit(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
		}

		// After commit, history should work
		req = httptest.NewRequest("GET", "/api/nodes/test-node/git/history", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "test-node"})
		w = httptest.NewRecorder()

		server.handleGitHistory(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d after commit, got %d", http.StatusOK, w.Code)
		}

		var commits []GitCommit
		json.NewDecoder(w.Body).Decode(&commits)
		if len(commits) != 1 {
			t.Errorf("Expected 1 commit after first commit, got %d", len(commits))
		}
	})
}