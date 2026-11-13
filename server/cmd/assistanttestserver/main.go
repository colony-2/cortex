// assistanttestserver is a local-only acceptance host. It composes the production
// HTTP adapter with durable SQLite; model responses alone are deterministic.
// Ordinary c2j workers run in separate processes, never in the serving adapter.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/colony-2/c2j/pkg/gitprofile"
	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/jobinspect"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/c2j/pkg/secrets"
	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	ca "github.com/colony-2/cortex-assistant/assistant"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/workflow"
)

const tenant = "assistant-browser"
const commit = "0561937f92a8e15c9eedb0d3045ab432f8e2c17a"

var errAuditWorkerPending = errors.New("worker log not complete")

const producerRecipe = `id: assistant-source-producer
schema_version: 2
version: "1.0.0"
input_schema: {}
sequence:
 - id: emit
   op: command_execution
   inputs:
    working_directory: '{{ context.environment.op.outbox }}'
    run: |
     printf 'Child evidence marker 世界 retained-original-20261112\n' > evidence.md
     printf '{"review":"independent-json-marker"}' > data.json
     printf '\211PNGunsupported-marker' > picture.png
     printf 'Expired evidence marker\n' > gone.md
     head -c 40000 /dev/zero | tr '\000' x > large.txt
     head -c 40000 /dev/zero | tr '\000' y > overflow.txt
outputs:
 document: '${{ sequence.emit.artifacts["evidence.md"] }}'
 json: '${{ sequence.emit.artifacts["data.json"] }}'
 gone: '${{ sequence.emit.artifacts["gone.md"] }}'
 picture: '${{ sequence.emit.artifacts["picture.png"] }}'
 large: '${{ sequence.emit.artifacts["large.txt"] }}'
 overflow: '${{ sequence.emit.artifacts["overflow.txt"] }}'
`
const sourceRecipe = `id: assistant-review-source
schema_version: 2
version: "1.0.0"
input_schema:
 prompt: {type: string}
sequence:
 - id: child
   op: recipe.run_and_get_result
   inputs:
    name: producer
    cell_name: root
    inputs: {}
 - id: review
   op: input
   inputs:
    form:
     kind: review
     title: Discuss the retained evidence
     documents:
      a-child: '${{ sequence.child.outputs.outputs.document }}'
      b-json: '${{ sequence.child.outputs.outputs.json }}'
      c-large: '${{ sequence.child.outputs.outputs.large }}'
      d-overflow: '${{ sequence.child.outputs.outputs.overflow }}'
      e-picture: '${{ sequence.child.outputs.outputs.picture }}'
      f-unavailable: '${{ sequence.child.outputs.outputs.gone }}'
     fields:
      - {id: decision, question: Your decision, type: short_answer, required: true}
 - id: replacement
   op: input
   inputs:
    form:
     kind: review
     title: Replacement request must be selected explicitly
     documents:
      a-child: '${{ sequence.child.outputs.outputs.document }}'
     fields:
      - {id: decision, question: Your decision, type: short_answer, required: true}
outputs:
 receipt: '${{ sequence.review.outputs.receipt }}'
`

type secretFixture struct{ key string }

func (s secretFixture) ResolveSecrets(_ context.Context, q jobdb.SecretResolveRequest) (jobdb.SecretResolveResult, error) {
	out := jobdb.SecretResolveResult{Secrets: map[string]jobdb.SecretValue{}}
	for _, n := range q.Names {
		if n == "fixture_key" {
			out.Secrets[n] = jobdb.SecretValue{Value: s.key}
		}
	}
	return out, nil
}

type fixture struct {
	workerSecretLeaks             int
	rt                            *sqlite.Runtime
	server                        atomic.Pointer[cortex.Server]
	cfg                           cortex.Config
	dir, worker, jobdbURL, caPath string
	secret                        string
	mu                            sync.Mutex
	requests                      []map[string]any
	workers                       map[string]*exec.Cmd
	workerLog                     []string
	workerRuns                    []map[string]any
	paused                        bool
	hold                          chan struct{}
	providerMode                  string
	lose                          bool
}

