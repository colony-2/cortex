package recipehistory

import (
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"github.com/vibethis/server/recipe-history/pkg/history"
)

// Export public types and constants for easier access

// Re-export key interfaces and types that consumers will need
type History = history.Client
type Client = history.Client
type Transformer = history.Transformer
type ParsedHistory = history.ParsedHistory
type EventInfo = history.EventInfo

// Re-export constructor functions
func NewClient(temporalClient client.Client, logger *zap.Logger) *Client {
	return history.NewClient(temporalClient, logger)
}

func NewTransformer() *Transformer {
	return history.NewTransformer()
}