package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	
	recipeworker "github.com/vibethis/server/recipe-worker"
)

// NewRecipeRunCommand creates the recipe run command
func NewRecipeRunCommand() *cobra.Command {
	var (
		inputJSON  string
		inputFile  string
		jobID      string
		wait       bool
		outputFmt  string
		namespace  string
		serverAddr string
	)

	cmd := &cobra.Command{
		Use:   "run <recipe-name>",
		Short: "Execute a recipe, creating a new job",
		Long: `Executes a recipe by creating a new job with the specified inputs.

The command will wait for the job to complete by default and display the results.`,
		Example: `  # Run a recipe with inline JSON input
  ono recipe run my-recipe --input '{"param1": "value1"}'

  # Run with input from file
  ono recipe run my-recipe --input-file params.json

  # Run without waiting for completion
  ono recipe run my-recipe --input-file params.json --wait=false

  # Run with custom job ID
  ono recipe run my-recipe --job-id my-job-123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecipeRun(cmd, args[0], inputJSON, inputFile, jobID, wait, outputFmt, namespace, serverAddr)
		},
	}

	cmd.Flags().StringVar(&inputJSON, "input", "", "Input parameters as JSON")
	cmd.Flags().StringVar(&inputFile, "input-file", "", "Input parameters from file")
	cmd.Flags().StringVar(&jobID, "job-id", "", "Custom job ID (default: auto-generated)")
	cmd.Flags().BoolVar(&wait, "wait", true, "Wait for job completion")
	cmd.Flags().StringVar(&outputFmt, "output", "text", "Output format: text, json")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Temporal namespace")
	cmd.Flags().StringVar(&serverAddr, "address", "localhost:7233", "Temporal server address")

	return cmd
}

func runRecipeRun(cmd *cobra.Command, recipeName, inputJSON, inputFile, jobID string, wait bool, outputFmt, namespace, serverAddr string) error {
	// Parse inputs
	inputs := make(map[string]interface{})
	if inputJSON != "" && inputFile != "" {
		return fmt.Errorf("cannot specify both --input and --input-file")
	}

	if inputJSON != "" {
		if err := json.Unmarshal([]byte(inputJSON), &inputs); err != nil {
			return fmt.Errorf("invalid JSON input: %w", err)
		}
	} else if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return fmt.Errorf("failed to read input file: %w", err)
		}
		if err := json.Unmarshal(data, &inputs); err != nil {
			return fmt.Errorf("invalid JSON in input file: %w", err)
		}
	}

	// Generate job ID if not provided
	if jobID == "" {
		jobID = fmt.Sprintf("%s-%s", recipeName, uuid.New().String()[:8])
	}

	// Get recipe directory from config
	recipesDir := getRecipesDir()
	
	// Create logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create recipe registry
	registry, err := recipeworker.NewRegistry(logger, recipesDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create recipe registry: %w", err)
	}

	// Start registry to discover recipes
	if err := registry.Start(); err != nil {
		return fmt.Errorf("failed to start recipe registry: %w", err)
	}
	defer registry.Stop()

	// Get the recipe
	r, err := registry.GetRecipe(recipeName)
	if err != nil {
		return fmt.Errorf("recipe not found: %w", err)
	}

	if r.Workflow == nil {
		return fmt.Errorf("recipe does not have a workflow defined")
	}

	// Connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort:  serverAddr,
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	// Get the task queue for this recipe
	taskQueue := fmt.Sprintf("ono-recipes-%s", recipeName)

	// Execute the workflow
	workflowOptions := client.StartWorkflowOptions{
		ID:        jobID,
		TaskQueue: taskQueue,
		// TODO: Configure workflow options based on recipe
	}

	we, err := c.ExecuteWorkflow(context.Background(), workflowOptions, r.Workflow.Name, inputs)
	if err != nil {
		return fmt.Errorf("failed to start workflow: %w", err)
	}

	// Output initial status
	if outputFmt == "json" {
		output := map[string]interface{}{
			"job_id":      jobID,
			"recipe":      recipeName,
			"status":      "started",
			"workflow_id": we.GetID(),
			"run_id":      we.GetRunID(),
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		encoder.Encode(output)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Job started:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  Job ID: %s\n", jobID)
		fmt.Fprintf(cmd.OutOrStdout(), "  Recipe: %s\n", recipeName)
		fmt.Fprintf(cmd.OutOrStdout(), "  Status: Running\n")
	}

	// Wait for completion if requested
	if wait {
		fmt.Fprintf(cmd.OutOrStdout(), "\nWaiting for job to complete...\n")
		
		var result map[string]interface{}
		err = we.Get(context.Background(), &result)
		
		if err != nil {
			// Job failed
			if outputFmt == "json" {
				output := map[string]interface{}{
					"job_id": jobID,
					"recipe": recipeName,
					"status": "failed",
					"error":  err.Error(),
				}
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				encoder.Encode(output)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "\nJob failed: %v\n", err)
			}
			return fmt.Errorf("job execution failed")
		}

		// Job succeeded
		if outputFmt == "json" {
			output := map[string]interface{}{
				"job_id": jobID,
				"recipe": recipeName,
				"status": "completed",
				"result": result,
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			encoder.Encode(output)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "\nJob completed successfully!\n")
			if len(result) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Result:\n")
				for k, v := range result {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %v\n", k, v)
				}
			}
		}
	}

	return nil
}