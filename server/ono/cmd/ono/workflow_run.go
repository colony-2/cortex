package ono

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"vibethis/ono/pkg/compiler"
	"vibethis/ono/pkg/yaml"
)

var (
	workflowFile    string
	workflowID      string
	runNamespace    string
	workflowInputs  map[string]string
	taskQueue       string
)

var workflowRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a YAML workflow",
	Long: `Run a workflow defined in a YAML file.
	
Examples:
  # Run a workflow with auto-generated ID
  ono workflow run -f research_workflow.yaml
  
  # Run with specific workflow ID
  ono workflow run -f workflow.yaml --id my-workflow-123
  
  # Run with inputs
  ono workflow run -f workflow.yaml --input topic="AI Safety" --input depth=5`,
	RunE: executeWorkflow,
}

func executeWorkflow(cmd *cobra.Command, args []string) error {
	// Parse the YAML workflow
	parser := yaml.NewParser()
	project, err := parser.ParseProject(workflowFile)
	if err != nil {
		return fmt.Errorf("failed to parse workflow file: %w", err)
	}

	if project.Workflow == nil {
		return fmt.Errorf("no workflow definition found in %s", workflowFile)
	}

	// Generate workflow ID if not provided
	if workflowID == "" {
		workflowID = fmt.Sprintf("%s-%s", project.Workflow.Name, uuid.New().String()[:8])
	}

	// Connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", serverHost, serverPort),
		Namespace: runNamespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	// Create activity registry and compiler
	registry := compiler.NewActivityRegistry()
	
	// Register activities in the registry
	for i := range project.Activities {
		registry.RegisterActivity(&project.Activities[i])
	}
	
	// Create compiler with the registry
	comp := compiler.NewCompiler(registry)

	// Compile workflow (for validation)
	_, err = comp.CompileWorkflow(project.Workflow)
	if err != nil {
		return fmt.Errorf("failed to compile workflow: %w", err)
	}
	
	ctx := context.Background()

	// Note: In production, you would have a worker running separately
	// For demonstration, we'll just show the workflow would be submitted

	// Prepare inputs
	inputs := make(map[string]interface{})
	for key, value := range workflowInputs {
		inputs[key] = value
	}

	// Execute workflow
	fmt.Printf("Starting workflow: %s\n", workflowID)
	fmt.Printf("Type: %s\n", project.Workflow.Name)
	fmt.Printf("Task Queue: %s\n", taskQueue)
	fmt.Printf("Namespace: %s\n\n", runNamespace)

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
	}

	// Execute the workflow
	we, err := c.ExecuteWorkflow(ctx, workflowOptions, project.Workflow.Name, inputs)
	if err != nil {
		return fmt.Errorf("failed to start workflow: %w", err)
	}

	fmt.Printf("\nWorkflow started successfully!\n")
	fmt.Printf("Workflow ID: %s\n", we.GetID())
	fmt.Printf("Run ID: %s\n", we.GetRunID())
	
	// Optionally wait for the workflow to complete
	fmt.Printf("\nWaiting for workflow to complete...\n")
	
	var result interface{}
	err = we.Get(ctx, &result)
	if err != nil {
		return fmt.Errorf("workflow execution failed: %w", err)
	}
	
	fmt.Printf("\nWorkflow completed successfully!\n")
	fmt.Printf("Result: %v\n", result)
	
	// Show workflow steps
	if project.Workflow.Workflow.Steps != nil {
		fmt.Printf("\nWorkflow steps that would execute:\n")
		for i, step := range project.Workflow.Workflow.Steps {
			fmt.Printf("%d. %s\n", i+1, step.ID)
			if step.Activity != "" {
				fmt.Printf("   Activity: %s\n", step.Activity)
			}
			if len(step.Parallel) > 0 {
				fmt.Printf("   Parallel activities:\n")
				for _, p := range step.Parallel {
					fmt.Printf("   - %s\n", p.ID)
				}
			}
		}
	}

	return nil
}