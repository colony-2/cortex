package cli

import (
	"context"
	"fmt"

	"github.com/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
	"go.temporal.io/sdk/client"
)

var (
	workflowFile   string
	workflowID     string
	runNamespace   string
	workflowInputs map[string]string
	taskQueue      string
	projectPath    string
)

var workflowRunCmd = &cobra.Command{
	Use:   "run [workflow-name]",
	Short: "Run a workflow",
	Long: `Run a workflow that has been defined in a YAML file or project.
	
The workflow definition is loaded from either a file or project directory,
then executed on the Temporal server.

Examples:
  # Run a workflow from a file with auto-generated ID
  ono workflow run --file research_workflow.yaml
  
  # Run a workflow from a project directory
  ono workflow run --project /path/to/project research-workflow
  
  # Run with specific workflow ID
  ono workflow run --file workflow.yaml --id my-workflow-123
  
  # Run with inputs
  ono workflow run --file workflow.yaml --input topic="AI Safety" --input depth=5
  
  # Run with custom task queue
  ono workflow run --file workflow.yaml --task-queue my-queue`,
	Args: cobra.MaximumNArgs(1),
	RunE: executeWorkflow,
}

func executeWorkflow(cmd *cobra.Command, args []string) error {
	var project *yamlpkg.Project
	var err error
	parser := yamlpkg.NewParser()

	// Determine how to load the workflow
	if workflowFile != "" && projectPath != "" {
		return fmt.Errorf("cannot specify both --file and --project flags")
	}

	if workflowFile != "" {
		// Load from file
		project, err = parser.ParseProject(workflowFile)
		if err != nil {
			return fmt.Errorf("failed to parse workflow file: %w", err)
		}
	} else if projectPath != "" {
		// Load from project directory
		if len(args) == 0 {
			return fmt.Errorf("workflow name required when using --project flag")
		}
		workflowName := args[0]

		// Parse the project directory
		project, err = parser.ParseProject(projectPath)
		if err != nil {
			return fmt.Errorf("failed to parse project: %w", err)
		}

		// Find the specified workflow
		if project.Workflow == nil || project.Workflow.Name != workflowName {
			return fmt.Errorf("workflow '%s' not found in project", workflowName)
		}
	} else {
		return fmt.Errorf("either --file or --project must be specified")
	}

	if project.Workflow == nil {
		return fmt.Errorf("no workflow definition found")
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

	ctx := context.Background()

	// Quick validation - ensure workflow is compilable
	registry := compiler.NewActivityRegistry()
	for i := range project.Activities {
		registry.RegisterActivity(&project.Activities[i])
	}
	comp := compiler.NewCompiler(registry)
	if _, err = comp.CompileWorkflow(project.Workflow); err != nil {
		return fmt.Errorf("workflow validation failed: %w", err)
	}

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
	if result != nil {
		fmt.Printf("Result: %v\n", result)
	}

	return nil
}
