//go:build integration
// +build integration

package shai_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	shai "github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

const testImage = "debian-dev:dev"
const supervisorEnvVar = "SHAI_ALIAS_SUPERVISOR_BIN"

func TestMain(m *testing.M) {
	bin := os.Getenv(supervisorEnvVar)
	if bin == "" {
		fmt.Fprintf(os.Stderr, "%s must point to built shai binary when running integration tests\n", supervisorEnvVar)
		os.Exit(1)
	}
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		fmt.Fprintf(os.Stderr, "%s invalid (%v)\n", bin, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestAliasIntegrationListShowsAliases(t *testing.T) {
	workspace := setupAliasWorkspace(t)
	lines := runInDevcontainer(t, workspace, "shai-alias --list")
	assertContainsLine(t, lines, "hosthello")
	assertContainsLine(t, lines, "withargs")
}

func TestAliasIntegrationRunsHostCommand(t *testing.T) {
	workspace := setupAliasWorkspace(t)
	lines := runInDevcontainer(t, workspace, "shai-alias hosthello first second")
	assertContainsSubstring(t, lines, "HOST_HELLO:first second")
}

func TestAliasIntegrationArgValidation(t *testing.T) {
	workspace := setupAliasWorkspace(t)
	lines := runInDevcontainer(t, workspace, "shai-alias withargs --msg=test")
	assertContainsSubstring(t, lines, "HOST_ARGS:--msg=test")

	lines = runInDevcontainer(t, workspace, "if shai-alias withargs --msg=Bad; then echo unexpected; else echo denied; fi")
	assertContainsSubstring(t, lines, "denied")
}

func TestAliasIntegrationMasksManifest(t *testing.T) {
	workspace := setupAliasWorkspace(t)
	lines := runInDevcontainer(t, workspace, "if [ -s /src/.shai-cmds ]; then echo visible; else echo masked; fi")
	assertContainsSubstring(t, lines, "masked")

	content, err := os.ReadFile(filepath.Join(workspace, ".shai-cmds"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(content), "hosthello") {
		t.Fatalf("host manifest unexpectedly altered: %s", content)
	}
}

// --- helpers ---

func setupAliasWorkspace(t *testing.T) string {
	t.Helper()
	requireDocker(t)

	workspace := t.TempDir()
	writeDevcontainerConfig(t, workspace)

	scriptsDir := filepath.Join(workspace, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}

	writeExecutable(t, filepath.Join(scriptsDir, "host-hello.sh"), "#!/bin/sh\necho \"HOST_HELLO:$*\"\n")
	writeExecutable(t, filepath.Join(scriptsDir, "host-args.sh"), "#!/bin/sh\necho \"HOST_ARGS:$*\"\n")

	manifest := strings.TrimSpace(`
hosthello  -                   ./scripts/host-hello.sh
withargs   ^(--msg=[a-z]+)$    ./scripts/host-args.sh
`)
	if err := os.WriteFile(filepath.Join(workspace, ".shai-cmds"), []byte(manifest+"\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return workspace
}

func runInDevcontainer(t *testing.T, workspace, shellCmd string) []string {
	t.Helper()
	lines := newLineCollector()
	cfg := shai.EphemeralConfig{
		WorkingDir:          workspace,
		ReadWritePaths:      []string{"."},
		HideProgressMarkers: true,
		PostSetupExec: &shai.ExecSpec{
			Command: []string{"/bin/bash", "-lc", shellCmd},
			Workdir: "/src",
			UseTTY:  false,
		},
		Output: shai.LineSink(lines.Collect),
	}
	runner, err := shai.NewEphemeralRunner(cfg)
	if err != nil {
		t.Fatalf("NewEphemeralRunner: %v", err)
	}
	defer runner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("runner run (%s): %v", shellCmd, err)
	}
	return lines.Events()
}

func requireDocker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "info")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker info failed: %v\n%s", err, out)
	}

	cmd = exec.CommandContext(ctx, "docker", "image", "inspect", testImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker image %s not available (build via `docker build -t %s docker`): %v\n%s", testImage, testImage, err, out)
	}
}

func writeDevcontainerConfig(t *testing.T, workspace string) {
	t.Helper()
	dstDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir devcontainer dir: %v", err)
	}
	config := fmt.Sprintf(`{
  "name": "alias-integration",
  "image": "%s",
  "workspaceFolder": "/src"
}
`, testImage)
	if err := os.WriteFile(filepath.Join(dstDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatalf("write devcontainer config: %v", err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type lineCollector struct {
	lines []string
}

func newLineCollector() *lineCollector {
	return &lineCollector{lines: make([]string, 0, 32)}
}

func (l *lineCollector) Collect(stream, line string) {
	l.lines = append(l.lines, stream+":"+line)
}

func (l *lineCollector) Events() []string {
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}

func assertContainsLine(t *testing.T, lines []string, needle string) {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return
		}
	}
	t.Fatalf("expected output to contain %q, got %v", needle, lines)
}

func assertContainsSubstring(t *testing.T, lines []string, needle string) {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return
		}
	}
	t.Fatalf("expected substring %q in output %v", needle, lines)
}
