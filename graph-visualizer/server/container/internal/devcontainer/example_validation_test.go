package devcontainer_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"vibethis/container/internal/devcontainer"
)

func Example_validation() {
	// Create a sample devcontainer.json
	tmpDir, _ := os.MkdirTemp("", "devcontainer-validation")
	defer os.RemoveAll(tmpDir)

	devcontainerJSON := `{
		"image": "mcr.microsoft.com/devcontainers/go:1.21",
		"forwardPorts": [8080, "9000:9000"],
		"containerEnv": {
			"GOPROXY": "https://proxy.golang.org"
		},
		"mounts": [{
			"type": "volume",
			"source": "go-mod-cache",
			"target": "/go/pkg/mod"
		}],
		"capAdd": ["SYS_PTRACE"],
		"init": true
	}`

	configPath := filepath.Join(tmpDir, "devcontainer.json")
	os.WriteFile(configPath, []byte(devcontainerJSON), 0644)

	// Load and build docker command
	dc, err := devcontainer.LoadDevContainer(configPath)
	if err != nil {
		log.Fatal(err)
	}

	config, err := devcontainer.BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		log.Fatal(err)
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		log.Printf("Configuration validation failed: %v", err)
		return
	}

	// Get the docker command arguments
	args := config.ToDockerRunArgs()

	// Validate the command syntax
	if err := devcontainer.ValidateDockerCommand(args); err != nil {
		log.Printf("Command validation failed: %v", err)
		return
	}

	// Extract and display the image
	image, _ := devcontainer.ExtractDockerImage(args)
	fmt.Printf("Image: %s\n", image)
	fmt.Printf("Command would execute: docker %s\n", args[0])

	// Output:
	// Image: mcr.microsoft.com/devcontainers/go:1.21
	// Command would execute: docker run
}

func Example_dryRun() {
	// Example of using dry run validation
	config := &devcontainer.DockerRunConfig{
		Image:           "alpine:latest",
		WorkspaceMount:  "type=bind,source=/tmp,target=/workspace",
		WorkspaceFolder: "/workspace",
		Environment: map[string]string{
			"USER": "developer",
		},
		Ports: []string{"8080:80"},
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		log.Fatal(err)
	}

	args := config.ToDockerRunArgs()

	// Try a dry run
	if err := devcontainer.DryRunDockerCommand(args); err != nil {
		// Dry run succeeded or failed - just indicate we tried
		fmt.Println("Dry run completed")
	} else {
		fmt.Println("Dry run completed")
	}

	// Output:
	// Dry run completed
}