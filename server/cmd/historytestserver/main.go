// historytestserver creates real retained review records for Chromium checks.
// Its fixture controls are available only in this dedicated test executable.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/c2j/pkg/starter"
	workerops "github.com/colony-2/c2j/pkg/worker/ops"
	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/workflow"
)

const tenant = "history-browser"

var jobKey = jobdb.JobKey{TenantId: tenant, JobId: "reviews"}

type fixture struct {
	mu      sync.Mutex
	path    string
	runtime *sqliteruntime.Runtime
	server  atomic.Pointer[cortex.Server]
	forms   []input.InputForm
}

func (f *fixture) open(ctx context.Context) error {
	rt, err := sqliteruntime.NewFromConfig(ctx, sqliteruntime.Config{DBPath: f.path})
	if err != nil {
		return err
	}
	f.runtime = rt
	engine, err := workflow.NewEngineBuilder().WithRuntime(rt).WithWorkerTenantId(tenant).BuildEngine()
	if err != nil {
		return err
	}
	server, err := cortex.NewServer(cortex.Config{DefaultTenantID: tenant}, jobdbschema.WorkflowEngine{Engine: engine, Registry: rt})
	if err != nil {
		return err
	}
	f.server.Store(server)
	return nil
}
func (f *fixture) seed(ctx context.Context) error {
	_, err := f.runtime.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: tenant, Name: "history-fixture", Repo: "example.test/history"})
	if err != nil {
		return err
	}
	doc := jobdb.NewArtifactFromBytes("original.md", []byte("Original first review document, retained in SQLite.\n"))
	_, err = f.runtime.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: "original-producer", JobType: "source", Cell: "history-fixture", CellRepo: "example.test/history", Data: jobdb.NewTaskDataOrPanic(nil, doc)}})
	if err != nil {
		return err
	}
	key, err := doc.ArtifactKey()
	if err != nil {
		return err
	}
	for _, id := range []string{"first-review", "second-review"} {
		f.forms = append(f.forms, input.InputForm{Kind: "review", RequestID: id, Title: id, Documents: map[string]artifacts.Ref{"original": artifacts.NewStoredRef(key)}, Fields: []input.FormField{{ID: "decision", Question: "Your decision", Type: input.FieldTypeShortAnswer, Required: true}, {ID: "attachment", Question: "Supporting attachment", Type: input.FieldTypeFileUpload}}})
	}
	metadata, err := json.Marshal(starter.JobMetadata{Version: 1, Title: "Retained review fixture", Prompt: "Review the original document and retain accepted answers."})
	if err != nil {
		return err
	}
	policy := jobdb.DefaultRunPolicy()
	policy.Retry.MaximumAttempts = 1
	_, err = f.runtime.SubmitJob(ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: jobKey.JobId, JobType: "recipe", Cell: "history-fixture", CellRepo: "example.test/history", Data: jobdb.NewTaskDataOrPanic(map[string]any{}), Metadata: metadata, RunPolicy: policy}})
	if err != nil {
		return err
	}
	return f.advance(ctx)
}
func (f *fixture) advance(ctx context.Context) error {
	// Controlled execution creates native collection waits and consumes committed
	// answers. History endpoints never call this worker.
	worker := workflow.ExecutionWorker{JobType: "recipe", Execute: func(c workflow.ExecutionContext, _ jobdb.JobData) (jobdb.JobData, error) {
		for i, form := range f.forms {
			raw, err := json.Marshal(form)
			if err != nil {
				return nil, err
			}
			var value map[string]any
			if err = json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			_, err = c.DoTask(workflow.TaskCall{Site: fmt.Sprint("review-", i), TaskType: "input:collect_user_input", ContractVersion: 1, Command: raw}, func(context.Context) (jobdb.TaskData, error) {
				return workerops.NewInvocationTaskData(workerops.ActivityInvocationRequest{Input: value})
			})
			if err != nil {
				return nil, err
			}
		}
		return jobdb.NewTaskDataOrPanic(map[string]any{"completed": true}), nil
	}}
	run, err := workflow.GetJobForRun(ctx, f.runtime, workflow.GetJobForRunRequest{JobKey: jobKey, JobWorker: worker, WorkerID: "history-fixture", LeaseDuration: time.Minute})
	if err != nil {
		return err
	}
	_, err = run.Run(nil)
	return err
}
func (f *fixture) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var err error
	switch r.URL.Path {
	case "/api/__test/history/advance":
		err = f.advance(r.Context())
	case "/api/__test/history/reopen":
		err = f.runtime.Close(r.Context())
		if err == nil {
			err = f.open(r.Context())
		}
	case "/api/__test/history/hide":
		_, err = f.runtime.DeleteJobs(r.Context(), jobdb.DeleteJobsRequest{TenantID: tenant, JobIDs: []string{jobKey.JobId}})
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func main() {
	addr := flag.String("addr", "127.0.0.1:18085", "local test address")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx = testjobdb.Context(ctx)
	dir, err := os.MkdirTemp("", "cortex-history-browser-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	f := &fixture{path: filepath.Join(dir, "history.db")}
	if err = f.open(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.runtime.Close(context.Background()) }()
	if err = f.seed(ctx); err != nil {
		log.Fatal(err)
	}
	handler := testjobdb.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/__test/history/advance" || r.URL.Path == "/api/__test/history/reopen" || r.URL.Path == "/api/__test/history/hide" {
			f.control(w, r)
			return
		}
		f.server.Load().ServeHTTP(w, r)
	}))
	server := &http.Server{Addr: *addr, Handler: handler}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("history browser fixture listening on %s; retained SQLite at %s", *addr, f.path)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
