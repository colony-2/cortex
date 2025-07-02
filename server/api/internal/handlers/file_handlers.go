package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

// GetFiles handles GET /api/nodes/{nodeId}/files
func (h *Handlers) GetFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	files, err := h.files.ListFiles(r.Context(), node.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return in the format expected by the frontend
	response := struct {
		Files interface{} `json:"files"`
		Path  string      `json:"path"`
	}{
		Files: files,
		Path:  r.URL.Query().Get("path"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetFile handles GET /api/nodes/{nodeId}/files/{filePath}
func (h *Handlers) GetFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	filePath := vars["filePath"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	content, err := h.files.ReadFile(r.Context(), node.Path, filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

// PutFile handles PUT /api/nodes/{nodeId}/files/{filePath}
func (h *Handlers) PutFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	filePath := vars["filePath"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON body
	var fileContent struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &fileContent); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.files.WriteFile(r.Context(), node.Path, filePath, []byte(fileContent.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}