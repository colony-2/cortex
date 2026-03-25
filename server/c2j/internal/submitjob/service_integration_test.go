package submitjob

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/c2j/internal/runjob"
	"github.com/colony-2/swf-go/pkg/swf"
	remoteruntime "github.com/colony-2/swf-go/pkg/swf/runtime/remote"
	toyruntime "github.com/colony-2/swf-go/pkg/swf/runtime/toy"
	"net/http/httptest"
)

func TestRun_SubmitsJobThatCanBeRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tenantID := "tenant-submit-test"
	recipeYAML := strings.TrimSpace(`
id: nucleus_submit_recipe
desc: simple recipe used to verify c2j submission
version: "1.0"
sequence:
  - id: echo
    op: command_execution
    inputs:
      run: "echo hello-from-submit"
      working_directory: "."
outputs:
  result: "{{ sequence.echo.outputs.stdout }}"
`) + "\n"

	recipePath := filepath.Join(t.TempDir(), "nucleus_submit_recipe.yaml")
	if err := os.WriteFile(recipePath, []byte(recipeYAML), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}

	baseRepo, baseHash := createGitRepo(t)

	underlying := toyruntime.New()
	server := httptest.NewServer(remoteruntime.NewServer(underlying))
	defer server.Close()

	var submitStdout bytes.Buffer
	if err := Run(ctx, Options{
		TenantID:   tenantID,
		SWFURL:     server.URL,
		RecipeFile: recipePath,
		RepoPath:   baseRepo,
		GitRef:     baseHash,
		CellPath:   ".",
		CellName:   ".",
		JSONOutput: true,
		Stdout:     &submitStdout,
	}); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	var submitted struct {
		TenantID string `json:"tenant_id"`
		JobID    string `json:"job_id"`
		Recipe   string `json:"recipe"`
	}
	if err := json.Unmarshal(submitStdout.Bytes(), &submitted); err != nil {
		t.Fatalf("decode submit output: %v", err)
	}
	if submitted.JobID == "" {
		t.Fatalf("expected job id in submit output: %s", submitStdout.String())
	}

	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	if err := runjob.Run(ctx, runjob.Options{
		JobID:        submitted.JobID,
		TenantID:     tenantID,
		SWFURL:       server.URL,
		WaitTimeout:  5 * time.Second,
		PollInterval: 10 * time.Millisecond,
		InputMode:    "fail",
		Stdout:       &runStdout,
		Stderr:       &runStderr,
	}); err != nil {
		t.Fatalf("run submitted job: %v\nstderr:\n%s", err, runStderr.String())
	}

	runtime, err := remoteruntime.New(server.URL, server.Client())
	if err != nil {
		t.Fatalf("create remote runtime: %v", err)
	}
	engine, err := swf.NewEngineBuilder().WithRuntime(runtime).BuildEngine()
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}

	run, err := engine.GetJobRun(ctx, swf.GetJobRunRequest{
		JobKey:         swf.JobKey{TenantId: tenantID, JobId: submitted.JobID},
		IncludeOutputs: true,
	})
	if err != nil {
		t.Fatalf("get job run: %v", err)
	}
	output, err := run.GetOutput(engine, tenantID)
	if err != nil {
		t.Fatalf("get output: %v", err)
	}
	raw, err := output.GetData()
	if err != nil {
		t.Fatalf("get output data: %v", err)
	}

	got := map[string]any{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got["result"] != "hello-from-submit" {
		t.Fatalf("unexpected output: %#v", got)
	}
	if strings.Contains(runStderr.String(), "warning: replay unavailable") {
		t.Fatalf("unexpected replay warning:\n%s", runStderr.String())
	}
}

func createGitRepo(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	run("git", "init")
	run("git", "config", "user.email", "c2j-test@example.com")
	run("git", "config", "user.name", "Nucleus Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("c2j\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	return dir, run("git", "rev-parse", "HEAD")
}
