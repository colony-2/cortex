package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/divisive-ai/vibethis/server/openapi/pkg/openapi"
)

// GetGraph handles GET /api/graph
func (h *Handlers) GetGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := h.graph.BuildGraph(r.Context())
	if err != nil {
		// Include the actual error in the response for debugging
		http.Error(w, fmt.Sprintf("Failed to build graph: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert core.Graph to openapi.Graph
	apiCells := make([]openapi.Cell, len(graph.Cells))
	for i, cell := range graph.Cells {
		apiCells[i] = openapi.Cell{
			Id:           cell.ID,
			Name:         cell.Name,
			Path:         cell.Path,
			Type:         cell.Type,
			Dependencies: cell.Dependencies,
		}
	}

	apiEdges := make([]openapi.Edge, len(graph.Edges))
	for i, edge := range graph.Edges {
		apiEdges[i] = openapi.Edge{
			Id:     edge.ID,
			Source: edge.Source,
			Target: edge.Target,
		}
	}

	apiGraph := openapi.Graph{
		Cells: apiCells,
		Edges: apiEdges,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiGraph)
}
