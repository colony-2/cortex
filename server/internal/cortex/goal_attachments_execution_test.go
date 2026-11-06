package cortex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/cellref"
	"github.com/colony-2/c2j/pkg/contextual"
	"github.com/colony-2/c2j/pkg/execution"
	"github.com/colony-2/c2j/pkg/executionruntime"
	gitops "github.com/colony-2/c2j/pkg/git/export"
	inputops "github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/jobdbschema"
	coreops "github.com/colony-2/c2j/pkg/ops"
	extops "github.com/colony-2/c2j/pkg/ops/extensions"
	recipeops "github.com/colony-2/c2j/pkg/ops/recipe"
	"github.com/colony-2/c2j/pkg/recipe"
	"github.com/colony-2/c2j/pkg/starter"
	"github.com/colony-2/c2j/pkg/worker/compiler"
	workerexport "github.com/colony-2/c2j/pkg/worker/export"
	workerops "github.com/colony-2/c2j/pkg/worker/ops"
	workerworkflow "github.com/colony-2/c2j/pkg/worker/workflow"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

// An explicitly empty test provider handles optional recipe secret lookups.
// No production credentials are read or passed into the fixture.
type emptyAttachmentSecrets struct{}

func (emptyAttachmentSecrets) ResolveSecrets(_ context.Context, _ jobdb.SecretResolveRequest) (jobdb.SecretResolveResult, error) {
	return jobdb.SecretResolveResult{Secrets: map[string]jobdb.SecretValue{}}, nil
}

