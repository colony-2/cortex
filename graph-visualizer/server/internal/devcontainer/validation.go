package devcontainer

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ValidateDockerCommand checks if a docker command is syntactically valid
// using Docker's dry-run feature
func ValidateDockerCommand(args []string) error {
	// Check if docker is available
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	// For docker run commands, we can use --dry-run if available
	// Otherwise, we'll do basic syntax validation
	if len(args) > 0 && args[0] == "run" {
		return validateDockerRunCommand(args)
	}

	return nil
}

// checkDockerAvailable verifies that docker CLI is installed and accessible
func checkDockerAvailable() error {
	cmd := exec.Command("docker", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}
	return nil
}

// validateDockerRunCommand performs validation specific to docker run commands
func validateDockerRunCommand(args []string) error {
	// Basic validation of required elements
	if len(args) < 2 {
		return fmt.Errorf("docker run requires at least an image argument")
	}

	// Find the image argument (should be the last non-flag argument)
	imageIndex := -1
	for i := len(args) - 1; i >= 1; i-- { // Start from end, skip "run"
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			// Make sure it's not a flag's argument
			if i > 1 && isArgFlag(args[i-1]) {
				continue
			}
			imageIndex = i
			break
		}
	}

	if imageIndex == -1 {
		return fmt.Errorf("no image specified in docker run command")
	}

	// Validate known flags and their arguments
	if imageIndex > 1 {
		if err := validateDockerRunFlags(args[1:imageIndex]); err != nil {
			return err
		}
	}

	return nil
}

// validateDockerRunFlags checks that flags have the required arguments
func validateDockerRunFlags(flags []string) error {
	flagsWithArgs := map[string]bool{
		"-p": true, "--publish": true,
		"-e": true, "--env": true,
		"-v": true, "--volume": true,
		"-w": true, "--workdir": true,
		"-u": true, "--user": true,
		"--mount": true,
		"--cap-add": true,
		"--security-opt": true,
		"--network": true,
		"--name": true,
		"--hostname": true,
		"--memory": true,
		"--cpus": true,
	}

	i := 0
	for i < len(flags) {
		flag := flags[i]
		
		// Check if this flag requires an argument
		requiresArg := false
		for knownFlag, needsArg := range flagsWithArgs {
			if flag == knownFlag {
				requiresArg = needsArg
				break
			}
		}

		if requiresArg {
			if i+1 >= len(flags) || strings.HasPrefix(flags[i+1], "-") {
				return fmt.Errorf("flag %s requires an argument", flag)
			}
			i += 2 // Skip the flag and its argument
		} else {
			i++
		}
	}

	return nil
}

// DryRunDockerCommand attempts to run the docker command with modifications
// to make it safe (like adding --dry-run or using 'echo' as entrypoint)
func DryRunDockerCommand(args []string) error {
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	if len(args) > 0 && args[0] == "run" {
		// Create a modified version of the command that exits immediately
		dryRunArgs := make([]string, 0, len(args)+4)
		
		// Find the image index properly
		imageIndex := -1
		for i := len(args) - 1; i >= 1; i-- {
			if !strings.HasPrefix(args[i], "-") {
				// Make sure it's not a flag's argument
				if i > 1 && isArgFlag(args[i-1]) {
					continue
				}
				imageIndex = i
				break
			}
		}

		if imageIndex < 0 {
			return fmt.Errorf("no image found in docker run command")
		}

		// Build the dry run command
		// Copy everything up to the image
		for i := 0; i < imageIndex; i++ {
			// Skip -it flags for dry run to avoid TTY issues
			if args[i] == "-it" || args[i] == "-i" || args[i] == "-t" {
				continue
			}
			dryRunArgs = append(dryRunArgs, args[i])
		}
		
		// Add our dry-run modifications
		dryRunArgs = append(dryRunArgs, "--entrypoint", "echo")
		
		// Add the image and a simple echo argument
		dryRunArgs = append(dryRunArgs, args[imageIndex], "dry-run")
		
		// Run the modified command
		cmd := exec.Command("docker", dryRunArgs...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker command validation failed: %w\nstderr: %s", err, stderr.String())
		}
	}

	return nil
}

// ExtractDockerImage gets the image name from docker run arguments
func ExtractDockerImage(args []string) (string, error) {
	if len(args) == 0 || args[0] != "run" {
		return "", fmt.Errorf("not a docker run command")
	}

	// Find the image (last non-flag argument)
	for i := len(args) - 1; i > 0; i-- {
		if !strings.HasPrefix(args[i], "-") {
			// Make sure it's not a flag's argument
			if i > 1 && isArgFlag(args[i-1]) {
				continue
			}
			return args[i], nil
		}
	}

	return "", fmt.Errorf("no image found in docker run command")
}

func isArgFlag(flag string) bool {
	argFlags := []string{
		"-p", "--publish", "-e", "--env", "-v", "--volume",
		"-w", "--workdir", "-u", "--user", "--mount",
		"--cap-add", "--security-opt", "--network",
		"--name", "--hostname", "--memory", "--cpus",
		"--entrypoint",
	}
	
	for _, f := range argFlags {
		if flag == f {
			return true
		}
	}
	return false
}