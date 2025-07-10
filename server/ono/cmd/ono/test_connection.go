package ono

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var testConnectionCmd = &cobra.Command{
	Use:   "test-connection",
	Short: "Test connection to Temporal server",
	RunE:  runTestConnection,
}

func init() {
	WorkflowCmd.AddCommand(testConnectionCmd)
}

func runTestConnection(cmd *cobra.Command, args []string) error {
	// Try to connect to Temporal
	fmt.Println("Testing connection to Temporal server at 127.0.0.1:7233...")
	
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
				HostPort: "127.0.0.1:7233",
			},
		},
		{
			name: "With namespace",
			options: client.Options{
				HostPort:  "127.0.0.1:7233",
				Namespace: "default",
			},
		},
		{
			name: "Localhost",
			options: client.Options{
				HostPort: "localhost:7233",
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