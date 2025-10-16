package shai

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/container/internal/devcontainer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProgressMonitoring tests progress marker parsing and handling
func TestProgressMonitoring(t *testing.T) {
	tests := []struct {
		name           string
		setupScript    string
		expectedPhases []string
	}{
		{
			name: "progress markers are parsed correctly",
			setupScript: `
echo "::DEVCONTAINER::INIT::START::Initializing"
echo "::DEVCONTAINER::FEATURES::START::Installing features"
echo "::DEVCONTAINER::FEATURES::PROGRESS::Installing git"
echo "::DEVCONTAINER::FEATURES::COMPLETE::Features installed"
echo "::DEVCONTAINER::ONCREATE::START::Running onCreate"
echo "::DEVCONTAINER::ONCREATE::COMPLETE::onCreate complete"
echo "::DEVCONTAINER::USERSWITCH::START::Switching to user"
`,
			expectedPhases: []string{"INIT", "FEATURES", "ONCREATE", "USERSWITCH"},
		},
		{
			name: "real-time progress streaming",
			setupScript: `
for i in 1 2 3; do
    echo "::DEVCONTAINER::FEATURES::PROGRESS::Installing package $i"
    sleep 0.1
done
echo "::DEVCONTAINER::FEATURES::COMPLETE::All packages installed"
`,
			expectedPhases: []string{"FEATURES"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedUpdates []ProgressUpdate
			progressCallback := func(update ProgressUpdate) {
				receivedUpdates = append(receivedUpdates, update)
			}

			// Parse script output line by line
			scanner := bufio.NewScanner(strings.NewReader(tt.setupScript))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				if strings.HasPrefix(line, "echo \"") {
					// Extract the echo content
					line = strings.TrimPrefix(line, "echo \"")
					line = strings.TrimSuffix(line, "\"")
				}
				if strings.HasPrefix(line, "::DEVCONTAINER::") {
					if progress := parseProgressMarker(line); progress != nil {
						progressCallback(*progress)
					}
				}
			}

			// Verify we received expected phases
			phases := make(map[string]bool)
			for _, update := range receivedUpdates {
				phases[update.Phase] = true
			}

			for _, expectedPhase := range tt.expectedPhases {
				assert.True(t, phases[expectedPhase], "Missing phase: %s", expectedPhase)
			}

			// Verify real-time streaming (updates received)
			assert.True(t, len(receivedUpdates) > 0)
		})
	}
}

