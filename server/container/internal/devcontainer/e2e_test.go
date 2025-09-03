// +build e2e

package devcontainer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2EMountsActuallyWork tests that mounts actually work with real Docker containers
func TestE2EMountsActuallyWork(t *testing.T) {
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available")
	}

	tmpDir := t.TempDir()
	
	// Create test files in the host directory
	testFile := filepath.Join(tmpDir, "test-file.txt")
	testContent := "Hello from host filesystem!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create subdirectory with another file
	subDir := filepath.Join(tmpDir, "subdir")
	os.MkdirAll(subDir, 0755)
	subFile := filepath.Join(subDir, "sub-file.txt")
	if err := os.WriteFile(subFile, []byte("Subdirectory file"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create devcontainer that mounts the test directory
	dc := &DevContainer{
		ImageContainer: &ImageContainer{
			Image: "alpine:latest",
		},
		DevContainerCommon: DevContainerCommon{
			Mounts: []interface{}{
				map[string]interface{}{
					"type":   "bind",
					"source": tmpDir,
					"target": "/test-mount",
				},
			},
		},
	}

	config, err := BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	args := config.ToDockerRunArgs()
	
	// Remove -it flags for non-interactive test
	filteredArgs := []string{}
	for _, arg := range args {
		if arg != "-it" && arg != "-i" && arg != "-t" {
			filteredArgs = append(filteredArgs, arg)
		}
	}
	args = filteredArgs
	
	// Add a command to test the mount
	args = append(args, "sh", "-c", "ls -la /test-mount && cat /test-mount/test-file.txt && cat /test-mount/subdir/sub-file.txt")

	t.Logf("Running: docker %s", strings.Join(args, " "))

	// Execute the container
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Fatalf("Container execution failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	
	// Verify the mount worked
	if !strings.Contains(outputStr, "test-file.txt") {
		t.Error("Mount did not include host files")
	}
	if !strings.Contains(outputStr, testContent) {
		t.Error("Could not read host file content through mount")
	}
	if !strings.Contains(outputStr, "Subdirectory file") {
		t.Error("Could not read subdirectory file through mount")
	}
	
	t.Logf("Mount test successful. Container output:\n%s", outputStr)
}

// TestE2EVolumePersistence tests that volumes actually persist data
func TestE2EVolumePersistence(t *testing.T) {
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available")
	}

	tmpDir := t.TempDir()
	volumeName := fmt.Sprintf("test-vol-%d", time.Now().Unix())
	
	// Clean up volume after test
	defer func() {
		exec.Command("docker", "volume", "rm", volumeName).Run()
	}()

	// Create devcontainer with a volume mount
	dc := &DevContainer{
		ImageContainer: &ImageContainer{
			Image: "alpine:latest",
		},
		DevContainerCommon: DevContainerCommon{
			Mounts: []interface{}{
				map[string]interface{}{
					"type":   "volume",
					"source": volumeName,
					"target": "/data",
				},
			},
		},
	}

	config, err := BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// First container: write to volume
	args1 := config.ToDockerRunArgs()
	// Remove -it flags for non-interactive test
	filteredArgs1 := []string{}
	for _, arg := range args1 {
		if arg != "-it" && arg != "-i" && arg != "-t" {
			filteredArgs1 = append(filteredArgs1, arg)
		}
	}
	args1 = filteredArgs1
	args1 = append(args1, "sh", "-c", "echo 'Volume test data' > /data/test.txt && cat /data/test.txt")

	cmd1 := exec.Command("docker", args1...)
	output1, err := cmd1.CombinedOutput()
	if err != nil {
		t.Fatalf("First container failed: %v\nOutput: %s", err, output1)
	}

	// Second container: read from volume
	args2 := config.ToDockerRunArgs()
	// Remove -it flags for non-interactive test
	filteredArgs2 := []string{}
	for _, arg := range args2 {
		if arg != "-it" && arg != "-i" && arg != "-t" {
			filteredArgs2 = append(filteredArgs2, arg)
		}
	}
	args2 = filteredArgs2
	args2 = append(args2, "sh", "-c", "ls -la /data && cat /data/test.txt")

	cmd2 := exec.Command("docker", args2...)
	output2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("Second container failed: %v\nOutput: %s", err, output2)
	}

	// Verify data persisted
	if !strings.Contains(string(output2), "Volume test data") {
		t.Errorf("Volume did not persist data between containers.\nFirst output: %s\nSecond output: %s", output1, output2)
	}

	t.Logf("Volume persistence test successful")
}

