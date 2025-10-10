package story

import (
	"time"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/workflow"
)

type MarkerEnvelope struct {
	Kind       string           `json:"kind"`
	Stage      string           `json:"stage"`
	Path       string           `json:"path,omitempty"`
	Invocation MarkerInvocation `json:"invocation"`
	Payload    interface{}      `json:"payload,omitempty"`
}

type MarkerInvocation struct {
	ID       string `json:"id"`
	NodePath string `json:"node_path,omitempty"`
	Seq      int    `json:"seq"`
	RecipeID string `json:"recipe_id,omitempty"`
}

// InlineOpStartPayload captures metadata recorded before an inline op runs.
type InlineOpStartPayload struct {
	OpType    string                 `json:"op_type"`
	Inputs    map[string]interface{} `json:"inputs,omitempty"`
	StartedAt time.Time              `json:"started_at"`
}

// InlineOpCompletePayload captures the successful completion result for an inline op.
type InlineOpCompletePayload struct {
	OpType      string                 `json:"op_type"`
	Outputs     map[string]interface{} `json:"outputs,omitempty"`
	CompletedAt time.Time              `json:"completed_at"`
	Attempts    int                    `json:"attempts"`
}

// InlineOpFailPayload captures failure metadata for an inline op.
type InlineOpFailPayload struct {
	OpType    string    `json:"op_type"`
	ErrorType string    `json:"error_type"`
	Message   string    `json:"message"`
	FailedAt  time.Time `json:"failed_at"`
	Attempts  int       `json:"attempts"`
}

// InlineOpTimeoutPayload captures timeout information when an inline op exceeds its deadline.
type InlineOpTimeoutPayload struct {
	OpType    string        `json:"op_type"`
	TimeoutAt time.Time     `json:"timeout_at"`
	Duration  time.Duration `json:"duration"`
}

// ChildRecipeTriggerPayload records when a child recipe workflow is launched.
type ChildRecipeTriggerPayload struct {
	ChildWorkflowID string                 `json:"child_workflow_id"`
	ChildRunID      string                 `json:"child_run_id,omitempty"`
	TriggeredAt     time.Time              `json:"triggered_at"`
	RecipeName      string                 `json:"recipe_name,omitempty"`
	Inputs          map[string]interface{} `json:"inputs,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// ChildRecipeResultPayload records the completion status of a child recipe workflow.
type ChildRecipeResultPayload struct {
	ChildWorkflowID string                 `json:"child_workflow_id"`
	ChildRunID      string                 `json:"child_run_id"`
	Status          string                 `json:"status"`
	CompletedAt     time.Time              `json:"completed_at"`
	Outputs         map[string]interface{} `json:"outputs,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	RecipeName      string                 `json:"recipe_name,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// InlineLogPayload captures structured log annotations emitted inline.
type InlineLogPayload struct {
	Level    string                 `json:"level"`
	Message  string                 `json:"message"`
	Fields   map[string]interface{} `json:"fields,omitempty"`
	LoggedAt time.Time              `json:"logged_at"`
}

func RecordInlineOpStart(ctx workflow.Context, inv coreops.Invocation, payload InlineOpStartPayload) {
	record(ctx, "inline-op", "start", inv, payload)
}

func RecordInlineOpComplete(ctx workflow.Context, inv coreops.Invocation, payload InlineOpCompletePayload) {
	record(ctx, "inline-op", "complete", inv, payload)
}

func RecordInlineOpFail(ctx workflow.Context, inv coreops.Invocation, payload InlineOpFailPayload) {
	record(ctx, "inline-op", "fail", inv, payload)
}

func RecordInlineOpTimeout(ctx workflow.Context, inv coreops.Invocation, payload InlineOpTimeoutPayload) {
	record(ctx, "inline-op", "timeout", inv, payload)
}

func RecordChildRecipeTrigger(ctx workflow.Context, inv coreops.Invocation, payload ChildRecipeTriggerPayload) {
	record(ctx, "child-recipe", "trigger", inv, payload)
}

func RecordChildRecipeResult(ctx workflow.Context, inv coreops.Invocation, payload ChildRecipeResultPayload) {
	record(ctx, "child-recipe", "result", inv, payload)
}

func RecordInlineLog(ctx workflow.Context, inv coreops.Invocation, payload InlineLogPayload) {
	record(ctx, "log", "log", inv, payload)
}

func record(ctx workflow.Context, kind, stage string, inv coreops.Invocation, payload interface{}) {
	env := MarkerEnvelope{
		Kind:  kind,
		Stage: stage,
		Path:  inv.NodePath,
		Invocation: MarkerInvocation{
			ID:       inv.ID,
			NodePath: inv.NodePath,
			Seq:      inv.InvokeSeq,
			RecipeID: inv.RecipeID,
		},
		Payload: payload,
	}
	if env.Invocation.ID == "" {
		env.Invocation.ID = inv.Hash()
	}
	workflow.SideEffect(ctx, func(workflow.Context) interface{} {
		return env
	})
}

// DecodeMarkerEnvelope attempts to decode a marker envelope from the SideEffect marker details payload.
func DecodeMarkerEnvelope(details map[string]*commonpb.Payloads, dc converter.DataConverter) (*MarkerEnvelope, error) {
	if details == nil {
		return nil, nil
	}
	const dataKey = "data"
	payloads, ok := details[dataKey]
	if !ok || payloads == nil {
		return nil, nil
	}
	if dc == nil {
		dc = converter.GetDefaultDataConverter()
	}
	var env MarkerEnvelope
	if err := dc.FromPayloads(payloads, &env); err != nil {
		return nil, err
	}
	return &env, nil
}