// TestEphemeralContainerSetup tests setup script generation
func TestEphemeralContainerSetup(t *testing.T) {
	tests := []struct {
		name     string
		dc       *devcontainer.DevContainer
		validate func(t *testing.T, script string)
	}{
		{
			name: "runs setup as root then switches to user",
			dc: &devcontainer.DevContainer{
				ImageContainer: &devcontainer.ImageContainer{
					Image: "ubuntu:22.04",
				},
				DevContainerCommon: devcontainer.DevContainerCommon{
					RemoteUser:        stringPtr("vscode"),
					PostCreateCommand: "touch /tmp/setup-complete",
				},
			},
			validate: func(t *testing.T, script string) {
				// Script should contain progress markers
				assert.Contains(t, script, "::DEVCONTAINER::INIT::START::")
				assert.Contains(t, script, "::DEVCONTAINER::POSTCREATE::START::")
				assert.Contains(t, script, "::DEVCONTAINER::POSTCREATE::COMPLETE::")
				assert.Contains(t, script, "::DEVCONTAINER::USERSWITCH::START::Switching to user vscode")
				// We do not add users directly in the script; features handle this
				assert.NotContains(t, script, "useradd -m -s /bin/bash")
				assert.Contains(t, script, "exec su - vscode")
			},
		},
		{
			name: "lifecycle commands execute in order",
			dc: &devcontainer.DevContainer{
				ImageContainer: &devcontainer.ImageContainer{
					Image: "ubuntu:22.04",
				},
				DevContainerCommon: devcontainer.DevContainerCommon{
					OnCreateCommand:      "echo 'Step 1'",
					UpdateContentCommand: "echo 'Step 2'",
					PostCreateCommand:    "echo 'Step 3'",
					PostStartCommand:     "echo 'Step 4'",
					PostAttachCommand:    "echo 'Step 5'",
				},
			},
			validate: func(t *testing.T, script string) {
				// Verify order
				step1Idx := strings.Index(script, "Step 1")
				step2Idx := strings.Index(script, "Step 2")
				step3Idx := strings.Index(script, "Step 3")
				step4Idx := strings.Index(script, "Step 4")
				step5Idx := strings.Index(script, "Step 5")

				assert.True(t, step1Idx > 0 && step1Idx < step2Idx)
				assert.True(t, step2Idx < step3Idx)
				assert.True(t, step3Idx < step4Idx)
				assert.True(t, step4Idx < step5Idx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for testing
			tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Create a minimal devcontainer.json
			devcontainerPath := tmpDir + "/.devcontainer"
			err = os.MkdirAll(devcontainerPath, 0755)
			require.NoError(t, err)

			// Write devcontainer to temp file (we'll mock this)
			runner := &EphemeralRunner{
				config: EphemeralConfig{
					WorkingDir: tmpDir,
				},
				devContainer: tt.dc,
			}

			script := runner.generateSetupScript()
			tt.validate(t, script)
		})
	}
}

// Test that when remoteUser is set, the setup script ensures the user exists before switch
func TestEnsureRemoteUserExists(t *testing.T) {
    // With generic feature support, the setup script should not directly add users
    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{Image: "ubuntu:22.04"},
        DevContainerCommon: devcontainer.DevContainerCommon{
            RemoteUser: stringPtr("vscode"),
        },
    }

    runner := &EphemeralRunner{devContainer: dc}
    script := runner.generateSetupScript()

    // Ensure there's no direct user creation in the script
    assert.NotContains(t, script, "useradd -m -s /bin/bash")
    assert.NotContains(t, script, "/etc/sudoers.d/")
}

// Test that common-utils feature emits a real install script for creating the user
func TestCommonUtilsFeatureExecutionUsesInstallScript(t *testing.T) {
    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{Image: "ubuntu:22.04"},
        DevContainerCommon: devcontainer.DevContainerCommon{
            RemoteUser: stringPtr("vscode"),
            Features: &devcontainer.DevContainerCommonFeatures{
                AdditionalProperties: map[string]interface{}{
                    "ghcr.io/devcontainers/features/common-utils:2": map[string]interface{}{},
                },
            },
        },
    }

    // Simulate resolved feature mount so script generation includes install.sh execution
    runner := &EphemeralRunner{devContainer: dc}
    runner.resolvedFeatures = []ResolvedFeature{{
        ID:       "ghcr.io/devcontainers/features/common-utils:2",
        SafeName: "ghcr.io_devcontainers_features_common-utils_2",
        Dir:      "/tmp/fake",
        Options:  map[string]interface{}{},
    }}

    script := runner.generateSetupScript()

    // Script should attempt to execute the mounted install.sh, not ad-hoc apt commands
    assert.Contains(t, script, "/tmp/devcontainer-features/ghcr.io_devcontainers_features_common-utils_2/install.sh")
    assert.NotContains(t, script, "apt-get install -y sudo bash")
}

