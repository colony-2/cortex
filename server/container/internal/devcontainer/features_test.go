package devcontainer

import (
	"testing"
)

func TestFeatureHandling(t *testing.T) {
	tests := []struct {
		name     string
		dc       *DevContainer
		validate func(*testing.T, *DevContainer)
	}{
		{
			name: "basic features",
			dc: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					Features: &DevContainerCommonFeatures{
						Fish:   "latest",
						Gradle: "7.6",
						AdditionalProperties: map[string]interface{}{
							"ghcr.io/devcontainers/features/go:1": map[string]interface{}{
								"version": "1.21",
							},
							"ghcr.io/devcontainers/features/node:1": map[string]interface{}{
								"version": "18",
							},
						},
					},
				},
			},
			validate: func(t *testing.T, dc *DevContainer) {
				if dc.Features == nil {
					t.Fatal("features should not be nil")
				}
				if dc.Features.Fish != "latest" {
					t.Error("fish feature not preserved")
				}
				if dc.Features.Gradle != "7.6" {
					t.Error("gradle feature not preserved")
				}
				
				props := dc.Features.AdditionalProperties
				if props == nil {
					t.Fatal("additional properties should not be nil")
				}
				
				if _, exists := props["ghcr.io/devcontainers/features/go:1"]; !exists {
					t.Error("Go feature not found")
				}
				if _, exists := props["ghcr.io/devcontainers/features/node:1"]; !exists {
					t.Error("Node feature not found")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.dc)
		})
	}
}

func TestFeaturesInDockerBuild(t *testing.T) {
	// Features are typically handled during container image build process
	// For Docker run commands, we can document what features were requested
	dc := &DevContainer{
		ImageContainer: &ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/base:ubuntu",
		},
		DevContainerCommon: DevContainerCommon{
			Features: &DevContainerCommonFeatures{
				AdditionalProperties: map[string]interface{}{
					"ghcr.io/devcontainers/features/go:1": map[string]interface{}{
						"version": "1.21",
					},
				},
			},
		},
	}

	// Features don't directly affect docker run commands
	// They're typically pre-installed in the image or handled by build tools
	config, err := BuildDockerRunCommand(dc, "/workspace")
	if err != nil {
		t.Fatal(err)
	}

	// Features themselves don't add docker run args
	// But the test verifies the system can handle feature configurations
	if config.Image != "mcr.microsoft.com/devcontainers/base:ubuntu" {
		t.Error("image should be preserved even with features")
	}
}