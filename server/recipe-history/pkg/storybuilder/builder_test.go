package storybuilder

import (
	"fmt"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/story"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBuilder_BuildStory(t *testing.T) {
	base := time.Now().UTC()

	rec := recipe.Recipe{RecipeImpl: &recipe.RecipeSequence{
		RecipeMetadata: recipe.RecipeMetadata{NodeMetadata: recipe.NodeMetadata{ID: "root"}},
		SequenceData: recipe.SequenceData{Sequence: []recipe.Node{
			{NodeImpl: &recipe.NodeOp{NodeMetadata: recipe.NodeMetadata{ID: "inline-node"}, OpData: recipe.OpData{Op: "inline"}}},
			{NodeImpl: &recipe.NodeOp{NodeMetadata: recipe.NodeMetadata{ID: "activity-node"}, OpData: recipe.OpData{Op: "activity"}}},
			{NodeImpl: &recipe.NodeOp{NodeMetadata: recipe.NodeMetadata{ID: "child-node"}, OpData: recipe.OpData{Op: "child"}}},
		}},
	}}

	builder := New("test-recipe", &rec)
	builder.SetExecutionInfo(&workflowservice.DescribeWorkflowExecutionResponse{
		WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
			Execution: &commonpb.WorkflowExecution{WorkflowId: "wf-id", RunId: "run-id"},
			Status:    enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED,
			StartTime: timestamppb.New(base),
			CloseTime: timestamppb.New(base.Add(10 * time.Second)),
		},
	})

	events := []*historypb.HistoryEvent{
		workflowEvent(1, base, enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED),
		inlineMarkerEvent(2, base.Add(1*time.Second), story.MarkerEnvelope{
			Kind:  "inline-op",
			Stage: "start",
			Invocation: story.MarkerInvocation{
				ID:       "inv-inline",
				NodePath: "root/inline-node",
				RecipeID: "test-recipe",
			},
			Payload: story.InlineOpStartPayload{
				OpType:    "inline",
				Inputs:    map[string]interface{}{"foo": "bar"},
				StartedAt: base.Add(1 * time.Second),
			},
		}),
		inlineMarkerEvent(3, base.Add(2*time.Second), story.MarkerEnvelope{
			Kind:  "inline-op",
			Stage: "complete",
			Invocation: story.MarkerInvocation{
				ID:       "inv-inline",
				NodePath: "root/inline-node",
				RecipeID: "test-recipe",
			},
			Payload: story.InlineOpCompletePayload{
				OpType:      "inline",
				Outputs:     map[string]interface{}{"result": "ok"},
				CompletedAt: base.Add(2 * time.Second),
				Attempts:    1,
			},
		}),
		activityScheduledEvent(4, base.Add(3*time.Second), &workerops.ActivityInvocationRequest{
			Invocation: ops.Invocation{ID: "inv-activity", NodePath: "root/activity-node", RecipeID: "test-recipe"},
			Input:      map[string]interface{}{"cmd": "echo"},
		}),
		activityStartedEvent(5, base.Add(4*time.Second), 4, 1),
		activityCompletedEvent(6, base.Add(5*time.Second), 4, map[string]interface{}{"status": "done"}),
		childMarkerEvent(7, base.Add(6*time.Second), story.MarkerEnvelope{
			Kind:  "child-recipe",
			Stage: "trigger",
			Invocation: story.MarkerInvocation{
				ID:       "inv-child",
				NodePath: "root/child-node",
				RecipeID: "test-recipe",
			},
			Payload: story.ChildRecipeTriggerPayload{
				ChildWorkflowID: "child-wf",
				ChildRunID:      "child-run",
				TriggeredAt:     base.Add(6 * time.Second),
				RecipeName:      "child-recipe",
				Inputs:          map[string]interface{}{"arg": 1},
			},
		}),
		childMarkerEvent(8, base.Add(7*time.Second), story.MarkerEnvelope{
			Kind:  "child-recipe",
			Stage: "result",
			Invocation: story.MarkerInvocation{
				ID:       "inv-child",
				NodePath: "root/child-node",
				RecipeID: "test-recipe",
			},
			Payload: story.ChildRecipeResultPayload{
				ChildWorkflowID: "child-wf",
				ChildRunID:      "child-run",
				Status:          "completed",
				CompletedAt:     base.Add(7 * time.Second),
				Outputs:         map[string]interface{}{"child": "ok"},
			},
		}),
		signalEvent(9, base.Add(8*time.Second), "user-response:inv-inline", map[string]interface{}{"user_id": "alice"}),
		workflowEvent(10, base.Add(9*time.Second), enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED),
	}

	for _, e := range events {
		builder.Process(e)
	}

	story := builder.Build()

	require.Equal(t, "root", story.Metadata.RecipeName)
	require.Equal(t, "wf-id", story.Metadata.WorkflowID)
	require.Equal(t, "run-id", story.Metadata.RunID)
	require.Equal(t, "completed", story.Metadata.Status)
	require.Len(t, story.Nodes, 3)

	inlineNode := findNode(t, story.Nodes, "root/inline-node")
	require.Len(t, inlineNode.Runs, 1)
	inlineRun := inlineNode.Runs[0]
	require.Equal(t, "completed", inlineRun.Status)
	require.Equal(t, "bar", inlineRun.Inputs["foo"])
	require.Equal(t, "ok", inlineRun.Outputs["result"])
	require.NotNil(t, inlineRun.StartedAt)
	require.NotNil(t, inlineRun.CompletedAt)

	activityNode := findNode(t, story.Nodes, "root/activity-node")
	require.Len(t, activityNode.Runs, 1)
	activityRun := activityNode.Runs[0]
	require.Equal(t, "completed", activityRun.Status)
	require.Equal(t, "done", activityRun.Outputs["status"])

	childNode := findNode(t, story.Nodes, "root/child-node")
	require.Len(t, childNode.Runs, 1)
	childRun := childNode.Runs[0]
	require.Equal(t, "completed", childRun.Status)
	require.Equal(t, "ok", childRun.Outputs["child"])
	require.NotNil(t, childRun.StartedAt)
	require.NotNil(t, childRun.CompletedAt)

	// Timeline should include signal entry with user_id
	foundSignal := false
	for _, entry := range story.Timeline {
		if entry.Kind == "input-response" {
			foundSignal = true
			require.Equal(t, "alice", entry.Data["user_id"])
		}
	}
	require.True(t, foundSignal, "expected input-response entry in timeline")
}

