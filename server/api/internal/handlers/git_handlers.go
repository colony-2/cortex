package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// GetGitStatus handles GET /api/nodes/{nodeId}/git/status
func (h *Handlers) GetGitStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	status, err := h.git.GetStatus(r.Context(), node.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// GetGitDiff handles GET /api/nodes/{nodeId}/git/diff
func (h *Handlers) GetGitDiff(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	staged := r.URL.Query().Get("staged") == "true"
	diff, err := h.git.GetDiff(r.Context(), node.Path, staged)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(diff))
}

// GetGitHistory handles GET /api/nodes/{nodeId}/git/history
func (h *Handlers) GetGitHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	history, err := h.git.GetHistory(r.Context(), node.Path, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// CreateGitCommit handles POST /api/nodes/{nodeId}/git/commit
func (h *Handlers) CreateGitCommit(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	var body struct {
		Message string   `json:"message"`
		Files   []string `json:"files,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if body.Message == "" {
		http.Error(w, "Commit message is required", http.StatusBadRequest)
		return
	}

	// Stage files if specified
	if len(body.Files) > 0 {
		if err := h.git.StageFiles(r.Context(), node.Path, body.Files); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Create commit
	if err := h.git.CreateCommit(r.Context(), node.Path, body.Message); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}