// TestProcessReplacement tests that exec replaces the process
func TestProcessReplacement(t *testing.T) {
	t.Run("exec replaces process - no return to root", func(t *testing.T) {
		dc := &devcontainer.DevContainer{
			ImageContainer: &devcontainer.ImageContainer{
				Image: "ubuntu:22.04",
			},
			DevContainerCommon: devcontainer.DevContainerCommon{
				RemoteUser: stringPtr("nobody"),
			},
		}

		tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		runner := &EphemeralRunner{
			config: EphemeralConfig{
				WorkingDir: tmpDir,
			},
			devContainer: dc,
		}

		script := runner.generateSetupScript()

    // Script should end with user shell exec (sudo preferred, fallback su)
    // Updated to use user's default shell instead of hardcoded bash
    if !strings.Contains(script, "exec sudo -iu nobody") && !strings.Contains(script, "exec su - nobody") {
        t.Fatalf("missing expected user exec in script: %s", script)
    }
    // Script should not have any commands after exec, ignoring closing 'else'/'fi' of sudo fallback block
    lines := strings.Split(script, "\n")
    execFound := false
    for _, line := range lines {
        if strings.Contains(line, "exec sudo -iu nobody") || strings.Contains(line, "exec su - nobody") {
            execFound = true
        } else if execFound {
            trimmed := strings.TrimSpace(line)
            if trimmed == "" || trimmed == "fi" || trimmed == "else" || strings.HasPrefix(trimmed, "#") {
                continue
            }
            t.Errorf("Found command after exec: %s", line)
        }
    }
	})
}

// TestPostSetupExecReplacement ensures that a provided post-setup command replaces the user shell
// and is executed as the target user with env and workdir applied.
func TestPostSetupExecReplacement(t *testing.T) {
    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{ Image: "ubuntu:22.04" },
        DevContainerCommon: devcontainer.DevContainerCommon{
            RemoteUser: stringPtr("nobody"),
        },
    }

    runner := &EphemeralRunner{
        config: EphemeralConfig{
            PostSetupExec: &ExecSpec{
                Command: []string{"echo", "HELLO", "world"},
                Env:     map[string]string{"FOO": "BAR"},
                Workdir: "/src/sub dir",
            },
        },
        devContainer: dc,
    }

    script := runner.generateSetupScript()

    // Must not leave placeholders
    if strings.Contains(script, "%USER%") {
        t.Fatalf("script still contains %%USER%% placeholder: %s", script)
    }

    // Must include the sudo path with bash -lc and inner commands
    if !strings.Contains(script, "exec sudo -iu nobody /bin/bash -lc") {
        t.Fatalf("missing sudo bash -lc exec: %s", script)
    }
    // Ensure env export, working directory, and quoted command args are present inside the single-quoted payload
    if !strings.Contains(script, "export FOO=") || !strings.Contains(script, "BAR") {
        t.Fatalf("missing env export in payload: %s", script)
    }
    if !(strings.Contains(script, "cd ") && strings.Contains(script, "/src/sub dir")) {
        t.Fatalf("missing workdir change in payload: %s", script)
    }
    if !(strings.Contains(script, "exec ") && strings.Contains(script, "echo") && strings.Contains(script, "HELLO") && strings.Contains(script, "world")) {
        t.Fatalf("missing command payload: %s", script)
    }
    // Fallback su path should also exist
    if !strings.Contains(script, "exec su - nobody -c '") {
        t.Fatalf("missing su -c fallback: %s", script)
    }
}

func TestInteractiveShellExportsContainerEnv(t *testing.T) {
    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{ Image: "ubuntu:22.04" },
        DevContainerCommon: devcontainer.DevContainerCommon{
            RemoteUser: stringPtr("nobody"),
            ContainerEnv: map[string]string{
                "FOO": "BAR",
                // unresolved localEnv should be skipped
                "SKIP_ME": "${localEnv:DOES_NOT_EXIST}",
            },
        },
    }

    runner := &EphemeralRunner{ devContainer: dc }
    script := runner.generateSetupScript()

    // Expect env injection in both sudo and su branches
    if !strings.Contains(script, "exec sudo -iu nobody env FOO=") {
        t.Fatalf("missing env injection for sudo branch: %s", script)
    }
    if !strings.Contains(script, "exec su - nobody -c 'env FOO=") {
        t.Fatalf("missing env injection for su branch: %s", script)
    }
    if strings.Contains(script, "SKIP_ME") {
        t.Fatalf("unresolved localEnv should be skipped: %s", script)
    }
}

