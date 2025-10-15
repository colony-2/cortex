package rewindpath

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	et "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
	inputop "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	recipeop "github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	recipecore "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	runmetadata "github.com/divisive-ai/vibethis/server/recipe-core/pkg/runmetadata"
	"github.com/divisive-ai/vibethis/server/recipe-history/pkg/history"
	"github.com/divisive-ai/vibethis/server/recipe-history/pkg/storybuilder"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
	operatorservicepb "go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestExecutionPathBuilder_WithEmbeddedTemporal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rewind integration in short mode")
	}

	port := et.FindFreePort()
	dbPath := filepath.Join(t.TempDir(), "temporal.db")
	const namespace = "rewind-path"

	opts := et.Options{
		FrontendIP:               "127.0.0.1",
		FrontendPort:             port,
		DatabaseFile:             dbPath,
		Namespaces:               []string{namespace},
		DisableScanners:          true,
		DisableParentClosePolicy: true,
		DisableNexus:             true,
		EnableInternalWorker:     false,
		LogLevel:                 "error",
	}

	srv, err := et.NewServer(opts)
	require.NoError(t, err)
	require.NoError(t, srv.Start())
	t.Cleanup(func() { _ = srv.Stop() })

	nsClient, err := client.NewNamespaceClient(client.Options{HostPort: srv.GetFrontendAddress()})
	require.NoError(t, err)
	defer nsClient.Close()
	registerCtx, cancelRegister := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelRegister()
	if regErr := nsClient.Register(registerCtx, &workflowservice.RegisterNamespaceRequest{
		Namespace:                        namespace,
		WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
	}); regErr != nil {
		var already *serviceerror.NamespaceAlreadyExists
		if !errors.As(regErr, &already) {
			require.NoError(t, regErr)
		}
	}

	temporalClient, err := et.NewClient(et.ClientOptions{HostPort: srv.GetFrontendAddress(), Namespace: namespace})
	require.NoError(t, err)
	t.Cleanup(func() { temporalClient.Close() })

	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := temporalClient.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{Namespace: namespace})
		return err == nil
	}, 30*time.Second, 200*time.Millisecond, "namespace not ready")

	require.NoError(t, ensureInputSearchAttributes(t, temporalClient, namespace))

	ops.Register(inputop.GetOp(), recipeop.GetOp(), newTestInlineOp())

	rootRecipe, err := loadRecipeFromYAML(rootRecipeYAML)
	require.NoError(t, err)
	midRecipe, err := loadRecipeFromYAML(midRecipeYAML)
	require.NoError(t, err)
	leafRecipe, err := loadRecipeFromYAML(leafRecipeYAML)
	require.NoError(t, err)

	recipeMap := map[string]*recipecore.RecipeFile{
		rootRecipe.Recipe.GetMetdata().ID: rootRecipe,
		midRecipe.Recipe.GetMetdata().ID:  midRecipe,
		leafRecipe.Recipe.GetMetdata().ID: leafRecipe,
	}
	// Allow lookups by workflow registration name as well.
	recipeMap["rewind-root"] = rootRecipe
	recipeMap["rewind-mid"] = midRecipe
	recipeMap["rewind-leaf"] = leafRecipe

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	registry.SetDependencies(ops.NewServiceDepsBuilder().Build())

	taskQueue := "rewind-path-task-queue"
	temporalWorker := worker.New(temporalClient, taskQueue, worker.Options{})
	registry.EnableActivitiesInWorker(temporalWorker)

	temporalWorker.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, rootRecipe.Recipe, input)
	}, workflow.RegisterOptions{Name: "rewind-root"})

	temporalWorker.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, midRecipe.Recipe, input)
	}, workflow.RegisterOptions{Name: "rewind-mid"})

	temporalWorker.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, leafRecipe.Recipe, input)
	}, workflow.RegisterOptions{Name: "rewind-leaf"})

	require.NoError(t, temporalWorker.Start())
	t.Cleanup(func() { temporalWorker.Stop() })
	time.Sleep(500 * time.Millisecond)

	workflowInputs := map[string]interface{}{
		"basegitrepo": "placeholder",
		"basegithash": "HEAD",
		"ticketid":    "TICKET-456",
		"cellname":    "cell-123",
		"payload":     "initial",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	rootWorkflowID1 := fmt.Sprintf("rewind-root-%d", time.Now().UnixNano())
	we1, err := temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        rootWorkflowID1,
		TaskQueue: taskQueue,
	}, "rewind-root", workflowInputs)
	require.NoError(t, err)

	metaSignal := runmetadata.Signal{TargetRunID: we1.GetRunID()}
	signalCtx, cancelSignal := context.WithTimeout(context.Background(), 10*time.Second)
	require.NoError(t, temporalClient.SignalWorkflow(signalCtx, rootWorkflowID1, we1.GetRunID(), metadataSignalName, metaSignal))
	cancelSignal()

	var firstOutput map[string]interface{}
	require.NoError(t, we1.Get(ctx, &firstOutput))

	historyClient := history.NewClient(temporalClient, func(name string) (*recipecore.Recipe, error) {
		if file, ok := recipeMap[name]; ok {
			return &file.Recipe, nil
		}
		return nil, fmt.Errorf("recipe %s not found", name)
	}, zap.NewNop())

	rootStory1, err := historyClient.BuildStory(ctx, rootRecipe.Recipe.GetMetdata().ID, rootWorkflowID1, we1.GetRunID())
	require.NoError(t, err)

	rootChild1 := findFirstChildRun(rootStory1.Nodes)
	require.NotNil(t, rootChild1)
	midWorkflowID := rootChild1.ChildWorkflowID
	midRunID1 := rootChild1.ChildRunID
	require.NotEmpty(t, midWorkflowID)
	require.NotEmpty(t, midRunID1)

	midStory1, err := historyClient.BuildStory(ctx, midRecipe.Recipe.GetMetdata().ID, midWorkflowID, midRunID1)
	require.NoError(t, err)

	midChild1 := findFirstChildRun(midStory1.Nodes)
	require.NotNil(t, midChild1)
	leafWorkflowID := midChild1.ChildWorkflowID
	leafRunID1 := midChild1.ChildRunID
	require.NotEmpty(t, leafWorkflowID)
	require.NotEmpty(t, leafRunID1)

	leafStory1, err := historyClient.BuildStory(ctx, leafRecipe.Recipe.GetMetdata().ID, leafWorkflowID, leafRunID1)
	require.NoError(t, err)

	leafTargetRun := findRunByOutput(leafStory1.Nodes, "leaf-target")
	require.NotNil(t, leafTargetRun)
	targetHash := leafTargetRun.InvocationHash
	require.NotEmpty(t, targetHash)

	builder := New(historyClient)
	result, err := builder.Build(ctx, Request{
		LeafRecipeName:       leafRecipe.Recipe.GetMetdata().ID,
		LeafWorkflowID:       leafWorkflowID,
		LeafRunID:            leafRunID1,
		TargetInvocationHash: targetHash,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Segments, 3)

	require.Equal(t, rootChild1.InvocationHash, result.Segments[0].InvocationHash)
	require.EqualValues(t, rootChild1.ResumeEventID, result.Segments[0].EventID)
	require.Equal(t, midWorkflowID, result.Segments[0].WorkflowID)
	require.Equal(t, midRunID1, result.Segments[0].RunID)
	require.NotNil(t, result.Segments[0].WaitForChild)

	require.Equal(t, midChild1.InvocationHash, result.Segments[1].InvocationHash)
	require.EqualValues(t, midChild1.ResumeEventID, result.Segments[1].EventID)
	require.Equal(t, leafWorkflowID, result.Segments[1].WorkflowID)
	require.Equal(t, leafRunID1, result.Segments[1].RunID)
	require.NotNil(t, result.Segments[1].WaitForChild)
	require.True(t, *result.Segments[1].WaitForChild)

	require.Equal(t, targetHash, result.Segments[2].InvocationHash)
	require.Equal(t, leafWorkflowID, result.Segments[2].WorkflowID)
	require.Equal(t, leafRunID1, result.Segments[2].RunID)
	require.EqualValues(t, leafTargetRun.ResumeEventID, result.Segments[2].EventID)

}

