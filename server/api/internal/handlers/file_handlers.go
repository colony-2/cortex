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

	files, err := h.files.ListFiles(r.Context(), nodeID)
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

	content, err := h.files.ReadFile(r.Context(), nodeID, filePath)
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

	if err := h.files.WriteFile(r.Context(), nodeID, filePath, []byte(fileContent.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}