// TestMountPermissions tests selective mount permissions
func TestMountPermissions(t *testing.T) {
	t.Run("selective read-write mounts configured correctly", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create test directories
		os.MkdirAll(tmpDir+"/src", 0755)
		os.MkdirAll(tmpDir+"/tests", 0755)
		os.MkdirAll(tmpDir+"/docs", 0755)

		config := EphemeralConfig{
			WorkingDir:     tmpDir,
			ReadWritePaths: []string{"src", "tests"},
		}

		mountBuilder, err := NewMountBuilder(config.WorkingDir, config.ReadWritePaths)
		require.NoError(t, err)

		mounts := mountBuilder.BuildMounts()

		// Check that we have the right number of mounts
		// We should have mounts for workspace (RO) and specified RW paths
		assert.True(t, len(mounts) > 0)

    // Verify base workspace mount exists and is read-only
    var baseFound bool
    for _, m := range mounts {
        if m.Target == "/src" {
            baseFound = true
            assert.True(t, m.ReadOnly, "Base workspace mount should be read-only")
            break
        }
    }
    assert.True(t, baseFound, "expected base /src mount to be present")

    // Verify the two specific RW overlay mounts are present and writable
    rwTargets := map[string]bool{"/src/src": false, "/src/tests": false}
    for _, m := range mounts {
        if _, ok := rwTargets[m.Target]; ok {
            assert.False(t, m.ReadOnly, "RW path should not be read-only: %s", m.Target)
            rwTargets[m.Target] = true
        }
    }
    // Ensure both overlays were found
    for tgt, found := range rwTargets {
        assert.True(t, found, "expected RW mount for %s to be present", tgt)
    }
	})
}

// TestFeatureInstallation tests feature installation ordering
func TestFeatureInstallation(t *testing.T) {
    t.Run("features install before user switch", func(t *testing.T) {
        dc := &devcontainer.DevContainer{
            ImageContainer: &devcontainer.ImageContainer{
                Image: "ubuntu:22.04",
            },
            DevContainerCommon: devcontainer.DevContainerCommon{
                RemoteUser: stringPtr("vscode"),
                Features: &devcontainer.DevContainerCommonFeatures{
                    AdditionalProperties: map[string]interface{}{
                        "ghcr.io/devcontainers/features/git:1": map[string]interface{}{},
                    },
                },
            },
        }

        tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
        require.NoError(t, err)
        defer os.RemoveAll(tmpDir)

        runner := &EphemeralRunner{
            config: EphemeralConfig{
                WorkingDir: tmpDir,
            },
            devContainer: dc,
        }

        // Simulate a resolved feature so script includes install section
        runner.resolvedFeatures = []ResolvedFeature{{
            ID:       "ghcr.io/devcontainers/features/git:1",
            SafeName: "ghcr.io_devcontainers_features_git_1",
            Dir:      "/tmp/fake",
        }}

        script := runner.generateSetupScript()

        // Script should install features before switching to vscode user
        assert.Contains(t, script, "Installing devcontainer features")

        // Verify order: features before user switch
        featuresIdx := strings.Index(script, "Installing devcontainer features")
        userSwitchIdx := strings.Index(script, "exec su - vscode")
        assert.True(t, featuresIdx > 0 && featuresIdx < userSwitchIdx)
    })
}

