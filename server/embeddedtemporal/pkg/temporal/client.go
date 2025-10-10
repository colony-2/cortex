package temporal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ClientOptions provides options for creating a Temporal client
type ClientOptions struct {
	HostPort       string
	Namespace      string
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

	// Retry dial to tolerate server warmup on slow runners.
	var lastErr error
	for i := 0; i < 240; i++ { // ~120s total
		c, err := client.Dial(clientOpts)
		if err == nil {
			// Ensure target namespace exists (register on demand)
			if err := ensureNamespaceExists(clientOpts.HostPort, clientOpts.Namespace); err != nil {
				c.Close()
				lastErr = fmt.Errorf("ensure namespace exists: %w", err)
			} else {
				return c, nil
			}
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("failed to create client after retries: %w", lastErr)
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

// ensureNamespaceExists attempts to describe the namespace and registers it if missing.
func ensureNamespaceExists(hostPort, namespace string) error {
	if namespace == "" || namespace == client.DefaultNamespace {
		return nil
	}
	nc, err := client.NewNamespaceClient(client.Options{HostPort: hostPort})
	if err != nil {
		return err
	}
	defer nc.Close()

	// Budget ~60s for namespace to become available
	deadline := time.Now().Add(60 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := nc.Describe(ctx, namespace)
		cancel()
		if err == nil {
			return nil
		}

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		regErr := nc.Register(ctx, &workflowservice.RegisterNamespaceRequest{
			Namespace:                        namespace,
			WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
			Description:                      "Auto-registered by embedded client",
		})
		cancel()

		if regErr == nil {
			// Give the cluster a brief moment to propagate and re-check.
			time.Sleep(200 * time.Millisecond)
			continue
		}

		var alreadyExists *serviceerror.NamespaceAlreadyExists
		if errors.As(regErr, &alreadyExists) {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if time.Now().After(deadline) {
			return regErr
		}
		time.Sleep(1 * time.Second)
	}
}
