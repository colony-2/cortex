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
	apiNodes := make([]openapi.Node, len(graph.Nodes))
	for i, node := range graph.Nodes {
		apiNodes[i] = openapi.Node{
			Id:           node.ID,
			Name:         node.Name,
			Path:         node.Path,
			Type:         node.Type,
			Dependencies: node.Dependencies,
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
		Nodes: apiNodes,
		Edges: apiEdges,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiGraph)
}