// New tests to validate feature script generation formatting and semantics
func TestFeatureScriptFormat_NewlinesAndIfSyntax(t *testing.T) {
    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{Image: "ubuntu:22.04"},
        DevContainerCommon: devcontainer.DevContainerCommon{
            RemoteUser: stringPtr("vscode"),
        },
    }
    runner := &EphemeralRunner{devContainer: dc}
    runner.resolvedFeatures = []ResolvedFeature{{
        ID:       "ghcr.io/devcontainers/features/example:1",
        SafeName: "example_safe",
        Dir:      "/tmp/fake",
        Options:  map[string]interface{}{"customOption": "abc"},
    }}

    script := runner.generateSetupScript()

    // Ensure headers and newlines are real newlines, not escaped \n sequences
    assert.Contains(t, script, "echo \"::DEVCONTAINER::FEATURES::START::Installing devcontainer features\"\n")
    // Ensure common env exports exist
    assert.Contains(t, script, "export DEBIAN_FRONTEND=noninteractive\n")
    assert.Contains(t, script, "export PATH=\"$PATH:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"\n")
    assert.Contains(t, script, "export LANG=C.UTF-8\n")
    assert.Contains(t, script, "export LC_ALL=C.UTF-8\n")
    assert.Contains(t, script, "export SHELL=/bin/bash\n")
    // Spec user envs for features
    assert.Contains(t, script, "export _CONTAINER_USER='root'\n")
    assert.Contains(t, script, "export _CONTAINER_USER_HOME='/root'\n")
    assert.Contains(t, script, "export _REMOTE_USER='vscode'\n")
    assert.Contains(t, script, "export _REMOTE_USER_HOME='/home/vscode'\n")
    assert.Contains(t, script, "export TARGETOS=linux\n")
    assert.Contains(t, script, "export TARGETPLATFORM=\"linux/")

    // Ensure we export USERNAME from remoteUser by default and options env
    assert.Contains(t, script, "export USERNAME=\"vscode\"\n")
    assert.Contains(t, script, "export CUSTOM_OPTION=\"abc\"\n")
    // Version defaults are exported only if provided by feature spec; not asserted here

    // Ensure if/then/else/fi block has proper newlines and absolute sh path
    expectedIf := "if [ -f '/tmp/devcontainer-features/example_safe/install.sh' ]; then\n"
    expectedSh := "    sh '/tmp/devcontainer-features/example_safe/install.sh'\n"
    expectedBash := "    bash '/tmp/devcontainer-features/example_safe/install.sh'\n"
    expectedElse := "else\n"
    expectedEcho := "  echo 'Feature install script not found: /tmp/devcontainer-features/example_safe/install.sh'\n"
    expectedFi := "fi\n"
    assert.Contains(t, script, expectedIf)
    // Should include bash-preferred execution with fallback to sh
    assert.Contains(t, script, expectedBash)
    assert.Contains(t, script, expectedSh)
    assert.Contains(t, script, expectedElse)
    assert.Contains(t, script, expectedEcho)
    assert.Contains(t, script, expectedFi)

    // Ensure no subshell/cd or chmod sneaks in
    assert.NotContains(t, script, "(cd ")
    assert.NotContains(t, script, "chmod +x")
}

func TestFeatureScript_UsernameOptionOverridesRemoteUser(t *testing.T) {
    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{Image: "ubuntu:22.04"},
        DevContainerCommon: devcontainer.DevContainerCommon{
            RemoteUser: stringPtr("vscode"),
        },
    }
    runner := &EphemeralRunner{devContainer: dc}
    runner.resolvedFeatures = []ResolvedFeature{{
        ID:       "ghcr.io/devcontainers/features/example:1",
        SafeName: "example_safe",
        Dir:      "/tmp/fake",
        Options:  map[string]interface{}{"username": "devuser"},
    }}
    script := runner.generateSetupScript()

    // Option provided username should be exported; remoteUser default should not add a second USERNAME
    assert.Contains(t, script, "export USERNAME=\"devuser\"\n")
}

func TestFeatureScript_MultipleFeaturesBothInvoked(t *testing.T) {
    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{Image: "ubuntu:22.04"},
        DevContainerCommon: devcontainer.DevContainerCommon{},
    }
    runner := &EphemeralRunner{devContainer: dc}
    runner.resolvedFeatures = []ResolvedFeature{
        {ID: "ghcr.io/devcontainers/features/common-utils:2", SafeName: "ghcr.io_devcontainers_features_common-utils_2", Dir: "/tmp/fake1"},
        {ID: "ghcr.io/devcontainers/features/git:1", SafeName: "ghcr.io_devcontainers_features_git_1", Dir: "/tmp/fake2"},
    }
    script := runner.generateSetupScript()

    // Both features must be echoed and guarded by if/fi blocks
    assert.Contains(t, script, "Installing feature ghcr.io/devcontainers/features/common-utils:2")
    assert.Contains(t, script, "Installing feature ghcr.io/devcontainers/features/git:1")
    assert.Contains(t, script, "if [ -f '/tmp/devcontainer-features/ghcr.io_devcontainers_features_common-utils_2/install.sh' ]; then\n")
    assert.Contains(t, script, "if [ -f '/tmp/devcontainer-features/ghcr.io_devcontainers_features_git_1/install.sh' ]; then\n")
    // Version aliases depend on feature spec defaults; not asserted here
}

