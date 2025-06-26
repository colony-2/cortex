package main

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Find the node's directory
	nodePath, err := s.findNodePath(nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	repo, err := git.PlainOpen(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	worktree, err := repo.Worktree()
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
		switch fileStatus.Staging {
		case git.Added:
			gitStatus.Added = append(gitStatus.Added, file)
		case git.Modified:
			gitStatus.Modified = append(gitStatus.Modified, file)
		case git.Deleted:
			gitStatus.Deleted = append(gitStatus.Deleted, file)
		}

		switch fileStatus.Worktree {
		case git.Added:
			if fileStatus.Staging == git.Untracked {
				gitStatus.Untracked = append(gitStatus.Untracked, file)
			}
		case git.Modified:
			if fileStatus.Staging != git.Modified {
				gitStatus.Modified = append(gitStatus.Modified, file)
			}
		case git.Deleted:
			if fileStatus.Staging != git.Deleted {
				gitStatus.Deleted = append(gitStatus.Deleted, file)
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

	repo, err := git.PlainOpen(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	ref, err := repo.Head()
	if err != nil {
		http.Error(w, "Failed to get HEAD", http.StatusInternalServerError)
		return
	}

	commit, err := repo.CommitObject(ref.Hash())
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
		patch, err := change.Patch()
		if err != nil {
			continue
		}

		action, err := change.Action()
		if err != nil {
			continue
		}

		fileDiff := GitFileDiff{
			Path:   change.To.Name,
			Status: action.String(),
			Patch:  patch.String(),
		}

		stats := patch.Stats()
		for _, stat := range stats {
			if stat.Name == change.To.Name {
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

	repo, err := git.PlainOpen(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	worktree, err := repo.Worktree()
	if err != nil {
		http.Error(w, "Failed to get worktree", http.StatusInternalServerError)
		return
	}

	// Add all changes
	err = worktree.AddWithOptions(&git.AddOptions{All: true})
	if err != nil {
		http.Error(w, "Failed to add changes", http.StatusInternalServerError)
		return
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

	repo, err := git.PlainOpen(nodePath)
	if err != nil {
		http.Error(w, "Not a git repository", http.StatusBadRequest)
		return
	}

	ref, err := repo.Head()
	if err != nil {
		http.Error(w, "Failed to get HEAD", http.StatusInternalServerError)
		return
	}

	commitIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		http.Error(w, "Failed to get commit log", http.StatusInternalServerError)
		return
	}

	var commits []GitCommit
	boxName := filepath.Base(nodePath)
	
	err = commitIter.ForEach(func(c *object.Commit) error {
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