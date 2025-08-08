package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
)

// GetContainerStatus handles GET /api/cells/{cellId}/container/status
func (h *Handlers) GetContainerStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	// Get the cell to find its path
	cell, err := h.graph.GetCell(r.Context(), cellID)
	if err != nil {
		http.Error(w, "Cell not found", http.StatusNotFound)
		return
	}

	// Check for devcontainer.json
	devcontainerPath := filepath.Join(cell.Path, ".devcontainer", "devcontainer.json")
	devcontainerContent := ""
	hasDevcontainer := false
	
	if data, err := os.ReadFile(devcontainerPath); err == nil {
		devcontainerContent = string(data)
		hasDevcontainer = true
	}

	containerID, err := h.storage.GetContainerID(r.Context(), cellID)
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

// CreateContainer handles POST /api/cells/{cellId}/container/create
func (h *Handlers) CreateContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	// Get the cell to find its path
	cell, err := h.graph.GetCell(r.Context(), cellID)
	if err != nil {
		http.Error(w, "Cell not found", http.StatusNotFound)
		return
	}

	containerID, err := h.container.Create(r.Context(), cell.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save container ID
	if err := h.storage.SaveContainerID(r.Context(), cellID, containerID); err != nil {
		http.Error(w, "Failed to save container ID", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"containerId": containerID})
}

// StartContainer handles POST /api/cells/{cellId}/container/start
func (h *Handlers) StartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	containerID, err := h.storage.GetContainerID(r.Context(), cellID)
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

// StopContainer handles POST /api/cells/{cellId}/container/stop
func (h *Handlers) StopContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	containerID, err := h.storage.GetContainerID(r.Context(), cellID)
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

// RestartContainer handles POST /api/cells/{cellId}/container/restart
func (h *Handlers) RestartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	containerID, err := h.storage.GetContainerID(r.Context(), cellID)
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

// ResetContainer handles POST /api/cells/{cellId}/container/reset
func (h *Handlers) ResetContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	containerID, err := h.storage.GetContainerID(r.Context(), cellID)
	if err != nil {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	if err := h.container.Remove(r.Context(), containerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete container ID from storage
	if err := h.storage.DeleteContainerID(r.Context(), cellID); err != nil {
		http.Error(w, "Failed to delete container ID", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// UpdateDevcontainer handles PUT /api/cells/{cellId}/container/devcontainer
func (h *Handlers) UpdateDevcontainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	// Get the cell to find its path
	cell, err := h.graph.GetCell(r.Context(), cellID)
	if err != nil {
		http.Error(w, "Cell not found", http.StatusNotFound)
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
	devcontainerDir := filepath.Join(cell.Path, ".devcontainer")
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