func workflowEvent(id int64, ts time.Time, typ enumspb.EventType) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   id,
		EventType: typ,
		EventTime: timestamppb.New(ts),
	}
}

func inlineMarkerEvent(id int64, ts time.Time, env story.MarkerEnvelope) *historypb.HistoryEvent {
	payloads := mustPayloads(env)
	return &historypb.HistoryEvent{
		EventId:   id,
		EventType: enumspb.EVENT_TYPE_MARKER_RECORDED,
		EventTime: timestamppb.New(ts),
		Attributes: &historypb.HistoryEvent_MarkerRecordedEventAttributes{
			MarkerRecordedEventAttributes: &historypb.MarkerRecordedEventAttributes{
				MarkerName: "SideEffect",
				Details:    map[string]*commonpb.Payloads{"data": payloads},
			},
		},
	}
}

func childMarkerEvent(id int64, ts time.Time, env story.MarkerEnvelope) *historypb.HistoryEvent {
	return inlineMarkerEvent(id, ts, env)
}

func activityScheduledEvent(id int64, ts time.Time, req *workerops.ActivityInvocationRequest) *historypb.HistoryEvent {
	payloads := mustPayloads(req)
	return &historypb.HistoryEvent{
		EventId:   id,
		EventType: enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
		EventTime: timestamppb.New(ts),
		Attributes: &historypb.HistoryEvent_ActivityTaskScheduledEventAttributes{
			ActivityTaskScheduledEventAttributes: &historypb.ActivityTaskScheduledEventAttributes{
				ActivityId:   fmt.Sprintf("act-%d", id),
				ActivityType: &commonpb.ActivityType{Name: req.Invocation.NodePath},
				Input:        payloads,
			},
		},
	}
}

func activityStartedEvent(id int64, ts time.Time, scheduledID int64, attempt int32) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   id,
		EventType: enumspb.EVENT_TYPE_ACTIVITY_TASK_STARTED,
		EventTime: timestamppb.New(ts),
		Attributes: &historypb.HistoryEvent_ActivityTaskStartedEventAttributes{
			ActivityTaskStartedEventAttributes: &historypb.ActivityTaskStartedEventAttributes{
				ScheduledEventId: scheduledID,
				Attempt:          attempt,
			},
		},
	}
}

func activityCompletedEvent(id int64, ts time.Time, scheduledID int64, result map[string]interface{}) *historypb.HistoryEvent {
	payloads := mustPayloads(result)
	return &historypb.HistoryEvent{
		EventId:   id,
		EventType: enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED,
		EventTime: timestamppb.New(ts),
		Attributes: &historypb.HistoryEvent_ActivityTaskCompletedEventAttributes{
			ActivityTaskCompletedEventAttributes: &historypb.ActivityTaskCompletedEventAttributes{
				ScheduledEventId: scheduledID,
				Result:           payloads,
			},
		},
	}
}

func signalEvent(id int64, ts time.Time, name string, payload map[string]interface{}) *historypb.HistoryEvent {
	input := mustPayloads(payload)
	return &historypb.HistoryEvent{
		EventId:   id,
		EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
		EventTime: timestamppb.New(ts),
		Attributes: &historypb.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
			WorkflowExecutionSignaledEventAttributes: &historypb.WorkflowExecutionSignaledEventAttributes{
				SignalName: name,
				Input:      input,
			},
		},
	}
}

func mustPayloads(v interface{}) *commonpb.Payloads {
	payloads, err := converter.GetDefaultDataConverter().ToPayloads(v)
	if err != nil {
		panic(err)
	}
	return payloads
}

func findNode(t *testing.T, nodes []*StoryNode, path string) *StoryNode {
	for _, node := range nodes {
		if node.Path == path {
			return node
		}
	}
	t.Fatalf("node %s not found", path)
	return nil
}
