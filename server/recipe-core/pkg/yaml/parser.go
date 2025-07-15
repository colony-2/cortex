package yaml

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Parser handles parsing of YAML workflow definitions
type Parser struct{}

// NewParser creates a new YAML parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseProject parses a project from a directory or single file
func (p *Parser) ParseProject(path string) (*Project, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		return p.parseProjectDirectory(path)
	}
	return p.parseSingleFile(path)
}

// Project represents a complete workflow project
type Project struct {
	Manifest   *ProjectManifest
	Workflow   *WorkflowDefinition
	Activities []ActivityDefinition
	Agents     map[string]AgentDefinition
}

func (p *Parser) parseProjectDirectory(dir string) (*Project, error) {
	// Look for project.yaml
	manifestPath := filepath.Join(dir, "project.yaml")
	manifest, err := p.parseManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse project manifest: %w", err)
	}

	project := &Project{
		Manifest: manifest,
		Agents:   make(map[string]AgentDefinition),
	}

	// Parse workflow
	if manifest.Files.Workflow != "" {
		workflowPath := filepath.Join(dir, manifest.Files.Workflow)
		workflow, err := p.parseWorkflow(workflowPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow: %w", err)
		}
		project.Workflow = workflow
	}

	// Parse activities
	if manifest.Files.Activities != "" {
		activitiesPath := filepath.Join(dir, manifest.Files.Activities)
		activities, err := p.parseActivities(activitiesPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse activities: %w", err)
		}
		project.Activities = activities
	}

	// Parse agents
	if manifest.Files.Agents != "" {
		agentsPath := filepath.Join(dir, manifest.Files.Agents)
		agents, err := p.parseAgents(agentsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse agents: %w", err)
		}
		project.Agents = agents
	}

	return project, nil
}

func (p *Parser) parseSingleFile(path string) (*Project, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Try to parse as a complete single-file definition
	var singleFile struct {
		Workflow   *WorkflowDefinition  `yaml:"workflow"`
		Activities []ActivityDefinition `yaml:"activities"`
		Agents     map[string]AgentDefinition `yaml:"agents"`
	}

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&singleFile); err != nil {
		return nil, fmt.Errorf("failed to parse single file: %w", err)
	}

	return &Project{
		Workflow:   singleFile.Workflow,
		Activities: singleFile.Activities,
		Agents:     singleFile.Agents,
	}, nil
}

func (p *Parser) parseManifest(path string) (*ProjectManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest ProjectManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func (p *Parser) parseWorkflow(path string) (*WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var workflow WorkflowDefinition
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, err
	}

	return &workflow, nil
}

func (p *Parser) parseActivities(path string) ([]ActivityDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var activitiesFile ActivitiesFile
	if err := yaml.Unmarshal(data, &activitiesFile); err != nil {
		return nil, err
	}

	return activitiesFile.Activities, nil
}

func (p *Parser) parseAgents(path string) (map[string]AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var agentsFile AgentsFile
	if err := yaml.Unmarshal(data, &agentsFile); err != nil {
		return nil, err
	}

	return agentsFile.Agents, nil
}

// ParseWorkflowReader parses a workflow definition from an io.Reader
func (p *Parser) ParseWorkflowReader(r io.Reader) (*WorkflowDefinition, error) {
	var workflow WorkflowDefinition
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&workflow); err != nil {
		return nil, fmt.Errorf("failed to decode workflow: %w", err)
	}
	return &workflow, nil
}

// ParseActivitiesReader parses activity definitions from an io.Reader
func (p *Parser) ParseActivitiesReader(r io.Reader) ([]ActivityDefinition, error) {
	var activitiesFile ActivitiesFile
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&activitiesFile); err != nil {
		return nil, fmt.Errorf("failed to decode activities: %w", err)
	}
	return activitiesFile.Activities, nil
}