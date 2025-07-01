package handlers

import (
	"encoding/json"
	"net/http"
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