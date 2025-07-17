package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
)

// GetContainerStatus handles GET /api/nodes/{nodeId}/container/status
func (h *Handlers) GetContainerStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Check for devcontainer.json
	devcontainerPath := filepath.Join(node.Path, ".devcontainer", "devcontainer.json")
	devcontainerContent := ""
	hasDevcontainer := false
	
	if data, err := os.ReadFile(devcontainerPath); err == nil {
		devcontainerContent = string(data)
		hasDevcontainer = true
	}

	containerID, err := h.storage.GetContainerID(r.Context(), nodeID)
	if err != nil {
		// No container ID stored
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "none",
			"hasDevcontainer": hasDevcontainer,
			"devcontainerContent": devcontainerContent,
		})
		return
	}

	status, err := h.container.GetStatus(r.Context(), containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      string(status),
		"containerId": containerID,
		"hasDevcontainer": hasDevcontainer,
		"devcontainerContent": devcontainerContent,
	})
}

// CreateContainer handles POST /api/nodes/{nodeId}/container/create
func (h *Handlers) CreateContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	containerID, err := h.container.Create(r.Context(), node.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save container ID
	if err := h.storage.SaveContainerID(r.Context(), nodeID, containerID); err != nil {
		http.Error(w, "Failed to save container ID", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"containerId": containerID})
}

// StartContainer handles POST /api/nodes/{nodeId}/container/start
func (h *Handlers) StartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := h.storage.GetContainerID(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	if err := h.container.Start(r.Context(), containerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// StopContainer handles POST /api/nodes/{nodeId}/container/stop
func (h *Handlers) StopContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := h.storage.GetContainerID(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	if err := h.container.Stop(r.Context(), containerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// RestartContainer handles POST /api/nodes/{nodeId}/container/restart
func (h *Handlers) RestartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := h.storage.GetContainerID(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	if err := h.container.Restart(r.Context(), containerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ResetContainer handles POST /api/nodes/{nodeId}/container/reset
func (h *Handlers) ResetContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := h.storage.GetContainerID(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	if err := h.container.Remove(r.Context(), containerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete container ID from storage
	if err := h.storage.DeleteContainerID(r.Context(), nodeID); err != nil {
		http.Error(w, "Failed to delete container ID", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// UpdateDevcontainer handles PUT /api/nodes/{nodeId}/container/devcontainer
func (h *Handlers) UpdateDevcontainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	// Get the node to find its path
	node, err := h.graph.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Parse request body
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate JSON content
	var jsonCheck interface{}
	if err := json.Unmarshal([]byte(req.Content), &jsonCheck); err != nil {
		http.Error(w, "Invalid JSON content", http.StatusBadRequest)
		return
	}

	// Create .devcontainer directory if it doesn't exist
	devcontainerDir := filepath.Join(node.Path, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		http.Error(w, "Failed to create .devcontainer directory", http.StatusInternalServerError)
		return
	}

	// Write devcontainer.json file
	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(req.Content), 0644); err != nil {
		http.Error(w, "Failed to write devcontainer.json", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}