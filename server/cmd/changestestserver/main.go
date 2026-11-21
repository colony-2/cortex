// changestestserver serves the embedded production UI with real saved writer diffs,
// native review waits and a remote JobDB backed by SQLite. Fixture controls are
// confined to this test executable.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/contextual"
	"github.com/colony-2/c2j/pkg/git/gitcommit"
	"github.com/colony-2/c2j/pkg/git/gitstate"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/jobinspect/changestest"
	"github.com/colony-2/c2j/pkg/starter"
	coretask "github.com/colony-2/c2j/pkg/task"
	workerops "github.com/colony-2/c2j/pkg/worker/ops"
	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/colony2/server/internal/webdist"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	wf "github.com/colony-2/jobdb/pkg/workflow"
)

const tenant = "changes-browser"

type fixture struct {
	Job     string   `json:"job"`
	Base    string   `json:"base"`
	Head    string   `json:"head"`
	Commits []string `json:"commits"`
	All     string   `json:"all"`
	worker  wf.ExecutionWorker
	tasks   []wf.TaskWorker
}

type recordedWorker struct {
	steps   []changestest.Step
	patches [][]byte
	missing bool
}

func (recordedWorker) Name() string { return "changes:recorded" }
func (w recordedWorker) Run(_ wf.TaskContext, data jobdb.TaskData) (jobdb.TaskData, error) {
	raw, err := data.GetData()
	if err != nil {
		return nil, err
	}
	var in struct {
		Index int `json:"index"`
	}
	if err = json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	step := w.steps[in.Index]
	envelope, err := coretask.NewOutputEnvelope(coretask.OutputKindActivityInvocationOutput, map[string]any{"workspace_before": step.State, "workspace_after": step.State, "workspace_scope_id": "main", "git": contextual.GitCommitContext{ParentHash: step.State.BaseCommit, PersistHash: step.State.HeadCommit}, "output": map[string]any{}})
	if err != nil {
		return nil, err
	}
	var files []jobdb.Artifact
	if step.Bundle != nil {
		files = append(files, jobdb.NewArtifactFromBytes(gitstate.ThinPackArtifactName, step.Bundle))
	}
	if !w.missing {
		files = append(files, jobdb.NewArtifactFromBytes("diff_from_base.diff", w.patches[in.Index]))
	}
	return jobdb.NewTaskData(envelope, files...)
}

