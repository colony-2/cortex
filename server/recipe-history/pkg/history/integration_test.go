package history

import (
    "context"
    "encoding/json"
    "testing"
    "time"

    coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
    recipecore "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
    workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
    "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
    serveropsrecipe "github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
    "go.temporal.io/api/common/v1"
    "go.temporal.io/api/enums/v1"
    "go.temporal.io/api/history/v1"
    apiwf "go.temporal.io/api/workflow/v1"
    "go.temporal.io/sdk/activity"
    "go.temporal.io/sdk/converter"
    "go.temporal.io/sdk/testsuite"
    "go.temporal.io/sdk/workflow"
    "google.golang.org/protobuf/types/known/timestamppb"
)

// Integration test using WorkflowTestSuite (no external Temporal server)
func TestHistoryWithWorkflowTestSuite(t *testing.T) {
    t.Parallel()

    // Build a parent recipe with command_execution → recipe(op) → command_execution
    parent := &recipecore.RecipeFile{
        ID: "parent", Version: "1.0.0",
        Recipe: recipecore.Recipe{RecipeImpl: &recipecore.RecipeSequence{
            RecipeMetadata: recipecore.RecipeMetadata{Version: "1.0.0"},
            SequenceData: recipecore.SequenceData{Sequence: []recipecore.Node{
                {NodeImpl: &recipecore.NodeOp{NodeMetadata: recipecore.NodeMetadata{Inputs: map[string]interface{}{"run": "echo parent-hello"}}, OpData: recipecore.OpData{Op: "command_execution"}}},
                {NodeImpl: &recipecore.NodeOp{NodeMetadata: recipecore.NodeMetadata{Inputs: map[string]interface{}{
                    "name":   "child-workflow",
                    "inputs": map[string]interface{}{},
                }}, OpData: recipecore.OpData{Op: "recipe"}}},
                {NodeImpl: &recipecore.NodeOp{NodeMetadata: recipecore.NodeMetadata{Inputs: map[string]interface{}{"run": "echo done"}}, OpData: recipecore.OpData{Op: "command_execution"}}},
            }},
        }},
    }

    // Ensure the inline recipe op from server/ops is registered (override any test-op with same type)
    coreops.Register(serveropsrecipe.GetOp())

    // Activity registry from recipe-worker (loads registered ops, including override)
    registry, err := workerops.NewActivityRegistry()
    if err != nil { t.Fatalf("registry init failed: %v", err) }

    // Temporal test environment
    suite := &testsuite.WorkflowTestSuite{}
    env := suite.NewTestWorkflowEnvironment()

    // Register all activity-mapped ops with the env under their type name
    for typ, reg := range registry.GetAll() {
        if reg.Activity.ExecuteAsActivity() {
            env.RegisterActivityWithOptions(reg.Activity.Execute, activity.RegisterOptions{Name: typ})
        }
    }

    // Register child workflow that parent will invoke via the inline recipe op
    const childWorkflowName = "child-workflow"
    childRecipe := recipecore.Recipe{RecipeImpl: &recipecore.RecipeSequence{
        RecipeMetadata: recipecore.RecipeMetadata{Version: "1.0.0"},
        SequenceData:   recipecore.SequenceData{Sequence: []recipecore.Node{
            {NodeImpl: &recipecore.NodeOp{NodeMetadata: recipecore.NodeMetadata{Inputs: map[string]interface{}{"run": "echo child"}}, OpData: recipecore.OpData{Op: "command_execution"}}},
        }},
    }}
    childWf := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
        return compiler.ExecuteRecipe(ctx, registry, childRecipe, inputs)
    }
    env.RegisterWorkflowWithOptions(childWf, workflow.RegisterOptions{Name: childWorkflowName})

    // Workflow that executes the parent recipe (which will invoke child-workflow)
    wfName := parent.ID + "-workflow"
    wf := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
        return compiler.ExecuteRecipe(ctx, registry, parent.Recipe, inputs)
    }
    env.RegisterWorkflowWithOptions(wf, workflow.RegisterOptions{Name: wfName})

    // Collect synthetic history via listeners
    base := time.Now()
    nextEventID := int64(1)
    scheduledByActID := map[string]int64{}
    var events []*history.HistoryEvent
    env.SetOnActivityStartedListener(func(info *activity.Info, ctx context.Context, args converter.EncodedValues) {
        // SCHEDULED
        schedID := nextEventID; nextEventID++
        scheduledByActID[info.ActivityID] = schedID
        events = append(events, &history.HistoryEvent{
            EventId:   schedID,
            EventTime: timestamppb.New(base.Add(time.Duration(schedID) * time.Millisecond)),
            EventType: enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
            Attributes: &history.HistoryEvent_ActivityTaskScheduledEventAttributes{
                ActivityTaskScheduledEventAttributes: &history.ActivityTaskScheduledEventAttributes{
                    ActivityId: info.ActivityID,
                    ActivityType: &common.ActivityType{Name: info.ActivityType.Name},
                },
            },
        })
        // STARTED
        startedID := nextEventID; nextEventID++
        events = append(events, &history.HistoryEvent{
            EventId:   startedID,
            EventTime: timestamppb.New(base.Add(time.Duration(startedID) * time.Millisecond)),
            EventType: enums.EVENT_TYPE_ACTIVITY_TASK_STARTED,
            Attributes: &history.HistoryEvent_ActivityTaskStartedEventAttributes{
                ActivityTaskStartedEventAttributes: &history.ActivityTaskStartedEventAttributes{
                    ScheduledEventId: schedID,
                },
            },
        })
    })

    env.SetOnActivityCompletedListener(func(info *activity.Info, result converter.EncodedValue, err error) {
        // COMPLETED with result payload
        schedID := scheduledByActID[info.ActivityID]
        var decoded map[string]interface{}
        _ = result.Get(&decoded)
        // encode as JSON payload
        var payloads []*common.Payload
        if decoded != nil {
            b, _ := json.Marshal(decoded)
            payloads = []*common.Payload{{Data: b}}
        }
        completedID := nextEventID; nextEventID++
        events = append(events, &history.HistoryEvent{
            EventId:   completedID,
            EventTime: timestamppb.New(base.Add(time.Duration(completedID) * time.Millisecond)),
            EventType: enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED,
            Attributes: &history.HistoryEvent_ActivityTaskCompletedEventAttributes{
                ActivityTaskCompletedEventAttributes: &history.ActivityTaskCompletedEventAttributes{
                    ScheduledEventId: schedID,
                    Result:           &common.Payloads{Payloads: payloads},
                },
            },
        })
    })

    // Execute workflow
    env.ExecuteWorkflow(wfName, map[string]interface{}{})
    if !env.IsWorkflowCompleted() { t.Fatalf("workflow not completed") }
    if err := env.GetWorkflowError(); err != nil { t.Fatalf("workflow error: %v", err) }

    // Build synthetic history and run transformer
    hist := &history.History{Events: events}
    tr := NewTransformer(func(name string) (*recipecore.Recipe, error) { return &parent.Recipe, nil })
    acts, err := tr.HistoryToActivityExecutions(hist, parent.ID)
    if err != nil { t.Fatalf("transform history failed: %v", err) }

    // Validate: two parent command executions + one child command execution
    if len(acts) != 3 { t.Fatalf("expected 3 activities (including child), got %d", len(acts)) }
    found := map[string]bool{"parent-hello": false, "child": false, "done": false}
    for i, a := range acts {
        if a.Name != "command_execution" { t.Fatalf("activity %d name: %s", i, a.Name) }
        if a.Status != "completed" { t.Fatalf("activity %d status: %s", i, a.Status) }
        if a.Duration == nil { t.Fatalf("activity %d missing duration", i) }
        if a.Result == nil { t.Fatalf("activity %d missing result", i) }
        if s, _ := a.Result["stdout"].(string); s != "" {
            if _, ok := found[s]; ok { found[s] = true }
        }
    }
    for k, v := range found { if !v { t.Fatalf("missing stdout %q in activities", k) } }

    // Also validate WorkflowExecutionsToJobs mapping shape with a minimal stub
    start := timestamppb.New(base)
    close := timestamppb.New(base.Add(10 * time.Second))
    execInfo := &apiwf.WorkflowExecutionInfo{
        Execution:  &common.WorkflowExecution{WorkflowId: "wf-1", RunId: "run-1"},
        Type:       &common.WorkflowType{Name: wfName},
        StartTime:  start,
        CloseTime:  close,
        Status:     enums.WORKFLOW_EXECUTION_STATUS_COMPLETED,
        HistoryLength: 3,
    }
    job, err := tr.WorkflowExecutionToJob(execInfo, parent.ID)
    if err != nil { t.Fatalf("execution to job failed: %v", err) }
    if job.Status != recipecore.JobStatusCompleted { t.Fatalf("job status: %s", job.Status) }
    if job.Duration == nil || *job.Duration <= 0 { t.Fatalf("job duration missing/invalid") }
}
