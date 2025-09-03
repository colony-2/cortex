package devcontainer

import (
	"os/exec"
	"strings"
	"testing"
)

func TestValidateDockerCommand(t *testing.T) {
	// Skip if docker is not available
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available, skipping validation tests")
	}

	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple run",
			args:    []string{"run", "--rm", "-it", "alpine:latest"},
			wantErr: false,
		},
		{
			name:    "valid run with ports",
			args:    []string{"run", "--rm", "-p", "8080:80", "nginx:latest"},
			wantErr: false,
		},
		{
			name:    "valid run with mount",
			args:    []string{"run", "--mount", "type=bind,source=/tmp,target=/data", "ubuntu:22.04"},
			wantErr: false,
		},
		{
			name:    "missing image",
			args:    []string{"run", "--rm", "-it"},
			wantErr: true,
			errMsg:  "no image specified",
		},
		{
			name:    "flag missing argument",
			args:    []string{"run", "-p", "-it", "alpine:latest"},
			wantErr: true,
			errMsg:  "flag -p requires an argument",
		},
		{
			name:    "empty command",
			args:    []string{},
			wantErr: false, // Not a run command, so passes validation
		},
		{
			name:    "multiple valid flags",
			args:    []string{"run", "--rm", "-it", "-e", "FOO=bar", "-w", "/app", "-u", "1000:1000", "node:18"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDockerCommand(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDockerCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

func TestExtractDockerImage(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantImage string
		wantErr   bool
	}{
		{
			name:      "simple run command",
			args:      []string{"run", "--rm", "alpine:latest"},
			wantImage: "alpine:latest",
			wantErr:   false,
		},
		{
			name:      "run with many flags",
			args:      []string{"run", "--rm", "-it", "-p", "8080:80", "-e", "FOO=bar", "nginx:1.21"},
			wantImage: "nginx:1.21",
			wantErr:   false,
		},
		{
			name:      "run with mount before image",
			args:      []string{"run", "--mount", "type=bind,source=/tmp,target=/data", "ubuntu:22.04"},
			wantImage: "ubuntu:22.04",
			wantErr:   false,
		},
		{
			name:      "run with entrypoint",
			args:      []string{"run", "--entrypoint", "/bin/sh", "alpine:latest"},
			wantImage: "alpine:latest",
			wantErr:   false,
		},
		{
			name:      "not a run command",
			args:      []string{"ps", "-a"},
			wantImage: "",
			wantErr:   true,
		},
		{
			name:      "empty args",
			args:      []string{},
			wantImage: "",
			wantErr:   true,
		},
		{
			name:      "run without image",
			args:      []string{"run", "--rm", "-it"},
			wantImage: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotImage, err := ExtractDockerImage(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractDockerImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotImage != tt.wantImage {
				t.Errorf("ExtractDockerImage() = %v, want %v", gotImage, tt.wantImage)
			}
		})
	}
}

func TestDryRunDockerCommand(t *testing.T) {
	// Skip if docker is not available
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available, skipping dry run tests")
	}

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "valid alpine run",
			args:    []string{"run", "--rm", "alpine:latest"},
			wantErr: false,
		},
		{
			name:    "valid ubuntu with mount",
			args:    []string{"run", "--rm", "--mount", "type=bind,source=/tmp,target=/data", "ubuntu:22.04"},
			wantErr: false,
		},
		{
			name:    "invalid image name",
			args:    []string{"run", "--rm", "this-image-definitely-does-not-exist:v99999"},
			wantErr: true,
		},
		{
			name:    "run with complex flags",
			args:    []string{"run", "--rm", "-it", "-e", "TEST=1", "-p", "8080:80", "--cap-add", "SYS_PTRACE", "alpine:latest"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DryRunDockerCommand(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("DryRunDockerCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationWithRealDevContainer(t *testing.T) {
	// Skip if docker is not available
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available, skipping integration tests")
	}

	// Test with a real devcontainer configuration
	dc := &DevContainer{
		ImageContainer: &ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/go:1.21",
		},
		DevContainerCommon: DevContainerCommon{
			ContainerEnv: map[string]string{
				"GOPROXY": "https://proxy.golang.org",
			},
			ForwardPorts: []interface{}{float64(8080)},
			Mounts: []interface{}{
				map[string]interface{}{
					"type":   "volume",
					"source": "go-mod-cache",
					"target": "/go/pkg/mod",
				},
			},
			Init:       boolPtr(true),
			Privileged: boolPtr(false),
			CapAdd:     []string{"SYS_PTRACE"},
		},
		NonComposeBase: &NonComposeBase{
			RunArgs: []string{"--network", "bridge"},
		},
	}

	config, err := BuildDockerRunCommand(dc, "/tmp/test-workspace")
	if err != nil {
		t.Fatalf("BuildDockerRunCommand failed: %v", err)
	}

	args := config.ToDockerRunArgs()

	// Validate the generated command
	t.Logf("Generated command: docker %s", strings.Join(args, " "))
	if err := ValidateDockerCommand(args); err != nil {
		t.Errorf("Generated docker command failed validation: %v", err)
	}

	// Extract and verify the image
	image, err := ExtractDockerImage(args)
	if err != nil {
		t.Errorf("Failed to extract image: %v", err)
	}
	if image != "mcr.microsoft.com/devcontainers/go:1.21" {
		t.Errorf("Extracted wrong image: got %s, want mcr.microsoft.com/devcontainers/go:1.21", image)
	}

	// Try a dry run (this might fail if the image doesn't exist locally)
	// We don't fail the test on this, just log it
	if err := DryRunDockerCommand(args); err != nil {
		t.Logf("Dry run failed (this is okay if image not pulled): %v", err)
	}
}

func TestDockerFlagsValidation(t *testing.T) {
	tests := []struct {
		name    string
		flags   []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid port flag",
			flags:   []string{"-p", "8080:80"},
			wantErr: false,
		},
		{
			name:    "port flag without value",
			flags:   []string{"-p"},
			wantErr: true,
			errMsg:  "flag -p requires an argument",
		},
		{
			name:    "multiple flags",
			flags:   []string{"-e", "FOO=bar", "-p", "8080:80", "--init"},
			wantErr: false,
		},
		{
			name:    "mount flag",
			flags:   []string{"--mount", "type=bind,source=/host,target=/container"},
			wantErr: false,
		},
		{
			name:    "security options",
			flags:   []string{"--cap-add", "SYS_PTRACE", "--security-opt", "seccomp=unconfined"},
			wantErr: false,
		},
		{
			name:    "missing argument for env",
			flags:   []string{"-e", "-p", "8080:80"},
			wantErr: true,
			errMsg:  "flag -e requires an argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDockerRunFlags(tt.flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDockerRunFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}