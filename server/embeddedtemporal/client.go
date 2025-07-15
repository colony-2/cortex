package embeddedtemporal

import (
	"fmt"

	"go.temporal.io/sdk/client"
)

// ClientOptions provides options for creating a Temporal client
type ClientOptions struct {
	HostPort   string
	Namespace  string
	MetricsHandler client.MetricsHandler
}

// NewClient creates a new Temporal client configured for the embedded server
func NewClient(opts ClientOptions) (client.Client, error) {
	if opts.HostPort == "" {
		return nil, fmt.Errorf("host port is required")
	}

	if opts.Namespace == "" {
		opts.Namespace = client.DefaultNamespace
	}

	clientOpts := client.Options{
		HostPort:  opts.HostPort,
		Namespace: opts.Namespace,
	}

	if opts.MetricsHandler != nil {
		clientOpts.MetricsHandler = opts.MetricsHandler
	}

	return client.Dial(clientOpts)
}

// NewNamespaceClient creates a new Temporal namespace client configured for the embedded server
func NewNamespaceClient(hostPort string) (client.NamespaceClient, error) {
	if hostPort == "" {
		return nil, fmt.Errorf("host port is required")
	}

	return client.NewNamespaceClient(client.Options{
		HostPort: hostPort,
	})
}