package client

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/nucleus/internal/config"
	"go.temporal.io/sdk/client"
)

func NewTemporalClient(cfg *config.Config) (client.Client, error) {
	options := client.Options{
		HostPort:  cfg.TemporalServer,
		Namespace: cfg.Namespace,
	}

	temporalClient, err := client.Dial(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create Temporal client: %w", err)
	}

	return temporalClient, nil
}