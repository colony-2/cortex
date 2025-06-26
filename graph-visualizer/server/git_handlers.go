package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gorilla/mux"
	"github.com/google/uuid"
)

type GitStatus struct {
	Added      []string `json:"added"`
	Modified   []string `json:"modified"`
	Deleted    []string `json:"deleted"`
	Untracked  []string `json:"untracked"`
	TotalCount int      `json:"totalCount"`
}

// gitRepoInfo holds information about the git repository and the relative path to the box
type gitRepoInfo struct {
	repo       *git.Repository
	repoPath   string
	boxRelPath string // relative path from repo root to box directory
}

type GitCommit struct {
	Hash      string    `json:"hash"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type GitDiff struct {
	Files []GitFileDiff `json:"files"`
}

type GitFileDiff struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

// findGitRepo finds the git repository starting from the given path and walking up
func (s *Server) findGitRepo(startPath string) (*gitRepoInfo, error) {
	currentPath := startPath
	for {
		// Try to open git repo at current path
		repo, err := git.PlainOpen(currentPath)
		if err == nil {
			// Found a git repo, calculate relative path
			relPath, err := filepath.Rel(currentPath, startPath)
			if err != nil {
				return nil, err
			}
			return &gitRepoInfo{
				repo:       repo,
				repoPath:   currentPath,
				boxRelPath: relPath,
			}, nil
		}

		// Move to parent directory
		parent := filepath.Dir(currentPath)
		if parent == currentPath || parent == "." || parent == "/" {
			// Reached root without finding a git repo
			return nil, fmt.Errorf("not a git repository")
		}
		currentPath = parent
	}
}

// isPathInBox checks if a file path is within the box directory
func isPathInBox(filePath, boxRelPath string) bool {
	if boxRelPath == "." {
		return true
	}
	// Check if the file path starts with the box relative path
	return strings.HasPrefix(filePath, boxRelPath+string(os.PathSeparator)) ||
		filePath == boxRelPath
}

// stripBoxPrefix removes the box directory prefix from a file path
func stripBoxPrefix(filePath, boxRelPath string) string {
	if boxRelPath == "." {
		return filePath
	}
	prefix := boxRelPath + string(os.PathSeparator)
	return strings.TrimPrefix(filePath, prefix)
}

func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Find the node's directory
	nodePath, err := s.findNodePath(nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Find git repository
	gitInfo, err := s.findGitRepo(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	worktree, err := gitInfo.repo.Worktree()
	if err != nil {
		http.Error(w, "Failed to get worktree", http.StatusInternalServerError)
		return
	}

	status, err := worktree.Status()
	if err != nil {
		http.Error(w, "Failed to get git status", http.StatusInternalServerError)
		return
	}

	gitStatus := GitStatus{
		Added:     []string{},
		Modified:  []string{},
		Deleted:   []string{},
		Untracked: []string{},
	}

	for file, fileStatus := range status {
		// Only include files within the box directory
		if !isPathInBox(file, gitInfo.boxRelPath) {
			continue
		}

		// Strip the box prefix for display
		displayPath := stripBoxPrefix(file, gitInfo.boxRelPath)

		// Check staging status
		switch fileStatus.Staging {
		case git.Added:
			gitStatus.Added = append(gitStatus.Added, displayPath)
		case git.Modified:
			gitStatus.Modified = append(gitStatus.Modified, displayPath)
		case git.Deleted:
			gitStatus.Deleted = append(gitStatus.Deleted, displayPath)
		}

		// Check worktree status
		if fileStatus.Worktree == git.Untracked {
			gitStatus.Untracked = append(gitStatus.Untracked, displayPath)
		} else if fileStatus.Staging == git.Unmodified {
			// File is in worktree but not staged
			switch fileStatus.Worktree {
			case git.Modified:
				gitStatus.Modified = append(gitStatus.Modified, displayPath)
			case git.Deleted:
				gitStatus.Deleted = append(gitStatus.Deleted, displayPath)
			}
		}
	}

	gitStatus.TotalCount = len(gitStatus.Added) + len(gitStatus.Modified) + 
		len(gitStatus.Deleted) + len(gitStatus.Untracked)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gitStatus)
}

func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Find the node's directory
	nodePath, err := s.findNodePath(nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Find git repository
	gitInfo, err := s.findGitRepo(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	ref, err := gitInfo.repo.Head()
	if err != nil {
		// Handle empty repository (no commits yet)
		if err.Error() == "reference not found" {
			diff := GitDiff{Files: []GitFileDiff{}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(diff)
			return
		}
		http.Error(w, "Failed to get HEAD", http.StatusInternalServerError)
		return
	}

	commit, err := gitInfo.repo.CommitObject(ref.Hash())
	if err != nil {
		http.Error(w, "Failed to get commit", http.StatusInternalServerError)
		return
	}

	parent, err := commit.Parent(0)
	if err == object.ErrParentNotFound {
		parent = nil
	} else if err != nil {
		http.Error(w, "Failed to get parent commit", http.StatusInternalServerError)
		return
	}

	var parentTree *object.Tree
	if parent != nil {
		parentTree, err = parent.Tree()
		if err != nil {
			http.Error(w, "Failed to get parent tree", http.StatusInternalServerError)
			return
		}
	}

	currentTree, err := commit.Tree()
	if err != nil {
		http.Error(w, "Failed to get current tree", http.StatusInternalServerError)
		return
	}

	diff := GitDiff{Files: []GitFileDiff{}}

	changes, err := currentTree.Diff(parentTree)
	if err != nil {
		http.Error(w, "Failed to compute diff", http.StatusInternalServerError)
		return
	}

	for _, change := range changes {
		// Check if this change affects files in the box
		filePath := ""
		if change.To.Name != "" {
			filePath = change.To.Name
		} else if change.From.Name != "" {
			filePath = change.From.Name
		}

		if !isPathInBox(filePath, gitInfo.boxRelPath) {
			continue
		}

		patch, err := change.Patch()
		if err != nil {
			continue
		}

		action, err := change.Action()
		if err != nil {
			continue
		}

		fileDiff := GitFileDiff{
			Path:   stripBoxPrefix(filePath, gitInfo.boxRelPath),
			Status: action.String(),
			Patch:  patch.String(),
		}

		stats := patch.Stats()
		for _, stat := range stats {
			if stat.Name == filePath {
				fileDiff.Additions = stat.Addition
				fileDiff.Deletions = stat.Deletion
				break
			}
		}

		diff.Files = append(diff.Files, fileDiff)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(diff)
}

func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Find the node's directory
	nodePath, err := s.findNodePath(nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Find git repository
	gitInfo, err := s.findGitRepo(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	worktree, err := gitInfo.repo.Worktree()
	if err != nil {
		http.Error(w, "Failed to get worktree", http.StatusInternalServerError)
		return
	}

	// Get current status to find files to add
	status, err := worktree.Status()
	if err != nil {
		http.Error(w, "Failed to get status", http.StatusInternalServerError)
		return
	}

	// Add only files within the box directory
	for file, fileStatus := range status {
		if !isPathInBox(file, gitInfo.boxRelPath) {
			continue
		}

		// Add file if it has changes
		if fileStatus.Worktree != git.Unmodified {
			_, err = worktree.Add(file)
			if err != nil {
				// Log error but continue with other files
				continue
			}
		}
	}

	// Generate commit message with box name prefix and UUID
	commitID := uuid.New().String()[:8]
	boxName := filepath.Base(nodePath)
	commitMessage := fmt.Sprintf("[%s] commit %s", boxName, commitID)

	// Create commit
	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "vibethis",
			Email: "vibethis@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		http.Error(w, "Failed to create commit", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Commit created successfully",
		"commitMessage": commitMessage,
	})
}

func (s *Server) handleGitHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Find the node's directory
	nodePath, err := s.findNodePath(nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Find git repository
	gitInfo, err := s.findGitRepo(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	ref, err := gitInfo.repo.Head()
	if err != nil {
		// Handle empty repository (no commits yet)
		if err.Error() == "reference not found" {
			var commits []GitCommit
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(commits)
			return
		}
		http.Error(w, "Failed to get HEAD", http.StatusInternalServerError)
		return
	}

	commitIter, err := gitInfo.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		http.Error(w, "Failed to get commit log", http.StatusInternalServerError)
		return
	}

	var commits []GitCommit
	boxName := filepath.Base(nodePath)
	
	err = commitIter.ForEach(func(c *object.Commit) error {
		// Check if commit affects files in the box directory
		if gitInfo.boxRelPath != "." {
			// Get the changes from this commit
			parent, err := c.Parent(0)
			if err == nil {
				parentTree, _ := parent.Tree()
				currentTree, _ := c.Tree()
				changes, _ := currentTree.Diff(parentTree)
				
				hasBoxChanges := false
				for _, change := range changes {
					filePath := ""
					if change.To.Name != "" {
						filePath = change.To.Name
					} else if change.From.Name != "" {
						filePath = change.From.Name
					}
					if isPathInBox(filePath, gitInfo.boxRelPath) {
						hasBoxChanges = true
						break
					}
				}
				
				if !hasBoxChanges {
					return nil
				}
			}
		}

		// Only include commits that start with [boxName]
		if strings.HasPrefix(c.Message, fmt.Sprintf("[%s]", boxName)) {
			commits = append(commits, GitCommit{
				Hash:      c.Hash.String()[:8],
				Author:    c.Author.Name,
				Email:     c.Author.Email,
				Message:   c.Message,
				Timestamp: c.Author.When,
			})
		}
		return nil
	})

	if err != nil {
		http.Error(w, "Failed to iterate commits", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commits)
}