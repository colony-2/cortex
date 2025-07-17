package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/divisive-ai/vibethis/server/ono/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/ono/pkg/workflows"
	"github.com/spf13/cobra"
	yamlpkg "github.com/vibethis/server/recipe-core/pkg/yaml"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

var (
	createProjectPath string
	createFilePath    string
)

var workflowCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create and register a workflow with Temporal",
	Long: `Create and register a workflow from a YAML file or project directory.
	
This command validates the workflow definition, ensures all activities
are properly defined, and registers the workflow type with Temporal by
starting a worker that implements the workflow.

The worker will continue running to keep the workflow type available.
Press Ctrl+C to stop the worker and unregister the workflow type.

Examples:
  # Create from a single workflow file
  ono workflow create --file research_workflow.yaml
  
  # Create from a project directory
  ono workflow create --project /path/to/project
  
  # Create a specific workflow in a project
  ono workflow create --project /path/to/project research-workflow
  
  # Create in a specific namespace
  ono workflow create --file workflow.yaml --namespace production`,
	Args: cobra.MaximumNArgs(1),
	RunE: createWorkflow,
}

func createWorkflow(cmd *cobra.Command, args []string) error {
	// Determine path
	if createProjectPath == "" && createFilePath == "" {
		return fmt.Errorf("either --project or --file must be specified")
	}

	if createProjectPath != "" && createFilePath != "" {
		return fmt.Errorf("cannot specify both --project and --file")
	}

	path := createProjectPath
	if path == "" {
		path = createFilePath
	}

	// Parse project
	parser := yamlpkg.NewParser()
	project, err := parser.ParseProject(path)
	if err != nil {
		return fmt.Errorf("failed to parse project: %w", err)
	}

	// If a specific workflow name was provided, validate only that one
	var workflowToValidate *yamlpkg.WorkflowDefinition
	if len(args) > 0 && createProjectPath != "" {
		workflowName := args[0]

		// Check main workflow
		if project.Workflow != nil && project.Workflow.Name == workflowName {
			workflowToValidate = project.Workflow
		}

		if workflowToValidate == nil {
			return fmt.Errorf("workflow '%s' not found in project", workflowName)
		}
	} else if project.Workflow != nil {
		workflowToValidate = project.Workflow
	} else {
		return fmt.Errorf("no workflow definition found")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Loading project from: %s\n", path)

	// Display project info
	if createProjectPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Project: %s\n", filepath.Base(createProjectPath))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Workflow: %s (v%s)\n", workflowToValidate.Name, workflowToValidate.Version)
	if workflowToValidate.Description != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", workflowToValidate.Description)
	}

	// Create activity registry
	registry := compiler.NewActivityRegistry()

	// Register all activities
	fmt.Fprintf(cmd.OutOrStdout(), "\nRegistering activities:\n")
	for i := range project.Activities {
		activity := &project.Activities[i]
		registry.RegisterActivity(activity)
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", activity.Name)
	}

	// Compile and validate workflow
	fmt.Fprintf(cmd.OutOrStdout(), "\nValidating workflow...\n")
	comp := compiler.NewCompiler(registry)
	_, err = comp.CompileWorkflow(workflowToValidate)
	if err != nil {
		return fmt.Errorf("workflow validation failed: %w", err)
	}

	// Display workflow structure
	fmt.Fprintf(cmd.OutOrStdout(), "\nWorkflow structure:\n")
	if workflowToValidate.Workflow.Steps != nil {
		for i, step := range workflowToValidate.Workflow.Steps {
			fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s", i+1, step.ID)
			if step.Activity != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (activity: %s)", step.Activity)
			}
			fmt.Fprintln(cmd.OutOrStdout())

			if len(step.Parallel) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "     Parallel activities:\n")
				for _, p := range step.Parallel {
					fmt.Fprintf(cmd.OutOrStdout(), "     - %s (activity: %s)\n", p.ID, p.Activity)
				}
			}
		}
	}

	// Display inputs
	if len(workflowToValidate.Inputs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nRequired inputs:\n")
		for _, input := range workflowToValidate.Inputs {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s", input.Name)
			if input.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", input.Description)
			}
			if input.Default != nil {
				fmt.Fprintf(cmd.OutOrStdout(), " [default: %v]", input.Default)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}

	// Display outputs from workflow spec
	if len(workflowToValidate.Workflow.Outputs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nOutputs:\n")
		for name, value := range workflowToValidate.Workflow.Outputs {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s = %s\n", name, value)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Workflow '%s' is valid!\n", workflowToValidate.Name)

	// Connect to Temporal server
	fmt.Fprintf(cmd.OutOrStdout(), "\nConnecting to Temporal server...\n")
	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", serverHost, serverPort),
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal server: %w", err)
	}
	defer c.Close()

	// Create task queue name for this workflow
	taskQueueName := fmt.Sprintf("%s-queue", workflowToValidate.Name)

	// Create a worker for this workflow
	w := worker.New(c, taskQueueName, worker.Options{
		MaxConcurrentWorkflowTaskPollers: 2,
		MaxConcurrentActivityTaskPollers: 2,
	})

	// Register the workflow
	workflowFunc := workflows.CreateDynamicWorkflow(workflowToValidate, project, registry, comp)
	w.RegisterWorkflow(workflowFunc)

	// Register all activities used by this workflow with their names
	for i := range project.Activities {
		activityDef := &project.Activities[i]
		activityFunc := workflows.CreateDynamicActivity(activityDef)
		w.RegisterActivityWithOptions(activityFunc, activity.RegisterOptions{
			Name: activityDef.Name,
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Workflow '%s' registered with task queue '%s'\n", workflowToValidate.Name, taskQueueName)

	// Start the worker
	fmt.Fprintf(cmd.OutOrStdout(), "\nStarting worker to handle workflow execution...\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Press Ctrl+C to stop the worker\n\n")

	// Run the worker
	err = w.Run(worker.InterruptCh())
	if err != nil {
		return fmt.Errorf("failed to run worker: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nWorker stopped. Workflow '%s' is no longer available.\n", workflowToValidate.Name)
	return nil
}

// Helper to check if path is a directory
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