// Helper utilities borrowed from storybuilder integration tests.

func loadRecipeFromYAML(data string) (*recipecore.RecipeFile, error) {
	r, err := recipecore.LoadRecipeFromReader(bytes.NewBufferString(data))
	if err != nil {
		return nil, err
	}
	meta := r.GetMetdata()
	if meta.ID == "" {
		meta.ID = "recipe"
	}
	return &recipecore.RecipeFile{ID: meta.ID, Version: meta.Version, Recipe: *r}, nil
}

func ensureInputSearchAttributes(t *testing.T, temporalClient client.Client, namespace string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	attributes := map[string]enumspb.IndexedValueType{
		"InputKey":         enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputStatus":      enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputFormTitle":   enumspb.INDEXED_VALUE_TYPE_TEXT,
		"InputBoxID":       enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputActivityID":  enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputCreatedAt":   enumspb.INDEXED_VALUE_TYPE_DATETIME,
		"InputExpiresAt":   enumspb.INDEXED_VALUE_TYPE_DATETIME,
		"InputRespondedBy": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputRespondedAt": enumspb.INDEXED_VALUE_TYPE_DATETIME,
	}
	_, err := temporalClient.OperatorService().AddSearchAttributes(ctx, &operatorservicepb.AddSearchAttributesRequest{
		Namespace:        namespace,
		SearchAttributes: attributes,
	})
	if err != nil {
		var already *serviceerror.InvalidArgument
		if errors.As(err, &already) {
			t.Logf("search attributes already exist: %v", err)
			return nil
		}
		return err
	}
	return nil
}

