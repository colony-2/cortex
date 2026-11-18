package cortex

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/stretchr/testify/require"
)

func TestC2JUpgradePreviousWorkerHistory(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	oldWorker := filepath.Join(t.TempDir(), "c2j-previous")
	build := exec.Command("bash", filepath.Join(root, "scripts/build-previous-worker.sh"), oldWorker)
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	info, err := buildinfo.ReadFile(oldWorker)
	require.NoError(t, err)
	versions := map[string]string{}
	versions[info.Main.Path] = info.Main.Version
	for _, dependency := range info.Deps {
		versions[dependency.Path] = dependency.Version
	}
	require.Equal(t, "v0.0.129-0.20261118001543-b9d8dae96cb3", versions["github.com/colony-2/c2j"])
	require.Equal(t, "v0.0.89-0.20261117003434-d58b8d5c4e3b", versions["github.com/colony-2/jobdb"])
	newWorker := filepath.Join(t.TempDir(), "c2j-current")
	build = exec.Command("bash", filepath.Join(root, "scripts/go.sh"), "build", "-o", newWorker, "github.com/colony-2/c2j/cmd/c2j")
	build.Dir = root
	output, err = build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	info, err = buildinfo.ReadFile(newWorker)
	require.NoError(t, err)
	require.Equal(t, "v0.0.129-0.20261118074036-c412b478897e", info.Main.Version)

	ctx := testjobdb.Context(context.Background())
	rt, err := sqlite.NewFromConfig(ctx, sqlite.Config{DBPath: filepath.Join(t.TempDir(), "previous.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close(ctx)) })
	backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(rt)))
	t.Cleanup(backend.Close)
	s, err := NewRemoteServer(Config{WorkingDir: writeC2JConfig(t), JobDBURL: backend.URL})
	require.NoError(t, err)
	api := httptest.NewServer(s)
	t.Cleanup(api.Close)
	base := api.URL + "/api/projects/previous/jobs/"
	postJSON[jobdb.Cell](t, api.URL+"/api/projects/previous/cells", map[string]string{"name": "test", "repo": "github.com/cortex-test/platform"})

	repo := t.TempDir()
	git, err := exec.LookPath("git")
	require.NoError(t, err)
	for _, dir := range []string{".c2/recipes", ".c2/hooks", ".fixture-bin"} {
		require.NoError(t, os.MkdirAll(filepath.Join(repo, dir), 0755))
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	gitLog := filepath.Join(t.TempDir(), "git-calls")
	shim := "#!/bin/sh\nprintf 'git\\n' >> " + quote(gitLog) + "\nexec " + quote(git) + " -c " + quote("url.file://"+repo+".insteadOf=https://github.com/cortex-test/platform.git") + " -c " + quote("url.file://"+repo+".insteadOf=https://github.com/cortex-test/platform") + " \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".fixture-bin/git"), []byte(shim), 0755))
	// Instrument Cortex itself as well as the producer subprocess. A worker-only
	// shim would not detect an unexpected Git fetch during reader replay.
	t.Setenv("PATH", filepath.Join(repo, ".fixture-bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	executionLog := filepath.Join(t.TempDir(), "executions")
	firstAttempt := filepath.Join(t.TempDir(), "first-attempt")
	var fetches atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		_, _ = w.Write([]byte("external bytes"))
	}))
	t.Cleanup(external.Close)
	rec := fmt.Sprintf(`id: previous
sequence:
 - id: hook
   include:
     recipe: cell://.c2/hooks/check.yaml
     if_missing: {outputs: {status: absent}}
 - id: run
   op: command_execution
   retry: {maximum_attempts: 2, initial_interval: 1ms}
   artifacts:
     remote.txt: {kind: external, name: remote.txt, external: {url: %q}}
   inputs:
     working_directory: '${{ context.environment.op.outbox }}'
     run: |
       printf 'execution\n' >> %s
       if test ! -f %s; then touch %s; exit 7; fi
       printf 'retained artifact' > report.txt
       cat '${{ context.environment.op.inbox }}/remote.txt'
outputs:
 text: '${{ sequence.run.outputs.stdout }}'
 hook: '${{ sequence.hook.outputs.status }}'
 report: '${{ sequence.run.artifacts["report.txt"] }}'
`, external.URL, quote(executionLog), quote(firstAttempt), quote(firstAttempt))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".c2/recipes/previous.yaml"), []byte(rec), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".c2/hooks/check.yaml"), []byte("id: check\nsequence: []\noutputs: {status: original}\n"), 0644))
	for _, invalid := range []bool{false, true} {
		body := "id: strict\nop: recipes.run\ninputs:\n  recipes: [{name: previous, cell_name: test, inputs: {}}]\n  git_ref: main\n"
		if invalid {
			body += "  unexpected: true\n"
		}
		require.NoError(t, os.WriteFile(filepath.Join(repo, fmt.Sprintf(".c2/recipes/strict-%t.yaml", invalid)), []byte(body), 0644))
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", ".c2"}, {"-c", "user.name=Cortex Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-m", "Previous worker fixture"}} {
		command := exec.Command(git, append([]string{"-C", repo}, args...)...)
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	env := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "C2J_") && !strings.HasPrefix(value, "C2OPS_") && !strings.HasPrefix(value, "PATH=") {
			env = append(env, value)
		}
	}
	env = append(env, "PATH="+os.Getenv("PATH"), "GIT_TERMINAL_PROMPT=0")
	run := func(executable, id string, success bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "run", "one", "--job-id", id, "--jobdb", backend.URL+"/previous", "--input-mode", "fail")
		command.Dir, command.Env = repo, env
		output, err := command.CombinedOutput()
		if success {
			require.NoError(t, err, "%s", output)
		} else {
			require.Error(t, err, "%s", output)
			require.Contains(t, string(output), "unexpected")
		}
	}
	job := postJSON[recipejob.RecipeJob](t, strings.TrimSuffix(base, "/"), map[string]any{"job_id": "old-history", "cell": "test", "recipe": "git+" + (&url.URL{Scheme: "file", Path: repo}).String() + "//.c2/recipes/previous.yaml@main"})
	run(oldWorker, job.JobID, true)

	inspect := func(id string) story.JobRunStory {
		t.Helper()
		key := jobdb.JobKey{TenantId: "previous", JobId: id}
		before, err := rt.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: key})
		require.NoError(t, err)
		gitBefore, err := os.ReadFile(gitLog)
		require.NoError(t, err)
		executedBefore, err := os.ReadFile(executionLog)
		require.NoError(t, err)
		fetchesBefore := fetches.Load()
		var first story.JobRunStory
		for n := 0; n < 2; n++ {
			st := getJSON[story.JobRunStory](t, base+id+"/story")
			require.NotNil(t, st.Root)
			if n == 0 {
				first = st
			} else {
				require.Equal(t, first, st)
			}
			outcome := getJSON[map[string]any](t, base+id+"/outcome")
			require.Equal(t, "completed", outcome["status"])
			out := outcome["output"].(map[string]any)
			require.Equal(t, "original", out["hook"])
			require.Equal(t, "external bytes", out["text"])
			var ref struct {
				Stored struct {
					Key jobdb.ArtifactKey `json:"key"`
				} `json:"stored"`
			}
			raw, err := json.Marshal(out["report"])
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &ref))
			response, err := http.Get(fmt.Sprintf("%s%s/tasks/%d/artifacts/report.txt", base, ref.Stored.Key.JobId, ref.Stored.Key.TaskOrdinal))
			require.NoError(t, err)
			var data strings.Builder
			_, err = io.Copy(&data, response.Body)
			response.Body.Close()
			require.NoError(t, err)
			require.Equal(t, 200, response.StatusCode)
			require.Equal(t, "retained artifact", data.String())
		}
		after, err := rt.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: key})
		require.NoError(t, err)
		require.Equal(t, before, after)
		gitAfter, err := os.ReadFile(gitLog)
		require.NoError(t, err)
		require.Equal(t, gitBefore, gitAfter, "inspection fetched Git")
		executedAfter, err := os.ReadFile(executionLog)
		require.NoError(t, err)
		require.Equal(t, executedBefore, executedAfter, "inspection executed an operation")
		require.Equal(t, fetchesBefore, fetches.Load(), "inspection retrieved an external artifact")
		return first
	}
	st := inspect(job.JobID)
	executed, err := os.ReadFile(executionLog)
	require.NoError(t, err)
	require.Equal(t, "execution\nexecution\n", string(executed), "fixture must actually retry")
	var scope string
	for _, point := range st.RestartScopes {
		if strings.Contains(point.ScopeID, `scope:"op:run"`) {
			scope = point.ScopeID
			break
		}
	}
	require.NotEmpty(t, scope)
	restarted := postJSON[recipejob.RecipeJob](t, base+job.JobID+"/restart", map[string]any{"scope_id": scope, "request_id": "previous-worker-restart", "goal": map[string]string{"mode": "new"}})
	run(oldWorker, restarted.JobID, true)
	inspect(restarted.JobID)
	for _, invalid := range []bool{false, true} {
		job := postJSON[recipejob.RecipeJob](t, strings.TrimSuffix(base, "/"), map[string]any{"job_id": fmt.Sprintf("strict-%t", invalid), "cell": "test", "recipe": "git+" + (&url.URL{Scheme: "file", Path: repo}).String() + fmt.Sprintf("//.c2/recipes/strict-%t.yaml@main", invalid)})
		run(newWorker, job.JobID, !invalid)
		detail := getJSON[recipejob.RecipeJob](t, base+job.JobID)
		if invalid {
			require.NotEqual(t, "success", detail.CompletionStatus)
			require.Contains(t, fmt.Sprint(getJSON[map[string]any](t, base+job.JobID+"/outcome")), "unexpected")
		} else {
			require.Equal(t, "success", detail.CompletionStatus)
		}
	}
}