func seed(ctx context.Context, rt jobdb.WorkflowRuntime, dir, scenario string, doc artifacts.Ref) (*fixture, error) {
	count := 4
	if scenario == "paged" || scenario == "v2-paged" {
		count = 52
	}
	opt := changestest.Options{Directory: filepath.Join(dir, scenario), Tenant: tenant, Cell: "changes", Repository: "example.test/changes", Commits: count, Revert: scenario == "revert", FastForward: scenario == "linear" || scenario == "v2", Published: scenario == "empty" || scenario == "v2-empty", OmitBaseBundle: scenario == "missing"}
	if scenario == "merge" {
		opt.MergeAt = 3
	}
	f, err := changestest.Build(ctx, rt, opt)
	if err != nil {
		return nil, err
	}
	if scenario == "large" {
		if err = os.WriteFile(filepath.Join(f.Checkout, "large.txt"), []byte(strings.Repeat("A long added line for the bounded preview.\n", 80000)), 0600); err != nil {
			return nil, err
		}
		out, e := gitcommit.PersistCommit(ctx, gitcommit.PersistCommitActivity{RepoPath: f.Checkout, StorageLocation: filepath.Join(dir, "large-pack"), RootHash: f.Base, ExpectedHeadHash: f.Head, CommitMessage: "Add large text", Timeout: 10 * time.Second})
		if e != nil {
			return nil, e
		}
		bundle, e := os.ReadFile(out.ThinPackPath)
		if e != nil {
			return nil, e
		}
		f.Head = out.CommitHash
		f.Commits = append(f.Commits, f.Head)
		f.Steps = append(f.Steps, changestest.Step{State: coretask.WorkspaceState{Repository: opt.Repository, Cell: opt.Cell, ScopeID: "main", BaseCommit: f.Base, HeadCommit: f.Head}, Bundle: bundle})
	}
	if scenario == "empty" || scenario == "v2-empty" {
		f.Base = f.Head
	}
	manifest := &fixture{Job: scenario, Base: f.Base, Head: f.Head, Commits: f.Commits}
	oracle, err := changestest.Git(ctx, f.Checkout, "diff", "--unified=10", f.Base, f.Head)
	if err != nil {
		return nil, err
	}
	manifest.All = string(oracle)
	patches := make([][]byte, len(f.Steps))
	for i, step := range f.Steps {
		base := step.State.BaseCommit
		if step.State.EmptyBase {
			base = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
		}
		saved, e := gitcommit.GenerateDiff(ctx, gitcommit.GenerateDiffInput{RepoPath: f.Checkout, FromHash: base, ToHash: step.State.HeadCommit, StorageLocation: filepath.Join(dir, scenario, "saved-diffs")})
		if e != nil {
			return nil, e
		}
		patches[i], err = os.ReadFile(saved.DiffPath)
		if err != nil {
			return nil, err
		}
	}
	if err = f.DeleteCheckout(); err != nil {
		return nil, err
	}
	if err = os.RemoveAll(filepath.Join(dir, scenario, "saved-diffs")); err != nil {
		return nil, err
	}
	worker := recordedWorker{steps: f.Steps, patches: patches, missing: scenario == "missing"}
	var v2Form input.InputForm
	if strings.HasPrefix(scenario, "v2") {
		raw, e := os.ReadFile("web/app/tests/fixtures/recipes/browser-review-v2.yaml")
		if e != nil {
			return nil, e
		}
		var fixture struct {
			Sequence []struct {
				Inputs struct {
					Form input.InputForm `json:"form"`
				} `json:"inputs"`
			} `json:"sequence"`
		}
		if e = json.Unmarshal(raw, &fixture); e != nil {
			return nil, e
		}
		v2Form = fixture.Sequence[1].Inputs.Form
		request := v2Form.Request.(map[string]any)
		request["review_id"] = scenario
		request["title"] = "Review committed changes"
		request["question"] = "## Ready to proceed?\n\nInspect the **Changes** tab, then choose a direction."
		request["documents"] = map[string]any{"overview": doc, "notes": doc}
		request["overview_document_id"] = "overview"
		request["document_metadata"] = map[string]any{"overview": map[string]any{"title": "Implementation overview", "role": "primary"}, "notes": map[string]any{"title": "Supporting notes", "role": "supporting"}}
		if scenario == "v2-empty" {
			request["documents"] = map[string]any{}
			delete(request, "document_metadata")
			delete(request, "overview_document_id")
		}
	}
	manifest.worker = wf.ExecutionWorker{JobType: "recipe", Execute: func(c wf.ExecutionContext, data wf.JobData) (wf.JobData, error) {
		publish := func(request string) error {
			form := input.InputForm{Kind: "review", RequestID: scenario + "-" + request, Title: "Review " + scenario + " changes", Documents: map[string]artifacts.Ref{"overview": doc}, Fields: []input.FormField{
				{ID: "response", Type: input.FieldTypeParagraphText, Question: "Your response", Required: true},
				{ID: "attachment", Type: input.FieldTypeFileUpload, Question: "Supporting attachment"},
			}}
			if strings.HasPrefix(scenario, "v2") {
				form = v2Form
				form.RequestID = scenario + "-" + request
			}
			raw, _ := json.Marshal(form)
			_, e := c.DoTask(wf.TaskCall{Site: request, TaskType: "input:collect_user_input", ContractVersion: 1, Command: raw}, func(context.Context) (jobdb.TaskData, error) {
				var value map[string]any
				if e := json.Unmarshal(raw, &value); e != nil {
					return nil, e
				}
				req := workerops.ActivityInvocationRequest{Input: value}
				req.GitTaskContext.Workspace = &contextual.WorkspaceContext{ScopeID: "review-documents"}
				return workerops.NewInvocationTaskData(req)
			})
			return e
		}

		var incoming jobdb.Artifact
		for i, step := range f.Steps {
			g := gitstate.GlobalGitTaskContext{BaseRepo: opt.Repository, BaseRef: "main", ParentHash: step.State.BaseCommit, ResolvedBaseHash: step.State.BaseCommit, PersistHash: step.State.HeadCommit, Workspace: &contextual.WorkspaceContext{Cell: opt.Cell, ScopeID: "main"}}
			if step.State.EmptyBase {
				g.Workspace.Initial = "empty"
			}
			out, e := c.DoTask(wf.TaskCall{Site: "workspace", TaskType: worker.Name(), ContractVersion: 1, Command: json.RawMessage(strconv.Quote(fmt.Sprint(i)))}, func(context.Context) (jobdb.TaskData, error) {
				payload := map[string]any{"context": g, "index": i}
				var files []jobdb.Artifact
				if incoming != nil {
					key, err := incoming.ArtifactKey()
					if err != nil {
						return nil, err
					}
					payload["restore_artifact"] = &key
					files = append(files, incoming)
				}
				return jobdb.NewTaskData(payload, files...)
			})
			if e != nil {
				return nil, e
			}
			files, e := out.GetArtifacts()
			if e != nil {
				return nil, e
			}
			for _, file := range files {
				if file.Name() == gitstate.ThinPackArtifactName {
					incoming = file
				}
			}
			if scenario == "moving" && i == 2 {
				if e := publish("first"); e != nil {
					return nil, e
				}
			}
		}
		for _, request := range []string{"first", "second"} {
			if scenario == "moving" && request == "first" {
				continue
			}
			if e := publish(request); e != nil {
				return nil, e
			}
		}

		return data, nil
	}}
	metadata, _ := json.Marshal(starter.JobMetadata{Version: 1, Title: "Review " + scenario, Prompt: "Inspect the committed changes and leave your response."})
	_, err = rt.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: scenario, JobType: "recipe", Cell: "changes", CellRepo: opt.Repository, Metadata: metadata,
		Data: jobdb.NewTaskDataOrPanic(map[string]any{"context": contextual.JobContext{GitBase: contextual.GitBaseContext{BaseRepo: opt.Repository}, Workspace: &contextual.WorkspaceContext{Cell: opt.Cell, ScopeID: "main"}}})}})
	if err != nil {
		return nil, err
	}
	manifest.tasks = []wf.TaskWorker{worker}
	run, err := wf.GetJobForRun(ctx, rt, wf.GetJobForRunRequest{JobKey: jobdb.JobKey{TenantId: tenant, JobId: scenario}, JobWorker: manifest.worker, TaskWorkers: []wf.TaskWorker{worker}, WorkerID: "changes-browser", LeaseDuration: time.Minute})
	if err != nil {
		return nil, err
	}
	_, err = run.Run(nil)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18086", "listen address")
	flag.Parse()
	ctx, stop := signal.NotifyContext(testjobdb.Context(context.Background()), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir, err := os.MkdirTemp("", "cortex-changes-browser-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	db, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: filepath.Join(dir, "jobs.db")})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close(context.Background())
	_, err = db.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: tenant, Name: "changes", Repo: "example.test/changes"})
	if err != nil {
		log.Fatal(err)
	}
	doc := jobdb.NewArtifactFromBytes("overview.md", []byte("# Changes to review\n\nRead the Changes tab, then leave your response.\n\n[Overview](overview.md)\n"))
	_, err = db.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: "documents", JobType: "source", Cell: "changes", CellRepo: "example.test/changes", Data: jobdb.NewTaskDataOrPanic(nil, doc)}})
	if err != nil {
		log.Fatal(err)
	}
	key, err := doc.ArtifactKey()
	if err != nil {
		log.Fatal(err)
	}
	fixtures := map[string]*fixture{}
	for _, scenario := range []string{"linear", "revert", "merge", "empty", "missing", "paged", "large", "moving", "v2", "v2-paged", "v2-empty", "v2-zoom"} {
		f, e := seed(ctx, db, dir, scenario, artifacts.NewStoredRef(key))
		if e != nil {
			log.Fatalf("%s: %v", scenario, e)
		}
		fixtures[scenario] = f
	}
	backend := httptest.NewServer(testjobdb.Handler(remote.NewServer(db)))
	defer backend.Close()
	server, err := cortex.NewRemoteServer(cortex.Config{DefaultTenantID: tenant, JobDBURL: backend.URL, StaticFS: webdist.FS()})
	if err != nil {
		log.Fatal(err)
	}
	router := http.NewServeMux()
	router.HandleFunc("/fixture/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/fixture/"), "/")
		f := fixtures[parts[0]]
		if f == nil {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "advance" && r.Method == http.MethodPost {
			run, e := wf.GetJobForRun(ctx, db, wf.GetJobForRunRequest{JobKey: jobdb.JobKey{TenantId: tenant, JobId: f.Job}, JobWorker: f.worker, TaskWorkers: f.tasks, WorkerID: "changes-browser", LeaseDuration: time.Minute})
			if e == nil {
				_, e = run.Run(nil)
			}
			if e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f)
	})
	router.Handle("/", server)
	srv := &http.Server{Addr: *addr, Handler: router}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	log.Printf("changes browser listening on %s", *addr)
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
