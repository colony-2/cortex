package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// GetGraph handles GET /api/graph
func (h *Handlers) GetGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := h.graph.BuildGraph(r.Context())
	if err != nil {
		http.Error(w, "Failed to build graph", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}

// UpdateDependencies handles PUT /api/nodes/{nodeId}/dependencies
func (h *Handlers) UpdateDependencies(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]

	var body struct {
		Dependencies []string `json:"dependencies"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.graph.UpdateNodeDependencies(r.Context(), nodeID, body.Dependencies); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}