func newTestInlineOp() ops.RegisterableOp {
	type inlineIn struct {
		Value string `json:"value"`
	}
	type inlineOut struct {
		Echo string `json:"echo"`
	}

	return ops.NewInlineOpV2[inlineIn, inlineOut](ops.OpMetadata{
		Type:           "story.inline",
		Description:    "Test inline op",
		Version:        "1.0.0",
		DefaultTimeout: time.Second * 10,
	}, func(inv ops.Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, in inlineIn) (inlineOut, error) {
		return inlineOut{Echo: in.Value}, nil
	})
}

func findFirstChildRun(nodes []*storybuilder.StoryNode) *storybuilder.NodeRun {
	for _, node := range nodes {
		for _, run := range node.Runs {
			if run.ChildRunID != "" {
				return run
			}
		}
		if found := findFirstChildRun(node.Children); found != nil {
			return found
		}
	}
	return nil
}

func findRunByOutput(nodes []*storybuilder.StoryNode, expected string) *storybuilder.NodeRun {
	if len(nodes) == 0 {
		return nil
	}
	for _, node := range nodes {
		for _, run := range node.Runs {
			if run.Outputs != nil {
				if val, ok := run.Outputs["echo"].(string); ok && val == expected {
					return run
				}
			}
		}
		if found := findRunByOutput(node.Children, expected); found != nil {
			return found
		}
	}
	return nil
}

const metadataSignalName = "recipe_run_metadata"

const rootRecipeYAML = `id: root-recipe
version: 1.0.0
sequence:
  - id: root-pre
    op: story.inline
    inputs:
      value: "root-pre"
  - id: root-child
    op: recipe
    inputs:
      name: rewind-mid
      inputs:
        basegitrepo: "{{ inputs.basegitrepo }}"
        basegithash: "{{ inputs.basegithash }}"
        ticketid: "{{ inputs.ticketid }}"
        cellname: "{{ inputs.cellname }}"
        payload: "from-root"
  - id: root-post
    op: story.inline
    inputs:
      value: "root-post"
`

const midRecipeYAML = `id: rewind-mid
version: 1.0.0
sequence:
  - id: mid-pre
    op: story.inline
    inputs:
      value: "mid-pre"
  - id: mid-child
    op: recipe
    inputs:
      name: rewind-leaf
      inputs:
        basegitrepo: "{{ inputs.basegitrepo }}"
        basegithash: "{{ inputs.basegithash }}"
        ticketid: "{{ inputs.ticketid }}"
        cellname: "{{ inputs.cellname }}"
        message: "from-mid"
  - id: mid-post
    op: story.inline
    inputs:
      value: "mid-post"
`

const leafRecipeYAML = `id: rewind-leaf
version: 1.0.0
sequence:
  - id: leaf-start
    op: story.inline
    inputs:
      value: "leaf-start"
  - id: leaf-target
    op: story.inline
    inputs:
      value: "leaf-target"
  - id: leaf-end
    op: story.inline
    inputs:
      value: "leaf-end"
`
