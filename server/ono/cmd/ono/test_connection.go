package ono

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var testConnectionNamespace string

var workflowTestConnectionCmd = &cobra.Command{
	Use:   "test-connection",
	Short: "Test connection to Temporal server",
	RunE:  runWorkflowTestConnection,
}

func runWorkflowTestConnection(cmd *cobra.Command, args []string) error {
	// Build connection string
	connectionString := fmt.Sprintf("%s:%d", serverHost, serverPort)
	fmt.Printf("Testing connection to Temporal server at %s...\n", connectionString)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Try different connection options
	configs := []struct {
		name    string
		options client.Options
	}{
		{
			name: "Default",
			options: client.Options{
				HostPort: connectionString,
			},
		},
		{
			name: "With namespace",
			options: client.Options{
				HostPort:  connectionString,
				Namespace: testConnectionNamespace,
			},
		},
	}
	
	for _, cfg := range configs {
		fmt.Printf("\nTrying %s configuration...\n", cfg.name)
		c, err := client.DialContext(ctx, cfg.options)
		if err != nil {
			fmt.Printf("  ❌ Failed: %v\n", err)
			continue
		}
		
		// Try to check system info
		_, err = c.WorkflowService().GetSystemInfo(ctx, &workflowservice.GetSystemInfoRequest{})
		if err != nil {
			fmt.Printf("  ❌ Connected but failed to get system info: %v\n", err)
			c.Close()
			continue
		}
		
		fmt.Printf("  ✅ Success! Connected with %s configuration\n", cfg.name)
		c.Close()
		return nil
	}
	
	return fmt.Errorf("failed to connect with any configuration")
}