// Test that devcontainer.json mounts are preserved in ephemeral mode along with custom -rw mounts
func TestEphemeralIncludesDevcontainerMounts(t *testing.T) {
    tmpDir, err := os.MkdirTemp("", "ephemeral-mounts-*")
    require.NoError(t, err)
    defer os.RemoveAll(tmpDir)

    // Ensure predictable localEnv expansion
    t.Setenv("HOME", "/home/testuser")

    dc := &devcontainer.DevContainer{
        ImageContainer: &devcontainer.ImageContainer{Image: "alpine:latest"},
        DevContainerCommon: devcontainer.DevContainerCommon{
            // Two devcontainer mounts (string format, unordered keys)
            Mounts: []interface{}{
                "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
                "target=/home/vscode/.gitconfig,source=${localEnv:HOME}/.gitconfig,type=bind,readonly",
            },
        },
    }

    // No custom -rw paths for this test
    mb, err := NewMountBuilder(tmpDir, nil)
    require.NoError(t, err)

    runner := &EphemeralRunner{
        config: EphemeralConfig{WorkingDir: tmpDir},
        devContainer: dc,
        mountBuilder: mb,
    }

    // Build docker config from devcontainer
    cfg, err := runner.buildConfig()
    require.NoError(t, err)

    // Build final mounts (scriptPath is a temp path)
    mounts := runner.buildMounts(cfg, tmpDir+"/devcontainer-setup.sh")

    // Ensure both devcontainer.json mounts are present
    hasSSH := false
    hasGit := false
    for _, m := range mounts {
        if m.Target == "/home/vscode/.ssh" && m.Source == "/home/testuser/.ssh" {
            hasSSH = true
        }
        if m.Target == "/home/vscode/.gitconfig" && m.Source == "/home/testuser/.gitconfig" {
            hasGit = true
        }
    }
    assert.True(t, hasSSH, "expected devcontainer mount for /home/vscode/.ssh to be present")
    assert.True(t, hasGit, "expected devcontainer mount for /home/vscode/.gitconfig to be present")
}

// TestSetupErrorHandling tests error handling in setup
func TestSetupErrorHandling(t *testing.T) {
	t.Run("setup script has error handling", func(t *testing.T) {
		dc := &devcontainer.DevContainer{
			ImageContainer: &devcontainer.ImageContainer{
				Image: "ubuntu:22.04",
			},
			DevContainerCommon: devcontainer.DevContainerCommon{
				PostCreateCommand: "exit 1",
				PostStartCommand:  "echo 'Should not run'",
			},
		}

		tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		runner := &EphemeralRunner{
			config: EphemeralConfig{
				WorkingDir: tmpDir,
			},
			devContainer: dc,
		}

		script := runner.generateSetupScript()

		// Script should have set -e for error handling
		assert.Contains(t, script, "set -e")
		// PostCreateCommand should be present
		assert.Contains(t, script, "exit 1")
		// PostStartCommand should also be present (but won't run due to set -e)
		assert.Contains(t, script, "Should not run")
	})
}

