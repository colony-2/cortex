package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// GetContainerStatus handles GET /api/nodes/{nodeId}/container/status
func (h *Handlers) GetContainerStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := h.storage.GetContainerID(r.Context(), nodeID)
	if err != nil {
		// No container ID stored
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "none"})
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
	})
}

// CreateContainer handles POST /api/nodes/{nodeId}/container/create
func (h *Handlers) CreateContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := h.container.Create(r.Context(), nodeID)
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