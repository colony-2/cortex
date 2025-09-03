package devcontainer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMountsParsing(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected int
	}{
		{
			name: "string format mounts",
			json: `{
				"image": "golang:1.21",
				"mounts": [
					"source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
					"source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly"
				]
			}`,
			expected: 2,
		},
		{
			name: "object format mounts",
			json: `{
				"image": "golang:1.21",
				"mounts": [
					{
						"type": "bind",
						"source": "/host/path",
						"target": "/container/path",
						"readonly": true
					}
				]
			}`,
			expected: 1,
		},
		{
			name: "mixed format mounts",
			json: `{
				"image": "golang:1.21",
				"mounts": [
					"source=/tmp,target=/tmp,type=bind",
					{
						"type": "volume",
						"source": "myvolume",
						"target": "/data"
					}
				]
			}`,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dc DevContainer
			err := json.Unmarshal([]byte(tt.json), &dc)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, len(dc.Mounts))

			// Verify BuildDockerRunCommand handles them correctly
			config, err := BuildDockerRunCommand(&dc, "/workspace")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, len(config.Mounts))
		})
	}
}

func TestMountsExpansion(t *testing.T) {
	jsonStr := `{
		"image": "golang:1.21",
		"mounts": [
			"source=${localWorkspaceFolder}/data,target=/data,type=bind",
			"source=${localEnv:HOME}/.ssh,target=/root/.ssh,type=bind,readonly"
		]
	}`

	var dc DevContainer
	err := json.Unmarshal([]byte(jsonStr), &dc)
	require.NoError(t, err)

	// Expand variables
	vars := map[string]string{
		"localWorkspaceFolder": "/my/workspace",
		"localEnv:HOME":        "/home/user",
	}
	ExpandVariables(&dc, vars)

	// Check expansion worked
	assert.Equal(t, 2, len(dc.Mounts))
	mount0, ok := dc.Mounts[0].(string)
	assert.True(t, ok)
	assert.Contains(t, mount0, "/my/workspace/data")

	mount1, ok := dc.Mounts[1].(string)
	assert.True(t, ok)
	assert.Contains(t, mount1, "/home/user/.ssh")
}

func TestRealWorldDevcontainer(t *testing.T) {
	// Test case from the user's actual devcontainer.json
	jsonStr := `{
		"name": "${localWorkspaceFolderBasename}",
		"image": "golang:1.21-bookworm",
		"features": {
			"ghcr.io/devcontainers/features/common-utils:2": {},
			"ghcr.io/devcontainers/features/git:1": {}
		},
		"workspaceFolder": "/workspace",
		"mounts": [
			"source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
			"source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly",
			"source=${localEnv:HOME}/.cache/go-build,target=/go/pkg,type=bind",
			"source=${localEnv:HOME}/.cache/go-mod,target=/go/mod,type=bind"
		],
		"remoteUser": "vscode",
		"customizations": {
			"vscode": {
				"extensions": ["golang.go"]
			}
		},
		"postCreateCommand": "go version && go mod download"
	}`

	var dc DevContainer
	err := json.Unmarshal([]byte(jsonStr), &dc)
	require.NoError(t, err, "Failed to unmarshal real-world devcontainer.json")

	assert.Equal(t, 4, len(dc.Mounts), "Should have 4 mounts")
	assert.Equal(t, "golang:1.21-bookworm", dc.Image)
	assert.NotNil(t, dc.RemoteUser)
	assert.Equal(t, "vscode", *dc.RemoteUser)

	// Test that BuildDockerRunCommand works
	config, err := BuildDockerRunCommand(&dc, "/my/project")
	require.NoError(t, err)
	assert.Equal(t, 4, len(config.Mounts))
	assert.Equal(t, "golang:1.21-bookworm", config.Image)
}