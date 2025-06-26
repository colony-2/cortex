package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	"vibethis/internal/devcontainer"
)

type ContainerStatus struct {
	Status      string `json:"status"` // "none", "stopped", "running"
	ContainerID string `json:"containerId,omitempty"`
}

type FileContent struct {
	Content string `json:"content"`
}

// getContainerStatusHandler returns the current status of a node's container
func getContainerStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := storage.GetContainerID(nodeID)
	if err != nil {
		http.Error(w, "Failed to get container status", http.StatusInternalServerError)
		return
	}

	status := ContainerStatus{
		Status:      "none",
		ContainerID: containerID,
	}

	if containerID != "" {
		// Check if container exists and its status
		dc := &devcontainer.DevContainerManager{
			WorkspaceRoot: filepath.Join(rootPath, nodeID),
		}
		
		isRunning, err := dc.IsContainerRunning(containerID)
		if err == nil {
			if isRunning {
				status.Status = "running"
			} else {
				status.Status = "stopped"
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// createContainerHandler creates a new devcontainer for a node
func createContainerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	workspacePath := filepath.Join(rootPath, nodeID)
	dc := &devcontainer.DevContainerManager{
		WorkspaceRoot: workspacePath,
	}

	// Create the container
	containerID, err := dc.CreateContainer()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create container: %v", err), http.StatusInternalServerError)
		return
	}

	// Save container ID
	if err := storage.SaveContainerID(nodeID, containerID); err != nil {
		http.Error(w, "Failed to save container ID", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"containerId": containerID})
}

// startContainerHandler starts an existing container
func startContainerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := storage.GetContainerID(nodeID)
	if err != nil || containerID == "" {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	dc := &devcontainer.DevContainerManager{
		WorkspaceRoot: filepath.Join(rootPath, nodeID),
	}

	if err := dc.StartContainer(containerID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start container: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// restartContainerHandler restarts a container
func restartContainerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := storage.GetContainerID(nodeID)
	if err != nil || containerID == "" {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	dc := &devcontainer.DevContainerManager{
		WorkspaceRoot: filepath.Join(rootPath, nodeID),
	}

	if err := dc.RestartContainer(containerID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to restart container: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// resetContainerHandler stops and removes a container
func resetContainerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	containerID, err := storage.GetContainerID(nodeID)
	if err != nil || containerID == "" {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	dc := &devcontainer.DevContainerManager{
		WorkspaceRoot: filepath.Join(rootPath, nodeID),
	}

	if err := dc.RemoveContainer(containerID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove container: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete container ID from storage
	if err := storage.DeleteContainerID(nodeID); err != nil {
		http.Error(w, "Failed to delete container ID", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// getNodeFileHandler reads a file from a node's directory
func getNodeFileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	filePath := vars["filePath"]

	// Security: ensure the file path doesn't escape the node directory
	cleanPath := filepath.Clean(filePath)
	fullPath := filepath.Join(rootPath, nodeID, cleanPath)
	
	// Verify the path is within the node directory
	if !filepath.HasPrefix(fullPath, filepath.Join(rootPath, nodeID)) {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

// putNodeFileHandler writes a file to a node's directory
func putNodeFileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	filePath := vars["filePath"]

	// Security: ensure the file path doesn't escape the node directory
	cleanPath := filepath.Clean(filePath)
	fullPath := filepath.Join(rootPath, nodeID, cleanPath)
	
	// Verify the path is within the node directory
	if !filepath.HasPrefix(fullPath, filepath.Join(rootPath, nodeID)) {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON body
	var fileContent FileContent
	if err := json.Unmarshal(body, &fileContent); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(fileContent.Content), 0644); err != nil {
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}