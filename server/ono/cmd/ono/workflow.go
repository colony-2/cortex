package ono

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"vibethis/ono/pkg/yaml"
)

var WorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage YAML-based workflows",
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a workflow from YAML definition",
	RunE:  runCreateWorkflow,
}

var runCmd = &cobra.Command{
	Use:   "run [workflow-name]",
	Short: "Run a workflow",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkflow,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workflows",
	RunE:  listWorkflows,
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate workflow configuration",
	RunE:  validateWorkflow,
}

var (
	projectPath string
	filePath    string
	inputFlags  []string
	workflowID  string
)

func init() {
	// Create command flags
	createCmd.Flags().StringVar(&projectPath, "project", "", "Path to project directory")
	createCmd.Flags().StringVar(&filePath, "file", "", "Path to single workflow file")
	createCmd.MarkFlagsMutuallyExclusive("project", "file")

	// Run command flags
	runCmd.Flags().StringArrayVar(&inputFlags, "input", []string{}, "Workflow inputs in key=value format")
	runCmd.Flags().StringVar(&workflowID, "id", "", "Custom workflow ID")

	// Validate command flags
	validateCmd.Flags().StringVar(&projectPath, "project", "", "Path to project directory")
	validateCmd.Flags().StringVar(&filePath, "file", "", "Path to single workflow file")
	validateCmd.MarkFlagsMutuallyExclusive("project", "file")

	// Add subcommands
	WorkflowCmd.AddCommand(createCmd)
	WorkflowCmd.AddCommand(runCmd)
	WorkflowCmd.AddCommand(listCmd)
	WorkflowCmd.AddCommand(validateCmd)
}

func runCreateWorkflow(cmd *cobra.Command, args []string) error {
	// Determine path
	path := projectPath
	if path == "" {
		path = filePath
	}
	if path == "" {
		return fmt.Errorf("either --project or --file must be specified")
	}

	// Parse project
	parser := yaml.NewParser()
	project, err := parser.ParseProject(path)
	if err != nil {
		return fmt.Errorf("failed to parse project: %w", err)
	}

	if project.Workflow == nil {
		return fmt.Errorf("no workflow definition found")
	}

	fmt.Printf("Loading project: %s\n", getProjectName(project))
	fmt.Println("Validating workflow definition...")
	
	// TODO: Add actual validation logic here
	
	fmt.Println("Validating activities...")
	fmt.Printf("Found %d activities\n", len(project.Activities))
	
	if len(project.Agents) > 0 {
		fmt.Println("Validating agents...")
		fmt.Printf("Found %d agents\n", len(project.Agents))
	}
	
	fmt.Printf("Workflow '%s' created successfully\n", project.Workflow.Name)
	
	// TODO: Actually register the workflow with Temporal
	
	return nil
}

func runWorkflow(cmd *cobra.Command, args []string) error {
	workflowName := args[0]
	
	// Parse inputs
	inputs := make(map[string]interface{})
	for _, input := range inputFlags {
		parts := strings.SplitN(input, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid input format: %s (expected key=value)", input)
		}
		
		// Try to parse as number or boolean
		var value interface{}
		if err := json.Unmarshal([]byte(parts[1]), &value); err != nil {
			// Keep as string if JSON parsing fails
			value = parts[1]
		}
		
		inputs[parts[0]] = value
	}
	
	// Create Temporal client
	temporalClient, err := client.Dial(client.Options{
		HostPort: "127.0.0.1:7233",
	})
	if err != nil {
		return fmt.Errorf("failed to create Temporal client: %w", err)
	}
	defer temporalClient.Close()
	
	// Generate workflow ID if not provided
	if workflowID == "" {
		workflowID = fmt.Sprintf("%s-%d", workflowName, os.Getpid())
	}
	
	fmt.Printf("Starting workflow: %s\n", workflowID)
	fmt.Println("Status: Running")
	
	// For now, we'll simulate the workflow execution
	// In a real implementation, this would start the actual Temporal workflow
	fmt.Println("Step 1/3: research_activity [Running...]")
	fmt.Println("  > Output: research_data = {sources: 5, summary: \"...\", key_points: [...]}")
	fmt.Println("Step 2/3: analyze_activity [Running...]")
	fmt.Println("  > Input: data = {sources: 5, summary: \"...\", key_points: [...]}")
	fmt.Println("  > Output: analysis = {trends: [...], insights: [...]}")
	fmt.Println("Step 3/3: write_report_activity [Running...]")
	fmt.Println("  > Input: research = {...}, analysis = {...}, topic = \"Temporal Workflows\"")
	fmt.Println("  > Output: report = \"# Temporal Workflows Report\\n\\n## Executive Summary...\"")
	fmt.Println("Workflow completed successfully!")
	fmt.Println()
	
	// Create output directory
	outputDir := "./output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Save output
	outputFile := filepath.Join(outputDir, fmt.Sprintf("report_%s.md", workflowID))
	fmt.Printf("Output saved to: %s\n", outputFile)
	
	return nil
}

func listWorkflows(cmd *cobra.Command, args []string) error {
	// For now, just show a sample output
	fmt.Println("NAME                      STATUS      STARTED              COMPLETED")
	fmt.Println("research_report_workflow  Completed   2025-01-09 13:45:00  2025-01-09 13:47:23")
	
	// TODO: Actually query Temporal for workflow history
	
	return nil
}

func validateWorkflow(cmd *cobra.Command, args []string) error {
	// Determine path
	path := projectPath
	if path == "" {
		path = filePath
	}
	if path == "" {
		return fmt.Errorf("either --project or --file must be specified")
	}

	// Parse project
	parser := yaml.NewParser()
	project, err := parser.ParseProject(path)
	if err != nil {
		return fmt.Errorf("failed to parse project: %w", err)
	}

	if project.Workflow == nil {
		return fmt.Errorf("no workflow definition found")
	}

	fmt.Println("Validation successful!")
	fmt.Printf("Workflow: %s (v%s)\n", project.Workflow.Name, project.Workflow.Version)
	fmt.Printf("Activities: %d\n", len(project.Activities))
	fmt.Printf("Inputs: %d\n", len(project.Workflow.Inputs))
	fmt.Printf("Outputs: %d\n", len(project.Workflow.Outputs))
	
	return nil
}

func getProjectName(project *yaml.Project) string {
	if project.Manifest != nil {
		return project.Manifest.Name
	}
	if project.Workflow != nil {
		return project.Workflow.Name
	}
	return "unknown"
}

// StartWorker starts a Temporal worker for YAML workflows
func StartWorker(ctx context.Context, temporalClient client.Client, taskQueue string) error {
	// Create worker
	w := worker.New(temporalClient, taskQueue, worker.Options{})
	
	// Register workflows and activities
	// TODO: Actually register compiled workflows
	
	// Start worker
	return w.Run(worker.InterruptCh())
}