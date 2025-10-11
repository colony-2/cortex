package storybuilder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	et "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
	inputop "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	recipeop "github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	recipecore "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/story"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	operatorservicepb "go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestStoryBuilder_WithEmbeddedTemporal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping embedded temporal integration in short mode")
	}

	port := et.FindFreePort()
	dbPath := filepath.Join(t.TempDir(), "temporal.db")
	const namespace = "story-builder"

	serverOpts := et.Options{
		FrontendIP:               "127.0.0.1",
		FrontendPort:             port,
		UIPort:                   0,
		DatabaseFile:             dbPath,
		Namespaces:               []string{namespace},
		DisableScanners:          true,
		DisableParentClosePolicy: true,
		DisableNexus:             true,
		EnableInternalWorker:     false,
		LogLevel:                 "error",
	}

	srv, err := et.NewServer(serverOpts)
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

	// register core ops used in the recipe
	ops.Register(inputop.GetOp(), recipeop.GetOp(), newTestInlineOp(), newTestActivityOp())

	parentRecipe, err := loadRecipeFromYAML(parentRecipeYAML)
	require.NoError(t, err)
	parentMeta := parentRecipe.Recipe.GetMetdata()
	t.Logf("parent recipe meta: id=%s nodeID=%s", parentMeta.ID, parentMeta.NodeMetadata.ID)
	if seq, ok := parentRecipe.Recipe.RecipeImpl.(*recipecore.RecipeSequence); ok {
		for _, node := range seq.SequenceData.Sequence {
			if stateNode, ok := node.NodeImpl.(*recipecore.NodeState); ok && stateNode.StateData.States != nil {
				if st, exists := stateNode.StateData.States.States["start"]; exists && len(st.Transitions) == 0 {
					st.Transitions = []recipecore.Transition{{To: "finish"}}
					stateNode.StateData.States.States["start"] = st
				}
			}
		}
	}
	childRecipe, err := loadRecipeFromYAML(childRecipeYAML)
	require.NoError(t, err)

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	registry.SetDependencies(ops.NewServiceDepsBuilder().Build())

	taskQueue := "story-builder-task-queue"
	temporalWorker := worker.New(temporalClient, taskQueue, worker.Options{})

	registry.EnableActivitiesInWorker(temporalWorker)

	temporalWorker.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, parentRecipe.Recipe, input)
	}, workflow.RegisterOptions{Name: "story-parent"})

	temporalWorker.RegisterWorkflowWithOptions(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, childRecipe.Recipe, input)
	}, workflow.RegisterOptions{Name: "story-child"})

	require.NoError(t, temporalWorker.Start())
	t.Cleanup(func() { temporalWorker.Stop() })
	time.Sleep(500 * time.Millisecond)

	repoPath, repoHash := initGitRepo(t)

	workflowInputs := map[string]interface{}{
		"basegitrepo": repoPath,
		"basegithash": repoHash,
		"ticketid":    "TICKET-123",
		"cellname":    "cell-123",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	we, err := temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        fmt.Sprintf("story-builder-%d", time.Now().UnixNano()),
		TaskQueue: taskQueue,
	}, "story-parent", workflowInputs)
	require.NoError(t, err)
	t.Logf("workflow started: id=%s run=%s", we.GetID(), we.GetRunID())

	time.Sleep(2 * time.Second)
	idFromAttributes := waitForInputInvocationFromSearchAttributes(t, temporalClient, namespace, we.GetID(), we.GetRunID())
	t.Logf("inline invocation from search attributes: %q", idFromAttributes)
	idFromHistory := waitForInputInvocationID(t, temporalClient, namespace, we.GetID(), we.GetRunID())
	t.Logf("inline invocation from history: %q", idFromHistory)
	baseComputedID := computeInputInvocationID(parentRecipe)
	t.Logf("computed base invocation id: %s", baseComputedID)
	invocationID := baseComputedID
	signalIDs := []string{invocationID}
	if idFromAttributes != "" {
		signalIDs = append(signalIDs, idFromAttributes)
		require.Equal(t, invocationID, idFromAttributes)
	}
	if idFromHistory != "" {
		signalIDs = append(signalIDs, idFromHistory)
		require.Equal(t, invocationID, idFromHistory)
	}
	signalIDs = uniqueStrings(signalIDs)
	t.Logf("using %d invocation ids", len(signalIDs))
	debugDescCtx, cancelDebugDesc := context.WithTimeout(context.Background(), 5*time.Second)
	debugDesc, debugErr := temporalClient.WorkflowService().DescribeWorkflowExecution(debugDescCtx, &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: namespace,
		Execution: &commonpb.WorkflowExecution{WorkflowId: we.GetID(), RunId: we.GetRunID()},
	})
	cancelDebugDesc()
	if debugErr == nil && debugDesc != nil {
		pending := debugDesc.GetPendingActivities()
		t.Logf("pending activities count: %d", len(pending))
		for idx, act := range pending {
			t.Logf("pending[%d]: state=%s", idx, act.GetState().String())
		}
		t.Logf("pending children count: %d", len(debugDesc.PendingChildren))
		if info := debugDesc.WorkflowExecutionInfo; info != nil && info.SearchAttributes != nil {
			t.Logf("workflow status: %s", info.Status.String())
			if info.StartTime != nil {
				t.Logf("workflow start: %s", info.StartTime.AsTime())
			}
			if info.CloseTime != nil {
				t.Logf("workflow close: %s", info.CloseTime.AsTime())
			}
			keys := make([]string, 0, len(info.SearchAttributes.IndexedFields))
			for key := range info.SearchAttributes.IndexedFields {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			t.Logf("search attribute keys: %v", keys)
			if info.Memo != nil {
				memoKeys := make([]string, 0, len(info.Memo.Fields))
				for key := range info.Memo.Fields {
					memoKeys = append(memoKeys, key)
				}
				sort.Strings(memoKeys)
				t.Logf("memo keys: %v", memoKeys)
			}
		}
	}
	signalPayload := inputop.UserResponseSignal{
		Fields:      map[string]interface{}{"answer": "yes"},
		UserID:      "tester",
		RespondedAt: time.Now(),
	}
	workflowID := we.GetID()
	runID := we.GetRunID()
	for _, candidate := range uniqueStrings(signalIDs) {
		signalName := fmt.Sprintf("user-response:%s", candidate)
		for _, attempt := range []struct {
			run string
			msg string
		}{{runID, "exact-run"}, {"", "latest-run"}} {
			signalCtx, cancelSignal := context.WithTimeout(context.Background(), 10*time.Second)
			err := temporalClient.SignalWorkflow(signalCtx, workflowID, attempt.run, signalName, signalPayload)
			cancelSignal()
			if err != nil {
				t.Logf("signal %s (%s) failed: %v", signalName, attempt.msg, err)
			} else {
				t.Logf("signal %s (%s) sent", signalName, attempt.msg)
			}
		}
	}
	deadlineSignals := time.Now().Add(5 * time.Second)
	var signalNames []string
	for time.Now().Before(deadlineSignals) {
		signalNames = collectSignalNames(t, temporalClient, namespace, we.GetID(), we.GetRunID())
		if len(signalNames) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Logf("signals observed: %v", signalNames)

	var workflowOutput map[string]interface{}
	require.NoError(t, we.Get(ctx, &workflowOutput))

	descCtx, cancelDesc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelDesc()
	desc, err := temporalClient.WorkflowService().DescribeWorkflowExecution(descCtx, &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: namespace,
		Execution: &commonpb.WorkflowExecution{
			WorkflowId: we.GetID(),
			RunId:      we.GetRunID(),
		},
	})
	require.NoError(t, err)

	historyCtx, cancelHistory := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelHistory()
	historyIter := temporalClient.GetWorkflowHistory(historyCtx, we.GetID(), we.GetRunID(), false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	builder := New("story-parent", &parentRecipe.Recipe)
	builder.SetExecutionInfo(desc)
	for historyIter.HasNext() {
		event, err := historyIter.Next()
		require.NoError(t, err)
		builder.Process(event)
	}
	story := builder.Build()
	require.NotNil(t, story)
	require.Equal(t, "story-parent", story.Metadata.RecipeName)
	require.Equal(t, "completed", story.Metadata.Status)
	require.Equal(t, we.GetID(), story.Metadata.WorkflowID)
	require.Equal(t, we.GetRunID(), story.Metadata.RunID)
	require.False(t, story.Metadata.StartedAt.IsZero())
	require.NotNil(t, story.Metadata.CompletedAt)
	require.NotNil(t, story.Metadata.Duration)
	require.Greater(t, story.Metadata.Duration.Milliseconds(), int64(0))

	inlineNode := findNodeByPath(t, story.Nodes, "story-parent/story-parent/inline-node")
	require.NotNil(t, inlineNode)
	require.Len(t, inlineNode.Runs, 1)
	inlineRun := inlineNode.Runs[0]
	require.Equal(t, "completed", inlineRun.Status)
	require.Equal(t, "inline", inlineRun.Outputs["echo"])
	require.Equal(t, "inline", inlineRun.Inputs["value"])
	require.NotNil(t, findEventByKind(inlineNode.Events, "inline-start"))
	require.NotNil(t, findEventByKind(inlineNode.Events, "inline-completed"))

	activityNode := findNodeByPath(t, story.Nodes, "story-parent/story-parent/activity-node")
	require.NotNil(t, activityNode)
	require.Len(t, activityNode.Runs, 1)
	activityRun := activityNode.Runs[0]
	require.Equal(t, "completed", activityRun.Status)
	require.Equal(t, "PING", activityRun.Outputs["upper"])
	require.NotNil(t, findEventByKind(activityNode.Events, "activity-scheduled"))
	require.NotNil(t, findEventByKind(activityNode.Events, "activity-started"))
	require.NotNil(t, findEventByKind(activityNode.Events, "activity-completed"))

	inputNode := findNodeByPath(t, story.Nodes, "story-parent/story-parent/input-node")
	require.NotNil(t, inputNode)
	require.Len(t, inputNode.Runs, 1)
	inputRun := inputNode.Runs[0]
	require.Equal(t, "completed", inputRun.Status)
	require.Equal(t, invocationID, inputRun.InvocationID)
	fields, ok := inputRun.Outputs["fields"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "yes", fields["answer"])
	inputEvent := findEventByKind(inputNode.Events, "input-response")
	require.NotNil(t, inputEvent)
	require.Equal(t, fmt.Sprintf("user-response:%s", invocationID), inputEvent.Data["signal"])
	eventFields, ok := inputEvent.Data["fields"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "yes", eventFields["answer"])
	require.Equal(t, "tester", inputEvent.Data["user_id"])

	childNode := findNodeByPath(t, story.Nodes, "story-parent/story-parent/child-node")
	require.NotNil(t, childNode)
	require.Len(t, childNode.Runs, 1)
	childRun := childNode.Runs[0]
	require.Equal(t, "completed", childRun.Status)
	require.NotNil(t, childRun.Outputs)
	require.NotNil(t, childRun.Inputs)
	require.NotEmpty(t, childRun.Inputs)
	require.NotEmpty(t, childRun.ChildWorkflowID)
	require.NotEmpty(t, childRun.ChildRunID)
	require.Equal(t, "story-child", childRun.ChildRecipeName)
	require.NotNil(t, childRun.StartedAt)
	require.NotNil(t, childRun.CompletedAt)
	childResult, ok := childRun.Outputs["result"].(map[string]interface{})
	require.True(t, ok)
	childInline, ok := childResult["child-inline"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "child", childInline["echo"])
	childTriggerEvent := findEventByKind(childNode.Events, "child-trigger")
	require.NotNil(t, childTriggerEvent)
	require.NotEmpty(t, childTriggerEvent.Data["child_workflow_id"])
	require.NotEmpty(t, childTriggerEvent.Data["child_run_id"])
	childResultEvent := findEventByKind(childNode.Events, "child-result")
	require.NotNil(t, childResultEvent)
	require.Equal(t, childTriggerEvent.Data["child_workflow_id"], childResultEvent.Data["child_workflow_id"])
	require.Equal(t, "completed", childResultEvent.Data["status"])
	require.Equal(t, "story-child", childResultEvent.Data["recipe_name"])

	stateMachineNode := findNodeByPath(t, story.Nodes, "story-parent/story-parent/state-machine")
	require.NotNil(t, stateMachineNode)
	stateTransition := findEventByKind(stateMachineNode.Events, "state-transition")
	require.NotNil(t, stateTransition)
	require.Equal(t, "start", stateTransition.Data["from"])
	require.Equal(t, "finish", stateTransition.Data["to"])
	stateTimeline := findTimelineEntryByKind(story.Timeline, "state-transition")
	require.NotNil(t, stateTimeline)
	require.Equal(t, fmt.Sprintf("node:%s", stateMachineNode.Path), stateTimeline.Ref)

	require.NotEmpty(t, story.Timeline)
	for i := 1; i < len(story.Timeline); i++ {
		prev := story.Timeline[i-1]
		curr := story.Timeline[i]
		if !prev.At.IsZero() && !curr.At.IsZero() {
			require.Falsef(t, curr.At.Before(prev.At), "timeline out of order at %d", i)
		}
		if prev.At.Equal(curr.At) || prev.At.IsZero() || curr.At.IsZero() {
			require.Truef(t, curr.EventID >= prev.EventID, "event ids not monotonic at %d", i)
		}
	}
	inputTimeline := findTimelineEntryByKind(story.Timeline, "input-response")
	require.NotNil(t, inputTimeline)
	require.Equal(t, fmt.Sprintf("node:%s", inputNode.Path), inputTimeline.Ref)
	timelineFields, ok := inputTimeline.Data["fields"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "yes", timelineFields["answer"])
	childTriggerEntry := findTimelineEntryByKind(story.Timeline, "child-trigger")
	require.NotNil(t, childTriggerEntry)
	require.Equal(t, fmt.Sprintf("node:%s", childNode.Path), childTriggerEntry.Ref)
	childResultEntry := findTimelineEntryByKind(story.Timeline, "child-result")
	require.NotNil(t, childResultEntry)
	require.Equal(t, childTriggerEntry.Data["child_workflow_id"], childResultEntry.Data["child_workflow_id"])
	require.Equal(t, fmt.Sprintf("node:%s", childNode.Path), childResultEntry.Ref)

	require.NotNil(t, workflowOutput)
	inlineOutput, ok := workflowOutput["inline-node"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "inline", inlineOutput["echo"])
	activityOutput, ok := workflowOutput["activity-node"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "PING", activityOutput["upper"])
	childOutput, ok := workflowOutput["child-node"].(map[string]interface{})
	require.True(t, ok)
	childResultMap, ok := childOutput["result"].(map[string]interface{})
	require.True(t, ok)
	childInlineOutput, ok := childResultMap["child-inline"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "child", childInlineOutput["echo"])
	inputOutput, ok := workflowOutput["input-node"].(map[string]interface{})
	require.True(t, ok)
	inputOutputFields, ok := inputOutput["fields"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "yes", inputOutputFields["answer"])
}

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

func computeInputInvocationHash(r *recipecore.RecipeFile) string {
	if r == nil {
		return ""
	}
	meta := r.Recipe.GetMetdata()
	recipeID := meta.ID
	if recipeID == "" {
		recipeID = r.ID
	}
	segment := meta.NodeMetadata.ID
	if segment == "" {
		segment = recipeID
		if segment == "" {
			segment = "recipe-sequence"
		}
	}
	nodeID := "input-node"
	boxID := ""
	activityID := ""
	if seq, ok := r.Recipe.RecipeImpl.(*recipecore.RecipeSequence); ok {
		for _, node := range seq.SequenceData.Sequence {
			nodeMeta := node.GetMetadata()
			if nodeMeta.ID != nodeID {
				continue
			}
			if nodeMeta.Inputs != nil {
				if v, ok := nodeMeta.Inputs["box_id"].(string); ok {
					boxID = v
				}
				if v, ok := nodeMeta.Inputs["activity_id"].(string); ok {
					activityID = v
				}
			}
			break
		}
	}
	inv := ops.Invocation{
		RecipeID:   recipeID,
		NodePath:   fmt.Sprintf("%s/%s", segment, nodeID),
		InvokeSeq:  0,
		BoxID:      boxID,
		ActivityID: activityID,
	}
	return inv.Hash()
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

func newTestActivityOp() ops.RegisterableOp {
	type activityIn struct {
		Msg string `json:"msg"`
	}
	type activityOut struct {
		Upper string `json:"upper"`
	}

	return ops.NewActivityMappedOpV2[activityIn, activityOut](ops.OpMetadata{
		Type:           "story.activity",
		Description:    "Test activity op",
		Version:        "1.0.0",
		DefaultTimeout: time.Second * 10,
	}, func(inv ops.Invocation, ctx context.Context, in activityIn) (activityOut, error) {
		return activityOut{Upper: strings.ToUpper(in.Msg)}, nil
	})
}

func findNodeByPath(t *testing.T, nodes []*StoryNode, path string) *StoryNode {
	if result := recurseFindNode(nodes, path); result != nil {
		return result
	}
	t.Fatalf("node %s not found", path)
	return nil
}

func recurseFindNode(nodes []*StoryNode, path string) *StoryNode {
	for _, node := range nodes {
		if node.Path == path {
			return node
		}
		if len(node.Children) > 0 {
			if found := recurseFindNode(node.Children, path); found != nil {
				return found
			}
		}
	}
	return nil
}

func initGitRepo(t *testing.T) (string, string) {
	dir := t.TempDir()
	require.NoError(t, run(dir, "git", "init"))
	require.NoError(t, run(dir, "git", "config", "user.email", "test@example.com"))
	require.NoError(t, run(dir, "git", "config", "user.name", "Test User"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o644))
	require.NoError(t, run(dir, "git", "add", "."))
	require.NoError(t, run(dir, "git", "commit", "-m", "init"))
	hashBytes, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
	require.NoError(t, err)
	return dir, string(bytes.TrimSpace(hashBytes))
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w (%s)", name, args, err, out)
	}
	return nil
}

func computeInputInvocationID(recipeFile *recipecore.RecipeFile) string {
	if recipeFile == nil {
		return ""
	}
	meta := recipeFile.Recipe.GetMetdata()
	recipeID := meta.ID
	if recipeID == "" {
		recipeID = recipeFile.ID
	}
	if recipeID == "" {
		recipeID = "recipe"
	}
	segment := meta.NodeMetadata.ID
	if segment == "" {
		segment = recipeID
	}
	nodePath := fmt.Sprintf("%s/%s/%s", recipeID, segment, "input-node")
	inv := ops.Invocation{
		RecipeID:   recipeID,
		NodePath:   nodePath,
		InvokeSeq:  0,
		BoxID:      "input-box",
		ActivityID: "input-activity",
	}
	return inv.Hash()
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		unique = append(unique, v)
	}
	return unique
}