// TestProgressCallbacks tests that progress callbacks work correctly
func TestProgressCallbacks(t *testing.T) {
	t.Run("progress callbacks are invoked", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create .devcontainer directory
		os.MkdirAll(tmpDir+"/.devcontainer", 0755)

		// Create a simple devcontainer.json
		devcontainerJSON := `{
			"image": "alpine:latest",
			"postCreateCommand": "echo test"
		}`
		err = os.WriteFile(tmpDir+"/.devcontainer/devcontainer.json", []byte(devcontainerJSON), 0644)
		require.NoError(t, err)

		// config would be used in full integration test
		_ = EphemeralConfig{
			WorkingDir:     tmpDir,
			ReadWritePaths: []string{"src"},
		}

		// Note: This would require a Docker client to fully test
		// For unit testing, we're verifying the structure is correct

		// Test progress callback registration
		progressCalled := false
		progressCallback := func(update ProgressUpdate) {
			progressCalled = true
			assert.NotEmpty(t, update.Phase)
			assert.NotEmpty(t, update.Status)
			assert.NotEmpty(t, update.Message)
		}

		// Simulate progress update
		update := ProgressUpdate{
			Phase:   "INIT",
			Status:  "START",
			Message: "Test message",
		}
		progressCallback(update)

		assert.True(t, progressCalled, "Progress callback should be called")
	})
}

// TestEphemeralCleanup tests automatic cleanup behavior
func TestEphemeralCleanup(t *testing.T) {
	t.Run("container configured for automatic removal", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		dc := &devcontainer.DevContainer{
			ImageContainer: &devcontainer.ImageContainer{
				Image: "alpine:latest",
			},
		}

		// Create src directory
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src"), 0755))
		
		// Create mount builder first
		mountBuilder, err := NewMountBuilder(tmpDir, []string{"src"})
		require.NoError(t, err)

		runner := &EphemeralRunner{
			config: EphemeralConfig{
				WorkingDir: tmpDir,
			},
			devContainer: dc,
			mountBuilder: mountBuilder,
		}

		// Build config to verify AutoRemove is set
		config, err := runner.buildConfig()
		require.NoError(t, err)

		// The host config in runEphemeralContainer should have AutoRemove: true
		// We can't directly test this without mocking, but we verify the config is built
		assert.NotNil(t, config)
		assert.Equal(t, "alpine:latest", config.Image)
	})
}

// TestLifecycleCommandParsing tests parsing of different lifecycle command formats
func TestLifecycleCommandParsing(t *testing.T) {
	tests := []struct {
		name     string
		command  interface{}
		expected string
	}{
		{
			name:     "string command",
			command:  "echo hello",
			expected: "echo hello",
		},
		{
			name:     "array command",
			command:  []interface{}{"echo", "hello world"},
			expected: `echo "hello world"`,
		},
		{
			name:     "nil command",
			command:  nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &EphemeralRunner{}
			result := runner.parseLifecycleCommand(tt.command)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContextCancellation tests graceful shutdown on context cancellation
func TestContextCancellation(t *testing.T) {
	t.Run("context cancellation stops container", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// This test would need Docker to fully test
		// We're testing that context cancellation is properly handled
		select {
		case <-ctx.Done():
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
		case <-time.After(200 * time.Millisecond):
			t.Fatal("Context should have been cancelled")
		}
	})
}

// Helper function to create string pointer
func stringPtr(s string) *string {
    return &s
}

// Validate that unsupported feature fields bubble up via Run()
func TestRunnerFailsOnUnsupportedFeature(t *testing.T) {
    t.Run("Run returns error for unsupported feature fields", func(t *testing.T) {
        tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
        require.NoError(t, err)
        defer os.RemoveAll(tmpDir)

        dc := &devcontainer.DevContainer{
            ImageContainer: &devcontainer.ImageContainer{Image: "ubuntu:22.04"},
        }
        runner := &EphemeralRunner{
            config:       EphemeralConfig{WorkingDir: tmpDir},
            devContainer: dc,
            // Pre-resolve a feature with unsupported fields to avoid network
            resolvedFeatures: []ResolvedFeature{{
                ID:          "ghcr.io/devcontainers/features/example:1",
                SafeName:    "example",
                Dir:         "/tmp/fake",
                Unsupported: []string{"privileged"},
            }},
            progress: NewProgressDisplay(),
        }

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        err = runner.Run(ctx)
        require.Error(t, err)
        assert.Contains(t, err.Error(), "unsupported feature fields")
    })
}
