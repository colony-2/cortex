package shai

import (
	"strings"
	"testing"

	"github.com/divisive-ai/vibethis/server/container/internal/devcontainer"
)

func TestGenerateSetupScript_UsesPostExecBlock(t *testing.T) {
	runner := &EphemeralRunner{
		config: EphemeralConfig{
			PostSetupExec: &ExecSpec{
				Command: []string{"shai-alias", "--list"},
				Env: map[string]string{
					"MY_ENV": "1",
				},
			},
		},
		devContainer: &devcontainer.DevContainer{
			DevContainerCommon: devcontainer.DevContainerCommon{
				ContainerEnv: map[string]string{
					"BASE_ENV": "ok",
				},
			},
		},
		aliasEnv: map[string]string{
			"SHAI_ALIAS_SSH_HOSTPORT": "host:1234",
		},
	}

	script := runner.generateSetupScript()

	if !strings.Contains(script, "'shai-alias'") {
		t.Fatalf("post exec block not present: %s", script)
	}
	if !strings.Contains(script, "export SHAI_ALIAS_SSH_HOSTPORT") {
		t.Fatalf("alias env missing from post exec block: %s", script)
	}
	if !strings.Contains(script, "exec sudo -iu root /bin/bash -lc") {
		t.Fatalf("expected sudo exec in script: %s", script)
	}
	if strings.Contains(script, "%USERSWITCH_BLOCK%") {
		t.Fatalf("placeholder not replaced:\n%s", script)
	}
}

func TestGenerateSetupScript_InteractiveEnvInjection(t *testing.T) {
	runner := &EphemeralRunner{
		devContainer: &devcontainer.DevContainer{
			DevContainerCommon: devcontainer.DevContainerCommon{
				ContainerEnv: map[string]string{
					"SHAI_ALIAS_SSH_USER": "shai",
				},
			},
		},
		aliasEnv: map[string]string{
			"SHAI_ALIAS_SSH_HOSTPORT": "host.docker.internal:12345",
		},
	}

	script := runner.generateSetupScript()

	if !strings.Contains(script, "SHAI_ALIAS_SSH_HOSTPORT") {
		t.Fatalf("alias env not injected into interactive block:\n%s", script)
	}
	if strings.Contains(script, "%USERSWITCH_BLOCK%") {
		t.Fatalf("placeholder not replaced:\n%s", script)
	}
}