func collectSignalNames(t *testing.T, temporalClient client.Client, namespace, workflowID, runID string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	iter := temporalClient.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	names := make([]string, 0, 8)
	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			t.Logf("history iteration for signals failed: %v", err)
			break
		}
		if attrs := event.GetWorkflowExecutionSignaledEventAttributes(); attrs != nil {
			names = append(names, attrs.GetSignalName())
		}
	}
	return uniqueStrings(names)
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
			// Attributes may already exist; try to ignore duplicates.
			t.Logf("add search attributes returned: %v", err)
			return nil
		}
		return err
	}
	return nil
}

func waitForInputInvocationFromSearchAttributes(t *testing.T, temporalClient client.Client, namespace, workflowID, runID string) string {
	deadline := time.Now().Add(30 * time.Second)
	dc := converter.GetDefaultDataConverter()
	for time.Now().Before(deadline) {
		descCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		desc, err := temporalClient.WorkflowService().DescribeWorkflowExecution(descCtx, &workflowservice.DescribeWorkflowExecutionRequest{
			Namespace: namespace,
			Execution: &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: runID},
		})
		cancel()
		if err == nil && desc != nil && desc.WorkflowExecutionInfo != nil {
			if sa := desc.WorkflowExecutionInfo.GetSearchAttributes(); sa != nil {
				if payload, ok := sa.GetIndexedFields()["InputKey"]; ok && payload != nil {
					var id string
					if err := dc.FromPayload(payload, &id); err == nil && id != "" {
						return id
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return ""
}

func waitForInputInvocationID(t *testing.T, temporalClient client.Client, namespace, workflowID, runID string) string {
	deadline := time.Now().Add(30 * time.Second)
	_ = namespace
	for time.Now().Before(deadline) {
		historyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		iter := temporalClient.GetWorkflowHistory(historyCtx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
		for iter.HasNext() {
			event, err := iter.Next()
			if err != nil {
				cancel()
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					break
				}
				t.Fatalf("history iteration failed: %v", err)
			}
			attrs := event.GetMarkerRecordedEventAttributes()
			if attrs == nil || attrs.MarkerName != "SideEffect" {
				continue
			}
			env, err := story.DecodeMarkerEnvelope(attrs.Details, nil)
			if err != nil || env == nil {
				continue
			}
			if env.Kind != "inline-op" || env.Stage != "start" {
				continue
			}
			payloadMap, ok := env.Payload.(map[string]interface{})
			if !ok {
				t.Logf("unexpected inline payload type: %T", env.Payload)
				continue
			}
			if opType, ok := payloadMap["op_type"].(string); !ok || opType != "input" {
				t.Logf("inline marker seen for op %v at path %q recipe %q seq %d", payloadMap["op_type"], env.Invocation.NodePath, env.Invocation.RecipeID, env.Invocation.Seq)
				continue
			}
			cancel()
			if env.Invocation.ID != "" {
				return env.Invocation.ID
			}
			inv := ops.Invocation{
				RecipeID:  env.Invocation.RecipeID,
				NodePath:  env.Invocation.NodePath,
				InvokeSeq: env.Invocation.Seq,
			}
			return inv.Hash()
		}
		cancel()
		time.Sleep(200 * time.Millisecond)
	}
	return ""
}

const parentRecipeYAML = `id: story-parent
version: 1.0.0
sequence:
  - id: inline-node
    op: story.inline
    inputs:
      value: "inline"
  - id: activity-node
    op: story.activity
    inputs:
      msg: "ping"
  - id: child-node
    op: recipe
    inputs:
      name: story-child
      inputs:
        basegitrepo: "{{ inputs.basegitrepo }}"
        basegithash: "{{ inputs.basegithash }}"
        ticketid: "{{ inputs.ticketid }}"
        cellname: "{{ inputs.cellname }}"
  - id: input-node
    op: input
    inputs:
      box_id: input-box
      activity_id: input-activity
      config:
        question: "Continue?"
        type: short_answer
  - id: state-machine
    state:
      initial: start
      states:
        start:
          sequence:
            - id: state-start-task
              op: story.inline
              inputs:
                value: "inline"
          transitions:
            - to: finish
              when: true
        finish:
          sequence:
            - id: state-finish-task
              op: story.inline
              inputs:
                value: "finish"
`

const childRecipeYAML = `id: story-child
version: 1.0.0
sequence:
  - id: child-inline
    op: story.inline
    inputs:
      value: "child"
`
