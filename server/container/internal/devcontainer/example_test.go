package devcontainer_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/divisive-ai/vibethis/server/container/internal/devcontainer"
)

func Example() {
	// Create a sample devcontainer.json for demonstration
	tmpDir, err := os.MkdirTemp("", "devcontainer-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	devcontainerJSON := `{
		"image": "mcr.microsoft.com/devcontainers/go:1.21",
		"workspaceFolder": "/workspaces/myapp",
		"forwardPorts": [8080],
		"containerEnv": {
			"GOPROXY": "https://proxy.golang.org"
		},
		"mounts": [{
			"type": "volume",
			"source": "go-mod-cache",
			"target": "/go/pkg/mod"
		}],
		"postCreateCommand": "go mod download"
	}`

	configPath := filepath.Join(tmpDir, ".devcontainer", "devcontainer.json")
	os.MkdirAll(filepath.Dir(configPath), 0755)
	os.WriteFile(configPath, []byte(devcontainerJSON), 0644)

	// Load the devcontainer configuration
	dc, err := devcontainer.LoadDevContainer(configPath)
	if err != nil {
		log.Fatal(err)
	}

	// Build docker run command
	config, err := devcontainer.BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		log.Fatal(err)
	}

	// Get the docker run arguments
	args := config.ToDockerRunArgs()

	// Print some of the key arguments
	fmt.Println("Image:", config.Image)
	fmt.Println("Workspace folder:", config.WorkspaceFolder)
	fmt.Println("Environment:", config.Environment)
	fmt.Println("Ports:", config.Ports)
	fmt.Println("Mounts:", len(config.Mounts))

	// The full docker command would be:
	// docker <args...>
	_ = args

	// Output:
	// Image: mcr.microsoft.com/devcontainers/go:1.21
	// Workspace folder: /workspaces/myapp
	// Environment: map[GOPROXY:https://proxy.golang.org]
	// Ports: [8080:8080]
	// Mounts: 1
}
