package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/divisive-ai/vibethis/server/openapi/pkg/openapi"
	"github.com/gorilla/mux"
)

// GetFiles handles GET /api/cells/{cellId}/files
func (h *Handlers) GetFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]

	// Get the cell to find its path
	cell, err := h.graph.GetCell(r.Context(), cellID)
	if err != nil {
		http.Error(w, "Cell not found", http.StatusNotFound)
		return
	}

	files, err := h.files.ListFiles(r.Context(), cell.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert files.FileInfo to openapi.FileInfo
	apiFiles := make([]openapi.FileInfo, len(files))
	for i, file := range files {
		apiFiles[i] = openapi.FileInfo{
			Name:  file.Name,
			Path:  file.Path,
			IsDir: file.IsDir,
			Size:  file.Size,
			Type:  file.Type,
		}
	}

	// Return in the format expected by the frontend
	response := struct {
		Files []openapi.FileInfo `json:"files"`
		Path  string             `json:"path"`
	}{
		Files: apiFiles,
		Path:  r.URL.Query().Get("path"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetFile handles GET /api/cells/{cellId}/files/{filePath}
func (h *Handlers) GetFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]
	filePath := vars["filePath"]

	// Get the cell to find its path
	cell, err := h.graph.GetCell(r.Context(), cellID)
	if err != nil {
		http.Error(w, "Cell not found", http.StatusNotFound)
		return
	}

	content, err := h.files.ReadFile(r.Context(), cell.Path, filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

// PutFile handles PUT /api/cells/{cellId}/files/{filePath}
func (h *Handlers) PutFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cellID := vars["cellId"]
	filePath := vars["filePath"]

	// Get the cell to find its path
	cell, err := h.graph.GetCell(r.Context(), cellID)
	if err != nil {
		http.Error(w, "Cell not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON body
	var fileContent openapi.WriteCellFileJSONBody
	if err := json.Unmarshal(body, &fileContent); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.files.WriteFile(r.Context(), cell.Path, filePath, []byte(fileContent.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