// End at the first consuming operation; never invoke an external model or publish.
func runGoalAttachmentConsumer(t *testing.T, selector string, embedded ...recipe.Recipe) {
	t.Helper()
	git, e := exec.LookPath("git")
	if e != nil {
		t.Fatal(e)
	}
	// Use an isolated source repository. Cloning Cortex itself also loads its
	// .c2/config.yaml and installs build tools unrelated to attachment delivery.
	repoRoot := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main", repoRoot},
		{"-C", repoRoot, "-c", "user.name=Cortex Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "Attachment fixture"},
	} {
		if output, err := exec.Command(git, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	bin := t.TempDir()
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	shim := "#!/bin/sh\nexec " + quote(git) + " -c " + quote("url.file://"+repoRoot+".insteadOf=https://github.com/acme/platform.git") + " -c " + quote("url.file://"+repoRoot+".insteadOf=https://github.com/acme/platform") + " \"$@\"\n"
	if e := os.WriteFile(filepath.Join(bin, "git"), []byte(shim), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(testjobdb.Context(context.Background()), 90*time.Second)
	defer cancel()
	rt := toy.New(toy.WithSecretProvider(emptyAttachmentSecrets{}))
	_, err := rt.CreateCell(ctx, jobdb.CreateCellRequest{TenantID: "attachment-execution", Name: "platform", Repo: "github.com/acme/platform"})
	if err != nil {
		t.Fatal(err)
	}
	scoped := executionruntime.New(rt, execution.Allocation{SchemaVersion: 1})
	rawEngine, err := jobworkflow.NewEngineBuilder().WithRuntime(scoped).WithWorkerTenantId("attachment-execution").BuildEngine()
	if err != nil {
		t.Fatal(err)
	}
	engine := jobdbschema.WorkflowEngine{Engine: rawEngine, Registry: rt}
	ctl := &workerworkflow.SWFWorkflowControl{Engine: engine}
	deps := coreops.NewServiceDepsBuilder().WithWorkflowControl(ctl).Build()
	// Match the CLI's Git operation catalog, including unvisited later states.
	originalOps := coreops.List()
	t.Cleanup(func() { coreops.Replace(originalOps...) })
	allOps := append(gitops.GetAll(), inputops.GetOp(), inputops.GetAutoFillOp(), inputops.GetAlternateOp(), extops.GetExecutionOp())
	allOps = append(allOps, recipeops.GetOps()...)
	allOps = append(allOps, workerexport.GetAll()...)
	for _, op := range allOps {
		if _, exists := coreops.Get(op.GetMetadata().Type); !exists {
			coreops.Register(op)
		}
	}
	registry, err := workerops.NewActivityRegistry()
	if err != nil {
		t.Fatal(err)
	}
	consumed := make(chan error, 1)
	for name, registration := range registry.GetAll() {
		if registration.Metadata.Type != "extension_execution" && registration.Metadata.Type != "command_execution" {
			continue
		}
		reg := registration
		original := reg.Step.Invoke
		reg.Step.Invoke = func(deps coreops.OpDependencies, ctx context.Context, in map[string]any) (map[string]any, error) {
			if reg.Metadata.Type == "command_execution" && len(embedded) == 0 {
				return original(deps, ctx, in)
			}
			paths, ok := deps.(coreops.OperationPathProvider)
			if !ok {
				consumed <- fmt.Errorf("missing operation paths")
				return nil, fmt.Errorf("stop fixture")
			}
			for name, want := range map[string]string{"brief.bin": "\x00\xff\x01", "雪.txt": "snow", "empty": ""} {
				actual, e := os.ReadFile(filepath.Join(paths.OperationPaths().Inbox, name))
				if e != nil || string(actual) != want {
					consumed <- fmt.Errorf("inbox %s: bytes %x error %v", name, actual, e)
					return nil, fmt.Errorf("stop fixture")
				}
			}
			t.Logf("Verified brief.bin (00ff01), 雪.txt (snow), and empty at consuming operation %s", deps.GitContext().NodePath)
			consumed <- nil
			return nil, fmt.Errorf("fixture stops after consuming initial attachments")
		}
		registry.UpdateRegistration(name, reg)
	}
	workers, err := compiler.NewRecipeWorkerWithOptions(deps, registry, compiler.RecipeJobWorkerOptions{RootSourceResolver: compiler.NewRecipeSourceResolver(compiler.RecipeSourceResolverOptions{}), Allocation: scoped.Allocation, WrapTaskWorker: scoped.WrapTaskWorker, StageExecution: scoped.Stage})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterWorkers(workers); err != nil {
		t.Fatal(err)
	}
	key, err := starter.StartRecipeJob(ctx, workflowctl.StartJob{Cell: "platform", CellRepo: "github.com/acme/platform", TenantId: "attachment-execution", RecipeName: selector, Inputs: map[string]any{"prompt": "Read attached requirements", "type": "build"}, GitRef: "HEAD", Artifacts: []jobdb.Artifact{jobdb.NewArtifactFromBytes("brief.bin", []byte{0, 255, 1}), jobdb.NewArtifactFromBytes("雪.txt", []byte("snow")), jobdb.NewArtifactFromBytes("empty", nil)}, JobContext: contextual.JobContext{Workflow: contextual.WorkflowContext{CellName: "platform"}, GitBase: contextual.GitBaseContext{BaseRepo: repoRoot, BaseRef: "HEAD"}, CellResolution: &cellref.Context{SelfCell: "platform", SelfRepo: "github.com/acme/platform"}}}, engine, embedded...)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); engine.Run(ctx) }()
	defer func() { cancel(); <-done }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			inspection, e := ctl.InspectJob(ctx, key.Key())
			if e != nil {
				t.Fatal(e)
			}
			if inspection.Terminal {
				select {
				case err := <-consumed:
					if err != nil {
						t.Fatal(err)
					}
					return
				default:
					t.Fatalf("job ended before consumer: %+v", inspection)
				}
			}
		case err := <-consumed:
			if err != nil {
				t.Fatal(err)
			}
			return
		case <-ctx.Done():
			t.Fatal("consumer not reached:", ctx.Err())
		}
	}
}

func TestGoalAttachmentRecipeContext(t *testing.T) {
	rec, err := recipe.LoadRecipeFromString([]byte(`id: attached-context
version: "1"
input_schema:
  prompt: {type: string}
  type: {type: string}
op: command_execution
artifacts:
  ./: '${{ context.artifacts }}'
inputs:
  run: "true"
`))
	if err != nil {
		t.Fatal(err)
	}
	runGoalAttachmentConsumer(t, "attached-context", *rec)
}

func TestGoalAttachmentNormalBuildForwarding(t *testing.T) {
	if os.Getenv("CORTEX_TEST_NORMAL_BUILD") != "1" {
		t.Skip("set CORTEX_TEST_NORMAL_BUILD=1 for pinned recipe integration")
	}
	runGoalAttachmentConsumer(t, "git+https://github.com/colony-2/recipes.git//build.yaml@531599adc1e95134ed6a552d0b2a96dfd3f84d25")
}
