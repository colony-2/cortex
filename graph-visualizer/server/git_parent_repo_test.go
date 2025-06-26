package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gorilla/mux"
)

func TestGitHandlersWithParentRepo(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "git-parent-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create parent directory with git repo
	parentDir := filepath.Join(tempDir, "parent")
	err = os.Mkdir(parentDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repository at parent level
	repo, err := git.PlainInit(parentDir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create box directories
	box1Dir := filepath.Join(parentDir, "box1")
	box2Dir := filepath.Join(parentDir, "box2")
	err = os.Mkdir(box1Dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir(box2Dir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Create test files in different boxes
	box1File := filepath.Join(box1Dir, "file1.txt")
	box2File := filepath.Join(box2Dir, "file2.txt")
	rootFile := filepath.Join(parentDir, "root.txt")

	err = os.WriteFile(box1File, []byte("box1 content"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(box2File, []byte("box2 content"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(rootFile, []byte("root content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create storage and server
	storage, err := NewPositionStorage(parentDir)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	server := NewServer(parentDir, storage)

	t.Run("TestGitStatusFiltersFilesByBox", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nodes/box1/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "box1"})
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

		// Should only see file1.txt, not file2.txt or root.txt
		if len(status.Untracked) != 1 {
			t.Errorf("Expected 1 untracked file in box1, got %d", len(status.Untracked))
		}
		if len(status.Untracked) > 0 && status.Untracked[0] != "file1.txt" {
			t.Errorf("Expected file1.txt, got %s", status.Untracked[0])
		}
	})

	// Commit files for further tests
	worktree, _ := repo.Worktree()
	worktree.Add(".")
	worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})

	// Modify files
	err = os.WriteFile(box1File, []byte("modified box1 content"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(box2File, []byte("modified box2 content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("TestGitCommitOnlyAffectsBoxFiles", func(t *testing.T) {
		// Get initial status of box2
		req := httptest.NewRequest("GET", "/api/nodes/box2/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "box2"})
		w := httptest.NewRecorder()
		server.handleGitStatus(w, req)

		var beforeStatus GitStatus
		json.NewDecoder(w.Body).Decode(&beforeStatus)
		if len(beforeStatus.Modified) != 1 {
			t.Errorf("Expected 1 modified file in box2 before commit, got %d", len(beforeStatus.Modified))
		}

		// Commit box1
		req = httptest.NewRequest("POST", "/api/nodes/box1/git/commit", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "box1"})
		w = httptest.NewRecorder()

		server.handleGitCommit(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check that box2 still has its modified file
		req = httptest.NewRequest("GET", "/api/nodes/box2/git/status", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "box2"})
		w = httptest.NewRecorder()
		server.handleGitStatus(w, req)

		var afterStatus GitStatus
		json.NewDecoder(w.Body).Decode(&afterStatus)
		if len(afterStatus.Modified) != 1 {
			t.Errorf("Expected 1 modified file in box2 after box1 commit, got %d", len(afterStatus.Modified))
		}
	})

	t.Run("TestGitHistoryFiltersByBox", func(t *testing.T) {
		// Commit box2 changes
		req := httptest.NewRequest("POST", "/api/nodes/box2/git/commit", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "box2"})
		w := httptest.NewRecorder()
		server.handleGitCommit(w, req)

		// Get box1 history
		req = httptest.NewRequest("GET", "/api/nodes/box1/git/history", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "box1"})
		w = httptest.NewRecorder()
		server.handleGitHistory(w, req)

		var box1Commits []GitCommit
		json.NewDecoder(w.Body).Decode(&box1Commits)

		// Get box2 history
		req = httptest.NewRequest("GET", "/api/nodes/box2/git/history", nil)
		req = mux.SetURLVars(req, map[string]string{"nodeId": "box2"})
		w = httptest.NewRecorder()
		server.handleGitHistory(w, req)

		var box2Commits []GitCommit
		json.NewDecoder(w.Body).Decode(&box2Commits)

		// Each box should only see its own commits
		for _, commit := range box1Commits {
			if !strings.Contains(commit.Message, "[box1]") {
				t.Errorf("Box1 history contains non-box1 commit: %s", commit.Message)
			}
		}

		for _, commit := range box2Commits {
			if !strings.Contains(commit.Message, "[box2]") {
				t.Errorf("Box2 history contains non-box2 commit: %s", commit.Message)
			}
		}
	})

	t.Run("TestFindGitRepoWalksUp", func(t *testing.T) {
		// Create a deeply nested box
		nestedDir := filepath.Join(box1Dir, "nested", "deep")
		err = os.MkdirAll(nestedDir, 0755)
		if err != nil {
			t.Fatal(err)
		}

		gitInfo, err := server.findGitRepo(nestedDir)
		if err != nil {
			t.Errorf("Failed to find git repo from nested directory: %v", err)
		}

		if gitInfo.repoPath != parentDir {
			t.Errorf("Expected repo path %s, got %s", parentDir, gitInfo.repoPath)
		}

		expectedRelPath := filepath.Join("box1", "nested", "deep")
		if gitInfo.boxRelPath != expectedRelPath {
			t.Errorf("Expected relative path %s, got %s", expectedRelPath, gitInfo.boxRelPath)
		}
	})
}