func jsonBytes(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
func output(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func trusted(ctx context.Context) context.Context {
	return jobdb.WithPrimaryGoalSelectionAuthorizer(testjobdb.Context(ctx), func(_ context.Context, g jobdb.GoalKey, c jobdb.ResolvedCell) error {
		if g.TenantId != tenant || c.Cell != "root" {
			return jobdb.ErrPrimaryGoalSelectionForbidden
		}
		return nil
	})
}
func (f *fixture) compose() error {
	rt, e := remote.New(f.jobdbURL, http.DefaultClient)
	if e != nil {
		return e
	}
	eng, e := workflow.NewEngineBuilder().WithRuntime(rt).WithWorkerTenantId(tenant).BuildEngine()
	if e != nil {
		return e
	}
	s, e := cortex.NewServer(f.cfg, jobdbschema.WorkflowEngine{Engine: eng, Registry: rt})
	if e == nil {
		f.server.Store(s)
	}
	return e
}
func (f *fixture) provider(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/exact/root/chat/completions" {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+f.secret {
		w.WriteHeader(401)
		return
	}
	var q map[string]any
	if json.NewDecoder(io.LimitReader(r.Body, 512<<10)).Decode(&q) != nil {
		w.WriteHeader(400)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, q)
	hold, mode := f.hold, f.providerMode
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-r.Context().Done():
			return
		}
	}
	if mode == "error" {
		w.WriteHeader(401)
		output(w, map[string]any{"error": map[string]any{"message": f.secret}})
		return
	}
	tools, ok := q["tools"].([]any)
	if !ok || len(tools) != 1 {
		w.WriteHeader(400)
		return
	}
	tool := tools[0].(map[string]any)["function"].(map[string]any)
	citations := []string{}
	seen := map[string]bool{}
	messages, _ := q["messages"].([]any)
	for _, m := range messages {
		content, _ := m.(map[string]any)["content"].(string)
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "SOURCE ") {
				var d ca.SnapshotDocument
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "SOURCE ")), &d) == nil && !seen[d.ID] {
					citations = append(citations, d.ID)
					seen[d.ID] = true
				}
			}
		}
	}
	result := map[string]any{"text": "Grounded deterministic reply from the selected evidence.", "documents": []any{map[string]any{"name": "discussion.md", "text": "# Recorded discussion\n\nretained-download-marker 世界\n", "mime": "text/markdown"}}, "citations": citations, "return_artifact_ids": []string{}}
	call := map[string]any{"id": "result", "type": "function", "function": map[string]any{"name": tool["name"], "arguments": string(jsonBytes(result))}}
	message := map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{call}}
	choice := map[string]any{"index": 0, "finish_reason": "tool_calls", "message": message}
	output(w, map[string]any{"id": "deterministic-provider", "object": "chat.completion", "created": 1, "model": "fixture", "choices": []any{choice}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
}
func workerEnv(caPath string) []string {
	out := []string{}
	for _, v := range os.Environ() {
		name := strings.SplitN(v, "=", 2)[0]
		if strings.HasPrefix(name, "C2J_") || name == "JOBDB_URL" || name == "NIX_SSL_CERT_FILE" {
			continue
		}
		out = append(out, v)
	}
	return append(out, "NIX_SSL_CERT_FILE="+caPath, "GIT_TERMINAL_PROMPT=0")
}
func (f *fixture) launch(ctx context.Context, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.paused || f.workers[id] != nil {
		return
	}
	cmd := exec.CommandContext(ctx, f.worker, "run", "one", "--job-id", id, "--input-mode", "ops", "--jobdb", f.jobdbURL+"/"+tenant, "--on-not-ready", "fail", "--await-threshold", "100ms")
	cmd.Dir = f.dir
	cmd.Env = workerEnv(f.caPath)
	f.workers[id] = cmd
	go func() {
		b, e := cmd.CombinedOutput()
		safe := strings.ReplaceAll(string(b), f.secret, "[redacted]")
		if strings.Contains(string(b), f.secret) {
			f.mu.Lock()
			f.workerSecretLeaks++
			f.mu.Unlock()
		}
		f.mu.Lock()
		delete(f.workers, id)
		if cmd.Process != nil {
			f.workerRuns = append(f.workerRuns, map[string]any{"job_id": id, "pid": cmd.Process.Pid})
		}
		f.workerLog = append(f.workerLog, fmt.Sprintf("%s: %v\n%s", id, e, safe))
		f.mu.Unlock()
		_ = os.WriteFile(filepath.Join(f.dir, "workers.log"), []byte(strings.Join(f.logs(), "\n")), 0600)
	}()
}
func (f *fixture) logs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.workerLog...)
}
func (f *fixture) work(ctx context.Context) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, e := f.rt.ListJobs(trusted(ctx), jobdb.ListJobsRequest{TenantIds: []string{tenant}, AvailableRoutes: []jobdb.Route{{JobType: "recipe"}}, PageSize: 100})
			if e != nil {
				continue
			}
			for _, j := range rows.Jobs {
				f.launch(ctx, j.JobKey.JobId)
			}
		}
	}
}
func (f *fixture) control(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/__test/assistant/")
	switch action {
	case "provenance":
		b, e := os.ReadFile(filepath.Join(f.dir, "provenance.json"))
		if e != nil {
			http.Error(w, "provenance unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
		return
	case "audit":
		v, e := f.audit(trusted(r.Context()), r.URL.Query().Get("job"))
		if errors.Is(e, errAuditWorkerPending) {
			http.Error(w, "worker log pending", http.StatusConflict)
			return
		}
		if e != nil {
			http.Error(w, "audit unavailable", 500)
			return
		}
		output(w, v)
		return
	case "original":
		name := r.URL.Query().Get("name")
		if name != "discussion.md" && name != "evidence.md" {
			w.WriteHeader(403)
			return
		}
		var ordinal int64
		_, _ = fmt.Sscan(r.URL.Query().Get("task"), &ordinal)
		eng, e := workflow.NewEngineBuilder().WithRuntime(f.rt).WithWorkerTenantId(tenant).BuildEngine()
		if e != nil {
			w.WriteHeader(500)
			return
		}
		a, e := eng.GetArtifact(tenant, jobdb.ArtifactKey{JobId: r.URL.Query().Get("job"), TaskOrdinal: ordinal, Name: name})
		if e != nil {
			w.WriteHeader(404)
			return
		}
		reader, e := a.Open()
		if e != nil {
			w.WriteHeader(404)
			return
		}
		defer reader.Close()
		_, _ = io.Copy(w, reader)
		return
	case "legacy", "unrelated":
		eng, err := workflow.NewEngineBuilder().WithRuntime(f.rt).WithWorkerTenantId(tenant).BuildEngine()
		if err != nil {
			http.Error(w, "engine unavailable", 500)
			return
		}
		id := fmt.Sprintf("%s-%d", action, time.Now().UnixNano())
		recipe := "producer"
		args := map[string]any{}
		if action == "legacy" {
			recipe = "git+https://github.com/colony-2/cortex-assistant.git//discussion.yaml@" + commit
			b := f.cfg.AssistantRuntime.Bindings
			args = map[string]any{"protocol": ca.OrdinaryProtocol, "session_id": id, "prompt": "Explicit legacy ordinary discussion", "snapshot": ca.FrozenSnapshot{TenantID: tenant, Source: ca.Source{Kind: "global"}, At: time.Now().UTC(), Documents: []ca.SnapshotDocument{}}, "base_url": b.BaseURL, "api_key": b.APIKey, "provider": b.Provider, "model": b.Model}
		}
		ref, err := recipejob.SubmitRecipeJob(trusted(r.Context()), recipejob.BuildStartJobRequest{TenantID: tenant, JobID: id, GoalID: r.URL.Query().Get("goal"), Target: f.cfg.AssistantRuntime.Root, Recipe: recipe, Inputs: args}, jobdbschema.WorkflowEngine{Engine: eng, Registry: f.rt})
		if err != nil {
			http.Error(w, "submission unavailable", 500)
			return
		}
		output(w, map[string]any{"id": id, "job_id": ref.JobID, "goal_id": ref.GoalID})
		return
	case "evidence":
		f.mu.Lock()
		v := map[string]any{"requests": f.requests, "worker_log": f.workerLog, "worker_executions": f.workerRuns, "active_workers": len(f.workers), "assistant_commit": commit, "model_operation": ca.OrdinaryModelOperation}
		f.mu.Unlock()
		output(w, v)
		return
	case "inspect":
		api := jobinspect.Service{Runtime: f.rt, Tenant: tenant}
		v, e := api.Inspect(trusted(r.Context()), jobinspect.Request{Operation: "log", JobID: r.URL.Query().Get("job"), Limit: 100})
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		output(w, v)
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var e error
	switch action {
	case "expire", "expire-reply":
		e = filepath.WalkDir(filepath.Join(f.dir, "blobs"), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				b, e := os.ReadFile(path)
				if e != nil {
					return e
				}
				if action == "expire" && string(b) == "Expired evidence marker\n" || action == "expire-reply" && strings.Contains(string(b), "retained-download-marker") && strings.HasPrefix(string(b), "# Recorded discussion") {
					return os.Remove(path)
				}
			}
			return nil
		})
	case "replace":
		e = f.compose()
	case "pause":
		f.mu.Lock()
		f.paused = true
		f.mu.Unlock()
	case "resume":
		f.mu.Lock()
		f.paused = false
		f.mu.Unlock()
	case "hold":
		f.mu.Lock()
		if f.hold == nil {
			f.hold = make(chan struct{})
		}
		f.mu.Unlock()
	case "release":
		f.mu.Lock()
		if f.hold != nil {
			close(f.hold)
			f.hold = nil
		}
		f.mu.Unlock()
	case "error":
		f.mu.Lock()
		f.providerMode = "error"
		f.mu.Unlock()
	case "success":
		f.mu.Lock()
		f.providerMode = ""
		f.mu.Unlock()
	case "lose":
		f.mu.Lock()
		f.lose = true
		f.mu.Unlock()
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
func (f *fixture) audit(ctx context.Context, job string) (map[string]any, error) {
	f.mu.Lock()
	finished := false
	for _, run := range f.workerRuns {
		if run["job_id"] == job {
			finished = true
		}
	}
	active := f.workers[job] != nil
	f.mu.Unlock()
	// Inspect raw worker output only after CombinedOutput has returned. A zero
	// leak count while the process is still running would not establish this.
	if !finished || active {
		return nil, errAuditWorkerPending
	}
	eng, e := workflow.NewEngineBuilder().WithRuntime(f.rt).WithWorkerTenantId(tenant).BuildEngine()
	if e != nil {
		return nil, e
	}
	run, e := eng.GetJobRun(ctx, jobdb.GetJobRunRequest{JobKey: jobdb.JobKey{TenantId: tenant, JobId: job}, IncludeInputs: true, IncludeOutputs: true, IncludeArtifacts: true, IncludeAttemptInputs: true})
	if e != nil {
		return nil, e
	}
	hits := []string{}
	scanned := map[string]bool{}
	count := 0
	inspect := func(location string, data *jobdb.TaskIO) error {
		if data == nil {
			return nil
		}
		if strings.Contains(string(data.Data), f.secret) {
			hits = append(hits, location)
		}
		for _, a := range data.Artifacts {
			if a.Key == nil {
				return fmt.Errorf("missing artifact key")
			}
			id := string(jsonBytes(a.Key))
			if scanned[id] {
				continue
			}
			scanned[id] = true
			value, e := eng.GetArtifact(tenant, *a.Key)
			if e != nil {
				return e
			}
			reader, e := value.Open()
			if e != nil {
				return e
			}
			b, e := io.ReadAll(reader)
			ce := reader.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
			count++
			if strings.Contains(string(b), f.secret) {
				hits = append(hits, location+"/"+a.Name)
			}
		}
		return nil
	}
	if e = inspect("start", run.Start.Input); e != nil {
		return nil, e
	}
	for _, attempt := range run.Attempts {
		if e = inspect("job/output", attempt.Output); e != nil {
			return nil, e
		}
		for _, task := range attempt.Tasks {
			for _, a := range task.Attempts {
				if e = inspect(task.TaskRunID+"/input", a.Input); e != nil {
					return nil, e
				}
				if e = inspect(task.TaskRunID+"/output", a.Output); e != nil {
					return nil, e
				}
			}
		}
	}
	failedRecords := 0
	for _, entry := range run.Execution {
		if entry.Record.ErrorKind != "" {
			failedRecords++
		}
		for side, snapshot := range map[string]*jobdb.ExecutionSnapshot{"input": entry.Record.Input, "output": entry.Record.Output} {
			if snapshot == nil {
				continue
			}
			data := &jobdb.TaskIO{Data: snapshot.Data}
			for _, a := range snapshot.Artifacts {
				data.Artifacts = append(data.Artifacts, jobdb.ArtifactInfo{Name: a.Name, Key: a.Key})
			}
			if e = inspect(fmt.Sprintf("execution/%d/%s/%s", entry.Ordinal, entry.Record.ID, side), data); e != nil {
				return nil, e
			}
		}
	}
	f.mu.Lock()
	leaks := f.workerSecretLeaks
	requestsClean := !strings.Contains(string(jsonBytes(f.requests)), f.secret)
	f.mu.Unlock()
	return map[string]any{"job_id": job, "worker_log_complete": true, "retained_artifacts_scanned": count, "execution_records_scanned": len(run.Execution), "failed_execution_records": failedRecords, "retention_secret_locations": hits, "inline_history_clean": !strings.Contains(string(jsonBytes(run)), f.secret), "worker_log_secret_occurrences": leaks, "provider_payloads_clean": requestsClean}, nil
}

func git(dir string, args ...string) error {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	b, e := c.CombinedOutput()
	if e != nil {
		return fmt.Errorf("git %v: %v %s", args, e, b)
	}
	return nil
}
func main() {
	addr := flag.String("addr", "127.0.0.1:18086", "loopback test HTTP address")
	worker := flag.String("worker", "", "ordinary c2j executable from server/go.mod")
	assistant := flag.String("assistant-source", "", "clean exact merged assistant checkout")
	state := flag.String("state-dir", "", "scratch directory for retained test records")
	flag.Parse()
	if *worker == "" || *assistant == "" {
		log.Fatal("worker and exact assistant source required")
	}
	if !strings.HasPrefix(*addr, "127.0.0.1:") {
		log.Fatal("fixture must bind loopback")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir := *state
	var e error
	if dir == "" {
		dir, e = os.MkdirTemp("", "cortex-assistant-browser-")
		if e != nil {
			log.Fatal(e)
		}
		defer os.RemoveAll(dir)
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		log.Fatal(e)
	}
	check := exec.Command("git", "-C", *assistant, "rev-parse", "HEAD")
	head, e := check.Output()
	if e != nil || strings.TrimSpace(string(head)) != commit {
		log.Fatal("assistant source identity mismatch")
	}
	check = exec.Command("git", "-C", *assistant, "status", "--porcelain")
	clean, e := check.Output()
	if e != nil || len(clean) != 0 {
		log.Fatal("assistant source is dirty")
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		log.Fatal(e)
	}
	f := &fixture{dir: dir, worker: *worker, secret: "synthetic-" + hex.EncodeToString(raw), workers: map[string]*exec.Cmd{}}
	provider := httptest.NewTLSServer(http.HandlerFunc(f.provider))
	defer provider.Close()
	f.caPath = filepath.Join(dir, "provider-ca.pem")
	if e = os.WriteFile(f.caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); e != nil {
		log.Fatal(e)
	}
	rt, e := sqlite.NewFromConfig(trusted(ctx), sqlite.Config{DBPath: filepath.Join(dir, "runtime.db"), BlobDir: filepath.Join(dir, "blobs"), SecretProvider: secretFixture{f.secret}, MaxInlineArtifactBytes: 1})
	if e != nil {
		log.Fatal(e)
	}
	defer rt.Close(trusted(context.Background()))
	f.rt = rt
	runtimeHTTP := remote.NewServer(rt)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtimeHTTP.ServeHTTP(w, r.WithContext(trusted(r.Context())))
	}))
	defer backend.Close()
	f.jobdbURL = backend.URL
	root := filepath.Join(dir, "root")
	if e = os.MkdirAll(filepath.Join(root, ".c2/recipes"), 0755); e != nil {
		log.Fatal(e)
	}
	for name, body := range map[string]string{"source.yaml": sourceRecipe, "producer.yaml": producerRecipe} {
		if e = os.WriteFile(filepath.Join(root, ".c2/recipes", name), []byte(body), 0644); e != nil {
			log.Fatal(e)
		}
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"-c", "user.name=Cortex Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false", "commit", "-qm", "Deterministic source records"}} {
		if e = git(root, args...); e != nil {
			log.Fatal(e)
		}
	}
	export := filepath.Join(dir, "git")
	if e = os.MkdirAll(export, 0755); e != nil {
		log.Fatal(e)
	}
	for name, path := range map[string]string{"root.git": filepath.Join(root, ".git"), "assistant.git": filepath.Join(*assistant, ".git")} {
		if e = os.Symlink(path, filepath.Join(export, name)); e != nil {
			log.Fatal(e)
		}
	}
	gitPath, e := exec.LookPath("git")
	if e != nil {
		log.Fatal(e)
	}
	mirror := httptest.NewServer(&cgi.Handler{Path: gitPath, Args: []string{"http-backend"}, Dir: export, Env: []string{"GIT_PROJECT_ROOT=" + export, "GIT_HTTP_EXPORT_ALL=1"}})
	defer mirror.Close()
	profile := gitprofile.Profile{Version: 1, Entries: []gitprofile.Entry{{URL: mirror.URL + "/root.git", RewriteFrom: []string{"https://github.com/example/root.git"}}, {URL: mirror.URL + "/assistant.git", RewriteFrom: []string{"https://github.com/colony-2/cortex-assistant.git"}}}}
	for name, value := range map[string]any{gitprofile.ReadOnlyKey: "fixture", gitprofile.ProfilePrefix + "fixture": profile} {
		if _, e = rt.SetTenantConfigurationValue(trusted(ctx), jobdb.SetTenantConfigurationValueRequest{TenantID: tenant, Name: name, Value: jsonBytes(value)}); e != nil {
			log.Fatal(e)
		}
	}
	if _, e = rt.CreateCell(trusted(ctx), jobdb.CreateCellRequest{TenantID: tenant, Name: "root", Repo: "github.com/example/root"}); e != nil {
		log.Fatal(e)
	}
	target := recipejob.ResolvedTarget{CellName: "root", CellRepo: "github.com/example/root", RepositorySource: "https://github.com/example/root.git", DefaultRef: "main"}
	f.cfg = cortex.Config{CORSOrigins: []string{"http://127.0.0.1:15176"}, DefaultTenantID: tenant, WorkingDir: root, AssistantRuntime: &cortex.AssistantRuntimeConfig{Root: target, CursorKey: []byte(strings.Repeat("trusted-fixture-key-", 3)), Bindings: ca.OrdinaryBindings{BaseURL: provider.URL + "/exact/root", Provider: "openai", Model: "fixture", APIKey: secrets.Reference{Name: "fixture_key"}}, ExecutionReady: true, LegacyReleases: []ca.ReleaseDescriptor{{Recipe: "git+https://github.com/colony-2/cortex-assistant.git//discussion.yaml@" + commit, Commit: commit, Protocol: ca.OrdinaryProtocol}}}}
	if e = f.compose(); e != nil {
		log.Fatal(e)
	}
	eng, e := workflow.NewEngineBuilder().WithRuntime(rt).WithWorkerTenantId(tenant).BuildEngine()
	if e != nil {
		log.Fatal(e)
	}
	if _, e = recipejob.SubmitRecipeJob(trusted(ctx), recipejob.BuildStartJobRequest{TenantID: tenant, JobID: "source", Target: target, Recipe: "source", Inputs: map[string]any{"prompt": "Selected story marker source-review-20261112"}}, jobdbschema.WorkflowEngine{Engine: eng, Registry: rt}); e != nil {
		log.Fatal(e)
	}
	// Pin resolved inference bytes before workers execute them.
	pkg := exec.Command("nix", "--extra-experimental-features", "nix-command flakes", "build", "--no-link", "--no-write-lock-file", "--print-out-paths", strings.TrimPrefix(ca.OrdinaryModelOperation, "nix:"))
	b, e := pkg.Output()
	if e != nil {
		log.Fatal(e)
	}
	provenance := map[string]any{"assistant_commit": commit, "assistant_recipe": "git+https://github.com/colony-2/cortex-assistant.git//discussion-goal.yaml@" + commit, "model_operation": ca.OrdinaryModelOperation, "package_path": strings.TrimSpace(string(b))}
	if info, ok := debug.ReadBuildInfo(); ok {
		modules := map[string]any{}
		for _, dep := range info.Deps {
			if dep.Path == "github.com/colony-2/c2j" || dep.Path == "github.com/colony-2/jobdb" || dep.Path == "github.com/colony-2/cortex-assistant" {
				modules[dep.Path] = dep
			}
		}
		provenance["modules"] = modules
	}
	recipeBytes, e := os.ReadFile(filepath.Join(*assistant, "discussion-goal.yaml"))
	if e != nil {
		log.Fatal(e)
	}
	provenance["assistant_recipe_sha256"] = fmt.Sprintf("%x", sha256.Sum256(recipeBytes))
	for _, name := range []string{"bin/pydantic", "libexec/main.py", "share/c2j/op.json"} {
		data, e := os.ReadFile(filepath.Join(strings.TrimSpace(string(b)), name))
		if e != nil {
			log.Fatal(e)
		}
		provenance[name+"_sha256"] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	if e = os.WriteFile(filepath.Join(dir, "provenance.json"), jsonBytes(provenance), 0600); e != nil {
		log.Fatal(e)
	}
	go f.work(ctx)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/__test/assistant/") {
			f.control(w, r)
			return
		}
		if r.Method == "POST" && (strings.HasSuffix(r.URL.Path, "/assistant/sessions") || strings.HasSuffix(r.URL.Path, "/reopen") || strings.HasSuffix(r.URL.Path, "/close")) {
			f.mu.Lock()
			lose := f.lose
			f.lose = false
			f.mu.Unlock()
			if lose {
				rec := httptest.NewRecorder()
				f.server.Load().ServeHTTP(rec, r)
				if rec.Code == 200 {
					http.Error(w, "Fixture lost accepted response", 503)
					return
				}
				for k, v := range rec.Header() {
					w.Header()[k] = v
				}
				w.WriteHeader(rec.Code)
				_, _ = w.Write(rec.Body.Bytes())
				return
			}
		}
		f.server.Load().ServeHTTP(w, r)
	})
	srv := &http.Server{Addr: *addr, Handler: handler}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	log.Printf("assistant fixture listening on %s; durable records: %s", *addr, dir)
	if e = srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
