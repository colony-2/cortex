package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// GetGraph handles GET /api/graph
func (h *Handlers) GetGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := h.graph.BuildGraph(r.Context())
	if err != nil {
		// Include the actual error in the response for debugging
		http.Error(w, fmt.Sprintf("Failed to build graph: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}