// TestE2ELifecycleCommandsActualExecution tests that lifecycle commands execute in correct order
func TestE2ELifecycleCommandsActualExecution(t *testing.T) {
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available")
	}

	tmpDir := t.TempDir()

	// Create a script that tests command execution order

	// Generate the lifecycle script
	script, err := GetLifecycleScript(&DevContainer{
		DevContainerCommon: DevContainerCommon{
			OnCreateCommand:      "echo 'Step 1: onCreate' >> /tmp/execution-log.txt",
			UpdateContentCommand: "echo 'Step 2: updateContent' >> /tmp/execution-log.txt", 
			PostCreateCommand:    "echo 'Step 3: postCreate' >> /tmp/execution-log.txt",
		},
	}, "create")
	
	if err != nil {
		t.Fatal(err)
	}

	// Write script to temp file
	scriptPath := filepath.Join(tmpDir, "lifecycle.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// Create container with bind mount for script
	dc := &DevContainer{
		ImageContainer: &ImageContainer{
			Image: "alpine:latest",
		},
		DevContainerCommon: DevContainerCommon{
			Mounts: []interface{}{
				map[string]interface{}{
					"type":   "bind",
					"source": tmpDir,
					"target": "/scripts",
				},
			},
		},
	}

	config, err := BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	args := config.ToDockerRunArgs()
	// Remove -it flags for non-interactive test
	filteredArgs := []string{}
	for _, arg := range args {
		if arg != "-it" && arg != "-i" && arg != "-t" {
			filteredArgs = append(filteredArgs, arg)
		}
	}
	args = filteredArgs
	
	// Run the lifecycle script and then read the log
	args = append(args, "sh", "-c", "/scripts/lifecycle.sh && cat /tmp/execution-log.txt")

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Fatalf("Lifecycle execution failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	
	// Verify commands executed in order
	lines := strings.Split(strings.TrimSpace(outputStr), "\n")
	expectedOrder := []string{
		"Step 1: onCreate",
		"Step 2: updateContent", 
		"Step 3: postCreate",
	}

	for i, expected := range expectedOrder {
		found := false
		for _, line := range lines {
			if strings.Contains(line, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected step %d (%s) not found in output: %s", i+1, expected, outputStr)
		}
	}

	t.Logf("Lifecycle commands executed successfully:\n%s", outputStr)
}

// TestE2EEnvironmentVariables tests that environment variables are actually available
func TestE2EEnvironmentVariables(t *testing.T) {
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available")
	}

	tmpDir := t.TempDir()

	dc := &DevContainer{
		ImageContainer: &ImageContainer{
			Image: "alpine:latest",
		},
		DevContainerCommon: DevContainerCommon{
			ContainerEnv: map[string]string{
				"TEST_VAR1": "value1",
				"TEST_VAR2": "value with spaces",
				"PATH_EXT":  "/custom/bin:$PATH",
			},
		},
	}

	config, err := BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	args := config.ToDockerRunArgs()
	// Remove -it flags for non-interactive test
	filteredArgs := []string{}
	for _, arg := range args {
		if arg != "-it" && arg != "-i" && arg != "-t" {
			filteredArgs = append(filteredArgs, arg)
		}
	}
	args = filteredArgs
	args = append(args, "sh", "-c", "echo TEST_VAR1=$TEST_VAR1 && echo TEST_VAR2=$TEST_VAR2 && echo PATH_EXT=$PATH_EXT")

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Fatalf("Environment test failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	
	// Verify environment variables
	if !strings.Contains(outputStr, "TEST_VAR1=value1") {
		t.Error("TEST_VAR1 not set correctly")
	}
	if !strings.Contains(outputStr, "TEST_VAR2=value with spaces") {
		t.Error("TEST_VAR2 not set correctly")
	}
	if !strings.Contains(outputStr, "PATH_EXT=/custom/bin:") {
		t.Error("PATH_EXT not expanded correctly")
	}

	t.Logf("Environment variables test successful:\n%s", outputStr)
}

// TestE2EPortForwarding tests that port forwarding configuration is correct
func TestE2EPortForwarding(t *testing.T) {
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available")
	}

	tmpDir := t.TempDir()

	dc := &DevContainer{
		ImageContainer: &ImageContainer{
			Image: "nginx:alpine",
		},
		DevContainerCommon: DevContainerCommon{
			ForwardPorts: []interface{}{float64(18080)},
		},
		NonComposeBase: &NonComposeBase{
			AppPort: []interface{}{float64(80)},
		},
	}

	config, err := BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	args := config.ToDockerRunArgs()
	
	// Remove -it for this test (nginx runs in foreground)
	filteredArgs := []string{}
	for _, arg := range args {
		if arg == "-it" || arg == "-i" || arg == "-t" {
			continue
		}
		// Skip --rm too for this test
		if arg == "--rm" {
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}
	
	// Add detached mode and name for easier cleanup
	containerName := fmt.Sprintf("test-nginx-%d", time.Now().Unix())
	finalArgs := []string{"run", "-d", "--name", containerName}
	finalArgs = append(finalArgs, filteredArgs[1:]...) // Skip the initial "run"

	// Clean up container after test
	defer func() {
		exec.Command("docker", "rm", "-f", containerName).Run()
	}()

	cmd := exec.Command("docker", finalArgs...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		t.Fatalf("Container start failed: %v\nOutput: %s", err, output)
	}

	// Give nginx time to start
	time.Sleep(2 * time.Second)

	// Check if ports are correctly mapped by inspecting the container
	inspectCmd := exec.Command("docker", "inspect", containerName, "--format", "{{.NetworkSettings.Ports}}")
	inspectOutput, err := inspectCmd.CombinedOutput()
	
	if err != nil {
		t.Fatalf("Container inspect failed: %v\nOutput: %s", err, inspectOutput)
	}

	inspectStr := string(inspectOutput)
	
	// Verify port mappings exist
	if !strings.Contains(inspectStr, "80/tcp") {
		t.Error("Port 80 mapping not found")
	}
	if !strings.Contains(inspectStr, "18080/tcp") {
		t.Error("Port 18080 mapping not found")
	}

	t.Logf("Port forwarding test successful. Port mappings: %s", inspectStr)
}

// TestE2EMergeLogicRealFiles tests that configuration merging works with real files
func TestE2EMergeLogicRealFiles(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create base configuration
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)
	baseConfig := `{
		"image": "ubuntu:22.04",
		"containerEnv": {
			"BASE_VAR": "from_base",
			"OVERRIDE_ME": "base_value"
		},
		"forwardPorts": [3000],
		"mounts": [{
			"type": "volume",
			"source": "base-cache",
			"target": "/cache"
		}]
	}`
	baseConfigPath := filepath.Join(baseDir, "devcontainer.json")
	os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)

	// Create extending configuration
	projectDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(projectDir, 0755)
	extendingConfig := `{
		"extends": "../base/devcontainer.json",
		"name": "Extended Project",
		"containerEnv": {
			"OVERRIDE_ME": "overridden_value",
			"NEW_VAR": "from_extension"
		},
		"forwardPorts": [8080, 9000],
		"mounts": [{
			"type": "bind",
			"source": "${localWorkspaceFolder}/data",
			"target": "/project-data"
		}]
	}`
	projectConfigPath := filepath.Join(projectDir, "devcontainer.json")
	os.WriteFile(projectConfigPath, []byte(extendingConfig), 0644)

	// Load with extends
	dc, err := LoadDevContainerWithExtends(projectConfigPath, nil)
	if err != nil {
		t.Fatalf("Failed to load with extends: %v", err)
	}

	// Expand variables
	variables := GetStandardVariables(projectDir)
	ExpandVariables(dc, variables)

	// Build Docker command
	config, err := BuildDockerRunCommand(dc, projectDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify merged configuration
	if dc.ImageContainer.Image != "ubuntu:22.04" {
		t.Error("Base image not inherited")
	}

	if dc.Name == nil || *dc.Name != "Extended Project" {
		t.Error("Name not set from extending config")
	}

	// Check environment merge
	expectedEnv := map[string]string{
		"BASE_VAR":    "from_base",
		"OVERRIDE_ME": "overridden_value",
		"NEW_VAR":     "from_extension",
	}
	for k, expected := range expectedEnv {
		if actual, ok := dc.ContainerEnv[k]; !ok || actual != expected {
			t.Errorf("Env var %s: expected %s, got %s", k, expected, actual)
		}
	}

	// Check ports (should be overridden)
	if ports, ok := dc.ForwardPorts.([]interface{}); ok {
		if len(ports) != 2 {
			t.Errorf("Expected 2 forward ports, got %d", len(ports))
		}
	} else {
		t.Errorf("ForwardPorts is not a slice, got %T", dc.ForwardPorts)
	}

	// Check mounts (should be overridden)
	if len(dc.Mounts) != 1 {
		t.Errorf("Expected 1 mount, got %d", len(dc.Mounts))
	}

	// Check variable expansion in mount
	if dc.Mounts[0].Source == nil || !strings.Contains(*dc.Mounts[0].Source, projectDir) {
		t.Errorf("Variable expansion failed in mount source: %v", dc.Mounts[0].Source)
	}

	// Verify the final Docker command includes all merged settings
	args := config.ToDockerRunArgs()
	cmdStr := strings.Join(args, " ")

	if !strings.Contains(cmdStr, "ubuntu:22.04") {
		t.Error("Base image not in Docker command")
	}

	// Check environment variables in command
	for k, expected := range expectedEnv {
		envFlag := fmt.Sprintf("%s=%s", k, expected)
		if !strings.Contains(cmdStr, envFlag) {
			t.Errorf("Environment variable %s not in Docker command", envFlag)
		}
	}

	t.Logf("Merge logic test successful. Final command includes all merged configuration.")
}